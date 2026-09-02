# GCP Plugin for Formae

[![CI](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/ci.yml)
[![Nightly](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/nightly.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/nightly.yml)

Google Cloud Platform resource plugin for
[formae](https://github.com/platform-engineering-labs/formae). Manage GCP
infrastructure declaratively across Compute, GKE, Cloud Run, AlloyDB, Cloud SQL,
BigQuery, Bigtable, Cloud Storage, Eventarc, Dataproc, Workflows, Logging,
Monitoring and more.

## Supported Resources

This plugin supports **154 GCP resource types** across 32 services. See
[`schema/pkl/`](schema/pkl/) for field definitions.

| Resource Type | Description |
|---------------|-------------|
| `GCP::AlloyDB::Backup` | On-demand backup of an AlloyDB cluster |
| `GCP::AlloyDB::Cluster` | Cluster of AlloyDB for PostgreSQL databases |
| `GCP::AlloyDB::Instance` | Compute that actually serves an AlloyDB cluster |
| `GCP::AlloyDB::User` | Database user in an AlloyDB cluster |
| `GCP::ArtifactRegistry::Repository` | Repository for container images or language packages |
| `GCP::ArtifactRegistry::Rule` | Rule gates an operation on its repository |
| `GCP::BigQuery::Dataset` | BigQuery dataset (container for tables and views) |
| `GCP::BigQuery::Routine` | User-defined function, table function, or stored procedure inside a dataset |
| `GCP::BigQuery::Table` | BigQuery table |
| `GCP::Bigtable::Backup` | Cloud Bigtable table backup |
| `GCP::Bigtable::Cluster` | Cloud Bigtable cluster within an instance |
| `GCP::Bigtable::Instance` | Cloud Bigtable instance |
| `GCP::Bigtable::MaterializedView` | Cloud Bigtable materialized view |
| `GCP::Bigtable::Table` | Cloud Bigtable table |
| `GCP::CertificateAuthority::CaPool` | Group of certificate authorities forming a trust anchor for issuing certificates |
| `GCP::CloudRun::Execution` | Cloud Run job execution (discovered/read-only) |
| `GCP::CloudRun::Job` | Cloud Run job (run-to-completion) |
| `GCP::CloudRun::Revision` | Cloud Run service revision (discovered/read-only) |
| `GCP::CloudRun::Service` | Cloud Run service (serverless container) |
| `GCP::CloudRun::ServiceIamMember` | IAM (role, member) binding on a Cloud Run service |
| `GCP::CloudRun::Task` | Cloud Run job task (discovered/read-only) |
| `GCP::CloudRun::WorkerPool` | Cloud Run worker pool |
| `GCP::CloudScheduler::Job` | Cron-style scheduled unit of work that fires an HTTP or Pub/Sub target |
| `GCP::CloudTasks::Queue` | Queue that holds and dispatches tasks at a controlled rate |
| `GCP::Compute::Address` | Regional static IP address |
| `GCP::Compute::Autoscaler` | Autoscaler for a zonal managed instance group |
| `GCP::Compute::BackendBucket` | Load balancer backend that serves objects straight out of a Cloud Storage bucket |
| `GCP::Compute::BackendService` | Global backend service for load balancers |
| `GCP::Compute::BackendServiceSignedUrlKey` | Cloud CDN signed-URL key on a backend service |
| `GCP::Compute::Disk` | Persistent disk |
| `GCP::Compute::DiskResourcePolicyAttachment` | Attaches a `resourcePolicy` to a disk |
| `GCP::Compute::ExternalVpnGateway` | Describes the *other* end of a VPN: the on-prem or other-cloud device, by its public IP(s) |
| `GCP::Compute::Firewall` | VPC firewall rule |
| `GCP::Compute::ForwardingRule` | Regional forwarding rule |
| `GCP::Compute::GlobalAddress` | Global static IP address (incl. PSA VPC-peering ranges) |
| `GCP::Compute::GlobalForwardingRule` | Global forwarding rule |
| `GCP::Compute::GlobalNetworkEndpoint` | One endpoint inside a `globalNetworkEndpointGroup` |
| `GCP::Compute::GlobalNetworkEndpointGroup` | Group of endpoints outside Google Cloud, for external HTTP(S) load balancing and Cloud CDN |
| `GCP::Compute::HaVpnGateway` | Google side of an HA VPN |
| `GCP::Compute::HealthCheck` | Global health check |
| `GCP::Compute::HttpHealthCheck` | Legacy HTTP health check (the only kind `targetPool.healthChecks` accepts) |
| `GCP::Compute::HttpsHealthCheck` | Legacy HTTPS health check |
| `GCP::Compute::Image` | Bootable disk image, normally captured from a persistent disk to serve as a golden image |
| `GCP::Compute::Instance` | Compute Engine VM instance |
| `GCP::Compute::InstanceGroup` | Unmanaged instance group (with membership) |
| `GCP::Compute::InstanceGroupManager` | Managed instance group keeps `targetSize` VMs alive from an InstanceTemplate blueprint |
| `GCP::Compute::InstanceTemplate` | Immutable VM blueprint a managed instance group instantiates |
| `GCP::Compute::InstantSnapshot` | Near-instant, same-zone point-in-time copy of a disk |
| `GCP::Compute::MachineImage` | Whole-VM capture: every attached disk plus machine type, metadata, network config and licences |
| `GCP::Compute::Network` | VPC network |
| `GCP::Compute::NetworkAttachment` | Consumer end of a Private Service Connect interface |
| `GCP::Compute::NetworkEndpointGroup` | Network endpoint group (incl. serverless NEG for Cloud Run) |
| `GCP::Compute::NetworkFirewallPolicy` | Project-scoped network firewall policy |
| `GCP::Compute::NetworkFirewallPolicyAssociation` | Attaches a network firewall policy to a VPC network |
| `GCP::Compute::NetworkFirewallPolicyRule` | One rule inside a `networkFirewallPolicy` |
| `GCP::Compute::NetworkPeering` | VPC peering between two networks, for private-IP reachability without a VPN |
| `GCP::Compute::NodeTemplate` | Specification a sole-tenant `nodeGroup` stamps its physical nodes from: which node type |
| `GCP::Compute::PacketMirroring` | Copies selected VMs' traffic to an internal load balancer for inspection |
| `GCP::Compute::ProjectMetadataItem` | One key of the project's common instance metadata |
| `GCP::Compute::RegionAutoscaler` | Autoscaler for a regional managed instance group |
| `GCP::Compute::RegionBackendService` | Regional backend service |
| `GCP::Compute::RegionCompositeHealthCheck` | Aggregates one or more `regionHealthSource`s and reports the verdict at a forwarding rule |
| `GCP::Compute::RegionDisk` | Persistent disk synchronously replicated across two zones in a region |
| `GCP::Compute::RegionDiskResourcePolicyAttachment` | Attaches a `resourcePolicy` to a disk |
| `GCP::Compute::RegionHealthAggregationPolicy` | Decides when a whole backend service counts as healthy, rather than each backend individually |
| `GCP::Compute::RegionHealthCheck` | Regional health check |
| `GCP::Compute::RegionHealthSource` | Binds a backend service to a regional health aggregation policy |
| `GCP::Compute::RegionInstanceGroupManager` | Managed instance group spread across the zones of a region |
| `GCP::Compute::RegionInstanceTemplate` | Immutable VM blueprint a *regional* managed instance group instantiates |
| `GCP::Compute::RegionInstantSnapshot` | Near-instant point-in-time copy of a *regional* disk |
| `GCP::Compute::RegionNetworkFirewallPolicy` | Region-scoped network firewall policy |
| `GCP::Compute::RegionNetworkFirewallPolicyAssociation` | Attaches a regional network firewall policy to a VPC network |
| `GCP::Compute::RegionSecurityPolicy` | Set of rules matching incoming requests to a backend service |
| `GCP::Compute::RegionSecurityPolicyRule` | One rule inside a regional `regionSecurityPolicy` |
| `GCP::Compute::RegionSslPolicy` | TLS floor enforced by a regional target HTTPS proxy |
| `GCP::Compute::RegionTargetHttpProxy` | Regional target HTTP proxy |
| `GCP::Compute::RegionTargetHttpsProxy` | Regional target HTTPS proxy |
| `GCP::Compute::RegionTargetTcpProxy` | Regional target TCP proxy |
| `GCP::Compute::RegionUrlMap` | Regional URL map |
| `GCP::Compute::ResourcePolicy` | Schedule attached to disks or instances |
| `GCP::Compute::Route` | Static custom route in a VPC |
| `GCP::Compute::Router` | Cloud Router |
| `GCP::Compute::RouterInterface` | One interface of a Cloud Router |
| `GCP::Compute::RouterNamedSet` | Reusable list of prefixes on a Cloud Router |
| `GCP::Compute::RouterNat` | Cloud NAT configuration on a router |
| `GCP::Compute::RouterRoutePolicy` | BGP route policy on a Cloud Router: it filters or rewrites the routes the router imports from |
| `GCP::Compute::SecurityPolicy` | Cloud Armor security policy |
| `GCP::Compute::SecurityPolicyRule` | One rule inside a global `securityPolicy` |
| `GCP::Compute::ServiceAttachment` | Producer end of Private Service Connect: publishes an internal load balancer to other VPCs |
| `GCP::Compute::Snapshot` | Point-in-time incremental backup of a persistent disk |
| `GCP::Compute::SslCertificate` | SSL certificate (self-managed or Google-managed) |
| `GCP::Compute::SslPolicy` | TLS floor a target HTTPS/SSL proxy enforces: minimum protocol version plus a cipher profile |
| `GCP::Compute::Subnetwork` | VPC subnetwork |
| `GCP::Compute::TargetGrpcProxy` | Proxy a proxyless gRPC service mesh points its clients at |
| `GCP::Compute::TargetHttpProxy` | Global target HTTP proxy |
| `GCP::Compute::TargetHttpsProxy` | Global target HTTPS proxy |
| `GCP::Compute::TargetInstance` | Points a protocol-forwarding rule at a single VM |
| `GCP::Compute::TargetPool` | Target pool for network load balancing |
| `GCP::Compute::TargetSslProxy` | Global target SSL proxy |
| `GCP::Compute::TargetTcpProxy` | Global target TCP proxy |
| `GCP::Compute::TargetVpnGateway` | Classic, route-based VPN gateway |
| `GCP::Compute::UrlMap` | Global URL map |
| `GCP::Compute::VpnTunnel` | Replaces the tunnel |
| `GCP::Container::Cluster` | GKE cluster |
| `GCP::Container::NodePool` | GKE node pool |
| `GCP::Dataproc::AutoscalingPolicy` | Autoscaling policy for Dataproc clusters |
| `GCP::Dataproc::SessionTemplate` | Reusable configuration for a serverless Spark session |
| `GCP::Dataproc::WorkflowTemplate` | Spark job graph plus the cluster to run it on |
| `GCP::Datastream::ConnectionProfile` | Connection profile defines the connectivity to a source or destination used by Datastream streams |
| `GCP::DNS::ManagedZone` | Cloud DNS managed zone |
| `GCP::EssentialContacts::Contact` | Contact that receives notifications from Google Cloud for a project |
| `GCP::Eventarc::MessageBus` | Hub of an Eventarc Advanced setup: publishers send events to a bus |
| `GCP::Eventarc::Pipeline` | Where an Eventarc Advanced `messageBus` sends the events it accepts |
| `GCP::Eventarc::Trigger` | Routes events matching CloudEvents attribute filters to a destination |
| `GCP::Filestore::Instance` | Managed NFS file server |
| `GCP::GKEBackup::BackupPlan` | Schedule and configuration for backing up a GKE cluster's Kubernetes resources |
| `GCP::GKEHub::Feature` | GKE Hub (fleet) feature |
| `GCP::GKEHub::Membership` | GKE Hub (fleet) membership |
| `GCP::IAM::ProjectIamMember` | Project-level IAM (role, member) binding |
| `GCP::IAM::Role` | Custom IAM role |
| `GCP::IAM::ServiceAccount` | IAM service account |
| `GCP::KMS::KeyRing` | Logical grouping of CryptoKeys, scoped to a project + location |
| `GCP::Logging::LogMetric` | Logs-based metric |
| `GCP::Logging::LogScope` | Names a set of log views (or buckets) so a Logs Explorer session can query them together |
| `GCP::Logging::LogView` | Filtered window onto a log bucket |
| `GCP::Logging::ProjectExclusion` | Drops matching entries before they are stored or billed |
| `GCP::Logging::ProjectSink` | Routes matching log entries out of a project to a bucket, dataset, topic or Cloud Storage |
| `GCP::Logging::SavedQuery` | Stored Logs Explorer query |
| `GCP::Monitoring::AlertPolicy` | Policy that fires notifications when its conditions are met |
| `GCP::Monitoring::CustomService` | Hand-declared monitored service, as opposed to one Cloud Monitoring discovers |
| `GCP::Monitoring::Dashboard` | Custom Cloud Monitoring dashboard |
| `GCP::Monitoring::Group` | Dynamic collection of monitored resources selected by a filter |
| `GCP::Monitoring::MetricDescriptor` | Schema of a custom metric: its kind, value type, unit and labels |
| `GCP::Monitoring::NotificationChannel` | Destination (email, Slack, PagerDuty, ...) that alerting policies notify |
| `GCP::Monitoring::Slo` | Reliability target on a service: "`goal` of the time, over `rollingPeriod`, the indicator is good" |
| `GCP::Monitoring::UptimeCheckConfig` | Recurring check that an HTTP/TCP endpoint is reachable |
| `GCP::NetworkConnectivity::Hub` | Network Connectivity Center hub |
| `GCP::OrgPolicy::Policy` | Policy configures a constraint on a Google Cloud resource |
| `GCP::PubSub::Schema` | Pub/Sub schema |
| `GCP::PubSub::Subscription` | Pub/Sub subscription |
| `GCP::PubSub::Topic` | Pub/Sub topic |
| `GCP::Redis::Instance` | Managed Redis instance |
| `GCP::SecretManager::Secret` | Secret Manager secret |
| `GCP::SecretManager::SecretVersion` | Secret Manager secret version |
| `GCP::ServiceNetworking::Connection` | Private Service Access connection (VPC peering to Google services) |
| `GCP::SQL::Database` | Cloud SQL database |
| `GCP::SQL::DatabaseInstance` | Cloud SQL instance |
| `GCP::Storage::AnywhereCache` | Cloud Storage Anywhere Cache |
| `GCP::Storage::Bucket` | Cloud Storage bucket |
| `GCP::Storage::BucketAccessControl` | Cloud Storage bucket ACL entry |
| `GCP::Storage::DefaultObjectAccessControl` | Cloud Storage default object ACL for a bucket |
| `GCP::Storage::ObjectAccessControl` | Cloud Storage object ACL entry |
| `GCP::Vpcaccess::Connector` | Connector that lets serverless environments reach resources in a VPC network via internal IP |
| `GCP::Workflows::Workflow` | Workflow definition: the YAML or JSON program Workflows runs |

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
