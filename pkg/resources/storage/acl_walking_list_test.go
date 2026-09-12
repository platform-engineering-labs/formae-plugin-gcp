// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"reflect"
	"testing"
)

// A bucket with uniform bucket-level access has no legacy ACLs by definition:
// Cloud Storage answers the ACL collections on it with 400 rather than an
// empty list. Walking it is a wasted request that reads as a failure, and in a
// project where every bucket is uniform (the default for new buckets) the
// walk's every-bucket-failed guard turns "no ACLs exist" into an error on
// every discovery cycle. The bucket listing already says which ones are
// uniform, so those are left out before any ACL is asked for.
func TestBucketsWithLegacyACLsLeavesOutUniformBuckets(t *testing.T) {
	items := []interface{}{
		map[string]interface{}{"name": "legacy"},
		map[string]interface{}{"name": "uniform", "iamConfiguration": map[string]interface{}{
			"uniformBucketLevelAccess": map[string]interface{}{"enabled": true},
		}},
		map[string]interface{}{"name": "explicitly-legacy", "iamConfiguration": map[string]interface{}{
			"uniformBucketLevelAccess": map[string]interface{}{"enabled": false},
		}},
		map[string]interface{}{"name": "uniform-old-field", "iamConfiguration": map[string]interface{}{
			"bucketPolicyOnly": map[string]interface{}{"enabled": true},
		}},
		map[string]interface{}{"iamConfiguration": map[string]interface{}{}},
		"not a bucket",
	}
	got := bucketsWithLegacyACLs(items)
	want := []string{"legacy", "explicitly-legacy"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bucketsWithLegacyACLs = %v, want %v", got, want)
	}
}
