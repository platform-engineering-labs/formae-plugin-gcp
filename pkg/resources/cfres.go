// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package resources

import (
	// Import all provisioners to trigger their init() functions
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/bigquery"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/bigtable"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/cloudrun"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/compute"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/container"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/sql"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/storage"
)
