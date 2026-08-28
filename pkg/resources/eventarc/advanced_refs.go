// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package eventarc

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// An enrollment and a googleApiSource each point at another Eventarc Advanced
// resource through a single string field holding a full resource path. A forma
// wants to pass a resolvable instead - `bus.res.name`, which resolves to the
// bus's short name and gives formae the ordering edge - so the request expands
// the short name and the response shortens it back. Without both halves the
// declared value could never equal the value read back and every comparison
// step would report drift on a resource that is in fact correct.
//
// This is the scalar counterpart of pipelineRequestTransformer, which does the
// same job for the message bus nested inside each of a pipeline's destinations.

// expandRef rewrites body[field] from a bare name to the full resource path.
// A value that already contains a "/" is left alone, so a forma may still write
// the full path by hand.
func expandRef(body map[string]interface{}, field, collection, project, location string) {
	name, ok := body[field].(string)
	if !ok || name == "" || strings.Contains(name, "/") {
		return
	}
	body[field] = fmt.Sprintf("projects/%s/locations/%s/%s/%s", project, location, collection, name)
}

// shortenRef is the mirror of expandRef.
func shortenRef(out map[string]interface{}, field, collection string) {
	path, ok := out[field].(string)
	if !ok {
		return
	}
	if i := strings.LastIndex(path, "/"+collection+"/"); i >= 0 {
		out[field] = path[i+len(collection)+2:]
	}
}

// advancedRefTransformers builds the request/response pair for a resource whose
// only cross-resource references are scalar path fields. refs maps a property
// name to the collection its value names.
func advancedRefTransformers(collection string, refs map[string]string) (base.RequestTransformerFunc, base.ResponseTransformerFunc) {
	request := func(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
		body, err := eventarcRequestTransformer(props, ctx)
		if err != nil {
			return nil, err
		}
		location, _ := props["location"].(string)
		if location == "" {
			location = ctx.Location
		}
		for field, target := range refs {
			expandRef(body, field, target, ctx.Project, location)
		}
		return body, nil
	}

	response := func(props map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
		out := locationResponseTransformer(collection)(props, ctx)
		for field, target := range refs {
			shortenRef(out, field, target)
		}
		return out
	}

	return request, response
}
