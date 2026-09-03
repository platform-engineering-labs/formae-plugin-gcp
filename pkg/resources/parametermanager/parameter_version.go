// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package parametermanager

import (
	"encoding/base64"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// encodeParameterVersionPayload lifts the declared `data` string into the
// nested payload the API wants, base64 encoding it on the way:
//
//	{"data": "hello"}  ->  {"payload": {"data": "aGVsbG8="}}
//
// Same contract as GCP::SecretManager::SecretVersion's `data`: a forma declares
// UTF-8 plaintext and the plugin does the encoding, so nobody has to hand-write
// base64 in a config file. base.UnwrapValues has already run by this point, so
// a value wrapped with `formae.value(...).opaque` arrives here as the plain
// string.
//
// On update the whole thing is dropped instead. payload is immutable - PATCH
// with updateMask=payload answers 400 IMMUTABLE_FIELD, and a PATCH that carries
// payload with no mask at all answers 200 and silently ignores it, which is
// worse - and UpdateMaskFromBody would put it in the mask. A payload change is
// a replace, which the createOnly hint in the schema is what plans.
var encodeParameterVersionPayload = base.RequestTransformerFunc(
	func(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
		out := make(map[string]interface{}, len(props))
		for k, v := range props {
			if k == "data" {
				continue
			}
			out[k] = v
		}
		if ctx.Operation == resource.OperationUpdate {
			return out, nil
		}
		if data, ok := props["data"].(string); ok && data != "" {
			out["payload"] = map[string]interface{}{
				"data": base64.StdEncoding.EncodeToString([]byte(data)),
			}
		}
		return out, nil
	})

// parameterVersionRequestTransformer shapes every request body.
//
//	parameter - a path component, dropped unconditionally: the API rejects it as
//	            an unknown body field on create as well as on update.
//	data      - encoded into payload on create, dropped on update; see above.
//	name      - dropped on update only. Create reads the version id out of it
//	            for ?parameterVersionId=, and on update it is the path, so
//	            leaving it in would land "name" in the updateMask and the patch
//	            would be refused with 400 IMMUTABLE_FIELD.
var parameterVersionRequestTransformer = &base.CompositeRequestTransformer{
	Transformers: []base.RequestTransformer{
		base.DropFields("parameter"),
		base.DropFieldsOnUpdate("name"),
		encodeParameterVersionPayload,
	},
}

// parentParameterOf lifts the owning parameter out of a version's full resource
// path, projects/{p}/locations/global/parameters/{parameter}/versions/{version}.
// It returns "" for anything else, so a response that is not shaped like a
// version leaves the property alone rather than gaining a wrong one.
func parentParameterOf(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) == 8 && parts[4] == "parameters" && parts[6] == "versions" {
		return parts[5]
	}
	return ""
}

// dropParameterVersionPayload removes the payload from every response, and this
// is a security control rather than a drift fix.
//
// The payload is user data and may well be a secret - Parameter Manager's own
// documented use is holding configuration that references Secret Manager
// versions. Unlike Secret Manager, which never returns secret material, this
// API hands it straight back: a GET defaults to view=FULL and answers with
// payload.data, and so does the create response. Anything left in the response
// map becomes the resource's stored properties verbatim, so without this the
// payload of every parameter version in the project would sit in formae's state
// as trivially reversible base64 - readable to anyone who can read state, and
// copied into every plan that touches the resource.
//
// So the payload is stripped here and `data` is declared writeOnly in the
// schema, which is exactly how GCP::SecretManager::SecretVersion handles the
// same problem; the difference is only that Secret Manager's API withholds the
// material and this one has to be made to.
//
// The `view=BASIC` query parameter would also withhold it, but base has no hook
// for read-time query parameters and the versions:render method (which
// resolves Secret Manager references in the payload) is deliberately never
// called from any path here - it would put the resolved secret in state too.
var dropParameterVersionPayload = base.ResponseTransformerFunc(
	func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		delete(apiResponse, "payload")
		return apiResponse
	})

// parameterVersionResponseTransformer puts back what only the path carries,
// drops the payload, and shortens the name.
//
// The owning parameter is recovered from the reported name before the name is
// shortened - it is a path component, never a body field, so without this the
// property a forma declared is simply missing from state and reads as drift.
// Recovering it from the response rather than from the transform context is
// what makes discovery work too: a List goes through the "-" wildcard parent,
// so the context has no parent to offer, but every listed item still reports
// its own full path.
//
// `disabled` is left alone. The API omits it when false, which is the ordinary
// proto3 bool omission, and it is a top-level bool so a hasProviderDefault hint
// reaches it in the schema.
var parameterVersionResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		base.ResponseTransformerFunc(
			func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
				if name, ok := apiResponse["name"].(string); ok {
					if parameter := parentParameterOf(name); parameter != "" {
						apiResponse["parameter"] = parameter
					}
				}
				return apiResponse
			}),
		dropParameterVersionPayload,
		base.ShortNameResponseTransformer,
	},
}
