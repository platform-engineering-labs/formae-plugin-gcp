// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package secretmanager

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const SecretVersionResourceType = "GCP::SecretManager::SecretVersion"

// SecretVersionProvisioner manages a Secret Manager SecretVersion. Versions
// use non-CRUD verbs (`:addVersion` to create, `:destroy` to delete) and the
// payload is write-only, so this is a custom provisioner rather than a
// config-driven BaseResource entry.
type SecretVersionProvisioner struct {
	cfg *config.Config
}

var _ prov.Provisioner = (*SecretVersionProvisioner)(nil)

func NewSecretVersionProvisioner(cfg *config.Config) prov.Provisioner {
	return &SecretVersionProvisioner{cfg: cfg}
}

func init() {
	registry.Register(
		SecretVersionResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return NewSecretVersionProvisioner(cfg)
		},
	)
}

type secretVersionProps struct {
	Secret string `json:"secret"`
	Data   string `json:"data"`
}

// parentPath expands a short secret ID to its full resource path. A value that
// already looks like a full path is passed through.
func parentPath(secret, project string) string {
	if strings.HasPrefix(secret, "projects/") {
		return secret
	}
	return fmt.Sprintf("projects/%s/secrets/%s", project, secret)
}

func (p *SecretVersionProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props secretVersionProps
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("failed to parse properties: %v", err)), nil
	}
	if props.Secret == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest, "secret is required"), nil
	}
	if props.Data == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest, "data is required"), nil
	}

	cfg := config.FromTargetConfig(request.TargetConfig, nil /* path context only; this config never authenticates */)
	if cfg.Project == "" && p.cfg != nil {
		cfg.Project = p.cfg.Project
	}

	client, err := transport.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	parent := parentPath(props.Secret, cfg.Project)
	url := fmt.Sprintf("%s/%s:addVersion", SecretManagerAPI.BaseURL, parent)
	body := map[string]interface{}{
		"payload": map[string]interface{}{
			"data": base64.StdEncoding.EncodeToString([]byte(props.Data)),
		},
	}

	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "POST", URL: url, Body: body})
	if err != nil {
		transportErr := transport.WrapError(err, fmt.Sprintf("failed to add version to secret '%s'", props.Secret))
		return createFailure(transport.ToResourceErrorCode(transportErr.Code), transportErr.Message), nil
	}

	nativeID := utils.GetString(response.Body, "name")
	if nativeID == "" {
		return createFailure(resource.OperationErrorCodeServiceInternalError, "addVersion response missing version name"), nil
	}

	// Echo back only the server-readable identifier. `secret` and `data` are
	// createOnly inputs (like Cloud SQL's rootPassword) and are never read back,
	// so echoing them would risk a short-id-vs-full-path drift on the resolvable.
	propsJSON, _ := json.Marshal(map[string]interface{}{"name": nativeID})
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			NativeID:           nativeID,
			OperationStatus:    resource.OperationStatusSuccess,
			StatusMessage:      "Secret version created successfully",
			ResourceProperties: propsJSON,
		},
	}, nil
}

func (p *SecretVersionProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	cfg := config.FromTargetConfig(request.TargetConfig, p.cfg.Deps())
	if cfg.Project == "" && p.cfg != nil {
		cfg.Project = p.cfg.Project
	}

	client, err := transport.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	url := fmt.Sprintf("%s/%s", SecretManagerAPI.BaseURL, request.NativeID)
	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
	if err != nil {
		transportErr := transport.WrapError(err, fmt.Sprintf("failed to read secret version '%s'", request.NativeID))
		return &resource.ReadResult{ErrorCode: transport.ToResourceErrorCode(transportErr.Code)}, nil
	}

	// A destroyed version still GETs (state=DESTROYED) rather than 404ing. Treat
	// it as gone so an out-of-band destroy surfaces as a deletion, not drift.
	if utils.GetString(response.Body, "state") == "DESTROYED" {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	name := utils.GetString(response.Body, "name")
	if name == "" {
		name = request.NativeID
	}
	propsJSON, _ := json.Marshal(map[string]interface{}{"name": name})
	return &resource.ReadResult{
		ResourceType: request.ResourceType,
		Properties:   string(propsJSON),
	}, nil
}

func (p *SecretVersionProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// All fields are createOnly; a payload change plans a replace. Update is a
	// no-op safety net.
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

func (p *SecretVersionProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	cfg := config.FromTargetConfig(request.TargetConfig, p.cfg.Deps())
	if cfg.Project == "" && p.cfg != nil {
		cfg.Project = p.cfg.Project
	}

	client, err := transport.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// Destroy the version. A version that is already gone (404) or already
	// destroyed (400 FAILED_PRECONDITION) is treated as a successful delete.
	url := fmt.Sprintf("%s/%s:destroy", SecretManagerAPI.BaseURL, request.NativeID)
	_, err = client.SendRequest(ctx, transport.RequestOptions{Method: "POST", URL: url, Body: map[string]interface{}{}})
	if err != nil {
		transportErr := transport.WrapError(err, fmt.Sprintf("failed to destroy secret version '%s'", request.NativeID))
		code := transport.ToResourceErrorCode(transportErr.Code)
		if code != resource.OperationErrorCodeNotFound && code != resource.OperationErrorCodeInvalidRequest {
			return deleteFailure(code, transportErr.Message), nil
		}
	}

	return deleteSuccess(request.NativeID), nil
}

func (p *SecretVersionProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	cfg := config.FromTargetConfig(request.TargetConfig, p.cfg.Deps())
	if cfg.Project == "" && p.cfg != nil {
		cfg.Project = p.cfg.Project
	}
	if cfg.Project == "" {
		return &resource.ListResult{}, nil
	}

	client, err := transport.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// A caller may scope to one parent secret (AdditionalProperties["secret"]);
	// otherwise enumerate every version of every secret in the project. Versions
	// have no project-wide list endpoint, so discovery walks secrets first.
	var secretPaths []string
	if scoped := request.AdditionalProperties["secret"]; scoped != "" {
		secretPaths = []string{parentPath(scoped, cfg.Project)}
	} else {
		secretPaths, err = p.listSecretPaths(ctx, client, cfg.Project)
		if err != nil {
			return nil, err
		}
	}

	var ids []string
	for _, secretPath := range secretPaths {
		versionIDs, err := p.listVersionNames(ctx, client, secretPath)
		if err != nil {
			return nil, err
		}
		ids = append(ids, versionIDs...)
	}
	return &resource.ListResult{NativeIDs: ids}, nil
}

// listSecretPaths returns the full path of every secret in the project,
// following pagination.
func (p *SecretVersionProvisioner) listSecretPaths(ctx context.Context, client *transport.Client, project string) ([]string, error) {
	var paths []string
	pageToken := ""
	for {
		url := fmt.Sprintf("%s/projects/%s/secrets", SecretManagerAPI.BaseURL, project)
		if pageToken != "" {
			url, _ = transport.AddQueryParams(url, map[string]string{"pageToken": pageToken})
		}
		response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
		if err != nil {
			return nil, transport.WrapError(err, "failed to list secrets")
		}
		if secrets, ok := response.Body["secrets"].([]interface{}); ok {
			for _, s := range secrets {
				if sm, ok := s.(map[string]interface{}); ok {
					if name := utils.GetString(sm, "name"); name != "" {
						paths = append(paths, name)
					}
				}
			}
		}
		pageToken = utils.GetString(response.Body, "nextPageToken")
		if pageToken == "" {
			break
		}
	}
	return paths, nil
}

// listVersionNames returns the full name of every non-destroyed version under a
// secret path, following pagination.
func (p *SecretVersionProvisioner) listVersionNames(ctx context.Context, client *transport.Client, secretPath string) ([]string, error) {
	var ids []string
	pageToken := ""
	for {
		url := fmt.Sprintf("%s/%s/versions", SecretManagerAPI.BaseURL, secretPath)
		if pageToken != "" {
			url, _ = transport.AddQueryParams(url, map[string]string{"pageToken": pageToken})
		}
		response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
		if err != nil {
			return nil, transport.WrapError(err, fmt.Sprintf("failed to list versions of '%s'", secretPath))
		}
		if versions, ok := response.Body["versions"].([]interface{}); ok {
			for _, v := range versions {
				if vm, ok := v.(map[string]interface{}); ok {
					// A destroyed version still lists; Read maps it to NotFound,
					// so skip it here to keep discovery consistent.
					if utils.GetString(vm, "state") == "DESTROYED" {
						continue
					}
					if name := utils.GetString(vm, "name"); name != "" {
						ids = append(ids, name)
					}
				}
			}
		}
		pageToken = utils.GetString(response.Body, "nextPageToken")
		if pageToken == "" {
			break
		}
	}
	return ids, nil
}

func (p *SecretVersionProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	// addVersion / destroy are synchronous; nothing to poll.
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
			RequestID:       request.RequestID,
		},
	}, nil
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
