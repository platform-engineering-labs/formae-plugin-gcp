// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func unmarshalProps(raw json.RawMessage) (map[string]interface{}, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(raw, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}
	return base.UnwrapValues(props), nil
}

func mustMarshal(props map[string]interface{}) json.RawMessage {
	encoded, err := json.Marshal(props)
	if err != nil {
		return json.RawMessage("{}")
	}
	return encoded
}

func createFailure(code resource.OperationErrorCode, message string) *resource.CreateResult {
	return &resource.CreateResult{ProgressResult: &resource.ProgressResult{
		Operation:       resource.OperationCreate,
		OperationStatus: resource.OperationStatusFailure,
		ErrorCode:       code,
		StatusMessage:   message,
	}}
}

func deleteSuccess(nativeID, message string) *resource.DeleteResult {
	return &resource.DeleteResult{ProgressResult: &resource.ProgressResult{
		Operation:       resource.OperationDelete,
		OperationStatus: resource.OperationStatusSuccess,
		NativeID:        nativeID,
		StatusMessage:   message,
	}}
}

func deleteFailure(nativeID string, code resource.OperationErrorCode, message string) *resource.DeleteResult {
	return &resource.DeleteResult{ProgressResult: &resource.ProgressResult{
		Operation:       resource.OperationDelete,
		OperationStatus: resource.OperationStatusFailure,
		ErrorCode:       code,
		NativeID:        nativeID,
		StatusMessage:   message,
	}}
}
