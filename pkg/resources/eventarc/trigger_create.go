// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package eventarc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// triggerProvisioner exists because creating a trigger needs the id in two
// places at once, and the generic engine can only put it in one.
//
// Eventarc wants the short id in ?triggerId= AND the full resource path in the
// body's "name", which its own schema marks Required. base.Create reads the id
// out of body["name"] and deletes it, so the body reached the API without a
// name and every create failed:
//
//	The request was invalid: trigger.name is empty [field 'trigger.name']
//
// GCP::Eventarc::Trigger therefore could not be created at all. It shipped that
// way, and nothing caught it because the type had no conformance case.
type triggerProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*triggerProvisioner)(nil)

func (t *triggerProvisioner) Create(
	ctx context.Context,
	request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	client, err := transport.NewClient(ctx, t.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return t.createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}
	props = base.UnwrapValues(props)

	name, _ := props["name"].(string)
	if name == "" {
		return t.createFailure(resource.OperationErrorCodeInvalidRequest,
			"name is required"), nil
	}

	cfg := config.FromTargetConfig(request.TargetConfig, t.Config.Deps())
	location, _ := props["location"].(string)
	if location == "" {
		location = cfg.Location
	}

	body, err := triggerRequest(props, base.TransformContext{
		Project:   cfg.Project,
		Location:  location,
		Operation: resource.OperationCreate,
	})
	if err != nil {
		return t.createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to transform request: %v", err)), nil
	}

	nativeID := fmt.Sprintf("projects/%s/locations/%s/triggers/%s", cfg.Project, location, name)
	// The body carries the full path; the query param carries the short id.
	body["name"] = nativeID

	url, err := transport.AddQueryParam(
		fmt.Sprintf("%s/projects/%s/locations/%s/triggers", t.APIConfig.BaseURL, cfg.Project, location),
		"triggerId", name)
	if err != nil {
		return t.createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to build URL: %v", err)), nil
	}

	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST", URL: url, Body: body,
	})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to create trigger")
		return t.createFailure(transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
	}

	operationID := t.OperationConfig.OperationIDExtractor(response.Body)
	if operationID == "" {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusSuccess,
				NativeID:        nativeID,
				StatusMessage:   "triggers created successfully",
			},
		}, nil
	}

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        nativeID,
			RequestID:       t.OperationConfig.OperationURLBuilder(base.PathContext{}, operationID),
			StatusMessage:   "triggers creation in progress",
		},
	}, nil
}

// Status reads the trigger back once its operation completes, so the properties
// travel with the result - the same reason Bigtable needs it.
func (t *triggerProvisioner) Status(
	ctx context.Context,
	request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, t.BaseResource, t.Read, request)
}

func (t *triggerProvisioner) createFailure(
	code resource.OperationErrorCode, msg string,
) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
		},
	}
}
