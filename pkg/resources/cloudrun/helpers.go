// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

// getStringMap extracts a map[string]interface{} from a map
func getStringMap(m map[string]interface{}, key string) map[string]interface{} {
	if val, ok := m[key]; ok {
		if strMap, ok := val.(map[string]interface{}); ok {
			return strMap
		}
	}
	return nil
}

// getStringArray extracts a string array from a map
func getStringArray(props map[string]interface{}, key string) []string {
	if val, ok := props[key]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return nil
}
