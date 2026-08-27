// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package iam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/googleapi"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// NativeID format: "{project}|{role}|{member}". Roles can contain "/"
// (e.g. "roles/container.admin"), members can contain ":"
// (e.g. "user:nb@example.com"), neither contains "|".
const nativeIDDelim = "|"

// setIamPolicy can race on etag with parallel writers. Three tries is
// enough for typical contention without masking real bugs.
const maxSetPolicyAttempts = 3

// A principal created in the same apply (e.g. the service account a binding
// references) is eventually consistent: setIamPolicy can return 400 "does not
// exist" for a few seconds after the account's create call returns. Retry
// those with backoff (1+2+4+8 = ~15s worst case) so a same-apply SA + binding
// does not race. Without an ordering edge (member is a plain string, not a
// resolvable ref to the SA) the binding can even run before the SA create.
const maxMemberPropagationAttempts = 5

// isMemberNotYetPropagated reports whether err is the transient
// "Service account ... does not exist" 400 that a not-yet-propagated principal
// produces (as opposed to a genuinely missing member, which is a real error).
func isMemberNotYetPropagated(err error) bool {
	var ge *googleapi.Error
	if !errors.As(err, &ge) || ge.Code != 400 {
		return false
	}
	return strings.Contains(ge.Message, "does not exist")
}

// ProjectIamMemberProvisioner manages a single (role, member) binding on
// a GCP project's IAM policy. Each operation is read-modify-write on the
// parent policy; sibling bindings survive.
type ProjectIamMemberProvisioner struct {
	cfg *config.Config
}

var _ prov.Provisioner = (*ProjectIamMemberProvisioner)(nil)

func NewProjectIamMemberProvisioner(cfg *config.Config) prov.Provisioner {
	return &ProjectIamMemberProvisioner{cfg: cfg}
}

func (p *ProjectIamMemberProvisioner) newService(ctx context.Context, cfg *config.Config) (*cloudresourcemanager.Service, error) {
	opts, err := cfg.ToClientOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("client options: %w", err)
	}
	return cloudresourcemanager.NewService(ctx, opts...)
}

func cfgFrom(targetConfig json.RawMessage, fallback *config.Config) *config.Config {
	c := config.FromTargetConfig(targetConfig, fallback.Deps())
	if c.Project == "" && fallback != nil {
		c.Project = fallback.Project
	}
	return c
}

func buildNativeID(project, role, member string) string {
	return project + nativeIDDelim + role + nativeIDDelim + member
}

func parseNativeID(id string) (project, role, member string, err error) {
	parts := strings.SplitN(id, nativeIDDelim, 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid ProjectIamMember NativeID: %q", id)
	}
	return parts[0], parts[1], parts[2], nil
}

type memberProps struct {
	Project string `json:"project"`
	Role    string `json:"role"`
	Member  string `json:"member"`
}

func propsFromRaw(raw json.RawMessage) (*memberProps, error) {
	var p memberProps
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("decode properties: %w", err)
	}
	if p.Project == "" {
		return nil, errors.New("project is required")
	}
	if p.Role == "" {
		return nil, errors.New("role is required")
	}
	if p.Member == "" {
		return nil, errors.New("member is required")
	}
	return &p, nil
}

// addMember mutates the policy so (role, member) is present. Returns false
// if the member was already there.
func addMember(policy *cloudresourcemanager.Policy, role, member string) bool {
	for _, b := range policy.Bindings {
		if b.Role != role {
			continue
		}
		for _, m := range b.Members {
			if m == member {
				return false
			}
		}
		b.Members = append(b.Members, member)
		return true
	}
	policy.Bindings = append(policy.Bindings, &cloudresourcemanager.Binding{
		Role:    role,
		Members: []string{member},
	})
	return true
}

// removeMember strips (role, member) from the policy. If the binding ends
// up empty, the binding itself is dropped. Returns false if the member
// wasn't there to begin with.
func removeMember(policy *cloudresourcemanager.Policy, role, member string) bool {
	for i, b := range policy.Bindings {
		if b.Role != role {
			continue
		}
		for j, m := range b.Members {
			if m != member {
				continue
			}
			b.Members = append(b.Members[:j], b.Members[j+1:]...)
			if len(b.Members) == 0 {
				policy.Bindings = append(policy.Bindings[:i], policy.Bindings[i+1:]...)
			}
			return true
		}
		return false
	}
	return false
}

func hasMember(policy *cloudresourcemanager.Policy, role, member string) bool {
	for _, b := range policy.Bindings {
		if b.Role != role {
			continue
		}
		for _, m := range b.Members {
			if m == member {
				return true
			}
		}
		return false
	}
	return false
}

// withRetryOnConflict runs the read-modify-write loop. Returns (true, nil)
// on a successful policy mutation, (false, nil) if mutate decided no change
// was needed, or an error.
func (p *ProjectIamMemberProvisioner) withRetryOnConflict(
	ctx context.Context,
	svc *cloudresourcemanager.Service,
	project string,
	mutate func(*cloudresourcemanager.Policy) bool,
) (bool, error) {
	var lastErr error
	for attempt := 0; attempt < maxSetPolicyAttempts; attempt++ {
		policy, err := svc.Projects.GetIamPolicy(project, &cloudresourcemanager.GetIamPolicyRequest{}).Context(ctx).Do()
		if err != nil {
			return false, err
		}
		if !mutate(policy) {
			return false, nil
		}
		_, err = svc.Projects.SetIamPolicy(project, &cloudresourcemanager.SetIamPolicyRequest{Policy: policy}).Context(ctx).Do()
		if err == nil {
			return true, nil
		}
		lastErr = err
		var ge *googleapi.Error
		if !errors.As(err, &ge) || ge.Code != 409 {
			return false, err
		}
		// 409 = etag conflict, retry
	}
	return false, fmt.Errorf("setIamPolicy: %d attempts hit etag conflict: %w", maxSetPolicyAttempts, lastErr)
}

func (p *ProjectIamMemberProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	props, err := propsFromRaw(request.Properties)
	if err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	svc, err := p.newService(ctx, cfgFrom(request.TargetConfig, p.cfg))
	if err != nil {
		return nil, err
	}

	for attempt := 0; ; attempt++ {
		_, err = p.withRetryOnConflict(ctx, svc, props.Project, func(policy *cloudresourcemanager.Policy) bool {
			return addMember(policy, props.Role, props.Member)
		})
		if err == nil {
			break
		}
		if attempt >= maxMemberPropagationAttempts-1 || !isMemberNotYetPropagated(err) {
			return createFailure(mapGoogleErrorCode(err), err.Error()), nil
		}
		// Member (e.g. a same-apply service account) not yet propagated; back off.
		select {
		case <-ctx.Done():
			return createFailure(resource.OperationErrorCodeServiceInternalError, ctx.Err().Error()), nil
		case <-time.After(time.Duration(1<<attempt) * time.Second):
		}
	}

	return createSuccess(buildNativeID(props.Project, props.Role, props.Member), request.Properties), nil
}

func (p *ProjectIamMemberProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	project, role, member, err := parseNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	svc, err := p.newService(ctx, cfgFrom(request.TargetConfig, p.cfg))
	if err != nil {
		return nil, err
	}

	policy, err := svc.Projects.GetIamPolicy(project, &cloudresourcemanager.GetIamPolicyRequest{}).Context(ctx).Do()
	if err != nil {
		return &resource.ReadResult{ErrorCode: mapGoogleErrorCode(err)}, nil
	}

	if !hasMember(policy, role, member) {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	propsJSON, _ := json.Marshal(memberProps{Project: project, Role: role, Member: member})
	return &resource.ReadResult{
		ResourceType: request.ResourceType,
		Properties:   string(propsJSON),
	}, nil
}

func (p *ProjectIamMemberProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// All schema fields are createOnly. Formae plans Replace (delete +
	// create) for any change; Update is a no-op safety net.
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

func (p *ProjectIamMemberProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, role, member, err := parseNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	svc, err := p.newService(ctx, cfgFrom(request.TargetConfig, p.cfg))
	if err != nil {
		return nil, err
	}

	_, err = p.withRetryOnConflict(ctx, svc, project, func(policy *cloudresourcemanager.Policy) bool {
		return removeMember(policy, role, member)
	})
	if err != nil {
		// Idempotent delete: if the principal no longer exists (e.g. the service
		// account was deleted before its bindings — members are plain strings, not
		// resolvable refs, so formae can't order them), the binding is effectively
		// gone. GCP returns 400 "... does not exist"; treat that as success.
		if isMemberNotYetPropagated(err) {
			return deleteSuccess(request.NativeID), nil
		}
		return deleteFailure(mapGoogleErrorCode(err), err.Error()), nil
	}

	return deleteSuccess(request.NativeID), nil
}

func (p *ProjectIamMemberProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	cfg := cfgFrom(request.TargetConfig, p.cfg)
	if cfg.Project == "" {
		return &resource.ListResult{}, nil
	}

	svc, err := p.newService(ctx, cfg)
	if err != nil {
		return nil, err
	}

	policy, err := svc.Projects.GetIamPolicy(cfg.Project, &cloudresourcemanager.GetIamPolicyRequest{}).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, b := range policy.Bindings {
		for _, m := range b.Members {
			ids = append(ids, buildNativeID(cfg.Project, b.Role, m))
		}
	}
	return &resource.ListResult{NativeIDs: ids}, nil
}

func (p *ProjectIamMemberProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	// setIamPolicy is synchronous; nothing to poll. Whatever the caller
	// was waiting on already settled.
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
			RequestID:       request.RequestID,
		},
	}, nil
}

func mapGoogleErrorCode(err error) resource.OperationErrorCode {
	if err == nil {
		return resource.OperationErrorCodeNotSet
	}
	var ge *googleapi.Error
	if errors.As(err, &ge) {
		switch ge.Code {
		case 400:
			return resource.OperationErrorCodeInvalidRequest
		case 401, 403:
			return resource.OperationErrorCodeAccessDenied
		case 404:
			return resource.OperationErrorCodeNotFound
		case 409:
			return resource.OperationErrorCodeResourceConflict
		case 412:
			return resource.OperationErrorCodeResourceConflict
		case 429:
			return resource.OperationErrorCodeThrottling
		case 500, 502, 503, 504:
			return resource.OperationErrorCodeServiceInternalError
		}
	}
	return resource.OperationErrorCodeServiceInternalError
}

func createSuccess(nativeID string, props json.RawMessage) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           nativeID,
			ResourceProperties: props,
		},
	}
}

func createFailure(code resource.OperationErrorCode, msg string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
		},
	}
}

func deleteSuccess(nativeID string) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        nativeID,
		},
	}
}

func deleteFailure(code resource.OperationErrorCode, msg string) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
		},
	}
}
