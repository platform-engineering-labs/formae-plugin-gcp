// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

// bucketScopedBodyBuilder filters out bucket and name properties that are used in the URL
func bucketScopedBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{})
	for k, v := range props {
		// Skip properties that are part of the URL path
		if k == "bucket" {
			continue
		}
		// Keep name for the body (it's the resource name)
		body[k] = v
	}
	return body, nil
}

// aclBodyBuilder filters out bucket property and keeps entity and role
func aclBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{})
	for k, v := range props {
		// Skip bucket (it's in the URL), keep entity and role
		if k == "bucket" {
			continue
		}
		body[k] = v
	}
	return body, nil
}

// objectAclBodyBuilder keeps entity and role, dropping the two properties that
// address the entry in the URL. An object ACL is the only resource here with
// two of them.
func objectAclBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{})
	for k, v := range props {
		if k == "bucket" || k == "object" {
			continue
		}
		body[k] = v
	}
	return body, nil
}
