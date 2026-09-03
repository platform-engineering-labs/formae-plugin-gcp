// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package apikeys

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// globalLocation is the only location an API key can live in.
//
// The path has a location segment and the discovery document is explicit that
// it is decoration: "Key is a global resource; hence the only supported value
// for location is `global`". Injected by the path builder rather than declared,
// so a target's configured region cannot reach the URL. The resources here are
// registered ScopeGlobal, which clears ctx.Location for the same reason.
const globalLocation = "global"

// ApiKeysAPI - API Keys API v2.
var ApiKeysAPI = base.APIConfig{
	BaseURL:     "https://apikeys.googleapis.com/v2",
	APIVersion:  "v2",
	PathBuilder: apiKeysPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// ApiKeysOperations - asynchronous. create, patch and delete all answer with an
// Operation whose name is "operations/akmf.p7-{project-number}-{uuid}"; base
// polls BaseURL + "/" + that name until it reports done.
//
// Polling is not optional even though these settle in a second or two, and this
// API makes the reason unusually stark: a create that is going to fail with
// ALREADY_EXISTS still answers HTTP 200 with an accepted operation, and the
// failure appears only in the polled operation's "error" (verified live —
// see the keyId reservation note in resources.go).
var ApiKeysOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractAPIKeyNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// ApiKeysNativeID - full path
// "projects/{project}/locations/global/keys/{key}".
var ApiKeysNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseAPIKeyNativeID,
}

// apiKeysPathBuilder builds /projects/{project}/locations/global/keys[/{name}].
func apiKeysPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, globalLocation, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// parseAPIKeyNativeID restores the context a read needs.
func parseAPIKeyNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != keyCollection {
		return base.PathContext{}, fmt.Errorf("invalid api key native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      parts[1],
		ResourceType: parts[4],
		ResourceName: parts[5],
	}, nil
}

// extractAPIKeyNativeID builds the resource path, always with the project as
// the target spells it.
//
// This API answers every call — the create operation's payload, a GET, and each
// list item — with `name` in project-NUMBER form:
// "projects/989754770009/locations/global/keys/{key}". Both forms address the
// key (verified live: a GET under the project id answers 200), so the choice is
// only about which one lands in state, and it has to be one of them: a managed
// key whose native ID came from a create and a discovered key whose native ID
// came from a list would otherwise be the same key under two identities, and
// discovery would offer to import a resource that is already managed.
//
// The project id wins because it is what the target declares and what every
// other type in this plugin stores. So the key id is taken from the last
// segment and the path rebuilt from context; the response name is used verbatim
// only when there is no context to rebuild from, which in practice is the
// Status poll, and that prefers the native ID the create already established.
func extractAPIKeyNativeID(response map[string]interface{}, ctx base.PathContext) string {
	name, _ := response["name"].(string)
	// An Operation's name is not a resource path.
	if strings.HasPrefix(name, "operations/") {
		name = ""
	}

	keyID := ctx.ResourceName
	if keyID == "" && name != "" {
		keyID = name[strings.LastIndex(name, "/")+1:]
	}
	if keyID == "" || ctx.Project == "" {
		if strings.HasPrefix(name, "projects/") {
			return name
		}
		return ""
	}
	return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
		ctx.Project, globalLocation, keyCollection, keyID)
}

// extractOperationName returns the LRO operation name ("operations/akmf.p7-...")
// from a mutating response.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.HasPrefix(name, "operations/") {
		return name
	}
	return ""
}

// checkOperationStatus reports whether a polled Operation is done, mapping a
// present "error" to a terminal failure.
//
// The error mapping carries real weight here: this API reports a rejected
// create — a duplicate key id, a masked immutable field — as HTTP 200 plus an
// operation that later completes with an error, so an implementation that only
// looked at "done" would report a create that never happened as a success.
func checkOperationStatus(op map[string]interface{}) (bool, error) {
	done, _ := op["done"].(bool)
	if !done {
		return false, nil
	}
	if errObj, ok := op["error"].(map[string]interface{}); ok {
		msg, _ := errObj["message"].(string)
		if msg == "" {
			msg = "operation failed"
		}
		return true, fmt.Errorf("%s", msg)
	}
	return true, nil
}
