// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package resources

import (
	// Import all provisioners to trigger their init() functions
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/artifactregistry"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/bigquery"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/bigtable"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/certificateauthority"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/cloudrun"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/cloudscheduler"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/cloudtasks"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/compute"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/container"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/dataproc"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/datastream"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/dns"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/essentialcontacts"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/filestore"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/gkehub"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/iam"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/monitoring"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/pubsub"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/redis"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/secretmanager"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/servicenetworking"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/sql"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/storage"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/vpcaccess"
)
