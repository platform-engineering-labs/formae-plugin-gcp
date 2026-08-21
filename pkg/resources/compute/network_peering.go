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

// NetworkPeeringProvisioner manages one VPC peering on a network. A peering is
// not a REST resource: it lives inside the owning network's "peerings" array and
// is manipulated with addPeering / removePeering verbs, so CRUD is hand-written.
// Status delegates to the base, since both verbs return a global Compute
// operation.
//
// v1 has no updatePeering (the path 404s), so there is no Update: a change
// replaces the peering.
type NetworkPeeringProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*NetworkPeeringProvisioner)(nil)

// peeringNetworkField is the schema's name for the network the peering belongs
// to. It is a path component, never part of the request body.
const peeringNetworkField = "network"

// peeringPeerField is the schema's name for the far side. The API calls it
// "network" inside the networkPeering object, which would collide with the
// owning network, so the schema renames it.
const peeringPeerField = "peerNetwork"

func init() {
	registry.Register(NetworkPeeringResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &NetworkPeeringProvisioner{
				BaseResource: &base.BaseResource{
					Config:          cfg,
					APIConfig:       ComputeAPI,
					OperationConfig: ComputeOperations,
					ResourceConfig: base.ResourceConfig{
						ResourceType: "networks",
						Scope:        &base.ScopeConfig{Type: base.ScopeGlobal},
					},
					NativeIDConfig: ComputeNativeID,
				},
			}
		})
}

// buildPeeringNativeID composes
// "projects/{p}/global/networks/{network}/peerings/{name}".
func buildPeeringNativeID(project, network, name string) string {
	return fmt.Sprintf("projects/%s/global/networks/%s/peerings/%s", project, network, name)
}

// parsePeeringNativeID splits the composite id. A peering has no identity of its
// own in the API - it is (owning network, peering name) - so both must survive.
func parsePeeringNativeID(nativeID string) (project, network, name string, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 7 || parts[0] != "projects" || parts[2] != "global" ||
		parts[3] != "networks" || parts[5] != "peerings" {
		return "", "", "", fmt.Errorf("invalid network peering native ID: %s", nativeID)
	}
	return parts[1], parts[4], parts[6], nil
}

// peeringBody builds the addPeering request: the API wraps the peering in a
// "networkPeering" object, wants the far side under "network", and rejects the
// owning network (a path component) as an unknown field.
func peeringBody(props map[string]interface{}) map[string]interface{} {
	peering := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case peeringNetworkField, "state", "stateDetails":
			// path component / read-only
			continue
		case peeringPeerField:
			peering["network"] = v
		default:
			peering[k] = v
		}
	}
	return map[string]interface{}{"networkPeering": peering}
}

func (p *NetworkPeeringProvisioner) networkURL(project, network string) string {
	return fmt.Sprintf("%s/projects/%s/global/networks/%s", p.APIConfig.BaseURL, project, network)
}

func (p *NetworkPeeringProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil && cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

func (p *NetworkPeeringProvisioner) issueVerb(
	ctx context.Context, url string, body map[string]interface{}, project string,
) (string, *transport.Error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return "", transport.WrapError(err, "failed to create transport client")
	}
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST", URL: url, Body: body,
	})
	if err != nil {
		return "", transport.WrapError(err, "network peering verb failed")
	}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	return p.OperationConfig.OperationURLBuilder(base.PathContext{Project: project}, opID), nil
}

// findPeering pulls one named peering out of the owning network.
func (p *NetworkPeeringProvisioner) findPeering(
	ctx context.Context, project, network, name string,
) (map[string]interface{}, *transport.Error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, transport.WrapError(err, "failed to create transport client")
	}
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET", URL: p.networkURL(project, network),
	})
	if err != nil {
		return nil, transport.WrapError(err, "failed to read owning network")
	}
	peerings, _ := resp.Body["peerings"].([]interface{})
	for _, item := range peerings {
		peering, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if peering["name"] == name {
			return peering, nil
		}
	}
	return nil, nil // network exists, peering does not
}

func (p *NetworkPeeringProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	network, _ := props[peeringNetworkField].(string)
	name, _ := props["name"].(string)
	if network == "" || name == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"name and network are required"), nil
	}
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"target project is required"), nil
	}

	requestID, verbErr := p.issueVerb(ctx,
		p.networkURL(project, network)+"/addPeering", peeringBody(props), project)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        buildPeeringNativeID(project, network, name),
			RequestID:       requestID,
			StatusMessage:   "network peering creation in progress",
		},
	}, nil
}

func (p *NetworkPeeringProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, network, name, err := parsePeeringNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	peering, verbErr := p.findPeering(ctx, project, network, name)
	if verbErr != nil {
		return &resource.ReadResult{
			ErrorCode: transport.ToResourceErrorCode(verbErr.Code),
		}, nil
	}
	if peering == nil {
		// The network is there but the peering is gone - formae must learn that
		// rather than see a generic failure.
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	props := make(map[string]interface{}, len(peering)+1)
	for k, v := range peering {
		if k == "network" {
			// The API's "network" is the far side; the schema calls it peerNetwork.
			props[peeringPeerField] = v
			continue
		}
		props[k] = v
	}
	props[peeringNetworkField] = network

	encoded, mErr := json.Marshal(props)
	if mErr != nil {
		return nil, fmt.Errorf("failed to marshal peering properties: %w", mErr)
	}
	return &resource.ReadResult{Properties: string(encoded)}, nil
}

// Update is intentionally absent from the registered operations: v1 has no
// updatePeering method (the path returns 404), so a change must replace.
func (p *NetworkPeeringProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	return updateFailure(resource.OperationErrorCodeNotUpdatable,
		"network peerings cannot be updated in place; the change replaces the peering"), nil
}

func (p *NetworkPeeringProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, network, name, err := parsePeeringNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	requestID, verbErr := p.issueVerb(ctx,
		p.networkURL(project, network)+"/removePeering",
		map[string]interface{}{"name": name}, project)
	if verbErr != nil {
		return deleteFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   "network peering deletion in progress",
		},
	}, nil
}

// List enumerates peerings across every network in the project, since a peering
// is only discoverable through its owner.
func (p *NetworkPeeringProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    fmt.Sprintf("%s/projects/%s/global/networks", p.APIConfig.BaseURL, project),
	})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list networks")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	items, _ := resp.Body["items"].([]interface{})
	nativeIDs := []string{}
	for _, item := range items {
		network, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		netName, _ := network["name"].(string)
		peerings, _ := network["peerings"].([]interface{})
		for _, pe := range peerings {
			peering, ok := pe.(map[string]interface{})
			if !ok {
				continue
			}
			if name, ok := peering["name"].(string); ok && name != "" {
				nativeIDs = append(nativeIDs, buildPeeringNativeID(project, netName, name))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status routes through the shared read-back so post-create and post-update
// state carries the resource's real properties, not just what was declared.
func (p *NetworkPeeringProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}
