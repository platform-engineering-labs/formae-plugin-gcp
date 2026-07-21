# Changelog

All notable changes to the formae GCP plugin are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Install with `sudo formae plugin install gcp` on the host that runs the
formae agent.

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
