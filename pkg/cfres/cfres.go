// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cfres

import (
	// Import all provisioners to trigger their init() functions
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/bigquery"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/bigtable"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/cloudrun"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/compute"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/container"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/sql"
)
