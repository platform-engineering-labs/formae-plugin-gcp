// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// SignedUrlKeyProvisioner manages one Cloud CDN signed-URL key on a backend
// service. Without a key, signed URLs cannot be issued at all, so this is what
// makes `enableCDN` usable for private content.
//
// The key is not a REST resource: it is added and removed with the
// addSignedUrlKey / deleteSignedUrlKey verbs, and the API only ever reports the
// key *names* back (under cdnPolicy.signedUrlKeyNames) — the secret itself is
// write-only. There is nothing to update: rotating a key means removing it and
// adding the new value.
type SignedUrlKeyProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*SignedUrlKeyProvisioner)(nil)

// signedUrlKeyBackendProperty names the owning backend service. It is a path
// component, never part of the verb body.
const signedUrlKeyBackendProperty = "backendService"

func init() {
	registry.Register(BackendServiceSignedUrlKeyResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &SignedUrlKeyProvisioner{
				BaseResource: &base.BaseResource{
					Config:          cfg,
					APIConfig:       ComputeAPI,
					OperationConfig: ComputeOperations,
					ResourceConfig: base.ResourceConfig{
						ResourceType: "backendServices",
						Scope:        &base.ScopeConfig{Type: base.ScopeGlobal},
					},
					NativeIDConfig: ComputeNativeID,
				},
			}
		})
}

// buildSignedUrlKeyNativeID composes
// "projects/{p}/global/backendServices/{bs}/signedUrlKeys/{keyName}".
func buildSignedUrlKeyNativeID(project, backendService, keyName string) string {
	return fmt.Sprintf("projects/%s/global/backendServices/%s/signedUrlKeys/%s",
		project, backendService, keyName)
}

// parseSignedUrlKeyNativeID splits the composite id. A key has no identity
// beyond (backend service, key name), so both have to survive.
func parseSignedUrlKeyNativeID(nativeID string) (project, backendService, keyName string, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 7 || parts[0] != "projects" || parts[2] != "global" ||
		parts[3] != "backendServices" || parts[5] != "signedUrlKeys" || parts[6] == "" {
		return "", "", "", fmt.Errorf("invalid signed URL key native ID: %s", nativeID)
	}
	return parts[1], parts[4], parts[6], nil
}

func (p *SignedUrlKeyProvisioner) backendServiceURL(project, backendService string) string {
	return fmt.Sprintf("%s/projects/%s/global/backendServices/%s",
		p.APIConfig.BaseURL, project, backendService)
}

func (p *SignedUrlKeyProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil && cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

func (p *SignedUrlKeyProvisioner) issueVerb(
	ctx context.Context, url string, body map[string]interface{}, project string,
) (string, *transport.Error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return "", transport.WrapError(err, "failed to create transport client")
	}
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    url,
		Body:   body,
	})
	if err != nil {
		return "", transport.WrapError(err, "signed URL key verb failed")
	}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	return p.OperationConfig.OperationURLBuilder(base.PathContext{Project: project}, opID), nil
}

func (p *SignedUrlKeyProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	backendService, _ := props[signedUrlKeyBackendProperty].(string)
	keyName, _ := props["keyName"].(string)
	keyValue, _ := props["keyValue"].(string)
	if backendService == "" || keyName == "" || keyValue == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"backendService, keyName and keyValue are required"), nil
	}
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"target project is required"), nil
	}

	requestID, verbErr := p.issueVerb(ctx,
		p.backendServiceURL(project, backendService)+"/addSignedUrlKey",
		map[string]interface{}{"keyName": keyName, "keyValue": keyValue}, project)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        buildSignedUrlKeyNativeID(project, backendService, keyName),
			RequestID:       requestID,
			StatusMessage:   "signed URL key creation in progress",
		},
	}, nil
}

// Read checks the key name against the backend service's
// cdnPolicy.signedUrlKeyNames. The secret is never returned, so a read can only
// report presence — which is exactly what drift detection needs here.
func (p *SignedUrlKeyProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, backendService, keyName, err := parseSignedUrlKeyNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", cErr)
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    p.backendServiceURL(project, backendService),
	})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to read backend service")
		return &resource.ReadResult{
			ErrorCode: transport.ToResourceErrorCode(wrapped.Code),
		}, nil
	}

	if !signedUrlKeyPresent(resp.Body, keyName) {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	encoded, mErr := json.Marshal(map[string]interface{}{
		"keyName":                   keyName,
		signedUrlKeyBackendProperty: backendService,
	})
	if mErr != nil {
		return nil, fmt.Errorf("failed to marshal signed URL key properties: %w", mErr)
	}
	return &resource.ReadResult{Properties: string(encoded)}, nil
}

// signedUrlKeyPresent reports whether the backend service carries this key name.
func signedUrlKeyPresent(backendService map[string]interface{}, keyName string) bool {
	for _, n := range signedUrlKeyNames(backendService) {
		if n == keyName {
			return true
		}
	}
	return false
}

// signedUrlKeyNames pulls cdnPolicy.signedUrlKeyNames, which is absent entirely
// when no keys are configured.
func signedUrlKeyNames(backendService map[string]interface{}) []string {
	cdnPolicy, ok := backendService["cdnPolicy"].(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := cdnPolicy["signedUrlKeyNames"].([]interface{})
	if !ok {
		return nil
	}
	names := make([]string, 0, len(raw))
	for _, n := range raw {
		if name, ok := n.(string); ok && name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (p *SignedUrlKeyProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	return updateFailure(resource.OperationErrorCodeNotUpdatable,
		"a signed URL key cannot be changed in place; rotating it removes and re-adds the key"), nil
}

func (p *SignedUrlKeyProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, backendService, keyName, err := parseSignedUrlKeyNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	url := fmt.Sprintf("%s/deleteSignedUrlKey?keyName=%s",
		p.backendServiceURL(project, backendService), keyName)
	requestID, verbErr := p.issueVerb(ctx, url, nil, project)
	if verbErr != nil {
		return deleteFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   "signed URL key removal in progress",
		},
	}, nil
}

// List enumerates one backend service's key names. Keys live inside their
// backend service, so discovery has to be told which one to look in.
func (p *SignedUrlKeyProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	backendService := ""
	if request.AdditionalProperties != nil {
		backendService = request.AdditionalProperties[signedUrlKeyBackendProperty]
	}
	project := p.projectFor(request.TargetConfig, "")
	if backendService == "" || project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    p.backendServiceURL(project, backendService),
	})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list signed URL keys")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	names := signedUrlKeyNames(resp.Body)
	nativeIDs := make([]string, 0, len(names))
	for _, name := range names {
		nativeIDs = append(nativeIDs, buildSignedUrlKeyNativeID(project, backendService, name))
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status routes through the shared read-back so post-create and post-update
// state carries the resource's real properties, not just what was declared.
func (p *SignedUrlKeyProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}
