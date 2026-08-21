# Changelog

All notable changes to the formae GCP plugin are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Install with `sudo formae plugin install gcp` on the host that runs the
formae agent.

## [Unreleased]

### Added

- `GCP::Compute::InstanceTemplate` — the immutable global VM blueprint a
  managed instance group stamps out. Every field is createOnly; the Compute API
  has no update method for templates.
- `GCP::Compute::InstanceGroupManager` — zonal managed instance group. Set
  `targetSize` to size the group, or attach an autoscaler and let it own the
  size.
- `GCP::Compute::Autoscaler` — zonal autoscaler for a managed instance group.
  The API's `target` field is exposed as `instanceGroupManager` because
  `target` is reserved by formae's base Resource class (it names the deployment
  target).

Together these close the managed-instance-group gap: the plugin previously had
`Instance` and the unmanaged `InstanceGroup` but no way to declare a MIG.

- `GCP::Compute::ResourcePolicy` — regional snapshot schedule (daily or hourly
  cycle, retention, snapshot labels/locations) to attach to disks.
- `GCP::Compute::Image` — global bootable image captured from a persistent
  disk, for golden images fed to `instanceTemplate`'s
  `disks[].initializeParams.sourceImage` or to `disk.sourceImage`.
- `GCP::Compute::Snapshot` — global incremental disk backup. Pair it with
  `ResourcePolicy` for scheduled backups, or restore by pointing a new disk's
  `sourceSnapshot` at it.
- `GCP::Monitoring::Dashboard` — custom Cloud Monitoring dashboard with a
  caller-chosen ID, `gridLayout`, and `text` / `xyChart` widgets. Dashboards
  live on the Monitoring **v1** API, so the definition carries its own
  `APIConfig` alongside the v3 resources. Updates are not wired yet:
  `dashboards.patch` needs the current etag in the body and the generic
  optimistic-locking path did not satisfy conformance, so a change replaces.
- `GCP::Logging::ProjectSink` — routes matching log entries to a log bucket,
  GCS bucket, BigQuery dataset, or Pub/Sub topic, with optional per-sink
  exclusions. Supports in-place updates: `sinks.patch` derives its `updateMask`
  from the body, so the update body drops the immutable `name` and the
  server-assigned `writerIdentity`.
- `GCP::Compute::SslPolicy` — the TLS floor (minimum version + cipher profile)
  a target HTTPS/SSL proxy enforces, attachable from
  `targetHttpsProxy.sslPolicy`. Supports in-place updates; `sslPolicies.patch`
  requires the current `fingerprint`, supplied via optimistic locking.
- `GCP::Logging::ProjectExclusion` — drops matching log entries before they are
  stored or billed, the usual lever for cutting volume from a noisy source.
  Supports in-place updates.
- `GCP::Monitoring::CustomService` and `GCP::Monitoring::Slo` — SLOs as code. A
  custom service is the container; the SLO declares a `goal` over a
  `rollingPeriod` (or `calendarPeriod`) against a `serviceLevelIndicator`. The
  SLO is nested under its service
  (`/projects/{p}/services/{svc}/serviceLevelObjectives/{id}`) and supports
  in-place updates.
- `GCP::Logging::LogView` — a filtered window onto a log bucket, the unit of
  read access for logs (grant `roles/logging.viewAccessor` on a view and a team
  sees only the entries its `filter` selects). Nested three levels deep under
  `locations/{location}/buckets/{bucket}`, and supports in-place updates.

  Log buckets themselves are deliberately not modelled: their DELETE only marks
  the bucket `DELETE_REQUESTED` for 7 days, so a destroyed bucket still reads
  back as present.
- `GCP::Compute::RegionDisk` — a persistent disk synchronously replicated across
  two zones, for workloads that must survive losing one. Declare `type` and
  `replicaZones` in short form; the provisioner expands them to the full URLs
  the API requires (a bare `pd-balanced` is rejected as a malformed URL) and
  shortens them again on read.
- HA VPN: `GCP::Compute::ExternalVpnGateway` (the peer side, by public IP),
  `GCP::Compute::HaVpnGateway` (the Google side, with server-assigned interface
  IPs), and `GCP::Compute::VpnTunnel` (one IPsec tunnel between an interface of
  each, over a Cloud Router). Together with the existing `Router` these make a
  site-to-site VPN declarable.

  `vpnTunnel.sharedSecret` is write-only and dropped on read: the API echoes a
  masked `sharedSecret` plus a `sharedSecretHash`, and storing either would put
  non-authored secret material in state.

  Only one fixture creates an HaVpnGateway, because `VPN_GATEWAYS_PER_REGION` is
  capped at 2 — a second gateway fixture collides with it, or with a leftover
  from a previous run, and fails with `QUOTA_EXCEEDED`. `clean-environment.sh`
  now tears down VPN tunnels, both gateway kinds, and routers so a leftover
  cannot block the next run.
- `GCP::Compute::HttpHealthCheck` — the legacy global health check. It exists
  because `targetPool.healthChecks` accepts only these, not the modern
  `healthCheck`, so a network load balancer built on a target pool previously
  had no way to declare its probe. Supports in-place updates (Compute's PATCH
  needs no fingerprint here).
- `GCP::Compute::NetworkAttachment` — the consumer end of Private Service
  Connect *interfaces*: it lets a producer place an interface inside one of your
  subnets so their service reaches your VPC over private IPs. Supports in-place
  updates; `networkAttachments.patch` requires the current `fingerprint`,
  supplied via optimistic locking. `network` is server-derived from
  `subnetworks` and marked as a provider default.
- `GCP::Logging::SavedQuery` — a stored Logs Explorer query, so the queries an
  on-call rotation actually uses live in version control. Location-scoped;
  `visibility` is required (the API rejects a query that is neither PRIVATE nor
  SHARED). Supports in-place updates of `displayName`, `description` and the
  query `filter`.

  Known limitation: `loggingQuery.summaryFields` is carried correctly on create
  but goes missing on update, even though the API preserves it under the same
  `updateMask` (verified directly). The field is therefore marked `createOnly`
  so a change replaces the query rather than silently dropping the columns.
- `GCP::AlloyDB::Instance` — the compute that serves an AlloyDB cluster. The
  plugin previously shipped `AlloyDB::Cluster` with no way to declare an
  instance, so a cluster stored nothing reachable. Nested under its cluster
  (`/projects/{p}/locations/{loc}/clusters/{cluster}/instances/{id}`), which
  required parent-segment support in the AlloyDB path builder and a native-ID
  parser that keeps the owning cluster.

  `clean-environment.sh` now tears down AlloyDB instances and clusters —
  AlloyDB bills per instance-minute, so a leaked cluster is expensive rather
  than merely untidy.
- `GCP::Monitoring::MetricDescriptor` — the schema of a custom metric (kind,
  value type, unit, labels), so an `alertPolicy` or `dashboard` can reference a
  metric before any application has written a point to it. The metric type is
  the resource id and contains slashes, which needs a dedicated native-ID
  parser; the API calls the field `type`, reserved by formae's base Resource
  class, so the schema exposes it as `name` and the provisioner renames it on
  the wire.

  **No conformance fixture yet.** Create, Verify, Extract, Sync and Destroy all
  pass, but the OOB-delete step never completes: after the descriptor is deleted
  out of band the inventory keeps it, even though the API confirms it is gone
  (checked directly) and with the phase timeout raised to 6 minutes.
  `metricDescriptors.list` returns the project's entire built-in metric
  catalogue, paginated, which is the likely reason sync never converges —
  narrowing the list with a `filter` query parameter made Verify/Sync pass but
  moved the failure to Destroy, so that avenue was reverted rather than shipped
  half-working.
- `GCP::Compute::RegionInstanceGroupManager` — a managed instance group spread
  across zones of a region, which is the shape production normally wants; the
  zonal `InstanceGroupManager` covers single-zone workloads. `distributionPolicy`
  is a provider default: GCP picks the region's zones with an EVEN shape and
  echoes them back as full URLs.

  `clean-environment.sh` gained a regional-MIG pass — like regional disks, they
  report no zone, so the zonal loop skipped them.
- `GCP::Compute::RegionAutoscaler` — scales a `RegionInstanceGroupManager`,
  completing the regional autoscaling chain (template → regional MIG →
  autoscaler). Like the zonal autoscaler it exposes the API's `target` field as
  `instanceGroupManager`, since `target` is reserved by formae's base Resource
  class, and update is off because `autoscalers.patch` takes the name as
  `?autoscaler=NAME` rather than a path segment.
- `GCP::Logging::LogScope` — names a set of log views so a Logs Explorer session
  can query them together, typically to search several projects at once.

  Update is off: `logScopes.patch` works against the live API under every
  `updateMask` tried, but through the plugin the PATCH reports success while the
  stored description stays unchanged. `SavedQuery` — same package, same
  location-scoped shape, same mask-from-body path — updates correctly, so this
  is not an API limitation; isolating it needs visibility into the plugin's
  request payload.
- `GCP::Compute::TargetInstance` — points a protocol-forwarding `forwardingRule`
  at a single VM, for protocols a load balancer will not terminate (ESP, AH,
  SCTP, or raw TCP/UDP without balancing). `natPolicy` is a provider default
  (`NO_NAT` is the only value v1 accepts). No patch endpoint exists, so a change
  replaces.
- `GCP::Compute::ServiceAttachment` — the producer end of Private Service
  Connect: publishes an internal load balancer so consumers in other VPCs reach
  it over private IPs. Together with `NetworkAttachment` (the consumer end, for
  PSC interfaces) both halves of PSC are now declarable. `natSubnets` must point
  at subnets whose `purpose` is `PRIVATE_SERVICE_CONNECT`. Supports in-place
  updates; `serviceAttachments.patch` requires the current `fingerprint`,
  supplied via optimistic locking.
- `GCP::Compute::NetworkFirewallPolicy` — a project-scoped network firewall
  policy, the modern replacement for VPC `firewall` rules. Supports in-place
  updates of `description`.

  Three API details worth recording. The URL segment is `firewallPolicies`
  even though the API method group is `networkFirewallPolicies` — the obvious
  path returns an HTML 404. `patch` requires the current `fingerprint`, and
  refuses a body carrying `name` ("Can only change description and rules of the
  firewall policy with patch operation"), so `name` is dropped from update
  bodies. The server-populated `rules` field (GCP's implied rules) is
  deliberately absent from the schema so it does not read back as drift.

  Rules and network associations are **not** modelled yet: the API rejects them
  inline ("Rules must be added using the addRule method"), so each needs a
  verb-based provisioner (`addRule`/`patchRule`/`removeRule` and
  `addAssociation`/`removeAssociation`) in the shape of
  `pkg/resources/compute/instance_group.go`. A policy without them carries only
  the implied rules.
- `GCP::Compute::RegionNetworkFirewallPolicy` — the regional twin, for rules
  scoped to one region and for attachment by regional load balancers, which
  global policies cannot serve. Same `firewallPolicies` path segment and the
  same fingerprint-on-patch / no-`name`-in-patch-body constraints.
- `GCP::Compute::RegionSecurityPolicy` — regional Cloud Armor, for policies
  attached to regional load balancers. Rules are inline, as with the global
  `SecurityPolicy`.

  `type` is a provider default: the API accepts `CLOUD_ARMOR_REGIONAL` on create
  but reports it back as `CLOUD_ARMOR`, so declaring the input value verbatim
  would read as drift. Declare `CLOUD_ARMOR`.
- `GCP::Compute::NetworkFirewallPolicyRule` — one rule inside a network firewall
  policy, which is what makes a policy enforce anything. Implemented as a
  hand-written provisioner (`pkg/resources/compute/firewall_policy_rule.go`):
  a rule is not a REST sub-collection but a set of verbs on the policy
  (`addRule`, `getRule?priority=N`, `patchRule?priority=N`,
  `removeRule?priority=N`), so all of CRUD is bespoke while Status delegates to
  the base operation poll.

  Notable details: the native ID is composite
  (`projects/{p}/global/firewallPolicies/{policy}/rules/{priority}`) because the
  API addresses a rule by policy + priority; a removed rule answers `getRule`
  with **400, not 404**, so not-found is mapped explicitly or formae would never
  learn the rule is gone; and `List` skips priorities at or above 2147483644,
  where GCP's own implied rules live.

  **No conformance fixture yet.** Create, Sync, Destroy and OOB-delete all pass
  — so the verbs, native ID, List filtering and status wiring are right — but
  Verify/Extract/Update fail with "Property match should exist in actual
  resource". The `match` object never reaches stored state. This provisioner's
  `Read` is hand-written and demonstrably returns the API body verbatim
  (`getRule` includes `match`, verified directly), and the failure is identical
  whether `match` is a typed sub-object or an untyped `Mapping`. That places the
  loss downstream of the plugin's Read result rather than in the request path or
  the schema, and makes this the fourth instance of the same class of problem
  (see `Logging::SavedQuery.summaryFields`, `Monitoring::Dashboard` etag,
  `Logging::LogScope` update).
- `GCP::Compute::InstantSnapshot` — a near-instant, same-zone copy of a disk: a
  fast rollback point rather than a backup, since it is not replicated outside
  the zone. Requires an SSD-class source disk, so its fixture uses `pd-ssd`
  where the other disk fixtures use `pd-standard`. Only `setLabels` mutates one,
  so a change replaces.
- `GCP::Compute::MachineImage` — a whole-VM capture: every attached disk plus
  the instance's machine type, metadata and network config. Where `Image` copies
  one disk and `Snapshot` backs one up, a machine image is what you clone or
  move an entire VM from. Only `setLabels` mutates one, so a change replaces.
- `GCP::Compute::HttpsHealthCheck` — the legacy HTTPS health check, completing
  the pair with `HttpHealthCheck`. `targetPool.healthChecks` accepts only the
  legacy checks, so a network load balancer whose backends terminate TLS had no
  way to declare its probe. Supports in-place updates; like the HTTP one its
  PATCH needs no fingerprint.
- `GCP::AlloyDB::User` — a database user in an AlloyDB cluster, completing
  cluster + instance + user so an application has something to connect to.
  Nested under its cluster, and notably **synchronous**: `users.create` returns
  the User itself rather than an Operation, unlike clusters and instances, so it
  gets its own `OperationConfig`. `password` and `keepExtraRoles` are documented
  input-only and are dropped from reads so a password cannot reach stored state.

- `GCP::Compute::GlobalNetworkEndpointGroup` — an internet NEG, so an external
  HTTP(S) load balancer or Cloud CDN can serve from an origin hosted outside
  Google Cloud. The regional `NetworkEndpointGroup` covers serverless and in-VPC
  backends; this covers the public internet. The group itself holds no origins —
  see `GlobalNetworkEndpoint` below. No patch endpoint exists, so a change
  replaces.

- `GCP::Compute::GlobalNetworkEndpoint` — one origin inside a global NEG, which
  is what makes the group above useful: an empty group serves nothing. Not a
  REST resource but a member of the group, attached and detached with the
  `attachNetworkEndpoints` / `detachNetworkEndpoints` verbs and listed with a
  **POST** to `listNetworkEndpoints`, so it is a hand-written provisioner. The
  native ID is composite — `…/networkEndpointGroups/{group}/networkEndpoints/{host}|{port}`
  — pipe-separated because an IPv6 literal already contains colons. Address the
  origin with `fqdn` or `ipAddress`, never both; which one is valid depends on
  the group's `networkEndpointType`. Everything is create-only, so any change
  detaches and reattaches.

- `GCP::Compute::SecurityPolicyRule` — one rule inside a global Cloud Armor
  policy. A new policy carries only a catch-all allow at priority 2147483647, so
  a policy alone permits everything; the rules are what enforce anything. Like
  the firewall policy rule it is a set of verbs on the policy rather than a REST
  sub-collection, so `FirewallPolicyRuleProvisioner` was generalised into
  `PolicyRuleProvisioner`, parameterised by a `policyRuleKind` (collection
  segment, owning-policy property, and where GCP's own rules start). Both rule
  types now share one implementation. Only global `securityPolicies` is wired up;
  a `RegionSecurityPolicy` rule would need a regional kind.

  **Conformance is red on Verify, Extract and Update**, and not because of this
  resource: `match.config`, an object nested inside `match`, is absent from
  stored state immediately after create and update. Create, Sync, Destroy and
  out-of-band delete pass, and Sync reports *all* expected properties matched —
  `match.config` included — so the read path is complete and the loss sits in
  how post-create and post-update state is materialised. That narrows the
  standing nested-property-loss bug: it is not the plugin's read, and it is not
  a typing choice, since both an untyped `Mapping<String, Any>` (what
  `NetworkFirewallPolicyRule.match` uses) and the typed classes lose the field.
  Every verb was verified directly against the API before shipping.

  Do not declare `rules` inline on the policy and manage rules with this
  resource at the same time — both own the same list and each will remove what
  the other added.

- `GCP::Compute::TargetGrpcProxy` — the proxy a proxyless gRPC service mesh
  points its clients at. The other target proxies terminate connections for a
  load balancer; this one hands an xDS-aware gRPC client the routing rules from
  its `urlMap`, with no proxy in the data path. `urlMap` is required, unlike on
  the HTTP proxies, and `patch` is rejected without a `fingerprint`, so unlike
  the other target proxies this one registers optimistic locking. Conformance is
  green on all eight steps, Update included.

### Changed

- The AlloyDB 8-segment native-ID parser is now a
  `ClusterScopedNativeID(resourceType)` factory shared by instances and users,
  with each leaf type rejecting the other's ids rather than mis-parsing them.
- `alloydbInstance`'s resolvable now exposes `cluster`. A resource that needs a
  *serving* database — `alloydbUser` — must be created after the instance, not
  merely after the cluster, and AlloyDB rejects user creation until a primary is
- `GCP::BigQuery::Routine` schema module — a user-defined function, table
  function, or stored procedure inside a dataset. The **provisioner already
  existed** (`pkg/resources/bigquery/routine.go`, registered since before this
  work) but no PKL module did, so the type was impossible to declare from a
  forma. Field names follow exactly what the provisioner reads and returns.

  **No conformance fixture:** the dev service account lacks
  `bigquery.datasets.create`, so a fixture cannot create the dataset a routine
  lives in. The schema itself is verified by `pkl eval` and by the parity tests.

### Added — tests

- `registration_parity_test.go` compares the live registry against the published
  PKL schema in both directions, after `init()`. A schema module with no
  provisioner is declarable but fails at apply; a provisioner with no schema
  module cannot be declared at all. Both were present in this repo.

  Three known gaps remain, each recorded with its reason in `knownParityGaps`:
  `Bigtable::Backup` and `Bigtable::MaterializedView` (schema, no provisioner)
  and `Storage::ObjectAccessControl` (registration commented out — object-scoped
  ACLs need both bucket and object in the path). A third test asserts the
  known-gap list has not rotted, so a fixed gap cannot quietly linger and hide
  later regressions.

- `GCP::Compute::DiskResourcePolicyAttachment` — attaches a `ResourcePolicy` to
  a disk, which is what makes a snapshot schedule do anything: a policy defines
  *when* to snapshot but has nothing to snapshot until it is attached. A third
  hand-written provisioner, since the attachment is an entry in the disk's
  `resourcePolicies` array manipulated with `addResourcePolicies` /
  `removeResourcePolicies` rather than a REST resource.

  A bare policy name is expanded to the disk's region (derived from its zone); a
  full path is passed through. `Read` reports NotFound when the entry is absent
  from the disk's list, so formae learns a detached policy is gone. Nothing is
  updatable — the attachment is a (disk, policy) pair, so a change detaches and
  reattaches.

- `GCP::Compute::NetworkPeering` — a VPC peering, so workloads in two networks
  reach each other over private IPs without a VPN. Implemented as a hand-written
  provisioner (`pkg/resources/compute/network_peering.go`): a peering is not a
  REST resource but lives inside the owning network, manipulated with
  `addPeering` / `removePeering`.

  Notable details: the native ID is composite
  (`projects/{p}/global/networks/{network}/peerings/{name}`); the API calls the
  far side `network`, which collides with the owning network, so the schema
  exposes it as `peerNetwork` and the provisioner maps it back inside the
  `networkPeering` wrapper; `Read` locates the peering in the owner's `peerings`
  array and reports NotFound when it is absent, so formae learns a removed
  peering is gone; and `List` walks every network in the project, since a peering
  is only discoverable through its owner.

  Peering is **one-sided**: declare it on both networks for the link to reach
  ACTIVE. A single side sits at INACTIVE, "Waiting for peer network to connect",
  which is what the conformance fixture exercises. v1 has no `updatePeering`
  method (the path 404s), so a change replaces.

- `GCP::Compute::RegionInstantSnapshot` — the regional twin of
  `InstantSnapshot`, taken from a regional disk so it inherits that disk's
  cross-zone replication and survives losing a zone. Same SSD-class source
  requirement as the zonal one.

- `GCP::Compute::RegionSslPolicy` — the regional twin of `SslPolicy`, for
  regional target HTTPS proxies, which cannot reference the global policy.
  Supports in-place updates; `sslPolicies.patch` requires the current
  `fingerprint`, supplied via optimistic locking.

- `GCP::Compute::RegionInstanceTemplate` — the regional twin of
  `InstanceTemplate`, and the one to prefer for a `RegionInstanceGroupManager`:
  a regional template keeps the MIG's dependency inside its own region instead
  of reaching across to a global resource. Immutable, as the global one is.

- `GCP::Compute::RegionHealthSource` — binds a backend service to a
  `RegionHealthAggregationPolicy`: the policy says *how* to judge a service
  healthy, the health source says *which* service that judgement applies to.
  Cross-region load balancing consumes the pair, so this is what makes the
  policy useful. As with the policy, the URL segment (`healthSources`) differs
  from the API method group (`regionHealthSources`).

- `GCP::Compute::RegionHealthAggregationPolicy` — decides when a whole backend
  service counts as healthy rather than each backend individually, so a
  cross-region load balancer can stop sending traffic to a mostly-failed region
  instead of trickling requests into a half-dead one. As with the firewall
  policies, the URL segment (`healthAggregationPolicies`) differs from the API
  method group (`regionHealthAggregationPolicies`). Conformance: passes.

- `GCP::Compute::NodeTemplate` — the specification a sole-tenant `nodeGroup`
  stamps physical nodes from (node type, CPU overcommit, affinity labels).
  Creating a template provisions nothing and costs nothing; the group that
  reserves hardware is deliberately not modelled, since it bills for a whole
  sole-tenant host. `serverBinding` and `status` are server-populated and left
  out of the schema.

- `GCP::AlloyDB::Backup` — **implemented but not yet conformance-verified**; see
  the note at the end of this entry. An on-demand backup of a cluster. Location-scoped
  rather than nested: a backup names its cluster in the body and outlives it, so
  it can be restored into a new cluster after the original is gone. The schema
  takes a short `cluster` id (so a forma can reference it through a resolvable,
  which is also how the backup orders itself after the instance) and the
  provisioner expands it to the `clusterName` path the API wants, folding it back
  on read.

  Verification is outstanding because the ~25-minute cluster+instance+backup
  chain has repeatedly been killed mid-flight by the test runner, each time
  leaving a READY cluster and instance behind — billable, and only noticed by
  inspecting the project directly. Re-run it only with a teardown check
  afterwards.

  serving. Referencing the cluster id *through* the instance yields the same
  value while making that dependency explicit to formae.

### Changed

- The two location-scoped Logging native-ID parsers are now one
  `LocationScopedNativeID(resourceType)` factory, and `LoggingSavedQueryAPI` is
  renamed `LoggingLocationAPI` since both saved queries and log scopes share it.

### Changed

- The update-body filter is now `base.DropFieldsOnUpdate(...)`, shared by
  `Logging::ProjectSink` and `Logging::ProjectExclusion`. Several GCP APIs build
  their PATCH `updateMask` from the body's top-level fields, so an immutable
  field left in the body lands in the mask and the call is rejected
  (Cloud Logging answers `"name cannot be changed"`).
- New `base.DropFields(...)` companion to `base.DropFieldsOnUpdate(...)`, for
  properties that identify a resource's place in the URL rather than its payload
  (a nested resource's parent id and location). Several GCP APIs reject those
  outright as unknown body fields. `Monitoring::Slo` and `Logging::LogView` both
  use it.
- `monitoringPathBuilder` now emits a parent segment
  (`/projects/{p}/{parentType}/{parent}/{resourceType}`) when one is set, which
  is what lets SLOs nest under a service. Existing Monitoring resources pass no
  parent and are unaffected.
- `GCP::Compute::BackendBucket` — global LB backend that serves a Cloud Storage
  bucket, so a URL map can route static paths to GCS. **No conformance fixture
  yet:** `backendBuckets.insert` validates the bucket asynchronously and fails
  the operation with `GCS_BUCKET_NOT_FOUND`, so the test needs a real bucket,
  and the dev service account currently has no Storage permissions.

## [0.1.13]

### Fixed

- **Parented resources were undiscoverable by construction.** `BaseResource.List`
  returned an error whenever a resource declared `RequiresParent` and the caller
  supplied no parent — which is exactly how discovery calls it. It now leaves the
  parent empty and lets the API config decide, so a path builder can substitute
  the API's own wildcard where one exists. `monitoringPathBuilder` uses
  `services/-` for `serviceLevelObjectives`, which lists every service's SLOs;
  create and read always carry a real service and never reach that branch.

- **A wrapped value reached the API as an empty field.** A forma can wrap a
  property — `formae.value(secret).opaque` keeps a secret out of plans and state
  — and formae unwraps those before calling a plugin, so the normal apply path
  never sees a wrapper. The conformance harness's out-of-band path calls the
  plugin directly with the evaluated forma, wrappers intact, so
  `sharedSecret` arrived as `{"$value": "..."}` and GCP rejected it as empty
  ("A shared secret must be..."), failing `vpn-tunnel`'s CreateOOB step. New
  `base.UnwrapValues` unwraps them anywhere in the properties — nested and
  inside lists — while leaving resolvables (`$res`) alone, since formae resolves
  those and a half-resolved reference must not be mistaken for a literal.

- **Saved queries were created where discovery could not look.** The fixture
  pinned `location = "global"` while discovery, having no properties to declare a
  location with, lists in the target's location. Saved queries are supported in
  either, so the fixture (and its update twin, or Update would try to change an
  immutable field) now follows `v.gcpLocation`. A query pinned to global is still
  created and managed correctly — it just cannot be discovered, which is a
  property of the API, not of this plugin.

- **Log scopes were listed in the wrong location.** They exist only in `global` —
  the API rejects a region *and* the `-` wildcard ("which may only be global") —
  but discovery has no properties to declare a location with, so `List` used the
  target's region and never saw them. The path builder now pins `logScopes` to
  `global`.

- **Two resources could be managed but never discovered.** `TestPluginDiscovery`
  calls `List` with no hints, and both
  `GCP::Compute::GlobalNetworkEndpoint` and
  `GCP::Compute::DiskResourcePolicyAttachment` returned an empty list unless
  they were told which parent to look inside — so their CRUD lifecycle passed
  8/8 while discovery timed out waiting for them to appear in inventory.

  `GlobalNetworkEndpoint.List` now walks every global network endpoint group and
  reports the endpoints inside them (a named group is still honoured as a fast
  path). `DiskResourcePolicyAttachment.List` walks `aggregated/disks` and reports
  each attached policy, filtered to zonal scopes so a native ID it emits is one
  its own `Read` can resolve.


- `GCP::SecretManager::SecretVersion` `data` (the secret payload) was typed
  plain `String`, so it could not be marked opaque and was stored in cleartext
  in desired state. It now accepts `formae.Value`/`formae.SecretValue`
  (opaque-by-default), matching `GCP::SQL::Database` `rootPassword`, so the
  payload is hashed at rest end-to-end regardless of how it is supplied.
  Backward compatible: a plain `String` is still accepted. The `secret-version`
  conformance test asserts the round-trip: the plain payload is hashed at rest
  and verified by SHA-256 digest.

### Changed

- Bump `plugin-conformance-tests` to v0.2.6, which verifies opaque secret
  fields against their authored plaintext by SHA-256 digest (v0.2.5 compared
  the hashed value to cleartext and could not exercise opaque payloads).

## [0.1.12]

### Changed

- Bump examples to the latest formae 0.88.0 schema.

## [0.1.11]

### Changed

- Genuine secret-value fields are now typed `formae.SecretValue` so their values are hashed at rest end-to-end (previously stored in cleartext on the read/actual-state path). Covers `GCP::Compute::BackendService` and `GCP::Compute::RegionBackendService` `oauth2ClientSecret`, `GCP::Container::Cluster` master-auth `password`, and `GCP::SQL::Database` `rootPassword`. Requires a formae agent on the matching release; `minFormaeVersion` is bumped to 0.88.0.

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
