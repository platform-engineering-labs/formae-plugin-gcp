// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/googleapi"
	run "google.golang.org/api/run/v2"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const ServiceIamMemberResourceType = "GCP::CloudRun::ServiceIamMember"

// NativeID format: "{service}|{role}|{member}". The service segment is stored
// exactly as supplied (short name or full path) so Read echoes back the same
// representation the desired state used. Roles contain "/", members contain
// ":"; neither contains "|".
const siamDelim = "|"

// setIamPolicy can race on etag; three tries covers typical contention.
const siamMaxAttempts = 3

// ServiceIamMemberProvisioner manages a single (role, member) binding on a
// Cloud Run service's IAM policy. Granting roles/run.invoker to allUsers is how
// a service is made publicly reachable. Each operation is a read-modify-write
// on the service policy, so sibling bindings survive.
type ServiceIamMemberProvisioner struct {
	cfg *config.Config
}

var _ prov.Provisioner = (*ServiceIamMemberProvisioner)(nil)

func NewServiceIamMemberProvisioner(cfg *config.Config) prov.Provisioner {
	return &ServiceIamMemberProvisioner{cfg: cfg}
}

func init() {
	registry.Register(
		ServiceIamMemberResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return NewServiceIamMemberProvisioner(cfg)
		},
	)
}

type serviceIamMemberProps struct {
	Service  string `json:"service"`
	Location string `json:"location"`
	Project  string `json:"project"`
	Role     string `json:"role"`
	Member   string `json:"member"`
}

func (p *ServiceIamMemberProvisioner) newService(ctx context.Context, cfg *config.Config) (*run.Service, error) {
	opts, err := cfg.ToClientOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("client options: %w", err)
	}
	return run.NewService(ctx, opts...)
}

func siamCfg(targetConfig json.RawMessage, fallback *config.Config) *config.Config {
	c := config.FromTargetConfig(targetConfig)
	if fallback != nil {
		if c.Project == "" {
			c.Project = fallback.Project
		}
		if c.Location == "" {
			c.Location = fallback.Location
		}
		if c.Region == "" {
			c.Region = fallback.Region
		}
	}
	return c
}

// servicePath expands a short service name to the full IAM resource path
// "projects/{p}/locations/{loc}/services/{svc}". A value that already looks
// like a full path is passed through. location falls back to the target's
// Location then Region (Cloud Run is regional).
func servicePath(service string, cfg *config.Config, propLocation, propProject string) (string, error) {
	if strings.HasPrefix(service, "projects/") {
		return service, nil
	}
	project := propProject
	if project == "" {
		project = cfg.Project
	}
	location := propLocation
	if location == "" {
		location = cfg.Location
	}
	if location == "" {
		location = cfg.Region
	}
	if project == "" || location == "" || service == "" {
		return "", fmt.Errorf("service path needs project, location and service (got project=%q location=%q service=%q)", project, location, service)
	}
	return fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, service), nil
}

func siamBuildNativeID(service, role, member string) string {
	return service + siamDelim + role + siamDelim + member
}

func siamParseNativeID(id string) (service, role, member string, err error) {
	parts := strings.SplitN(id, siamDelim, 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid ServiceIamMember NativeID: %q", id)
	}
	return parts[0], parts[1], parts[2], nil
}

func siamProps(raw json.RawMessage) (*serviceIamMemberProps, error) {
	var p serviceIamMemberProps
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("decode properties: %w", err)
	}
	if p.Service == "" {
		return nil, errors.New("service is required")
	}
	if p.Role == "" {
		return nil, errors.New("role is required")
	}
	if p.Member == "" {
		return nil, errors.New("member is required")
	}
	return &p, nil
}

// siamAddMember adds (role, member) to the policy. Returns false if already present.
func siamAddMember(policy *run.GoogleIamV1Policy, role, member string) bool {
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
	policy.Bindings = append(policy.Bindings, &run.GoogleIamV1Binding{Role: role, Members: []string{member}})
	return true
}

// siamRemoveMember strips (role, member); drops an emptied binding. Returns false if absent.
func siamRemoveMember(policy *run.GoogleIamV1Policy, role, member string) bool {
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

func siamHasMember(policy *run.GoogleIamV1Policy, role, member string) bool {
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

func (p *ServiceIamMemberProvisioner) withRetryOnConflict(
	ctx context.Context,
	svc *run.Service,
	resourcePath string,
	mutate func(*run.GoogleIamV1Policy) bool,
) error {
	var lastErr error
	for attempt := 0; attempt < siamMaxAttempts; attempt++ {
		policy, err := svc.Projects.Locations.Services.GetIamPolicy(resourcePath).Context(ctx).Do()
		if err != nil {
			return err
		}
		if !mutate(policy) {
			return nil
		}
		_, err = svc.Projects.Locations.Services.SetIamPolicy(resourcePath, &run.GoogleIamV1SetIamPolicyRequest{Policy: policy}).Context(ctx).Do()
		if err == nil {
			return nil
		}
		lastErr = err
		var ge *googleapi.Error
		if !errors.As(err, &ge) || ge.Code != 409 {
			return err
		}
		// 409 = etag conflict, retry
	}
	return fmt.Errorf("setIamPolicy: %d attempts hit etag conflict: %w", siamMaxAttempts, lastErr)
}

func (p *ServiceIamMemberProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	props, err := siamProps(request.Properties)
	if err != nil {
		return siamCreateFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	cfg := siamCfg(request.TargetConfig, p.cfg)
	resourcePath, err := servicePath(props.Service, cfg, props.Location, props.Project)
	if err != nil {
		return siamCreateFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	svc, err := p.newService(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := p.withRetryOnConflict(ctx, svc, resourcePath, func(policy *run.GoogleIamV1Policy) bool {
		return siamAddMember(policy, props.Role, props.Member)
	}); err != nil {
		return siamCreateFailure(siamMapError(err), err.Error()), nil
	}

	// Echo desired props back verbatim so the stored state matches the plan.
	echo, _ := json.Marshal(serviceIamMemberProps{Service: props.Service, Role: props.Role, Member: props.Member})
	return siamCreateSuccess(siamBuildNativeID(props.Service, props.Role, props.Member), echo), nil
}

func (p *ServiceIamMemberProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	service, role, member, err := siamParseNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	cfg := siamCfg(request.TargetConfig, p.cfg)
	resourcePath, err := servicePath(service, cfg, "", "")
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	svc, err := p.newService(ctx, cfg)
	if err != nil {
		return nil, err
	}

	policy, err := svc.Projects.Locations.Services.GetIamPolicy(resourcePath).Context(ctx).Do()
	if err != nil {
		return &resource.ReadResult{ErrorCode: siamMapError(err)}, nil
	}

	if !siamHasMember(policy, role, member) {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	propsJSON, _ := json.Marshal(serviceIamMemberProps{Service: service, Role: role, Member: member})
	return &resource.ReadResult{
		ResourceType: request.ResourceType,
		Properties:   string(propsJSON),
	}, nil
}

func (p *ServiceIamMemberProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// All fields are createOnly; any change plans a replace. No-op safety net.
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

func (p *ServiceIamMemberProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	service, role, member, err := siamParseNativeID(request.NativeID)
	if err != nil {
		return siamDeleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	cfg := siamCfg(request.TargetConfig, p.cfg)
	resourcePath, err := servicePath(service, cfg, "", "")
	if err != nil {
		return siamDeleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	svc, err := p.newService(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// A missing service (404) means the binding is gone too - idempotent success.
	if err := p.withRetryOnConflict(ctx, svc, resourcePath, func(policy *run.GoogleIamV1Policy) bool {
		return siamRemoveMember(policy, role, member)
	}); err != nil {
		if code := siamMapError(err); code != resource.OperationErrorCodeNotFound {
			return siamDeleteFailure(code, err.Error()), nil
		}
	}

	return siamDeleteSuccess(request.NativeID), nil
}

func (p *ServiceIamMemberProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// Enumerating every binding requires walking every service's policy. Cloud
	// Run has no cheap cross-service policy listing, so return empty (discovery
	// of individual bindings is out of scope, matching ProjectIamMember's intent).
	return &resource.ListResult{}, nil
}

func (p *ServiceIamMemberProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	// setIamPolicy is synchronous; nothing to poll.
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
			RequestID:       request.RequestID,
		},
	}, nil
}

func siamMapError(err error) resource.OperationErrorCode {
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

func siamCreateSuccess(nativeID string, props json.RawMessage) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           nativeID,
			ResourceProperties: props,
		},
	}
}

func siamCreateFailure(code resource.OperationErrorCode, msg string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
		},
	}
}

func siamDeleteSuccess(nativeID string) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        nativeID,
		},
	}
}

func siamDeleteFailure(code resource.OperationErrorCode, msg string) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
		},
	}
}
