// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigquery

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// A row access policy hangs off a table, which hangs off a dataset, and
// discovery lists with no properties at all - so it can name neither, and the
// generic List would address "/projects/{p}/datasets//tables//rowAccessPolicies".
//
// BigQuery accepts no wildcard for either segment. Verified live:
//
//	GET .../projects/{p}/datasets/-/tables/-/rowAccessPolicies
//	  -> 404 "Not found: Dataset {p}:-"
//	GET .../projects/{p}/datasets/{real dataset}/tables/-/rowAccessPolicies
//	  -> 404 "Not found: Dataset {p}:{real dataset}"
//
// The second is worth noting: the wildcard table is what is rejected, but the
// message blames the dataset, which does exist. There is no "-" parent to
// substitute the way Certificate Manager and Network Security have one, so the
// only way to discover a policy is to walk the two collections above it, in the
// shape pkg/resources/servicedirectory/list.go established.
type rowAccessPolicyListProvisioner struct {
	prov.Provisioner
	cfg *config.Config
}

// registerRowAccessPolicyListWalker replaces only the List entry; create, read,
// update and delete keep the generic implementation. Called from the package
// init in resources.go - see the comment there for why not from an init here.
func registerRowAccessPolicyListWalker() {
	registry.Register(RowAccessPolicyResourceType,
		[]resource.Operation{resource.OperationList},
		func(cfg *config.Config) prov.Provisioner {
			return &rowAccessPolicyListProvisioner{
				Provisioner: bigQueryRegistry.CreateProvisioner(cfg, RowAccessPolicyResourceType),
				cfg:         cfg,
			}
		})
}

func (p *rowAccessPolicyListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A caller that names both parents is telling us exactly where to look, and
	// the generic List already builds that URL correctly.
	if request.AdditionalProperties != nil &&
		request.AdditionalProperties["datasetId"] != "" &&
		request.AdditionalProperties["tableId"] != "" {
		return p.Provisioner.List(ctx, request)
	}

	targetPath := config.PathFromTargetConfig(request.TargetConfig)
	if targetPath.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	projectURL := fmt.Sprintf("%s/projects/%s", BigQueryAPI.BaseURL, targetPath.Project)

	// A dataset the caller cannot list at all is a real failure - it is the
	// whole search space - so it is reported rather than swallowed.
	datasets, err := listBigQueryIDs(ctx, client, projectURL+"/datasets",
		"datasets", "datasetReference", "datasetId")
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list bigquery datasets")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	nativeIDs := make([]string, 0)
	for _, dataset := range datasets {
		datasetURL := projectURL + "/datasets/" + dataset
		tables, tableErr := listBigQueryIDs(ctx, client, datasetURL+"/tables",
			"tables", "tableReference", "tableId")
		if tableErr != nil {
			// One unreadable dataset must not hide the policies in the rest.
			continue
		}
		for _, table := range tables {
			policyURL := fmt.Sprintf("%s/tables/%s/%s", datasetURL, table, rowAccessPolicyCollection)
			policies, policyErr := listBigQueryIDs(ctx, client, policyURL,
				rowAccessPolicyCollection, rowAccessPolicyRefField, "policyId")
			if policyErr != nil {
				continue
			}
			for _, policy := range policies {
				nativeIDs = append(nativeIDs, fmt.Sprintf("projects/%s/datasets/%s/tables/%s/%s/%s",
					targetPath.Project, dataset, table, rowAccessPolicyCollection, policy))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// listBigQueryIDs GETs a collection and returns one id per item, read out of the
// nested reference object each BigQuery list item carries
// (datasetReference.datasetId, tableReference.tableId,
// rowAccessPolicyReference.policyId). None of these items has a "name" field, so
// the reference is the only place an id lives.
//
// nextPageToken is followed to the end. Stopping at the first page would drop
// datasets past the API default, and a dropped dataset hides every table under
// it and every policy under those. An empty collection answers "{}" with no key
// at all, which yields no ids and no error - the correct reading.
func listBigQueryIDs(
	ctx context.Context, client *transport.Client, url, collection, refField, idField string,
) ([]string, error) {
	ids := []string{}
	pageURL := url
	for {
		response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: pageURL})
		if err != nil {
			return nil, err
		}
		items, _ := response.Body[collection].([]interface{})
		for _, raw := range items {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			ref, ok := item[refField].(map[string]interface{})
			if !ok {
				continue
			}
			if id, _ := ref[idField].(string); id != "" {
				ids = append(ids, id)
			}
		}
		token, _ := response.Body["nextPageToken"].(string)
		if token == "" {
			return ids, nil
		}
		next, queryErr := transport.AddQueryParam(url, "pageToken", token)
		if queryErr != nil {
			return ids, nil
		}
		pageURL = next
	}
}
