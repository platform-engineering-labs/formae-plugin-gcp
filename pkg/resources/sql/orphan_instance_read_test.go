// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/api/googleapi"
)

// Cloud SQL answers a nested collection under a deleted instance with 403
// notAuthorized rather than 404, so a database whose instance is gone must be
// recognised from the error alone. Established live against project
// development-477117 on 2026-09-08:
//
//	GET instances/gone              -> 404 instanceDoesNotExist
//	GET instances/gone/databases/x  -> 403 notAuthorized
//	GET instances/gone/users        -> 403 notAuthorized
//	GET instances/gone/sslCerts     -> 403 notAuthorized
//	GET instances/gone/backupRuns   -> 403 notAuthorized
func TestParentInstanceGone(t *testing.T) {
	notAuthorized := &googleapi.Error{
		Code:    403,
		Message: "The client is not authorized to make this request.",
		Errors:  []googleapi.ErrorItem{{Reason: "notAuthorized"}},
	}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"403 notAuthorized", notAuthorized, true},
		{"403 notAuthorized, wrapped", fmt.Errorf("failed to read resource: %w", notAuthorized), true},
		{
			"403 for a real permission problem",
			&googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "forbidden"}}},
			false,
		},
		{
			"403 with no reason at all",
			&googleapi.Error{Code: 403},
			false,
		},
		{
			"404 - base already reads this as missing",
			&googleapi.Error{Code: 404, Errors: []googleapi.ErrorItem{{Reason: "instanceDoesNotExist"}}},
			false,
		},
		{"401", &googleapi.Error{Code: 401, Errors: []googleapi.ErrorItem{{Reason: "notAuthorized"}}}, false},
		{"not a googleapi error", errors.New("notAuthorized"), false},
		{"nil", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parentInstanceGone(tc.err); got != tc.want {
				t.Errorf("parentInstanceGone() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Every collection that hangs off an instance needs the hook; the instance
// itself must not have it, because a 403 on an instance is a real one.
func TestNestedSQLTypesTreatMissingParentAsGone(t *testing.T) {
	for _, rt := range []string{
		DatabaseResourceType,
		UserResourceType,
		SslCertResourceType,
		BackupRunResourceType,
	} {
		def, ok := sqlRegistry.Definitions[rt]
		if !ok {
			t.Fatalf("%s not registered", rt)
		}
		if def.ResourceConfig.ReadErrorTreatAsMissing == nil {
			t.Errorf("%s: ReadErrorTreatAsMissing not set - a read of an orphan "+
				"fails the whole sync command instead of reporting it gone", rt)
		}
	}

	def := sqlRegistry.Definitions[DatabaseInstanceResourceType]
	if def.ResourceConfig.ReadErrorTreatAsMissing != nil {
		t.Error("DatabaseInstance: 403 on an instance is a genuine authorization " +
			"failure - instances.get 404s when the instance is gone")
	}
}
