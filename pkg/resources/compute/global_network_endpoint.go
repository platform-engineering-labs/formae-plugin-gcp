// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// GlobalNetworkEndpointProvisioner manages one endpoint inside a global
// (internet) network endpoint group. An endpoint is not a REST resource: it is a
// member of the group, attached and detached with the attachNetworkEndpoints /
// detachNetworkEndpoints verbs, and enumerated with listNetworkEndpoints - which
// is a POST, not a GET. Without endpoints a global NEG serves nothing, so this
// is what makes the group useful.
//
// An endpoint is identified by (group, fqdn-or-ip, port); there is nothing else
// to it, so nothing is updatable - a change detaches and reattaches.
type GlobalNetworkEndpointProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*GlobalNetworkEndpointProvisioner)(nil)

func init() {
	registry.Register(GlobalNetworkEndpointResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &GlobalNetworkEndpointProvisioner{
				BaseResource: &base.BaseResource{
					Config:          cfg,
					APIConfig:       ComputeAPI,
					OperationConfig: ComputeOperations,
					ResourceConfig: base.ResourceConfig{
						ResourceType: "networkEndpointGroups",
						Scope:        &base.ScopeConfig{Type: base.ScopeGlobal},
					},
					NativeIDConfig: ComputeNativeID,
				},
			}
		})
}

// buildEndpointNativeID composes
// "projects/{p}/global/networkEndpointGroups/{neg}/networkEndpoints/{host}|{port}".
// The host and port are joined with a pipe because an FQDN may contain dots and
// an IPv6 literal contains colons, but neither contains a pipe.
func buildEndpointNativeID(project, group, host string, port int) string {
	return fmt.Sprintf("projects/%s/global/networkEndpointGroups/%s/networkEndpoints/%s|%d",
		project, group, host, port)
}

func parseEndpointNativeID(nativeID string) (project, group, host string, port int, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 7 || parts[0] != "projects" || parts[2] != "global" ||
		parts[3] != "networkEndpointGroups" || parts[5] != "networkEndpoints" {
		return "", "", "", 0, fmt.Errorf("invalid global network endpoint native ID: %s", nativeID)
	}
	host, portStr, found := strings.Cut(parts[6], "|")
	if !found || host == "" {
		return "", "", "", 0, fmt.Errorf("invalid endpoint key %q in native ID: %s", parts[6], nativeID)
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("invalid port in native ID %s: %w", nativeID, err)
	}
	return parts[1], parts[4], host, port, nil
}

// endpointOf builds the API's networkEndpoint object. A global NEG endpoint is
// addressed either by fqdn or by ipAddress, never both.
func endpointOf(props map[string]interface{}) (map[string]interface{}, string, int, error) {
	endpoint := map[string]interface{}{}
	host := ""
	if fqdn, ok := props["fqdn"].(string); ok && fqdn != "" {
		endpoint["fqdn"] = fqdn
		host = fqdn
	}
	if ip, ok := props["ipAddress"].(string); ok && ip != "" {
		endpoint["ipAddress"] = ip
		if host != "" {
			return nil, "", 0, fmt.Errorf("set either fqdn or ipAddress, not both")
		}
		host = ip
	}
	if host == "" {
		return nil, "", 0, fmt.Errorf("one of fqdn or ipAddress is required")
	}
	port, err := portOf(props)
	if err != nil {
		return nil, "", 0, err
	}
	endpoint["port"] = port
	return endpoint, host, port, nil
}

func portOf(props map[string]interface{}) (int, error) {
	switch v := props["port"].(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("port is required and must be a number")
	}
}

func (p *GlobalNetworkEndpointProvisioner) groupURL(project, group string) string {
	return fmt.Sprintf("%s/projects/%s/global/networkEndpointGroups/%s",
		p.APIConfig.BaseURL, project, group)
}

func (p *GlobalNetworkEndpointProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.PathFromTargetConfig(targetConfig); cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

func (p *GlobalNetworkEndpointProvisioner) issueVerb(
	ctx context.Context, url string, endpoint map[string]interface{}, project string,
) (string, *transport.Error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return "", transport.WrapError(err, "failed to create transport client")
	}
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    url,
		Body:   map[string]interface{}{"networkEndpoints": []interface{}{endpoint}},
	})
	if err != nil {
		return "", transport.WrapError(err, "network endpoint verb failed")
	}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	return p.OperationConfig.OperationURLBuilder(base.PathContext{Project: project}, opID), nil
}

// listEndpoints enumerates a group's members. listNetworkEndpoints is a POST.
func (p *GlobalNetworkEndpointProvisioner) listEndpoints(
	ctx context.Context, project, group string,
) ([]map[string]interface{}, *transport.Error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, transport.WrapError(err, "failed to create transport client")
	}
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    p.groupURL(project, group) + "/listNetworkEndpoints",
		Body:   map[string]interface{}{},
	})
	if err != nil {
		return nil, transport.WrapError(err, "failed to list network endpoints")
	}
	items, _ := resp.Body["items"].([]interface{})
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		wrapper, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if endpoint, ok := wrapper["networkEndpoint"].(map[string]interface{}); ok {
			out = append(out, endpoint)
		}
	}
	return out, nil
}

func (p *GlobalNetworkEndpointProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	group, _ := props["networkEndpointGroup"].(string)
	if group == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"networkEndpointGroup is required"), nil
	}
	endpoint, host, port, err := endpointOf(props)
	if err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"target project is required"), nil
	}

	requestID, verbErr := p.issueVerb(ctx,
		p.groupURL(project, group)+"/attachNetworkEndpoints", endpoint, project)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        buildEndpointNativeID(project, group, host, port),
			RequestID:       requestID,
			StatusMessage:   "network endpoint attachment in progress",
		},
	}, nil
}

func (p *GlobalNetworkEndpointProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, group, host, port, err := parseEndpointNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	endpoints, verbErr := p.listEndpoints(ctx, project, group)
	if verbErr != nil {
		return &resource.ReadResult{
			ErrorCode: transport.ToResourceErrorCode(verbErr.Code),
		}, nil
	}
	for _, endpoint := range endpoints {
		epPort, _ := portOf(endpoint)
		fqdn, _ := endpoint["fqdn"].(string)
		ip, _ := endpoint["ipAddress"].(string)
		if epPort != port || (fqdn != host && ip != host) {
			continue
		}
		props := map[string]interface{}{
			"networkEndpointGroup": group,
			"port":                 epPort,
		}
		if fqdn != "" {
			props["fqdn"] = fqdn
		}
		if ip != "" {
			props["ipAddress"] = ip
		}
		encoded, mErr := json.Marshal(props)
		if mErr != nil {
			return nil, fmt.Errorf("failed to marshal endpoint properties: %w", mErr)
		}
		return &resource.ReadResult{Properties: string(encoded)}, nil
	}
	// The group is there but this endpoint is not, so formae must learn it is
	// gone rather than see a generic failure.
	return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
}

func (p *GlobalNetworkEndpointProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	return updateFailure(resource.OperationErrorCodeNotUpdatable,
		"a network endpoint is a (host, port) pair; a change detaches and reattaches"), nil
}

func (p *GlobalNetworkEndpointProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, group, host, port, err := parseEndpointNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	// Detach needs the same shape as attach. Re-derive which field the host is:
	// an FQDN has letters, an IP does not.
	endpoint := map[string]interface{}{"port": port}
	if strings.ContainsAny(host, "abcdefghijklmnopqrstuvwxyz") {
		endpoint["fqdn"] = host
	} else {
		endpoint["ipAddress"] = host
	}

	requestID, verbErr := p.issueVerb(ctx,
		p.groupURL(project, group)+"/detachNetworkEndpoints", endpoint, project)
	if verbErr != nil {
		return deleteFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   "network endpoint detachment in progress",
		},
	}, nil
}

// List enumerates endpoints. Discovery calls this with no hints, so when no
// group is named it walks every global network endpoint group and reports the
// endpoints inside them — without that an endpoint can be managed but never
// discovered. A named group is still honoured as a fast path.
func (p *GlobalNetworkEndpointProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	var groups []string
	if request.AdditionalProperties != nil {
		if group := request.AdditionalProperties["networkEndpointGroup"]; group != "" {
			groups = []string{group}
		}
	}
	if groups == nil {
		discovered, gErr := p.listGroups(ctx, project)
		if gErr != nil {
			return nil, fmt.Errorf("%s", gErr.Message)
		}
		groups = discovered
	}

	nativeIDs := []string{}
	for _, group := range groups {
		endpoints, verbErr := p.listEndpoints(ctx, project, group)
		if verbErr != nil {
			// One unreadable group must not hide the others.
			continue
		}
		for _, endpoint := range endpoints {
			port, err := portOf(endpoint)
			if err != nil {
				continue
			}
			host, _ := endpoint["fqdn"].(string)
			if host == "" {
				host, _ = endpoint["ipAddress"].(string)
			}
			if host == "" {
				continue
			}
			nativeIDs = append(nativeIDs, buildEndpointNativeID(project, group, host, port))
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// listGroups names every global network endpoint group in the project, so
// discovery has somewhere to look.
func (p *GlobalNetworkEndpointProvisioner) listGroups(
	ctx context.Context, project string,
) ([]string, *transport.Error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, transport.WrapError(err, "failed to create transport client")
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL: fmt.Sprintf("%s/projects/%s/global/networkEndpointGroups",
			p.APIConfig.BaseURL, project),
	})
	if rErr != nil {
		return nil, transport.WrapError(rErr, "failed to list global network endpoint groups")
	}
	items, _ := resp.Body["items"].([]interface{})
	groups := make([]string, 0, len(items))
	for _, raw := range items {
		group, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := group["name"].(string); name != "" {
			groups = append(groups, name)
		}
	}
	return groups, nil
}

// Status routes through the shared read-back so post-create and post-update
// state carries the resource's real properties, not just what was declared.
func (p *GlobalNetworkEndpointProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}
