package utils

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// ParseProperties converts a JSON RawMessage or string into a map that can be used with GetString, GetBool, etc.
func ParseProperties(data interface{}) (map[string]interface{}, error) {
	var props map[string]interface{}

	switch v := data.(type) {
	case json.RawMessage:
		if err := json.Unmarshal(v, &props); err != nil {
			return nil, fmt.Errorf("failed to unmarshal json.RawMessage: %w", err)
		}
	case []byte:
		if err := json.Unmarshal(v, &props); err != nil {
			return nil, fmt.Errorf("failed to unmarshal []byte: %w", err)
		}
	case string:
		if err := json.Unmarshal([]byte(v), &props); err != nil {
			return nil, fmt.Errorf("failed to unmarshal string: %w", err)
		}
	case map[string]interface{}:
		// Already a map, just return it
		props = v
	default:
		return nil, fmt.Errorf("unsupported type: %T", data)
	}

	return props, nil
}

// MustParseProperties is like ParseProperties but panics on error.
// Useful in tests where you want to fail fast.
func MustParseProperties(data interface{}) map[string]interface{} {
	props, err := ParseProperties(data)
	if err != nil {
		panic(err)
	}
	return props
}

func GetString(props map[string]interface{}, key string) string {
	if val, ok := props[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func GetBool(props map[string]interface{}, key string) bool {
	if val, ok := props[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

func GetInt64(props map[string]interface{}, key string) int64 {
	if val, ok := props[key]; ok {
		switch v := val.(type) {
		case string:
			i, err := strconv.Atoi(v)
			if err == nil {
				return int64(i)
			}
		case int64:
			return int64(v)
		case int:
			return int64(v)
		case float64:
			return int64(v)
		}
	}
	return 0
}

func GetInt32(props map[string]interface{}, key string) int32 {
	if val, ok := props[key]; ok {
		switch v := val.(type) {
		case int32:
			return v
		case int64:
			return int32(v)
		case int:
			return int32(v)
		case float64:
			return int32(v)
		case float32:
			return int32(v)
		}
	}
	return 0
}

func GetObject(props map[string]interface{}, key string) map[string]interface{} {
	if val, ok := props[key]; ok {
		if obj, ok := val.(map[string]interface{}); ok {
			return obj
		}
	}
	return nil
}

func GetArray(props map[string]interface{}, key string) []interface{} {
	if val, ok := props[key]; ok {
		if arr, ok := val.([]interface{}); ok {
			return arr
		}
	}
	return nil
}

func ToInt64Ptr(v int64) *int64 {
	return &v
}
