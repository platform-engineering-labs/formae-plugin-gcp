// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package servicenetworking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/cloudresourcemanager/v1"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	servicenetworking "google.golang.org/api/servicenetworking/v1"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const ConnectionResourceType = "GCP::ServiceNetworking::Connection"

const defaultService = "servicenetworking.googleapis.com"

// ConnectionProvisioner manages a Private Service Access connection: the VPC
// peering between a consumer network and a Google service producer. There is at
// most one connection per (service, network), so the network is the identity.
// Create/delete are long-running; Status polls the operation.
type ConnectionProvisioner struct {
	cfg *config.Config
}

var _ prov.Provisioner = (*ConnectionProvisioner)(nil)

func NewConnectionProvisioner(cfg *config.Config) prov.Provisioner {
	return &ConnectionProvisioner{cfg: cfg}
}

func init() {
	registry.Register(
		ConnectionResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return NewConnectionProvisioner(cfg)
		},
	)
}

type connectionProps struct {
	Network               string   `json:"network"`
	ReservedPeeringRanges []string `json:"reservedPeeringRanges"`
	Service               string   `json:"service,omitempty"`
}

func cfgFrom(targetConfig json.RawMessage, fallback *config.Config) *config.Config {
	c := config.FromTargetConfig(targetConfig)
	if c.Project == "" && fallback != nil {
		c.Project = fallback.Project
	}
	return c
}

func (p *ConnectionProvisioner) clients(ctx context.Context, cfg *config.Config) (*servicenetworking.APIService, *cloudresourcemanager.Service, error) {
	opts, err := cfg.ToClientOptions(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("client options: %w", err)
	}
	sn, err := servicenetworking.NewService(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("servicenetworking client: %w", err)
	}
	crm, err := cloudresourcemanager.NewService(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("resource manager client: %w", err)
	}
	return sn, crm, nil
}

// lastSegment returns the final path element (handles a bare name, a
// projects/.../networks/name path, and a full self-link URL).
func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// networkPath resolves any network reference form to the project-number form the
// Service Networking API requires: projects/{number}/global/networks/{name}.
func networkPath(crm *cloudresourcemanager.Service, projectID, network string) (string, error) {
	name := lastSegment(network)
	proj, err := crm.Projects.Get(projectID).Do()
	if err != nil {
		return "", fmt.Errorf("get project number for %q: %w", projectID, err)
	}
	return fmt.Sprintf("projects/%d/global/networks/%s", proj.ProjectNumber, name), nil
}

func serviceOf(p *connectionProps) string {
	if p.Service != "" {
		return p.Service
	}
	return defaultService
}

func (p *ConnectionProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props connectionProps
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("parse properties: %v", err)), nil
	}
	if props.Network == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest, "network is required"), nil
	}

	cfg := cfgFrom(request.TargetConfig, p.cfg)
	sn, crm, err := p.clients(ctx, cfg)
	if err != nil {
		return nil, err
	}
	netPath, err := networkPath(crm, cfg.Project, props.Network)
	if err != nil {
		return createFailure(mapErr(err), err.Error()), nil
	}

	// Derive the peering range names. The reservedPeeringRanges resolvable exists
	// to order this connection after the range (that dependency edge is honoured),
	// but its VALUE can arrive unresolved due to a same-apply resolution race, so
	// we do not trust it: prefer any explicit non-empty names, else discover the
	// VPC_PEERING ranges on this network via the Compute API (the edge guarantees
	// they exist by now).
	ranges := validNames(props.ReservedPeeringRanges)
	if len(ranges) == 0 {
		ranges, err = discoverPeeringRanges(ctx, cfg, props.Network)
		if err != nil {
			return createFailure(mapErr(err), fmt.Sprintf("discover peering ranges: %v", err)), nil
		}
	}
	if len(ranges) == 0 {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"no VPC_PEERING reserved ranges found on the network; create a compute GlobalAddress with purpose=VPC_PEERING first"), nil
	}

	parent := "services/" + serviceOf(&props)
	op, err := sn.Services.Connections.Create(parent, &servicenetworking.Connection{
		Network:               netPath,
		ReservedPeeringRanges: ranges,
	}).Context(ctx).Do()
	if err != nil {
		return createFailure(mapErr(err), err.Error()), nil
	}

	// NativeID is the network as supplied (self-link or path) so Read round-trips
	// against the desired value. The create operation is long-running - report
	// InProgress and let Status poll it.
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        props.Network,
			RequestID:       op.Name,
			ResourceProperties: mustJSON(connectionProps{
				Network:               props.Network,
				ReservedPeeringRanges: ranges,
				Service:               props.Service,
			}),
		},
	}, nil
}

// validNames returns the non-empty short names from a reservedPeeringRanges
// input, dropping any unresolved-ref envelopes / empty entries.
func validNames(in []string) []string {
	out := make([]string, 0, len(in))
	for _, r := range in {
		if n := lastSegment(r); n != "" && !strings.Contains(n, "$") && !strings.Contains(n, "{") {
			out = append(out, n)
		}
	}
	return out
}

// discoverPeeringRanges lists the VPC_PEERING GlobalAddresses reserved on the
// given network and returns their names.
func discoverPeeringRanges(ctx context.Context, cfg *config.Config, networkRef string) ([]string, error) {
	opts, err := cfg.ToClientOptions(ctx)
	if err != nil {
		return nil, err
	}
	c, err := compute.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	netName := lastSegment(networkRef)
	out := []string{}
	// Filter in Go (avoids server-side filter-syntax pitfalls on enum fields).
	err = c.GlobalAddresses.List(cfg.Project).Pages(ctx, func(page *compute.AddressList) error {
		for _, a := range page.Items {
			if a.Purpose == "VPC_PEERING" && lastSegment(a.Network) == netName {
				out = append(out, a.Name)
			}
		}
		return nil
	})
	return out, err
}

func (p *ConnectionProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	cfg := cfgFrom(request.TargetConfig, p.cfg)
	sn, _, err := p.clients(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if request.RequestID == "" {
		return &resource.StatusResult{ProgressResult: &resource.ProgressResult{
			Operation: resource.OperationCheckStatus, OperationStatus: resource.OperationStatusSuccess, NativeID: request.NativeID,
		}}, nil
	}
	op, err := sn.Operations.Get(request.RequestID).Context(ctx).Do()
	if err != nil {
		return &resource.StatusResult{ProgressResult: &resource.ProgressResult{
			Operation: resource.OperationCheckStatus, OperationStatus: resource.OperationStatusFailure,
			ErrorCode: mapErr(err), StatusMessage: err.Error(), NativeID: request.NativeID, RequestID: request.RequestID,
		}}, nil
	}
	if !op.Done {
		return &resource.StatusResult{ProgressResult: &resource.ProgressResult{
			Operation: resource.OperationCheckStatus, OperationStatus: resource.OperationStatusInProgress,
			NativeID: request.NativeID, RequestID: request.RequestID,
		}}, nil
	}
	if op.Error != nil {
		return &resource.StatusResult{ProgressResult: &resource.ProgressResult{
			Operation: resource.OperationCheckStatus, OperationStatus: resource.OperationStatusFailure,
			ErrorCode:     resource.OperationErrorCodeServiceInternalError,
			StatusMessage: fmt.Sprintf("operation failed: %s", op.Error.Message), NativeID: request.NativeID,
		}}, nil
	}
	// On success, re-assert the resource properties so the stored state carries
	// reservedPeeringRanges after the async create completes (the create's
	// InProgress properties are otherwise superseded by this Status result). Best
	// effort: if the connection is gone (e.g. this was a delete op) skip.
	props := &resource.ProgressResult{
		Operation: resource.OperationCheckStatus, OperationStatus: resource.OperationStatusSuccess, NativeID: request.NativeID,
	}
	if ranges, derr := discoverPeeringRanges(ctx, cfg, request.NativeID); derr == nil && len(ranges) > 0 {
		props.ResourceProperties = mustJSON(connectionProps{Network: request.NativeID, ReservedPeeringRanges: ranges})
	}
	return &resource.StatusResult{ProgressResult: props}, nil
}

func (p *ConnectionProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	cfg := cfgFrom(request.TargetConfig, p.cfg)
	sn, crm, err := p.clients(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// NativeID is the network as supplied; the API lists by the project-number
	// path, so convert for the query but echo the NativeID back so state
	// round-trips against the desired network reference.
	netPath, err := networkPath(crm, cfg.Project, request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: mapErr(err)}, nil
	}
	resp, err := sn.Services.Connections.List("services/" + defaultService).Network(netPath).Context(ctx).Do()
	if err != nil {
		return &resource.ReadResult{ErrorCode: mapErr(err)}, nil
	}
	if len(resp.Connections) == 0 {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}
	c := resp.Connections[0]
	ranges := c.ReservedPeeringRanges
	// The List response can omit reservedPeeringRanges; fall back to the ranges
	// actually reserved on the network so read state always carries them.
	if len(ranges) == 0 {
		if discovered, derr := discoverPeeringRanges(ctx, cfg, request.NativeID); derr == nil {
			ranges = discovered
		}
	}
	propsJSON := mustJSON(connectionProps{Network: request.NativeID, ReservedPeeringRanges: ranges})
	return &resource.ReadResult{ResourceType: request.ResourceType, Properties: string(propsJSON)}, nil
}

func (p *ConnectionProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// All fields are createOnly; a range change plans a replace. No-op safety net.
	return &resource.UpdateResult{ProgressResult: &resource.ProgressResult{
		Operation: resource.OperationUpdate, OperationStatus: resource.OperationStatusSuccess, NativeID: request.NativeID,
	}}, nil
}

func (p *ConnectionProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	cfg := cfgFrom(request.TargetConfig, p.cfg)
	sn, crm, err := p.clients(ctx, cfg)
	if err != nil {
		return nil, err
	}
	netPath, err := networkPath(crm, cfg.Project, request.NativeID)
	if err != nil {
		return deleteFailure(mapErr(err), err.Error()), nil
	}
	// Find the peering name to address the connection; the consumer network is
	// the project-number path.
	resp, err := sn.Services.Connections.List("services/" + defaultService).Network(netPath).Context(ctx).Do()
	if err != nil {
		if code := mapErr(err); code == resource.OperationErrorCodeNotFound {
			return deleteSuccess(request.NativeID), nil
		}
		return deleteFailure(mapErr(err), err.Error()), nil
	}
	if len(resp.Connections) == 0 {
		return deleteSuccess(request.NativeID), nil
	}
	peering := resp.Connections[0].Peering
	if peering == "" {
		peering = "servicenetworking-googleapis-com"
	}
	name := fmt.Sprintf("services/%s/connections/%s", defaultService, peering)
	op, err := sn.Services.Connections.DeleteConnection(name, &servicenetworking.DeleteConnectionRequest{
		ConsumerNetwork: netPath,
	}).Context(ctx).Do()
	if err != nil {
		if code := mapErr(err); code == resource.OperationErrorCodeNotFound {
			return deleteSuccess(request.NativeID), nil
		}
		return deleteFailure(mapErr(err), err.Error()), nil
	}
	// Peering removal is long-running; report InProgress and let Status poll the
	// operation so the resource is only considered gone once the peering is
	// actually torn down.
	return &resource.DeleteResult{ProgressResult: &resource.ProgressResult{
		Operation:       resource.OperationDelete,
		OperationStatus: resource.OperationStatusInProgress,
		NativeID:        request.NativeID,
		RequestID:       op.Name,
	}}, nil
}

func (p *ConnectionProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// A connection is scoped to a specific network; there is no project-wide list
	// without one. Discovery of PSA connections is out of scope.
	return &resource.ListResult{}, nil
}

func mapErr(err error) resource.OperationErrorCode {
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
		case 409, 412:
			return resource.OperationErrorCodeResourceConflict
		case 429:
			return resource.OperationErrorCodeThrottling
		case 500, 502, 503, 504:
			return resource.OperationErrorCodeServiceInternalError
		}
	}
	return resource.OperationErrorCodeServiceInternalError
}

func mustJSON(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func createFailure(code resource.OperationErrorCode, msg string) *resource.CreateResult {
	return &resource.CreateResult{ProgressResult: &resource.ProgressResult{
		Operation: resource.OperationCreate, OperationStatus: resource.OperationStatusFailure, ErrorCode: code, StatusMessage: msg,
	}}
}

func deleteSuccess(nativeID string) *resource.DeleteResult {
	return &resource.DeleteResult{ProgressResult: &resource.ProgressResult{
		Operation: resource.OperationDelete, OperationStatus: resource.OperationStatusSuccess, NativeID: nativeID,
	}}
}

func deleteFailure(code resource.OperationErrorCode, msg string) *resource.DeleteResult {
	return &resource.DeleteResult{ProgressResult: &resource.ProgressResult{
		Operation: resource.OperationDelete, OperationStatus: resource.OperationStatusFailure, ErrorCode: code, StatusMessage: msg,
	}}
}
