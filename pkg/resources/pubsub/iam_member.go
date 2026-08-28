// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	TopicIamMemberResourceType        = "GCP::PubSub::TopicIamMember"
	SubscriptionIamMemberResourceType = "GCP::PubSub::SubscriptionIamMember"
)

// A binding is addressed by the three things that identify it, joined by a
// delimiter no part of a role, member or resource name can contain.
const iamDelim = "|"

// An IAM policy write is a read-modify-write against an etag, so two writers
// race. A 409 means someone else won; re-read and reapply.
const iamMaxAttempts = 3

// iamMemberProvisioner manages a single (role, member) binding on one Pub/Sub
// topic or subscription, read-modify-write so sibling bindings survive.
//
// Formae models a binding rather than the whole policy on purpose: a policy is
// shared with things outside the forma - GCP's own service agents write to it -
// and declaring the whole policy would delete their bindings on every apply.
//
// This is the same shape as the Cloud Run ServiceIamMember, against the REST
// transport instead of a generated SDK client.
type iamMemberProvisioner struct {
	cfg *config.Config
	// collection is "topics" or "subscriptions" - the only difference between
	// the two resource types.
	collection string
}

var _ prov.Provisioner = (*iamMemberProvisioner)(nil)

func init() {
	for rt, coll := range map[string]string{
		TopicIamMemberResourceType:        "topics",
		SubscriptionIamMemberResourceType: "subscriptions",
	} {
		resourceType, collection := rt, coll
		registry.Register(resourceType, []resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		}, func(cfg *config.Config) prov.Provisioner {
			return &iamMemberProvisioner{cfg: cfg, collection: collection}
		})
	}
}

// propertyName is the schema field naming the target: "topic" or "subscription".
func (p *iamMemberProvisioner) propertyName() string {
	return strings.TrimSuffix(p.collection, "s")
}

func (p *iamMemberProvisioner) resourceConfig(targetConfig json.RawMessage) *config.Config {
	if len(targetConfig) == 0 {
		return p.cfg
	}
	return config.FromTargetConfig(targetConfig, p.cfg.Deps())
}

// buildNativeID joins the full resource path, the role and the member.
func buildIamNativeID(resourcePath, role, member string) string {
	return strings.Join([]string{resourcePath, role, member}, iamDelim)
}

func parseIamNativeID(nativeID string) (resourcePath, role, member string, err error) {
	parts := strings.Split(nativeID, iamDelim)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf(
			"invalid IAM member native ID: %s (expected resource%srole%smember)", nativeID, iamDelim, iamDelim)
	}
	return parts[0], parts[1], parts[2], nil
}

// resourcePath turns a declared target into "projects/{project}/{collection}/{name}".
// A short name resolves against the target's project; a full path is kept.
func (p *iamMemberProvisioner) resourcePath(props map[string]interface{}, cfg *config.Config) (string, error) {
	name, _ := props[p.propertyName()].(string)
	if name == "" {
		return "", fmt.Errorf("%q is required", p.propertyName())
	}
	if strings.HasPrefix(name, "projects/") {
		return name, nil
	}
	project, _ := props["project"].(string)
	if project == "" {
		project = cfg.Project
	}
	if project == "" {
		return "", fmt.Errorf("project is required to resolve %q", name)
	}
	return fmt.Sprintf("projects/%s/%s/%s", project, p.collection, name), nil
}

type iamPolicy struct {
	Version  int          `json:"version,omitempty"`
	Etag     string       `json:"etag,omitempty"`
	Bindings []iamBinding `json:"bindings,omitempty"`
}

type iamBinding struct {
	Role      string                 `json:"role"`
	Members   []string               `json:"members,omitempty"`
	Condition map[string]interface{} `json:"condition,omitempty"`
}

func (p *iamMemberProvisioner) getPolicy(
	ctx context.Context, client *transport.Client, resourcePath string,
) (*iamPolicy, error) {
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    fmt.Sprintf("%s/%s:getIamPolicy", PubSubAPI.BaseURL, resourcePath),
	})
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(response.Body)
	if err != nil {
		return nil, err
	}
	var policy iamPolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

func (p *iamMemberProvisioner) setPolicy(
	ctx context.Context, client *transport.Client, resourcePath string, policy *iamPolicy,
) error {
	raw, err := json.Marshal(map[string]interface{}{"policy": policy})
	if err != nil {
		return err
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	_, err = client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    fmt.Sprintf("%s/%s:setIamPolicy", PubSubAPI.BaseURL, resourcePath),
		Body:   body,
	})
	return err
}

// withRetryOnConflict applies mutate to the current policy and writes it back,
// re-reading on an etag conflict. mutate reports whether it changed anything;
// when it did not, nothing is written.
func (p *iamMemberProvisioner) withRetryOnConflict(
	ctx context.Context,
	client *transport.Client,
	resourcePath string,
	mutate func(*iamPolicy) bool,
) error {
	var lastErr error
	for attempt := 0; attempt < iamMaxAttempts; attempt++ {
		policy, err := p.getPolicy(ctx, client, resourcePath)
		if err != nil {
			return err
		}
		if !mutate(policy) {
			return nil
		}
		if err = p.setPolicy(ctx, client, resourcePath, policy); err == nil {
			return nil
		}
		lastErr = err
		if transport.WrapError(err, "").Code != transport.ErrorCodeAlreadyExists {
			return err
		}
	}
	return fmt.Errorf("setIamPolicy: %d attempts hit an etag conflict: %w", iamMaxAttempts, lastErr)
}

func addMember(policy *iamPolicy, role, member string) bool {
	for i, b := range policy.Bindings {
		if b.Role != role || b.Condition != nil {
			continue
		}
		for _, m := range b.Members {
			if m == member {
				return false
			}
		}
		policy.Bindings[i].Members = append(policy.Bindings[i].Members, member)
		return true
	}
	policy.Bindings = append(policy.Bindings, iamBinding{Role: role, Members: []string{member}})
	return true
}

func removeMember(policy *iamPolicy, role, member string) bool {
	for i, b := range policy.Bindings {
		if b.Role != role || b.Condition != nil {
			continue
		}
		for j, m := range b.Members {
			if m != member {
				continue
			}
			policy.Bindings[i].Members = append(b.Members[:j], b.Members[j+1:]...)
			// An empty binding is not valid; drop it rather than writing it back.
			if len(policy.Bindings[i].Members) == 0 {
				policy.Bindings = append(policy.Bindings[:i], policy.Bindings[i+1:]...)
			}
			return true
		}
	}
	return false
}

func hasMember(policy *iamPolicy, role, member string) bool {
	for _, b := range policy.Bindings {
		if b.Role != role || b.Condition != nil {
			continue
		}
		for _, m := range b.Members {
			if m == member {
				return true
			}
		}
	}
	return false
}

func (p *iamMemberProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	cfg := p.resourceConfig(request.TargetConfig)
	client, err := transport.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return iamCreateFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	resourcePath, err := p.resourcePath(props, cfg)
	if err != nil {
		return iamCreateFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	role, _ := props["role"].(string)
	member, _ := props["member"].(string)
	if role == "" || member == "" {
		return iamCreateFailure(resource.OperationErrorCodeInvalidRequest,
			"role and member are required"), nil
	}

	if err := p.withRetryOnConflict(ctx, client, resourcePath, func(policy *iamPolicy) bool {
		return addMember(policy, role, member)
	}); err != nil {
		wrapped := transport.WrapError(err, "failed to add IAM binding")
		return iamCreateFailure(transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
	}

	stored, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal properties: %w", err)
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           buildIamNativeID(resourcePath, role, member),
			ResourceProperties: stored,
			StatusMessage:      "IAM binding added",
		},
	}, nil
}

// Read reports the binding as absent when the member is not in the policy -
// that is what "deleted" looks like for a binding, since the policy itself
// always exists.
func (p *iamMemberProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	cfg := p.resourceConfig(request.TargetConfig)
	client, err := transport.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	resourcePath, role, member, err := parseIamNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	policy, err := p.getPolicy(ctx, client, resourcePath)
	if err != nil {
		wrapped := transport.WrapError(err, "failed to read IAM policy")
		return &resource.ReadResult{ErrorCode: transport.ToResourceErrorCode(wrapped.Code)}, nil
	}
	if !hasMember(policy, role, member) {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	parts := strings.Split(resourcePath, "/")
	props := map[string]interface{}{
		"role":           role,
		"member":         member,
		p.propertyName(): parts[len(parts)-1],
		"project":        parts[1],
	}
	stored, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal properties: %w", err)
	}
	return &resource.ReadResult{Properties: string(stored)}, nil
}

// Update is never reached: every field is createOnly, so a change replaces.
func (p *iamMemberProvisioner) Update(
	_ context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       resource.OperationErrorCodeNotUpdatable,
			StatusMessage:   "an IAM binding is immutable; a change replaces it",
			NativeID:        request.NativeID,
		},
	}, nil
}

func (p *iamMemberProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	cfg := p.resourceConfig(request.TargetConfig)
	client, err := transport.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	resourcePath, role, member, err := parseIamNativeID(request.NativeID)
	if err != nil {
		return iamDeleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	if err := p.withRetryOnConflict(ctx, client, resourcePath, func(policy *iamPolicy) bool {
		return removeMember(policy, role, member)
	}); err != nil {
		wrapped := transport.WrapError(err, "failed to remove IAM binding")
		// The target being gone takes its policy with it, so the binding is gone too.
		code := transport.ToResourceErrorCode(wrapped.Code)
		if code == resource.OperationErrorCodeNotFound {
			return iamDeleteSuccess(request.NativeID), nil
		}
		return iamDeleteFailure(request.NativeID, code, wrapped.Message), nil
	}
	return iamDeleteSuccess(request.NativeID), nil
}

// List walks every topic or subscription in the project and emits one native ID
// per (role, member) pair. Pub/Sub has no way to ask for policies across a
// collection, so the walk is the only way to surface bindings at all.
func (p *iamMemberProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	cfg := p.resourceConfig(request.TargetConfig)
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}
	client, err := transport.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	collectionURL := fmt.Sprintf("%s/projects/%s/%s", PubSubAPI.BaseURL, cfg.Project, p.collection)
	var targets []string
	next := collectionURL
	for next != "" {
		response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: next})
		if err != nil {
			return nil, fmt.Errorf("failed to list %s: %w", p.collection, err)
		}
		items, _ := response.Body[p.collection].([]interface{})
		for _, raw := range items {
			switch item := raw.(type) {
			case map[string]interface{}:
				if name, ok := item["name"].(string); ok && name != "" {
					targets = append(targets, name)
				}
			case string:
				// A topic list answers with bare name strings in some versions.
				targets = append(targets, item)
			}
		}
		token, _ := response.Body["nextPageToken"].(string)
		if token == "" {
			break
		}
		if next, err = transport.AddQueryParam(collectionURL, "pageToken", token); err != nil {
			return nil, err
		}
	}

	nativeIDs := make([]string, 0, len(targets))
	for _, target := range targets {
		policy, err := p.getPolicy(ctx, client, target)
		if err != nil {
			// One unreadable target must not hide every other one's bindings.
			continue
		}
		for _, b := range policy.Bindings {
			if b.Condition != nil {
				// A conditional binding is not what this resource models.
				continue
			}
			for _, m := range b.Members {
				nativeIDs = append(nativeIDs, buildIamNativeID(target, b.Role, m))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status is synchronous: a policy write has landed by the time it returns.
func (p *iamMemberProvisioner) Status(
	_ context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

func iamCreateFailure(code resource.OperationErrorCode, msg string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
		},
	}
}

func iamDeleteSuccess(nativeID string) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        nativeID,
			StatusMessage:   "IAM binding removed",
		},
	}
}

func iamDeleteFailure(nativeID string, code resource.OperationErrorCode, msg string) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
			NativeID:        nativeID,
		},
	}
}
