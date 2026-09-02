// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// packetMirroringSelectors are the three ways a PacketMirroring names whose
// traffic it copies. They are additive, and the API requires at least one.
var packetMirroringSelectors = []string{"instances", "subnetworks", "tags"}

// packetMirroringRequestTransformer prepares a PacketMirroring body.
//
// On update it does two things the API forces:
//
//  1. Drops "network". packetMirrorings.patch refuses any body that carries the
//     network in a form other than the exact stored URL - "Network cannot be
//     changed" - and a forma names it by self link, short path or bare name
//     interchangeably. The field is createOnly, so nothing is lost by leaving it
//     out and a change to it replaces the resource instead.
//
//  2. Fills the absent selectors of "mirroredResources" with empty lists. patch
//     is a JSON merge patch: a key left out of the object keeps its old value,
//     so a forma that drops every tag would leave the tags mirroring in place
//     and the plugin would report success on a change that never happened. An
//     explicit empty list does clear the selector, which is what the forma
//     means. A selector that is present is sent as given - a list in the body
//     replaces rather than merges.
//
// Create bodies pass through untouched: "network" is required there, and
// padding the selectors would say nothing the body does not already say.
func packetMirroringRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	if ctx.Operation != resource.OperationUpdate {
		return props, nil
	}

	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "network" {
			continue
		}
		out[k] = v
	}

	if mirrored, ok := out["mirroredResources"].(map[string]interface{}); ok {
		filled := make(map[string]interface{}, len(mirrored)+len(packetMirroringSelectors))
		for k, v := range mirrored {
			filled[k] = v
		}
		for _, selector := range packetMirroringSelectors {
			if _, present := filled[selector]; !present {
				filled[selector] = []interface{}{}
			}
		}
		out["mirroredResources"] = filled
	}

	return out, nil
}

// packetMirroringResponseTransformer normalizes a PacketMirroring read.
//
// Every reference in this resource is an object rather than a bare string, and
// GCP answers each one with a second "canonicalUrl" holding the same target
// spelled by numeric id. It is output-only and appears nowhere in a forma, but
// it sits inside a sub-resource, where a schema hint cannot reach it - so it has
// to be stripped here or every read disagrees with the declaration and plans an
// update that changes nothing.
//
// "region" comes back as a full URL and is reduced to its last segment, matching
// every other regional compute type.
func packetMirroringResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{}, len(apiResponse))
	for k, v := range apiResponse {
		result[k] = v
	}

	for _, field := range []string{"network", "collectorIlb"} {
		if ref, ok := result[field].(map[string]interface{}); ok {
			result[field] = stripCanonicalURL(ref)
		}
	}

	if mirrored, ok := result["mirroredResources"].(map[string]interface{}); ok {
		cleaned := make(map[string]interface{}, len(mirrored))
		for k, v := range mirrored {
			switch k {
			case "instances", "subnetworks":
				cleaned[k] = stripCanonicalURLs(v)
			default:
				cleaned[k] = v
			}
		}
		result["mirroredResources"] = cleaned
	}

	if region, ok := result["region"].(string); ok && region != "" {
		result["region"] = base.ExtractLastSegment(region)
	}

	return result
}

// stripCanonicalURLs removes canonicalUrl from every entry of a list of
// reference objects, leaving anything that is not one alone.
func stripCanonicalURLs(value interface{}) interface{} {
	entries, ok := value.([]interface{})
	if !ok {
		return value
	}
	cleaned := make([]interface{}, len(entries))
	for i, entry := range entries {
		if ref, ok := entry.(map[string]interface{}); ok {
			cleaned[i] = stripCanonicalURL(ref)
			continue
		}
		cleaned[i] = entry
	}
	return cleaned
}

// stripCanonicalURL copies a reference object without its canonicalUrl.
func stripCanonicalURL(ref map[string]interface{}) map[string]interface{} {
	cleaned := make(map[string]interface{}, len(ref))
	for k, v := range ref {
		if k == "canonicalUrl" {
			continue
		}
		cleaned[k] = v
	}
	return cleaned
}

// PacketMirroringRequestTransformer is the request transformer for PacketMirroring.
var PacketMirroringRequestTransformer = base.RequestTransformerFunc(packetMirroringRequestTransformer)

// PacketMirroringResponseTransformer is the response transformer for PacketMirroring.
var PacketMirroringResponseTransformer = base.ResponseTransformerFunc(packetMirroringResponseTransformer)
