# Changelog

All notable changes to the formae GCP plugin are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Install with `sudo formae plugin install gcp` on the host that runs the
formae agent.

## [0.1.10]

### Added

- `GCP::CloudRun::Service` exposes `template.vpcAccess`, letting a service
  route egress through a Serverless VPC connector or direct VPC network
  (`connector`, `networkInterfaces`, `egress`).

### Fixed

- `GCP::CloudRun::Service` create no longer hangs: the operation is now
  polled to completion, and `selfLink` is normalized so a create-then-read
  cycle no longer reports spurious drift.
- `GCP::IAM::ServiceAccount` delete is now async, completing only once the
  account has left the list, so a delete immediately followed by a
  synchronization or discovery run no longer resurrects it.
- Transport-layer read errors are now classified: authentication failures and
  unreachable endpoints map to distinct Formae error codes instead of a
  generic failure, improving diagnostics on misconfigured credentials or
  network issues.

## [0.1.9]

### Added

- `GCP::Compute::InstanceGroup` now manages VM membership via an `instances`
  field (instance self-links or `Instance` resolvables), reconciled with
  `addInstances` / `removeInstances`; `namedPorts` are now mutable via
  `setNamedPorts`. This lets a GCE VM back an external HTTPS load balancer.

### Changed

- `GCP::Compute::SslCertificate.privateKey` accepts a wrapped value
  (`formae.value(read(...).text).opaque`), keeping the PEM private key out of
  rendered plans and stored state.

### Fixed

- `GCP::Compute::SslCertificate` SELF_MANAGED certificates now send
  `certificate` / `privateKey` nested under `selfManaged`, as the API requires.
  Creation previously failed with "Self-managed certificate details must be
  specified if type = SELF_MANAGED".
- `GCP::IAM::ServiceAccount` creation now accounts for IAM eventual consistency:
  the create completes only once the account is listable, so a synchronization
  or discovery run immediately after create no longer drops it from inventory.
- `GCP::Compute::InstanceGroup` read no longer surfaces provider-populated
  `network` / `subnetwork` as spurious drift.

## [0.1.8]

### Added

- Pub/Sub resources — `GCP::PubSub::Topic`, `GCP::PubSub::Subscription`, and
  `GCP::PubSub::Schema`.
- Secret Manager — `GCP::SecretManager::Secret` (automatic, Google-managed
  replication by default).
- Cloud DNS — `GCP::DNS::ManagedZone` for public and private DNS zones.
- IAM — `GCP::IAM::ServiceAccount` for service accounts and `GCP::IAM::Role`
  for custom project roles.
- Compute — `GCP::Compute::Route` for static VPC routes and
  `GCP::Compute::SecurityPolicy` for Cloud Armor policies.

### Changed

- `GCP::BigQuery::Dataset` and `GCP::BigQuery::Table` now support updates.
  Previously create/delete only; mutable fields such as description, labels,
  and (for tables) schema can now be changed in place.

## [0.1.7]

### Added

- `GCP::IAM::ProjectIamMember` for managing a single member-role binding on a
  project, without touching the rest of the project's IAM policy.

### Fixed

- Router and RouterNat resolvable property paths now use camelCase (`id`,
  `name`, `selfLink`), so references to these resources resolve correctly.
- Provider-immutable fields across the Compute, Container, GKE Hub, and Storage
  schemas are now marked create-only, so changing them plans a replace instead
  of attempting an update the provider would reject. Requires formae 0.86.0 or
  later.

## [0.1.5]

### Added

- `GCP::Compute::RouterNat` for managing Cloud NAT configurations on a Cloud
  Router.

## [0.1.4]

### Added

- GKE Hub (Fleet) resources, `GCP::GKEHub::Feature` and `GCP::GKEHub::Membership`,
  can now be managed through formae. Use `Membership` to register GKE (or
  external) clusters into a fleet and `Feature` to enable fleet-wide features on
  those clusters.

### Fixed

- `formae extract` now works correctly for BigQuery Table resources. Previously,
  extracting a managed table to PKL would fail with an internal error,
  preventing round-trip workflows (deploy, extract, redeploy).

## [0.1.2]

### Fixed

- Spurious diffs during updates and synchronization for resources where GCP
  populates default values (e.g. Disk licenses, guest OS features, Cloud Build
  worker pool settings). These fields are now correctly recognized as provider
  defaults.

## [0.1.1]

### Added

- Cloud Run resources (`GCP::CloudRun::Job` and `GCP::CloudRun::Service`) with
  full conformance tests.
- `location` to the GCP target configuration, giving explicit control over the
  target location for regional resources.

### Fixed

- `Disk.sourceImage` nullable type. The field was incorrectly required, causing
  validation failures when creating disks without a source image.
- Corrected nullish Pkl union types across several resource schemas.

## [0.1.0]

### Added

- Initial release of the GCP plugin as a standalone package built on the formae
  Plugin SDK.
