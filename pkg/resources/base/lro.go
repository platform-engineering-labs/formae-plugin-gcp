// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import "fmt"

// CheckLROStatus reports whether a Google long-running operation is done, and
// maps a genuine failure to an error.
//
// The subtlety is what counts as a failure. A finished operation may carry an
// "error" key that is present but empty - `"error": {}` - which is not a
// failure, it is an absent status. Treating the key's mere presence as failure
// reports a successful create as failed: formae then retries it, and the retry
// answers "Resource ... already exists" because the first attempt really did
// create the resource. That masks the original operation entirely, and the only
// visible symptom is a create that fails with a collision against itself.
//
// So an error counts only when it carries something: a message, or a non-zero
// google.rpc.Code. Code 0 is OK.
func CheckLROStatus(op map[string]interface{}) (bool, error) {
	done, _ := op["done"].(bool)
	if !done {
		return false, nil
	}

	errObj, ok := op["error"].(map[string]interface{})
	if !ok {
		return true, nil
	}

	msg, _ := errObj["message"].(string)
	code := lroErrorCode(errObj)
	if msg == "" && code == 0 {
		// Present but empty: the operation finished without an error.
		return true, nil
	}
	if msg == "" {
		msg = fmt.Sprintf("operation failed with code %d", code)
	}
	return true, fmt.Errorf("%s", msg)
}

// lroErrorCode reads google.rpc.Code off an operation error, whichever numeric
// shape the JSON decoder produced.
func lroErrorCode(errObj map[string]interface{}) int {
	switch c := errObj["code"].(type) {
	case float64:
		return int(c)
	case int:
		return c
	default:
		return 0
	}
}
