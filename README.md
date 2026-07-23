# GCP Plugin for Formae

[![CI](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/ci.yml)
[![Nightly](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/nightly.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/nightly.yml)

Google Cloud Platform resource plugin for
[formae](https://github.com/platform-engineering-labs/formae). Manage GCP
infrastructure declaratively across Compute, GKE, CloudRun, BigQuery,
Bigtable, Cloud SQL, and Cloud Storage.

## Supported Resources

This plugin supports **65 GCP resource types** across 13 services. See
[`schema/pkl/`](schema/pkl/) for field definitions.

| Resource Type | Description |
|---------------|-------------|
| `GCP::BigQuery::Dataset` | BigQuery dataset (container for tables and views) |
| `GCP::BigQuery::Table` | BigQuery table |
| `GCP::Bigtable::Instance` | Cloud Bigtable instance |
| `GCP::Bigtable::Cluster` | Cloud Bigtable cluster within an instance |
| `GCP::Bigtable::Table` | Cloud Bigtable table |
| `GCP::Bigtable::Backup` | Cloud Bigtable table backup |
| `GCP::Bigtable::MaterializedView` | Cloud Bigtable materialized view |
| `GCP::CloudRun::Service` | Cloud Run service (serverless container) |
| `GCP::CloudRun::Job` | Cloud Run job (run-to-completion) |
| `GCP::CloudRun::WorkerPool` | Cloud Run worker pool |
| `GCP::CloudRun::ServiceIamMember` | IAM (role, member) binding on a Cloud Run service |
| `GCP::CloudRun::Revision` | Cloud Run service revision (discovered/read-only) |
| `GCP::CloudRun::Execution` | Cloud Run job execution (discovered/read-only) |
| `GCP::CloudRun::Task` | Cloud Run job task (discovered/read-only) |
| `GCP::Compute::Network` | VPC network |
| `GCP::Compute::Subnetwork` | VPC subnetwork |
| `GCP::Compute::Firewall` | VPC firewall rule |
| `GCP::Compute::Route` | Static custom route in a VPC |
| `GCP::Compute::Router` | Cloud Router |
| `GCP::Compute::RouterNat` | Cloud NAT configuration on a router |
| `GCP::Compute::Instance` | Compute Engine VM instance |
| `GCP::Compute::Disk` | Persistent disk |
| `GCP::Compute::InstanceGroup` | Unmanaged instance group (with membership) |
| `GCP::Compute::Address` | Regional static IP address |
| `GCP::Compute::GlobalAddress` | Global static IP address (incl. PSA VPC-peering ranges) |
| `GCP::Compute::HealthCheck` | Global health check |
| `GCP::Compute::RegionHealthCheck` | Regional health check |
| `GCP::Compute::BackendService` | Global backend service for load balancers |
| `GCP::Compute::RegionBackendService` | Regional backend service |
| `GCP::Compute::NetworkEndpointGroup` | Network endpoint group (incl. serverless NEG for Cloud Run) |
| `GCP::Compute::UrlMap` | Global URL map |
| `GCP::Compute::RegionUrlMap` | Regional URL map |
| `GCP::Compute::TargetHttpProxy` | Global target HTTP proxy |
| `GCP::Compute::TargetHttpsProxy` | Global target HTTPS proxy |
| `GCP::Compute::TargetTcpProxy` | Global target TCP proxy |
| `GCP::Compute::TargetSslProxy` | Global target SSL proxy |
| `GCP::Compute::RegionTargetHttpProxy` | Regional target HTTP proxy |
| `GCP::Compute::RegionTargetHttpsProxy` | Regional target HTTPS proxy |
| `GCP::Compute::RegionTargetTcpProxy` | Regional target TCP proxy |
| `GCP::Compute::ForwardingRule` | Regional forwarding rule |
| `GCP::Compute::GlobalForwardingRule` | Global forwarding rule |
| `GCP::Compute::TargetPool` | Target pool for network load balancing |
| `GCP::Compute::SslCertificate` | SSL certificate (self-managed or Google-managed) |
| `GCP::Compute::SecurityPolicy` | Cloud Armor security policy |
| `GCP::Container::Cluster` | GKE cluster |
| `GCP::Container::NodePool` | GKE node pool |
| `GCP::GKEHub::Membership` | GKE Hub (fleet) membership |
| `GCP::GKEHub::Feature` | GKE Hub (fleet) feature |
| `GCP::DNS::ManagedZone` | Cloud DNS managed zone |
| `GCP::IAM::ServiceAccount` | IAM service account |
| `GCP::IAM::Role` | Custom IAM role |
| `GCP::IAM::ProjectIamMember` | Project-level IAM (role, member) binding |
| `GCP::PubSub::Topic` | Pub/Sub topic |
| `GCP::PubSub::Subscription` | Pub/Sub subscription |
| `GCP::PubSub::Schema` | Pub/Sub schema |
| `GCP::SecretManager::Secret` | Secret Manager secret |
| `GCP::SecretManager::SecretVersion` | Secret Manager secret version |
| `GCP::ServiceNetworking::Connection` | Private Service Access connection (VPC peering to Google services) |
| `GCP::SQL::DatabaseInstance` | Cloud SQL instance |
| `GCP::SQL::Database` | Cloud SQL database |
| `GCP::Storage::Bucket` | Cloud Storage bucket |
| `GCP::Storage::BucketAccessControl` | Cloud Storage bucket ACL entry |
| `GCP::Storage::ObjectAccessControl` | Cloud Storage object ACL entry |
| `GCP::Storage::DefaultObjectAccessControl` | Cloud Storage default object ACL for a bucket |
| `GCP::Storage::AnywhereCache` | Cloud Storage Anywhere Cache |

## Configuration

### Target Configuration

Configure a GCP target in your Forma file:

```pkl
import "@formae/formae.pkl"
import "@gcp/gcp.pkl"

target: formae.Target = new formae.Target {
  label = "gcp-target"
  namespace = "GCP"
  config = new gcp.Config {
    project = "my-gcp-project-id"
    region = "us-central1"
    // Optional, set when working with zonal resources
    // zone = "us-central1-a"
    // location = "us-central1"  // GKE / CloudRun
  }
}
```

### Credentials

The plugin reads credentials from environment variables. The plugin does
**not** read credentials from the target config — this prevents sensitive
material from being persisted in formae's database.

Resolution order:

1. **`GCP_CREDENTIALS_JSON`** — inline service-account JSON (highest priority; useful for CI secrets)
2. **`GCP_CREDENTIALS_FILE`** — path to a service-account JSON key file
3. **Application Default Credentials (ADC)** — fallback, e.g. Workload Identity Federation in CI or `gcloud auth application-default login` locally

```bash
# Option 1: credentials file
export GCP_CREDENTIALS_FILE=/path/to/service-account.json

# Option 2: inline JSON (commonly used with CI secrets)
export GCP_CREDENTIALS_JSON='{"type":"service_account",...}'

# Option 3: ADC (leave both unset; gcloud must be authenticated)
gcloud auth application-default login
```

## Examples

See the [examples/](examples/) directory.

**Networking** (`examples/network.pkl`) — VPC network with subnetwork:

```bash
formae apply --mode reconcile --watch examples/network.pkl
```

**Compute** (`examples/disk.pkl`) — persistent disk:

```bash
formae apply --mode reconcile --watch examples/disk.pkl
```

**Lifeline** (`examples/gcp-lifeline/`) — full networking stack (network +
subnetwork + firewall + instance):

```bash
formae apply --mode reconcile --watch examples/gcp-lifeline/main.pkl
```

**Load balancer** (`examples/gcp-loadbalancer/`) — backend service +
URL map + target proxy + forwarding rule.

## License

This plugin is licensed under the [Functional Source License, Version 1.1, ALv2
Future License (FSL-1.1-ALv2)](LICENSE).

Copyright 2025 Platform Engineering Labs Inc.
