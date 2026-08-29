// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const ObjectResourceType = "GCP::Storage::Object"

// objectProvisioner is hand-written because an object is content, not a
// resource description. Its create is a media upload to a different path than
// every other Cloud Storage call - /upload/storage/v1/... rather than
// /storage/v1/... - with the bytes sent verbatim, and its read has to fetch
// those bytes back separately from the metadata to notice they changed.
type objectProvisioner struct {
	cfg *config.Config
}

var _ prov.Provisioner = (*objectProvisioner)(nil)

func init() {
	registry.Register(ObjectResourceType, []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
		resource.OperationCheckStatus,
	}, func(cfg *config.Config) prov.Provisioner {
		return &objectProvisioner{cfg: cfg}
	})
}

// objectNativeID is "b/{bucket}/o/{name}". An object name may contain slashes -
// Cloud Storage has no directories, only names that look like paths - so
// everything after "/o/" is the name.
func objectNativeID(bucket, name string) string {
	return fmt.Sprintf("b/%s/o/%s", bucket, name)
}

func parseObjectNativeID(nativeID string) (bucket, name string, err error) {
	const marker = "/o/"
	if !strings.HasPrefix(nativeID, "b/") {
		return "", "", fmt.Errorf("invalid object native ID: %s", nativeID)
	}
	i := strings.Index(nativeID, marker)
	if i < 0 {
		return "", "", fmt.Errorf("invalid object native ID: %s", nativeID)
	}
	bucket = nativeID[len("b/"):i]
	name = nativeID[i+len(marker):]
	if bucket == "" || name == "" {
		return "", "", fmt.Errorf("invalid object native ID: %s", nativeID)
	}
	return bucket, name, nil
}

// uploadBase turns the JSON API base into the upload one. They differ only in
// the segment before /storage/v1, and getting it wrong answers 404 rather than
// anything that names the problem.
func uploadBase(apiBase string) string {
	return strings.Replace(apiBase, "/storage/v1", "/upload/storage/v1", 1)
}

func (o *objectProvisioner) props(raw json.RawMessage) (map[string]interface{}, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(raw, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}
	return base.UnwrapValues(props), nil
}

func (o *objectProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	client, err := transport.NewClient(ctx, o.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	props, err := o.props(request.Properties)
	if err != nil {
		return objectFailure(resource.OperationCreate, "", resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	bucket, _ := props["bucket"].(string)
	name, _ := props["name"].(string)
	if bucket == "" || name == "" {
		return objectFailure(resource.OperationCreate, "", resource.OperationErrorCodeInvalidRequest,
			"bucket and name are required"), nil
	}
	content, _ := props["content"].(string)
	contentType, _ := props["contentType"].(string)

	uploadURL := fmt.Sprintf("%s/b/%s/o?uploadType=media&name=%s",
		uploadBase(StorageAPI.BaseURL), url.PathEscape(bucket), url.QueryEscape(name))

	if _, err := client.SendRequest(ctx, transport.RequestOptions{
		Method:      "POST",
		URL:         uploadURL,
		RawBody:     []byte(content),
		ContentType: contentType,
	}); err != nil {
		wrapped := transport.WrapError(err, "failed to upload object")
		return objectFailure(resource.OperationCreate, "",
			transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
	}

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        objectNativeID(bucket, name),
			StatusMessage:   "object uploaded",
		},
	}, nil
}

// Update re-uploads. Cloud Storage has no partial write for an object's bytes,
// and an upload to the same name replaces it.
func (o *objectProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	created, err := o.Create(ctx, &resource.CreateRequest{
		Properties:   request.DesiredProperties,
		TargetConfig: request.TargetConfig,
		ResourceType: request.ResourceType,
	})
	if err != nil {
		return nil, err
	}
	// Report it as the update it is. Handing back the create's ProgressResult
	// verbatim reported Operation: create, which formae rejects for an update.
	progress := created.ProgressResult
	progress.Operation = resource.OperationUpdate
	if progress.StatusMessage == "object uploaded" {
		progress.StatusMessage = "object replaced"
	}
	return &resource.UpdateResult{ProgressResult: progress}, nil
}

// Read fetches the metadata and then the bytes. The bytes are a declared
// property, so without them a changed object would never read as changed.
func (o *objectProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	client, err := transport.NewClient(ctx, o.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	bucket, name, err := parseObjectNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	objectURL := fmt.Sprintf("%s/b/%s/o/%s",
		StorageAPI.BaseURL, url.PathEscape(bucket), url.PathEscape(name))

	metadata, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: objectURL})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to read object")
		return &resource.ReadResult{ErrorCode: transport.ToResourceErrorCode(wrapped.Code)}, nil
	}

	props := map[string]interface{}{
		"bucket": bucket,
		"name":   name,
	}
	for _, k := range []string{"contentType", "generation", "size"} {
		if v, ok := metadata.Body[k]; ok {
			props[k] = v
		}
	}
	if content, err := o.readContent(ctx, client, objectURL); err == nil {
		props["content"] = content
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal properties: %w", err)
	}
	return &resource.ReadResult{Properties: string(propsJSON)}, nil
}

// readContent fetches the object's bytes. alt=media returns them raw, which is
// not JSON, so the response is read as a string rather than decoded.
func (o *objectProvisioner) readContent(
	ctx context.Context, client *transport.Client, objectURL string,
) (string, error) {
	raw, err := client.SendRaw(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    objectURL + "?alt=media",
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (o *objectProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	client, err := transport.NewClient(ctx, o.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	bucket, name, err := parseObjectNativeID(request.NativeID)
	if err != nil {
		return objectDeleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	_, err = client.SendRequest(ctx, transport.RequestOptions{
		Method: "DELETE",
		URL: fmt.Sprintf("%s/b/%s/o/%s",
			StorageAPI.BaseURL, url.PathEscape(bucket), url.PathEscape(name)),
	})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to delete object")
		code := transport.ToResourceErrorCode(wrapped.Code)
		if code != resource.OperationErrorCodeNotFound {
			return objectDeleteFailure(request.NativeID, code, wrapped.Message), nil
		}
	}
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
			StatusMessage:   "object deleted",
		},
	}, nil
}

// List walks the project's buckets when no bucket is named, for the same reason
// the ACL types do: discovery lists with no parent, and Cloud Storage has no
// endpoint spanning buckets.
func (o *objectProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	cfg := config.FromTargetConfig(request.TargetConfig, o.cfg.Deps())
	client, err := transport.NewClient(ctx, o.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	buckets := []string{}
	if request.AdditionalProperties != nil {
		if b := request.AdditionalProperties["bucket"]; b != "" {
			buckets = append(buckets, b)
		}
	}
	if len(buckets) == 0 {
		if cfg.Project == "" {
			return &resource.ListResult{NativeIDs: []string{}}, nil
		}
		acl := &aclProvisioner{BaseResource: &base.BaseResource{Config: o.cfg, APIConfig: StorageAPI}}
		if buckets, err = acl.listBuckets(ctx, client, cfg.Project); err != nil {
			return nil, fmt.Errorf("failed to list buckets: %w", err)
		}
	}

	nativeIDs := []string{}
	for _, bucket := range buckets {
		response, err := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/b/%s/o", StorageAPI.BaseURL, url.PathEscape(bucket)),
		})
		if err != nil {
			// A shared project holds buckets this target cannot read.
			continue
		}
		items, _ := response.Body["items"].([]interface{})
		for _, raw := range items {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if name, ok := item["name"].(string); ok && name != "" {
				nativeIDs = append(nativeIDs, objectNativeID(bucket, name))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status is synchronous: an upload has landed by the time it returns.
func (o *objectProvisioner) Status(
	_ context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

func objectFailure(op resource.Operation, nativeID string, code resource.OperationErrorCode, msg string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       op,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
			NativeID:        nativeID,
		},
	}
}

func objectDeleteFailure(nativeID string, code resource.OperationErrorCode, msg string) *resource.DeleteResult {
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
