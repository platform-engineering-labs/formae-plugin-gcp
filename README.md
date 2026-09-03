# GCP Plugin for Formae

[![CI](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/ci.yml)
[![Nightly](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/nightly.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/nightly.yml)

Google Cloud Platform resource plugin for
[formae](https://github.com/platform-engineering-labs/formae). Manage GCP
infrastructure declaratively across Compute, GKE, Cloud Run, AlloyDB, Cloud SQL,
BigQuery, Bigtable, Cloud Storage, Eventarc, Dataproc, Workflows, Logging,
Monitoring and more.

## Supported Resources

This plugin supports **243 GCP resource types** across 47 services. See
[`schema/pkl/`](schema/pkl/) for field definitions.

| Resource Type | Description |
|---------------|-------------|
| `GCP::AlloyDB::Backup` | On-demand backup of an AlloyDB cluster |
| `GCP::AlloyDB::Cluster` | Cluster of AlloyDB for PostgreSQL databases |
| `GCP::AlloyDB::Instance` | Compute that actually serves an AlloyDB cluster |
| `GCP::AlloyDB::User` | Database user in an AlloyDB cluster |
| `GCP::AnalyticsHub::DataExchange` | The container a publisher shares data through: listings are published into an exchange, and subscribers browse it |
| `GCP::AnalyticsHub::Listing` | One published item inside a `DataExchange`: a BigQuery dataset a subscriber can link into their own project |
| `GCP::AnalyticsHub::QueryTemplate` | A data-clean-room construct: a routine a subscriber may run against shared data without seeing the rows |
| `GCP::ApiGateway::Api` | The top of the API Gateway hierarchy: an api holds api configs, and a gateway serves one config |
| `GCP::ApiGateway::ApiConfig` | A versioned snapshot of an api: the OpenAPI document describing it, plus the identity a gateway serving it runs as |
| `GCP::ApiGateway::Gateway` | The regional endpoint that serves one api config |
| `GCP::ApiKeys::Key` | An API key, with the restrictions that narrow which callers may use it and which services it may reach |
| `GCP::ArtifactRegistry::Repository` | Repository for container images or language packages |
| `GCP::ArtifactRegistry::Rule` | Rule gates an operation on its repository |
| `GCP::BigQuery::Connection` | A named handle BigQuery uses to reach something outside itself — Cloud SQL, Spark, or Google Cloud resources generally |
| `GCP::BigQuery::Dataset` | BigQuery dataset (container for tables and views) |
| `GCP::BigQuery::Routine` | User-defined function, table function, or stored procedure inside a dataset |
| `GCP::BigQuery::RowAccessPolicy` | Row-level access control on a table: a SQL predicate deciding which rows a principal may see |
| `GCP::BigQuery::Table` | BigQuery table |
| `GCP::Bigtable::AppProfile` | Decides how an application's requests are routed across an instance's clusters |
| `GCP::Bigtable::Backup` | Cloud Bigtable table backup |
| `GCP::Bigtable::Cluster` | Cloud Bigtable cluster within an instance |
| `GCP::Bigtable::Instance` | Cloud Bigtable instance |
| `GCP::Bigtable::MaterializedView` | Cloud Bigtable materialized view |
| `GCP::Bigtable::Table` | Cloud Bigtable table |
| `GCP::BinaryAuthorization::Attestor` | A named public key that must have signed an image before an admission rule will admit it |
| `GCP::BinaryAuthorization::PlatformPolicy` | A named, deletable document of image checks a GKE cluster opts into |
| `GCP::CertificateAuthority::CaPool` | Group of certificate authorities forming a trust anchor for issuing certificates |
| `GCP::CertificateAuthority::CertificateAuthority` | The CA that actually signs certificates |
| `GCP::CertificateAuthority::CertificateTemplate` | A reusable issuance policy: what a certificate requested against it may ask for, and what values are forced |
| `GCP::CertificateManager::Certificate` | A TLS certificate a load balancer can serve |
| `GCP::CertificateManager::CertificateMap` | Groups the certificates a load balancer serves, picking one per hostname through its entries |
| `GCP::CertificateManager::CertificateMapEntry` | One row of a certificate map: which certificates to serve for a given hostname |
| `GCP::CertificateManager::DnsAuthorization` | Proves control of a domain |
| `GCP::CertificateManager::TrustConfig` | The set of certificate authorities a load balancer will accept client certificates from — the anchor set for… |
| `GCP::CloudBuild::BuildTrigger` | Build trigger: when a build runs and what it runs, with the build declared inline |
| `GCP::CloudDeploy::Automation` | Promote or advance releases without a human, as rules on a delivery pipeline |
| `GCP::CloudDeploy::CustomTargetType` | A deploy action Cloud Deploy cannot perform itself, as Skaffold custom actions or container tasks |
| `GCP::CloudDeploy::DeliveryPipeline` | The promotion flow: an ordered list of stages, each naming a target, that a release walks |
| `GCP::CloudDeploy::DeployPolicy` | When deploys may not happen: time windows plus the pipelines and targets they restrict |
| `GCP::CloudDeploy::Target` | A deploy destination - a Cloud Run location, a GKE cluster, several at once, or a custom target type |
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
| `GCP::Compute::RegionBackendBucket` | Regional load balancer backend that serves objects straight out of a Cloud Storage bucket |
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
| `GCP::Compute::RegionSnapshot` | A point-in-time incremental backup of a regional persistent disk, stored in the same region as its source |
| `GCP::Compute::RegionSslPolicy` | TLS floor enforced by a regional target HTTPS proxy |
| `GCP::Compute::RegionTargetHttpProxy` | Regional target HTTP proxy |
| `GCP::Compute::RegionTargetHttpsProxy` | Regional target HTTPS proxy |
| `GCP::Compute::RegionTargetTcpProxy` | Regional target TCP proxy |
| `GCP::Compute::RegionUrlMap` | Regional URL map |
| `GCP::Compute::ResourcePolicy` | Schedule attached to disks or instances |
| `GCP::Compute::RolloutPlan` | Named staged rollout schedule - waves of locations, each gated on a validation step |
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
| `GCP::Compute::ZoneVmExtensionPolicy` | Keeps named VM extensions installed at a version on the VMs a selector picks out, in one zone |
| `GCP::Container::Cluster` | GKE cluster |
| `GCP::Container::NodePool` | GKE node pool |
| `GCP::DNS::ManagedZone` | Cloud DNS managed zone |
| `GCP::DNS::Policy` | Governs how DNS resolves for the VPC networks attached to it: whether an on-premises resolver may query into the… |
| `GCP::DNS::ResourceRecordSet` | What a managed zone actually serves: one name, one record type, and the data behind it |
| `GCP::DNS::ResponsePolicy` | A container for rules that override DNS resolution for the networks attached to it — the private-DNS equivalent of… |
| `GCP::DNS::ResponsePolicyRule` | Overrides what one DNS name resolves to for the networks attached to the rule's response policy |
| `GCP::Dataform::ReleaseConfig` | Which Git commitish of a repository to compile, and what to override while compiling it |
| `GCP::Dataform::Repository` | Container for the SQL workflow code that compiles into BigQuery jobs, plus its compilation settings |
| `GCP::Dataform::WorkflowConfig` | Which release config to execute, and on what schedule |
| `GCP::Dataform::Workspace` | Editable development checkout of a Dataform repository |
| `GCP::Dataproc::AutoscalingPolicy` | Autoscaling policy for Dataproc clusters |
| `GCP::Dataproc::SessionTemplate` | Reusable configuration for a serverless Spark session |
| `GCP::Dataproc::WorkflowTemplate` | Spark job graph plus the cluster to run it on |
| `GCP::Datastream::ConnectionProfile` | Connection profile defines the connectivity to a source or destination used by Datastream streams |
| `GCP::Datastream::PrivateConnection` | Private connectivity to a source that is not reachable over the public internet: a VPC peering between a network… |
| `GCP::Datastream::Route` | Tells a `PrivateConnection` which address to reach the source on |
| `GCP::Datastream::Stream` | What actually moves data: a source `ConnectionProfile`, a destination `ConnectionProfile`, and the config joining them |
| `GCP::EssentialContacts::Contact` | Contact that receives notifications from Google Cloud for a project |
| `GCP::Eventarc::Enrollment` | The routing rule of an Eventarc Advanced setup: a CEL expression matched against the events on a `MessageBus`, and… |
| `GCP::Eventarc::GoogleApiSource` | Routes this project's own Google API events onto a `MessageBus`, so an Eventarc Advanced setup can react to… |
| `GCP::Eventarc::MessageBus` | Hub of an Eventarc Advanced setup: publishers send events to a bus |
| `GCP::Eventarc::Pipeline` | Where an Eventarc Advanced `messageBus` sends the events it accepts |
| `GCP::Eventarc::Trigger` | Routes events matching CloudEvents attribute filters to a destination |
| `GCP::Filestore::Backup` | A copy of one file share of an `Instance`, kept independently of it: a backup outlives the instance it was taken… |
| `GCP::Filestore::Instance` | Managed NFS file server |
| `GCP::Filestore::Snapshot` | A point-in-time copy of an `Instance`'s file shares, stored inside the instance itself |
| `GCP::Firestore::Database` | A Firestore database: the container documents and indexes live in, with its own id, location and mode |
| `GCP::GKEBackup::BackupPlan` | Schedule and configuration for backing up a GKE cluster's Kubernetes resources |
| `GCP::GKEHub::Feature` | GKE Hub (fleet) feature |
| `GCP::GKEHub::Membership` | GKE Hub (fleet) membership |
| `GCP::IAM::ProjectIamMember` | Project-level IAM (role, member) binding |
| `GCP::IAM::Role` | Custom IAM role |
| `GCP::IAM::ServiceAccount` | IAM service account |
| `GCP::KMS::KeyRing` | Logical grouping of CryptoKeys, scoped to a project + location |
| `GCP::Logging::LogBucket` | Where log entries are actually retained |
| `GCP::Logging::LogMetric` | Logs-based metric |
| `GCP::Logging::LogScope` | Names a set of log views (or buckets) so a Logs Explorer session can query them together |
| `GCP::Logging::LogView` | Filtered window onto a log bucket |
| `GCP::Logging::ProjectExclusion` | Drops matching entries before they are stored or billed |
| `GCP::Logging::ProjectSink` | Routes matching log entries out of a project to a bucket, dataset, topic or Cloud Storage |
| `GCP::Logging::SavedQuery` | Stored Logs Explorer query |
| `GCP::Memcache::Instance` | A managed memcached cluster on a VPC network |
| `GCP::Monitoring::AlertPolicy` | Policy that fires notifications when its conditions are met |
| `GCP::Monitoring::CustomService` | Hand-declared monitored service, as opposed to one Cloud Monitoring discovers |
| `GCP::Monitoring::Dashboard` | Custom Cloud Monitoring dashboard |
| `GCP::Monitoring::Group` | Dynamic collection of monitored resources selected by a filter |
| `GCP::Monitoring::MetricDescriptor` | Schema of a custom metric: its kind, value type, unit and labels |
| `GCP::Monitoring::NotificationChannel` | Destination (email, Slack, PagerDuty, ...) that alerting policies notify |
| `GCP::Monitoring::Slo` | Reliability target on a service: "`goal` of the time, over `rollingPeriod`, the indicator is good" |
| `GCP::Monitoring::UptimeCheckConfig` | Recurring check that an HTTP/TCP endpoint is reachable |
| `GCP::NetworkConnectivity::Hub` | Network Connectivity Center hub |
| `GCP::NetworkConnectivity::InternalRange` | A reservation of internal IP space inside a VPC. It marks a CIDR range as spoken for so nothing else is allocated… |
| `GCP::NetworkConnectivity::PolicyBasedRoute` | A route chosen by what the traffic *is*, not only where it is going |
| `GCP::NetworkConnectivity::ServiceConnectionPolicy` | Permission, in advance, for a managed service to place Private Service Connect endpoints in a consumer's subnets |
| `GCP::NetworkConnectivity::Spoke` | Links one VPC network into a Network Connectivity Center hub's mesh |
| `GCP::NetworkSecurity::AddressGroup` | A named, reusable set of IP addresses and CIDR blocks |
| `GCP::NetworkSecurity::AuthorizationPolicy` | Who may talk to a service mesh workload: match rules and one verdict, ALLOW or DENY |
| `GCP::NetworkSecurity::BackendAuthenticationConfig` | What a load balancer trusts when it opens a TLS connection to a backend |
| `GCP::NetworkSecurity::ClientTlsPolicy` | The client half of a TLS connection a Google-managed proxy makes on your behalf |
| `GCP::NetworkSecurity::DnsThreatDetector` | Has Cloud DNS queries checked against a threat intelligence feed |
| `GCP::NetworkSecurity::GatewaySecurityPolicy` | The container for Secure Web Proxy rules, and the thing a gateway points at |
| `GCP::NetworkSecurity::GatewaySecurityPolicyRule` | One rule of a Secure Web Proxy policy: a session matcher, a priority, and ALLOW or DENY |
| `GCP::NetworkSecurity::SecurityProfile` | The policy half of Cloud NGFW's layer-7 inspection: what to do about a threat, not where to apply it |
| `GCP::NetworkSecurity::SecurityProfileGroup` | The binding a firewall policy rule actually names |
| `GCP::NetworkSecurity::ServerTlsPolicy` | Which certificate a Google-managed proxy serves, and whether it demands one in return |
| `GCP::NetworkSecurity::UrlList` | A named list of URL patterns for a Secure Web Proxy policy to match on, so a rule names one list rather than… |
| `GCP::NetworkServices::EndpointPolicy` | TLS posture and authorization policy handed to mesh endpoints matched by label |
| `GCP::NetworkServices::GrpcRoute` | gRPC routing matched on service and method, with retry and fault injection |
| `GCP::NetworkServices::HttpRoute` | Layer-7 HTTP routing for a service mesh: match, redirect, rewrite, mirror or forward |
| `GCP::NetworkServices::Mesh` | Cloud Service Mesh routing scope that routes attach to and sidecars configure from |
| `GCP::NetworkServices::ServiceLbPolicy` | How a global load balancer spreads traffic across backend regions |
| `GCP::NetworkServices::TcpRoute` | Layer-4 TCP routing by destination CIDR and port |
| `GCP::NetworkServices::TlsRoute` | TLS routing by SNI and ALPN without terminating the connection |
| `GCP::OrgPolicy::Policy` | Policy configures a constraint on a Google Cloud resource |
| `GCP::ParameterManager::Parameter` | A named container for configuration values, holding the format its versions are written in |
| `GCP::ParameterManager::ParameterVersion` | One immutable revision of a parameter's contents, holding the payload itself |
| `GCP::PubSub::Schema` | Pub/Sub schema |
| `GCP::PubSub::Snapshot` | A snapshot captures the acknowledgement state of a subscription at a point in time, so a subscription can later be… |
| `GCP::PubSub::Subscription` | Pub/Sub subscription |
| `GCP::PubSub::SubscriptionIamMember` | A single (role, member) binding on a `Subscription`'s IAM policy, managed as read-modify-write so sibling bindings… |
| `GCP::PubSub::Topic` | Pub/Sub topic |
| `GCP::PubSub::TopicIamMember` | A single (role, member) binding on a `Topic`'s IAM policy, managed as read-modify-write so sibling bindings survive |
| `GCP::Redis::AclPolicy` | Redis OSS ACL rules a Memorystore for Redis Cluster attaches |
| `GCP::Redis::Instance` | Managed Redis instance |
| `GCP::SQL::BackupRun` | One on-demand backup of a Cloud SQL instance |
| `GCP::SQL::Database` | Cloud SQL database |
| `GCP::SQL::DatabaseInstance` | Cloud SQL instance |
| `GCP::SQL::SslCert` | A client certificate for connecting to a Cloud SQL instance over mutual TLS. An instance with `requireSsl` set… |
| `GCP::SQL::User` | A database user on a Cloud SQL instance |
| `GCP::SecretManager::Secret` | Secret Manager secret |
| `GCP::SecretManager::SecretVersion` | Secret Manager secret version |
| `GCP::ServiceDirectory::Endpoint` | One address and port a service answers on |
| `GCP::ServiceDirectory::Namespace` | The top-level container of a Service Directory registry: services are registered inside a namespace, and endpoints… |
| `GCP::ServiceDirectory::Service` | A named service inside a namespace |
| `GCP::ServiceNetworking::Connection` | Private Service Access connection (VPC peering to Google services) |
| `GCP::Spanner::BackupSchedule` | A recurring backup of one database |
| `GCP::Spanner::Database` | A database inside a Spanner instance |
| `GCP::Spanner::Instance` | The compute and storage a Spanner deployment runs on |
| `GCP::Spanner::InstanceConfig` | A user-managed configuration naming where a Spanner instance's replicas may live |
| `GCP::Storage::AnywhereCache` | Cloud Storage Anywhere Cache |
| `GCP::Storage::Bucket` | Cloud Storage bucket |
| `GCP::Storage::BucketAccessControl` | Cloud Storage bucket ACL entry |
| `GCP::Storage::DefaultObjectAccessControl` | Cloud Storage default object ACL for a bucket |
| `GCP::Storage::Folder` | An IAM boundary inside a bucket: it lets a policy be attached to a prefix without granting it over the whole bucket |
| `GCP::Storage::ManagedFolder` | An IAM boundary inside a bucket: it lets a policy be attached to a prefix without granting it over the whole bucket |
| `GCP::Storage::Notification` | Publishes object change events from a `Bucket` to a Pub/Sub topic |
| `GCP::Storage::Object` | A single object in a `Bucket`, with its content declared inline |
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
