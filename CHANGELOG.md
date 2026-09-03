# Changelog

All notable changes to the formae GCP plugin are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Install with `sudo formae plugin install gcp` on the host that runs the
formae agent.

## [Unreleased]

### Changed

- Six nested classes now `extend formae.SubResource` instead of being plain
  classes: `ApiGateway::ApiConfig`'s `ApiDocument` and `OpenApiDocument`,
  `Compute::Instance`'s `AttachedDisk`, `MetadataItem` and `Metadata`, and
  `Memcache::Instance`'s `NodeConfig`. Schema extraction only walks nested
  classes that formally extend `SubResource`, so these classes' `@gcp.FieldHint`
  annotations never reached the schema and `Metadata.items` never got the
  sub-resource render path's absent/explicit-null/explicit-empty handling.
  Both now apply. Two `hasProviderDefault` hints activate
  (`AttachedDisk.deviceName`, `Metadata.fingerprint`, the latter also
  `createOnly`), so a provider-populated value on either field is now tolerated
  instead of reading as drift on every sync, and Compute's `GCP::Compute::Instance`
  gains its first observable `disks.deviceName` / `metadata.fingerprint` paths in
  conformance runs.

### Added

- `GCP::Compute::PacketMirroring` — a copy of selected VMs' traffic, delivered to
  an internal passthrough load balancer for inspection. `mirroredResources` says
  whose packets to copy — named instances, whole subnets, or network tags — and
  `collectorIlb` says where the copies go. The collector has to be a forwarding
  rule created with `isMirroringCollector`; an ordinary internal rule is refused.

  Two API behaviours the type compensates for. `network` is fixed once set, and
  a patch carrying it in any spelling other than the exact stored URL is refused
  with "Network cannot be changed" — so the field never goes out on an update and
  a change to it replaces the resource. And `packetMirrorings.patch` is a JSON
  merge patch, so a selector left out of `mirroredResources` keeps its old value:
  dropping every tag from a forma would otherwise leave the tags mirroring while
  the plugin reported success. The absent selectors are sent as explicit empty
  lists instead, which does clear them. The conformance case's update exercises
  exactly that, dropping a tag the create declared.

  Every reference in this resource is an object rather than a bare string, and
  GCP answers each with a second `canonicalUrl` naming the same target by numeric
  id. It is output-only and sits inside a sub-resource, where a schema hint
  cannot reach it, so it is stripped on read — without that, every read disagrees
  with the declaration and plans an update that changes nothing.

  The case costs nothing to run: mirroring a subnet rather than named instances
  means no VM has to exist to have something to mirror, and the collector's
  backend service carries no backends.
- `GCP::CertificateManager::Certificate` — a TLS certificate a load balancer can
  serve. A *managed* one is obtained and renewed by Google against a
  `DnsAuthorization` and carries no private key, which is the kind a repository
  can describe; `selfManaged` is the other half of the type. Creation returns
  immediately and a managed certificate then sits in `PROVISIONING` until the
  authorizing DNS record resolves, which is a property of the domain rather than
  of the resource.

  The API field is `managed`. A forma cannot use that name — `managed` is a fixed
  property of `formae.Resource` — so the schema calls it `managedCertificate` and
  the plugin renames it in both directions.

- `GCP::CertificateManager::CertificateMapEntry` — one row of a certificate map:
  which certificates to serve for a hostname. Give either `hostname` or
  `matcher`, where `matcher = "PRIMARY"` is the fallback when no hostname
  matches.

  A forma names the certificates and authorizations it points at by short id,
  because that is all a reference can yield, while the API wants a full path on
  the way in and answers with one — carrying the project *number* where the forma
  used the project id. Both fields are immutable, so the plugin expands the
  request and shortens the response; with only one half, every re-apply would
  plan a replacement of a certificate that a map entry still references, and the
  delete would then be refused.

  Deleting a `DnsAuthorization` while a certificate still references it is
  likewise reported as retryable rather than fatal: the certificate's own delete
  is a long-running operation, so the refusal is a race that clears itself.
- `GCP::NetworkSecurity::AddressGroup` — a named, reusable set of IP addresses and
  CIDR blocks that firewall policy and Cloud Armor rules match against, so a rule
  names one group instead of restating every address. Global, and free: a group
  provisions nothing and matches no traffic until a rule points at it.

  `type` and `capacity` are both fixed at creation — the API answers a capacity
  change with "capacity can't be changed" — so neither goes out on an update and a
  change to either replaces the resource. `purpose` carries a provider default of
  `["DEFAULT"]`; its only other value, `CLOUD_ARMOR`, needs a Cloud Armor
  Enterprise subscription and is refused outright without one, so the conformance
  case leaves the field unset.

  One API behaviour worth knowing: a patch whose update mask omits `labels` clears
  them, which is not how the other types in this API behave. The mask is built
  from the request body, so a forma that declares labels keeps sending them and
  the wipe cannot bite — but a patch hand-built with a narrower mask would lose
  them silently.

- `GCP::NetworkSecurity::UrlList` — a named list of URL patterns for a Secure Web
  Proxy policy to match on. Free, and inert until a policy names it.

  Regional, alone among the types in this API. Asked for `locations/global` it
  does not return an empty list, it fails the call with "Invalid location in
  resource URL path", so the scope is pinned per collection rather than inferred.
  `description` comes back as an empty string when it was never set, so it carries
  a provider default rather than staying absent.

- `GCP::NetworkSecurity::SecurityProfile` — the policy half of Cloud NGFW's
  layer-7 inspection: what to do about a threat, not where to apply it. Global.

  Creating one provisions no capacity. Inspection becomes billable only when a
  firewall endpoint — an organization-level resource — is attached, so a profile
  with nothing attached costs nothing, and the conformance case attaches nothing.

  `type` is fixed at creation and decides which of the four settings blocks
  applies; it is dropped from the patch body so it cannot enter the update mask.
  `etag` is never sent back on a write: replaying a stored one fails the call with
  409 "Provided etag is out of date" as soon as anything else has touched the
  profile. And `threatPreventionProfile.threatOverrides[].type` is output-only but
  nested, where a schema hint cannot reach it, so it is stripped on read —
  otherwise every read carries a property the schema never declared.

- `GCP::NetworkSecurity::SecurityProfileGroup` — the binding a firewall policy
  rule actually names, gathering up to one security profile of each kind. Global,
  and free for the same reason a profile is.

  Each of the four profile fields is a full resource path on the wire but a short
  name in a forma, which is what a resolvable yields. The plugin expands on the
  request and shortens on the response; both halves have to exist, because
  expanding without shortening would leave the declared value and the stored state
  permanently disagreeing and every re-apply planning a replacement of a group
  that has not changed. `etag` and `dataPathId` are server-owned and never sent.

  Note: only `threatPreventionProfile` is exercised by the conformance fixture.
  The other three references use the identical code path but are declared from the
  API's discovery document rather than from a live create.

- `GCP::NetworkConnectivity::InternalRange` — a reservation of internal IP space
  in a VPC. It marks a CIDR range as spoken for so nothing else is allocated over
  it; a subnet created later in the same space is rejected. Give it a range, or a
  `prefixLength` and let the API pick a free block of that size.

- `GCP::NetworkConnectivity::PolicyBasedRoute` — a route chosen by what the
  traffic is, not only where it is going. An ordinary route matches the
  destination; this also matches protocol and source range, which is how traffic
  is steered through an appliance. Every field is fixed at creation, so a change
  is a replacement.

  It is the one type here that will not accept a network self link — it answers
  `network uri ... is not in the form of projects/my-project/global/networks/my-network`
  — and it reports back whatever form it was given. Since a reference to a
  network resolves to a self link, the plugin cuts the request down and expands
  the response back; without both halves the field disagrees with itself on every
  re-apply and plans a replacement of the route already in place.

- `GCP::NetworkConnectivity::ServiceConnectionPolicy` — permission, in advance,
  for a managed service to place Private Service Connect endpoints in a
  consumer's subnets. Regional, unlike the rest of this API, which lives under
  `locations/global`. A patch may carry only `description`, `labels` and
  `pscConfig`; anything else in the body enters the update mask and the API
  refuses it.

- The Replace phase is exercised for the first time. Every conformance case in
  this plugin reported replace as skipped, because a case only tests it when a
  `-replace.pkl` accompanies it and none existed - so a run reporting "8/8" was
  really seven steps and a skip, and nothing had ever checked that replacing a
  resource works. The three Service Directory cases now carry one: each changes
  the immutable id, which is what forces a replace, and the harness verifies the
  native ID actually changed rather than the resource being updated in place.
  Cases that predate this work carry one too, so the phase is covered for types
  this batch did not add: a bucket, a Pub/Sub topic, a secret, an address, a
  health check, a network, a firewall rule, three logging types, a monitoring
  service and dashboard, an SLO, an instance template, a backend bucket and an
  IAM service account. Each changes an immutable name, which is what makes the
  operation a replace rather than an update.

- `GCP::Memcache::Instance` - Memorystore for Memcached: a managed memcached
  cluster on a VPC network. It is billed by node-hour for as long as it exists
  and takes twenty minutes or more to create, so its conformance case is the
  slowest here at 27 minutes and the fixture asks for the smallest thing the API
  accepts, one node of 1 GiB.

  An instance reaches its nodes over private service access and the API refuses
  one outright without it, so the case builds a VPC, the range reserved for the
  service producer to peer into, the peering itself, and only then the instance.
  The instance names the peering rather than the network: it needs private
  service access to exist, not merely the VPC.

  The authorized network is accepted in any of the three forms a forma can name
  one - short, path, or self link - and normalised to the single form the API
  takes. A reference to another resource's network property resolves to a self
  link, which memcache rejects outright.

- `GCP::Spanner::Instance` and `GCP::Spanner::Database` - Spanner is the one
  service here whose resources are billed for as long as they exist: the
  smallest regional instance is 100 processing units, a tenth of a node. A forma
  declaring one is spending money until it is destroyed, and the module doc says
  so.

  Two shapes the generic engine does not cover. An instance's id travels as
  `instanceId` alongside an `instance` object rather than as a name in the body
  or a query parameter, so the create body is assembled. And Spanner creates a
  database by executing a `CREATE DATABASE` statement rather than taking a name,
  so the plugin builds that statement from the name a forma declares.

  A database name is capped at 30 characters, which the usual test prefix plus a
  run id exceeds, so the fixture names it short - it does not need the prefix,
  because deleting an instance takes its databases with it.

- `GCP::BigQuery::Connection` - a named handle BigQuery uses to reach something
  outside itself. A `cloudResource` connection carries no configuration of its
  own: BigQuery mints a service account for it, and granting that account access
  is how a query reaches the resource. Holding one costs nothing.

  It lives behind a separate API - a different host and a location-based path -
  so it has its own package rather than joining BigQuery's, whose config points
  at bigquery.googleapis.com.

  The service account is reported as `cloudResourceServiceAccountId` rather than
  nested inside `cloudResource`, because a schema hint is only emitted for a
  top-level field: a nested one cannot be marked as server-filled and reads as a
  property the forma never declared. Hiding it would have been the smaller
  change and the wrong one - granting that account access is the point of the
  type.

  The connection's native ID carries the project as a forma names it. The API
  answers with the project number, and the native ID is where a later read gets
  its path context, so a number left there comes back as the project on every
  sync however the response is transformed.

- `GCP::DNS::Policy`, `GCP::DNS::ResponsePolicy` and
  `GCP::DNS::ResponsePolicyRule` - a policy decides how DNS behaves for the
  networks it is attached to; a response policy overrides what DNS answers for
  them, one rule at a time. None costs anything to hold: Cloud DNS bills zones
  and queries, and Cloud DNS had shipped with a single type until now.

  Three shapes the generic engine did not expect. A response policy's id field
  is `responsePolicyName` and a rule's is `ruleName`, not `name` - a listed item
  carries no path context to fall back on, so without handling them every
  response policy and rule would have listed with an empty native ID and never
  been discovered. Cloud DNS also stamps a `kind` discriminator on nested
  objects, not just the resource itself, and each survivor reads as a property
  the forma never declared.

  A rule hangs off its response policy, so the DNS path builder and native ID
  handle a parent now; both previously assumed a flat
  `/projects/{p}/{collection}/{name}`.

  Deleting a policy detaches its networks first. Cloud DNS refuses to delete one
  while a network is still attached, and nothing in the forma is holding it - the
  network is a prerequisite that outlives the policy - so the deletion looks
  unblocked and simply fails.

- `GCP::ApiGateway::Api`, `GCP::ApiGateway::ApiConfig` and
  `GCP::ApiGateway::Gateway` - API Gateway serves an api from a regional
  gateway: an api holds immutable configs, and a gateway names the config it
  serves rather than the api, because a change to a config produces a new one.
  Holding an api or a config costs nothing; API Gateway bills the calls a
  gateway serves.

  Apis and configs are always global while gateways are regional, so the path
  builder supplies the location for the first two rather than requiring one to
  be named. Every write is a long-running operation, and a fresh operation does
  not carry the resource - its metadata names the target it is building, which
  is where the native ID comes from. A config only exists underneath an api and
  there is no wildcard in the api position, so a parentless list walks the apis.

  A gateway names the region it runs in. API Gateway serves eleven regions and a
  target's is often not among them - creating a gateway in one it does not serve
  answers "Location ... is not found or access is unauthorized" - so the region
  cannot be taken from the target. It is not scoped either: every scope
  available overwrites the location, one by clearing it and one by substituting
  the target's, and a read that lost it addressed a wildcard path rather than
  the gateway. A parentless list spans every region with the location wildcard,
  which the API accepts, rather than looking only where the target happens to
  be.

  A config also reports its own full resource path. A gateway names the config
  it serves that way, while `name` holds the short id a forma declares, and a
  reference interpolated into a string is emitted as its envelope rather than
  resolved - so without it the reference could not be expressed at all.

- `GCP::ServiceDirectory::Namespace`, `GCP::ServiceDirectory::Service` and
  `GCP::ServiceDirectory::Endpoint` - Service Directory publishes where a
  service lives without running a registry: a namespace holds services and a
  service holds endpoints. Holding them costs nothing, only lookups are billed,
  so the whole hierarchy is testable without provisioning anything.

  All three are config-driven. The id travels as a create-time query parameter,
  updates are a PATCH with a mask built from the body, and every operation is
  synchronous. An endpoint is addressed by a namespace and a service at once and
  uses the two-property parent for it. Nothing can be listed across its parents -
  `locations/-` answers "Unsupported location: -", and `namespaces/-` and
  `services/-` both answer "Could not parse namespace name" - so a service walks
  the namespaces and an endpoint walks the services inside them.

### Changed

- Every `hasProviderDefault` schema annotation now carries a recorded
  disposition in `schema/provider-default-dispositions.json`, enforced by a
  unit test: new annotations fail CI until classified, and rows for removed
  fields fail as stale. All current annotations start as `pending`;
  classifications land per field as the provider-default audit reaches them.
### Fixed

- `GCP::SQL::Database` is discoverable. A database only exists underneath an
  instance and Cloud SQL cannot be asked across instances -
  `/projects/{p}/databases` answers 404 and `/projects/{p}/instances/-/databases`
  answers 400, so there is no wildcard to substitute - while discovery lists
  with no parent to name. A parentless list now walks the project's instances,
  skipping any it cannot read but reporting an error rather than an empty list
  if every one fails.

- `GCP::Storage::ObjectAccessControl` is registered, and `knownParityGaps` is
  empty. It hangs off a bucket *and* an object, and nothing could carry two
  parent properties - which is why its registration had been commented out.
  `ParentResourceConfig.SecondPropertyName` now joins them as
  `{bucket}/{object}`, the form the Storage path builder and native ID already
  expected. Every type the schema declares now has a provisioner behind it.
  Discovery needs more than that: it lists with neither parent to name, so an
  object ACL is found by walking the buckets and then their objects. Listing a
  bucket's objects with `projection=full` carries every object's `acl` inline,
  which keeps that to one request per bucket rather than one per object.

- `GCP::Bigtable::MaterializedView` matches its API and has a provisioner. The
  schema declared `sourceTable` and `cluster`; the API has neither. A
  materialized view belongs to an instance and is defined by a GoogleSQL
  `query`, alongside `deletionProtection`, `clusterStates` and `etag`. Nothing
  had noticed because the type had no provisioner either, so a forma declaring
  one failed at apply before any field could be rejected. Both are fixed, and
  `knownParityGaps` is down to one. Like a backup, it has to be bound to
  Bigtable's own provisioner: the generic one sends no id query parameter, and
  the API answered `Invalid id for collection materializedViews : Length should
  be between [1,128], but found 0`. The parameter is snake_case while the
  collection is camelCase, so trimming the plural alone produced
  `materializedView_id`, which the API ignored before rejecting the create for
  the empty id it had never been given. It is listed by walking the instances,
  for the same reason a backup is.

- `GCP::Bigtable::Backup` works. Its schema shipped with no provisioner behind
  it, so declaring a backup failed at apply - one of three types in
  `knownParityGaps` that were declarable and unusable. Much of what it needed
  was already there: the three-level path builder, the native ID handling and
  the cluster extraction in Create all handled backups. Registering the type was
  not enough, though - it also has to be bound to Bigtable's own provisioner,
  and being left out of that list sent it to the generic one instead, which
  knows nothing of the cluster a backup lives under and so addressed
  `/instances/{i}/backups`, a route that does not exist. It is now bound, with
  the transformers that expand `sourceTable` to the full path on the way out and
  recover the instance and cluster from the path on the way back, and with a
  list that walks the project's instances and uses the `clusters/-` wildcard
  within each - discovery lists with no parent to name, and no route spans
  instances. Two known parity gaps remain.

- `GCP::Eventarc::Trigger` can be created at all. Eventarc requires the short id
  in `?triggerId=` and the full resource path in the body's `name` - which its
  own schema marks Required - at the same time. `base.Create` reads the id out
  of `name` and deletes it, so the body reached the API without one and every
  create failed with "The request was invalid: trigger.name is empty". Trigger
  now has a Create that supplies both.

- `GCP::BigQuery::Routine` is discoverable. Its `List` refused to run without a
  `datasetId` in AdditionalProperties, and discovery lists with no properties at
  all - it cannot know a routine's parent, because the provisioner is
  hand-written and declares no `ParentResource`. A parentless list now walks
  every dataset in the project; a caller that names one still gets just that one.
  `GCP::BigQuery::Table` has the same shape and is left as it is for now, with a
  comment saying so: it has no conformance case, and an unverified fix is easily
  mistaken for a verified one.

- `GCP::Monitoring::MetricDescriptor` is discoverable. `metricDescriptors.list`
  returns every descriptor a project can see - well over a thousand built-in
  ones for GCP's own services - so a custom metric was somewhere in that pile
  and not necessarily on the first page: discovery listed, never saw the
  descriptor it had just created, and timed out. The list is now filtered to the
  prefix a project can own. That is not only an optimisation: a built-in
  descriptor cannot be created, changed or deleted, so it is not a resource
  formae can manage and does not belong in discovery.

  It is one prefix and not two because Cloud Monitoring rejects the obvious
  form: `metric.type = starts_with(...) OR metric.type = starts_with(...)`
  answers HTTP 400, "Within the 'metric' prefix, OR can only be used to connect
  a list of 'labels'". A rejected filter fails the whole list, and an empty list
  reads downstream as "the resource is gone" - sync tombstoned a descriptor that
  was really there. `external.googleapis.com/user/` descriptors are not listed
  as a result; they are written by the Cloud Monitoring agent, not by a forma.

- Conformance setup retries back off instead of going again immediately. The
  harness fetches the formae binary and starts an agent before it touches any
  cloud infrastructure, and a single instant retry only survives a blip shorter
  than the retry itself: in a 151-case matrix the package channel went away for
  long enough that both attempts hit it seconds apart, and two unrelated cases
  failed having run no test at all. Three attempts now, 10s then 30s apart. A
  failure that is not setup still fails on the first attempt.

- Leaked SSL certificates can be swept. A project holds at most 10 globally, so
  once the cap is reached any case creating one fails with
  "Quota 'SSL_CERTIFICATES' exceeded" rather than anything resembling a plugin
  bug. The sweep knew about `ssl-policies` but never about the certificates.

  It is opt-in, behind `FORMAE_SWEEP_SSL_CERTIFICATES=1`. Certificates are the
  one resource here whose removal is not obviously safe to decide
  automatically: unlike a namespace or an api, one can have been installed
  deliberately, and a global cap means a wrong deletion is felt project-wide.

- A `GCP::Storage::Object` reports its properties when created. Create returned
  a native ID and nothing else, so a freshly created object had no stored state
  and anything referencing one resolved against nothing and stayed an unresolved
  reference. That is how an object ACL reached the plugin with its object still
  a reference and addressed the bucket alone.

- An object name is percent-encoded in bucket-scoped paths.
  `conformance/acl-target.txt` is one object, not a folder and a file. The
  object provisioner escaped it in its own URLs but the shared path builder did
  not, so an object ACL addressed a path that does not exist and the API
  answered 404. The native ID keeps the name raw, and the parser now reads the
  object as everything between the object marker and the trailing type and name
  rather than as a single segment.

- A resource addressed by two parents is refused rather than silently collapsed.
  When the second parent was missing, the path context kept only the first and
  both the request URL and the native ID became the one-parent form - which the
  API accepts as a perfectly valid *different* resource. An object ACL created
  without its object became a bucket ACL: create reported one native ID,
  discovery reported another, and nothing downstream could tell them apart. A
  wrong resource created successfully is worse than a failed create.

- A `GCP::Eventarc::Trigger` can reference the `Workflow` it delivers to.
  `destination.workflow` was a plain `String`, so a forma could name a workflow
  but not reference it - and ordering comes only from resolvable references, so
  formae was free to create the trigger before the workflow it targets. It now
  accepts a resolvable, and the request expands the bare name into the full path
  Eventarc wants while the response shortens it back.

- A resource identified by anything other than `name` can be discovered. The
  generic list path required every listed item to carry a `name` before it would
  consult the API's own native-ID extractor, so a Cloud Storage ACL entry -
  identified by `entity`, with no name at all - produced nothing, and the list
  came back empty with no error. The extractor is now asked first, and the
  name-shaped path is the fallback.

- Fourteen Storage fields the schema itself documents as "(output only)" are
  marked as provider defaults, across the three ACL types and `AnywhereCache`:
  `projectTeam`, `entityId`, `generation`, `domain` and `email`. A
  `DefaultObjectAccessControl` reported drift on `projectTeam` the moment it was
  created, because the comment said output-only and the hint did not.

- Bigtable creates hand their properties back. `BaseResource.Status` does not
  read the resource once an asynchronous operation completes, and an async
  create returns no properties either - so nothing that referenced a Bigtable
  resource could resolve. A table declaring `instance.res.name` failed with
  "instance is required for nested resources" on an instance that was plainly
  declared, because the create it referenced handed back nothing to resolve
  from. Status now routes through `base.StatusWithRead`.

- Resolvables that pointed at a property name the schema does not have. A
  `GCP::Storage::Bucket` resolvable's `name` targeted `"Name"`, so every forma
  referencing `bucket.res.name` failed to apply with `source resource ... has no
  property "Name"` - which is what a bucket ACL case hit. GCP's JSON is
  camelCase, and 17 resolvable targets across `Bucket`, `AnywhereCache`, the
  three ACL types, `Container::Cluster` and `Container::NodePool` were
  capitalised. Corrected wherever the lowercase field is declared in the same
  file, which makes each one provable rather than guessed.

  `Bucket`'s `selfLink` resolvable is removed outright: the type has no
  `selfLink` property at all, so it could never resolve.

- `GCP::Bigtable` creates unwrap wrapped property values. The hand-written
  provisioner read properties directly while `base.Create` unwraps them first,
  so a wrapped value read as a plain string came back empty - surfacing as
  "instance is required for nested resources" on a table whose instance was
  declared.

- `GCP::Storage::BucketAccessControl` and `GCP::Storage::DefaultObjectAccessControl`
  are discoverable. An ACL entry lives at `/b/{bucket}/acl`, and Cloud Storage
  has no endpoint spanning buckets - no `-` wildcard in the bucket position, as
  privateca and Datastream offer - so discovery, which lists with no parent,
  asked for a URL with an empty bucket segment and found nothing. Both types now
  walk the project's buckets, skipping any whose ACLs cannot be read: a bucket
  with uniform bucket-level access rejects the read outright, and a shared
  project holds buckets a target does not own.

- Creating a `GCP::Datastream::ConnectionProfile` no longer half-succeeds. The
  API validates a profile against the source it describes, and does so inside
  the long-running operation - after the profile has been created. A profile
  naming a host that does not answer was therefore created *and* reported as
  failed; formae retried, and the retry collided with the profile the first
  attempt had made ("Resource ... already exists"), so the validation error
  never surfaced at all. Creates now send `force=true`, as stream creates
  already did.

- `GCP::Bigtable::Table` no longer reports drift the moment it is created. The
  API reports a table's `name` as the full path
  `projects/{p}/instances/{i}/tables/{t}` while a forma declares the short id
  and the instance separately, and Table was registered with the generic
  response transformer, which only fills in the project. Instance and Cluster
  each had one of their own; Table now does too, and recovers the instance from
  the path as well - it lives nowhere else in the response.

- `GCP::Compute::Address.networkTier` and `purpose`, and
  `GCP::Bigtable::Table.granularity`, are marked as provider defaults. GCP fills
  all three in when unset.

- `GCP::Storage::BucketAccessControl` and `GCP::Storage::DefaultObjectAccessControl`
  can be created at all. Their `role` field carried no `@gcp.FieldHint`, so the
  plugin never treated it as a resource property and never sent it - and the API
  rejects an ACL without one: "Access control must contain a role". Both types
  had shipped in this state, with no conformance case to reveal it.
  `ObjectAccessControl` has the same omission and is fixed alongside, though it
  still has no provisioner.

- Server-populated fields across nine services are marked as provider defaults,
  so a forma that does not declare them no longer reports drift the moment the
  resource is created. Twenty-four fields in all: every `proxyHeader` (GCP fills
  it in with NONE), the `fingerprint` and `labelFingerprint` hashes, and the
  `state`, `status`, `selfLink`, `kind`, `uid`, `createTime` and `updateTime`
  fields on Container, Storage, CloudRun, BigQuery and Compute types.

  Two lookalikes are deliberately left alone: `WorkflowTemplate.id` is the
  identifier a forma chooses, and `ExternalVpnGatewayInterface.id` is a
  caller-supplied 0-based index that `VpnTunnel` references.

- Six server-populated Compute fields are marked as provider defaults, so a
  forma that does not declare them no longer reports drift the moment the
  resource is created: `Address.labelFingerprint`, `effectiveLabels`,
  `terraformLabels`, `users` and `selfLink`; `TargetHttpsProxy.fingerprint` and
  `RegionTargetHttpsProxy.fingerprint`; `TargetSslProxy.proxyHeader`; and
  `ForwardingRule.labelFingerprint`. Each was reported as "not expected and not
  a provider default" the first time a conformance case exercised the type.

- A successful long-running operation is no longer reported as a failure. The
  status checker treated the mere presence of an `error` key as a failure, but a
  finished operation may carry `"error": {}` - present and empty, which is an
  absent status, not an error. Every affected create was reported failed after
  it had already succeeded; formae then retried it, and the retry answered
  "Resource ... already exists", which masked the original operation entirely.
  An error now counts only when it carries a message or a non-zero
  `google.rpc.Code`.

  Affected Datastream, Eventarc, Certificate Authority Service and Filestore -
  every package with an asynchronous create. All four had an identical copy of
  the checker; they now share one in `base`.

- `GCP::Datastream::Route` is discoverable. A route only exists underneath a
  private connection, and discovery lists with no parent to name, so the plugin
  asked for `/projects/{p}/locations/{l}/routes` - a 404 - and no route was ever
  found. Datastream accepts `-` in the private-connection position, so a
  parentless list now asks across every one. No parent-walking List needed,
  unlike Analytics Hub, which has no such wildcard.

- Bigtable's nested types can now reference their parent. `Table.instance`,
  `Cluster.instance`, `Backup.instance`/`cluster`/`sourceTable` and the same
  three on `MaterializedView` were plain `String`, so a forma could name a
  parent but not reference it. Ordering in a forma comes only from resolvable
  references, so declaring an instance and a table together gave formae no edge
  between them and nothing guaranteed the instance existed first. All eight
  fields now accept `(String|formae.Resolvable)`.

- Four Compute types no longer claim to support updates their API cannot
  perform: `TargetPool`, `TargetSslProxy`, `TargetTcpProxy` and
  `RegionTargetTcpProxy`. None of `targetPools`, `targetSslProxies`,
  `targetTcpProxies` or `regionTargetTcpProxies` has a `patch` or an `update`
  method - they offer only setters such as `setBackendService` and
  `setSslCertificates` - so an update planned a PATCH to a URL the API does not
  serve. A change now replaces.

  Found by writing the first conformance case for `TargetPool`, then checking
  every Compute definition against the discovery document: 62 scanned, 4 wrong.
  Three of the four had no conformance case at all, and the fourth
  (`TargetTcpProxy`) had one with no update fixture.

### Added

- `GCP::Storage::Object` - a single object in a bucket with its content declared
  inline, for the small files infrastructure is made of: a config document, a
  startup script, a static index page. The content is part of the forma, so it
  is part of the plan and the state; wrap it with `formae.value(...).opaque` to
  keep it out of both.

  Uploading needed two additions to the transport, which until now sent only
  JSON: `RawBody`/`ContentType` for sending bytes verbatim, and `SendRaw` for
  reading them back - an object's bytes are a declared property, so without
  reading them a changed object would never read as changed.

- A conformance case for `GCP::Bigtable::MaterializedView`.

- A conformance case for `GCP::Bigtable::Backup`, and `FORMAE_TEST_FUTURE_TIMESTAMP`
  in the environment conformance cases run with. A backup's `expireTime` must be
  an absolute timestamp between 6 hours and 90 days out, and Pkl has no clock, so
  a fixture cannot compute one.

- A conformance case for `GCP::Bigtable::Cluster`, the first to use the
  on-demand list. A cluster is an additional replica of an instance and the
  instance must be PRODUCTION - a DEVELOPMENT instance cannot have a second
  cluster - so the forma holds two billed nodes while it runs. It is excluded
  from the automatic matrix and run by naming it in debug-conformance.

- `testdata/on-demand-cases.txt`, a list of conformance cases excluded from the
  automatic CI and nightly matrix. Every other case runs on every push to main
  and every night; a case named here runs only when dispatched explicitly
  through debug-conformance. It exists so a resource that cannot be covered for
  free can still be covered at all, rather than being left with no conformance
  case because covering it would add spend to every run.

- A conformance case for `GCP::SQL::Database`, which shipped without one. It
  builds a `db-f1-micro` instance to hold the database, mirroring the
  cloudsql-instance case's private-IP settings.

- A conformance case for `GCP::Eventarc::Trigger`, the last Eventarc type
  without one. It delivers to a `Workflow`, which is free to define and needs no
  container image or network attachment.

- A conformance case for `GCP::BigQuery::Routine`. It is the first case to
  exercise any BigQuery type: `Dataset` and `Table` also shipped without one,
  and this case builds a dataset to hold the routine.

- A conformance case for `GCP::Monitoring::MetricDescriptor`, which shipped
  without one. A custom metric descriptor stands on its own and costs nothing,
  so the case declares one and nothing else.

- `iamConfiguration` on `GCP::Storage::Bucket`, so a forma can say whether a
  bucket uses uniform bucket-level access. A UBLA bucket is controlled by IAM
  alone and rejects ACLs outright, so without this field a bucket meant to carry
  a `BucketAccessControl` depended on a project or organisation default.

- A conformance case for `GCP::Bigtable::Table`, the first for any nested
  Bigtable type.

- Conformance cases for `GCP::Storage::BucketAccessControl` and
  `GCP::Storage::DefaultObjectAccessControl`. Both shipped without one.
  `GCP::Storage::ObjectAccessControl` still has none: it needs an object to
  attach to, and there is no `GCP::Storage::Object` type.

- Conformance cases for `GCP::Compute::Address`, `GCP::Compute::TargetPool`,
  `GCP::Compute::BackendBucket`, `GCP::Compute::TargetHttpsProxy`,
  `GCP::Compute::TargetSslProxy`, `GCP::Compute::RouterNat` and
  `GCP::Compute::RegionTargetTcpProxy`. All seven shipped without one, so
  nothing ever exercised them against the live API. That leaves
  `GCP::Compute::RegionTargetHttpsProxy` as the only untested Compute type: it
  needs a regional SSL certificate, and a regional MANAGED certificate is not
  supported, so the case would have to commit a self-managed key pair.

- `GCP::Storage::Notification` - publishes a bucket's object change events to a
  Pub/Sub topic. Cloud Storage publishes as the project's own service agent, so
  the topic must already grant that principal `roles/pubsub.publisher`; the new
  `GCP::PubSub::TopicIamMember` is what expresses that, and the conformance case
  declares it rather than relying on a grant made by hand.

- `GCP::PubSub::TopicIamMember` and `GCP::PubSub::SubscriptionIamMember` - a
  single (role, member) binding on a topic's or subscription's IAM policy,
  managed read-modify-write so sibling bindings survive. A binding is modelled
  rather than the whole policy because a policy is shared with principals
  outside the forma - GCP's own service agents write to it - and declaring the
  whole policy would delete their bindings on every apply.

- `GCP::Compute::RegionSnapshot` - a regional incremental disk backup. Distinct
  from the global `Snapshot` already shipped: that one lives at
  `/global/snapshots`, this one at `/regions/{region}/snapshots` and stays in
  its region.

- `GCP::Filestore::Backup` and `GCP::Filestore::Snapshot`. `Snapshot` ships
  without a conformance case: every tier that supports snapshots is
  enterprise-class, and `EnterpriseStorageGibPerRegion` is 0 in the shared test
  project, so a create cannot succeed there at all. Raising that quota is what
  would let the case exist. A backup copies one
  file share and outlives the instance it came from; a snapshot lives inside the
  instance and goes when it does.

  Snapshots are nested under an instance and Filestore has no wildcard in that
  position, so discovery walks the instances rather than asking for a URL with
  an empty segment.

- `GCP::Datastream::Stream`, `GCP::Datastream::PrivateConnection` and
  `GCP::Datastream::Route` - the rest of the creatable Datastream surface. A
  stream is what actually moves data; a connection profile on its own moves
  nothing. A private connection peers a VPC with Datastream's network for
  sources that are not publicly reachable, and a route tells it which address to
  reach the source on.

  Creating a stream sends `force=true`. Datastream validates a stream against
  its source at create time, so without it a stream whose source is not
  reachable at apply time fails on validation rather than on anything wrong with
  the declaration.

- `GCP::CertificateAuthority::CertificateAuthority` and
  `GCP::CertificateAuthority::CertificateTemplate`. A CA is what actually signs;
  a `CaPool` with no CA in it issues nothing. A template is a reusable issuance
  policy, location-scoped rather than pool-scoped.

  A CA is deleted with `skipGracePeriod`, because a plain DELETE does not delete
  it: it moves to state DELETED and sits there for 30 days, still holding its id
  and still billed. `ignoreActiveCertificates` and `ignoreDependentResources` go
  along so a CA that did issue something still tears down.

  Not implemented: `certificates`. The API has no delete for them - a
  certificate can only be revoked - so they are not a CRUD resource.

- `GCP::AnalyticsHub::DataExchange`, `GCP::AnalyticsHub::Listing` and
  `GCP::AnalyticsHub::QueryTemplate` - the whole creatable surface of Analytics
  Hub. An exchange is the container a publisher shares through, a listing
  publishes one BigQuery dataset into an exchange, and a query template is the
  data-clean-room construct that lets a subscriber run a routine against shared
  data without seeing the rows.

  Neither listings nor query templates have a URL spanning exchanges, and there
  is no wildcard in the parent position, so discovery - which lists with no
  parent to name - walks the exchanges and asks each one.

  Analytics Hub ids allow only letters, digits and underscores, so its test
  fixtures are underscore-named where every other fixture here is
  hyphen-named - and its cleanup sweep greps accordingly.

- `GCP::Eventarc::Enrollment` - the routing rule of an Eventarc Advanced setup:
  a CEL expression matched against the events on a `MessageBus`, and the
  `Pipeline` matching events are handed to. A bus without an enrollment routes
  nothing, so this is what makes the existing `MessageBus` and `Pipeline` types
  useful together.

- `GCP::Eventarc::GoogleApiSource` - routes this project's own Google API events
  onto a `MessageBus`. Only one is allowed per project per region, the same
  constraint `MessageBus` already carries.

  Both name other Advanced resources through a scalar path field, so both get
  the expand-on-write / shorten-on-read pair a forma needs to pass a resolvable
  (`bus.res.name`) instead of hand-writing a full path - the scalar counterpart
  of what `pipelineRequestTransformer` already does for a pipeline's nested
  destinations.

- `GCP::PubSub::Snapshot` - captures a subscription's acknowledgement state so
  the subscription can later be seeked back to it. Pub/Sub creates a snapshot by
  `PUT`ting to its resource path, and the create body is not the resource: it
  takes the `subscription` to snapshot, while the snapshot itself reports only
  the `topic` that subscription was attached to. `subscription` is therefore
  write-only, so it is sent on create and left out of drift detection.

### Fixed

- `GCP::CertificateManager::CertificateMap` — groups the certificates a load
  balancer serves, selected per hostname by its entries. A target HTTPS proxy
  points at a map rather than a single certificate, which is how one proxy
  serves many domains.

- `GCP::CertificateManager::DnsAuthorization` — proves control of a domain.
  Creating one returns a CNAME to publish; only issuing a managed certificate
  waits on that record resolving, so the authorization itself is immediate.

- `GCP::CertificateManager::TrustConfig` — the certificate authorities a load
  balancer will accept client certificates from, for mutual TLS. It must carry
  at least one trust store or allowlisted certificate; Certificate Manager
  rejects an empty one at create. Note that it appends a trailing newline to
  every `pemCertificate` it stores, whatever was sent, so a PEM declared
  without one drifts on every re-apply.

- `GCP::Logging::LogBucket` — where log entries are actually retained. A sink
  routes entries into a bucket and a view is a window onto one, so this is what
  decides how long logs live and where.

  Deleting a bucket does not remove it: it enters `DELETE_REQUESTED` and stays
  for seven days so it can be undeleted, and a get answers 200 with that state
  rather than 404. The plugin reports a bucket in that state as gone, so an
  out-of-band delete leaves inventory and discovery does not offer buckets on
  their way out.

  `locked` is modelled so an existing locked bucket reads correctly, but note a
  locked bucket can never be deleted and locking cannot be undone. Every project
  also has `_Default` and `_Required` buckets created by GCP, so discovery
  reports two per project that nobody declared.

### Fixed
- `GCP::Storage::ManagedFolder` — an IAM boundary inside a bucket, letting a
  policy be attached to a prefix without granting it over the whole bucket.
  Requires uniform bucket-level access.
- `GCP::Storage::Folder` — a real directory node, available only in a bucket
  created with a hierarchical namespace. Renaming one moves everything beneath
  it, where a managed folder only governs who may read a prefix.
- `GCP::Storage::Bucket` models `iamConfiguration.uniformBucketLevelAccess` and
  `hierarchicalNamespace`. Neither folder type can exist without them, and
  hierarchical namespace is fixed at creation — a bucket is created flat or
  hierarchical and cannot convert.

### Fixed

- `GCP::Storage::Bucket`'s resolvable names properties the resource actually
  has. It pointed at `"Id"`, `"SelfLink"` and `"Name"` — capitalised, matching
  nothing — so `bkt.res.name` resolved to no value and **any resource
  referencing a bucket reached the plugin with the reference unresolved**. It
  went unnoticed because nothing in the repository referenced a bucket until the
  folder types did. `selfLink` is removed: the bucket has no such property.
- Storage names containing a slash survive a native-ID round trip. Both folder
  types are named with a **trailing slash that is part of the identity**
  ("reports/" is not "reports"), and the parser took a single path segment, so
  the slash was dropped and the rebuilt URL addressed a folder that does not
  exist. The name is now taken whole and escaped when addressed — a no-op for
  every pre-existing storage name, none of which contains a slash.

### Fixed
- `GCP::DNS::ResourceRecordSet` — what a managed zone actually serves: one name,
  one record type, and the data behind it. This completes Cloud DNS.

  It is the only resource in the plugin addressed by **two** path segments
  (`.../rrsets/{name}/{type}`). Both travel in the native ID joined by a slash,
  which is unambiguous because a DNS name may contain dots but never a slash,
  and both are fixed at creation — changing either is a different record set,
  not an edit.

  Every zone is born with an SOA and an NS record set nobody declared, so
  discovery reports two per zone as unmanaged.

- `GCP::Bigtable::AppProfile` — decides how an application's requests are routed
  across an instance's clusters. Every instance has a default profile; this is
  how a workload gets its own routing without affecting the rest.

### Fixed

- `schema/pkl/bigtable/materialized_view.pkl` describes the real API. It
  demanded a `cluster` — materialized views are instance-scoped — and omitted
  `query`, which is required, so the module was declarable and broken on
  contact. It still has no provisioner and remains a recorded parity gap: the
  create query needs Bigtable SQL semantics that could not be settled from CI
  logs.
- `BigtableProvisioner` routes `Status` through `base.StatusWithRead`. It
  embedded `*base.BaseResource` and overrode only `Create`, so it inherited the
  raw `Status`, which reports success and no properties. A completed async
  create therefore left the resource with nothing to read, and a reference to a
  Bigtable instance never resolved — a table declared alongside its instance
  failed with "instance is required for nested resources". Affects `Instance`,
  `Cluster` and `Table`.
- `BigtableProvisioner.Create` unwraps wrapped property values, as `base.Create`
  already did. Without it any property carrying a reference read as empty.
- An app-profile create is treated as synchronous. `appProfiles.create` answers
  with the resource, not an Operation, so polling looked for an operation id
  that was never there and asked the bare base URL, which answers 404.
- `GCP::Bigtable::Table` accepts a resolvable for `instance`. As a plain
  `String` a table could only ever name an instance that already existed, so it
  could not be declared in the same forma as its instance — which is why the
  type has no conformance case to this day.
- The Bigtable native-ID parser no longer switches on each collection by name.
  An unlisted collection parsed to an empty resource type and read nothing,
  silently; every instance-scoped collection now falls through one branch, with
  cluster-scoped backups the only special case.

- `GCP::DNS::Policy` — governs resolution for the VPC networks attached to it:
  inbound forwarding from an on-premises resolver, alternative name servers,
  and query logging. A policy attached to no network is valid and applies to
  nothing.
- `GCP::DNS::ResponsePolicy` — the container for rules that override resolution
  for its networks, the private-DNS equivalent of a hosts file.
- `GCP::DNS::ResponsePolicyRule` — one override, saying what a given DNS name
  resolves to. Discovered by walking the response policies, since discovery
  lists with no properties and Cloud DNS has no wildcard for that segment.

  Cloud DNS does not agree with itself about what an identifier is called: a
  managed zone and a policy use `name`, a response policy uses
  `responsePolicyName`, a rule uses `ruleName`. A forma declares `name` for all
  of them and the plugin translates at the API boundary, so the inconsistency
  stays inside the plugin. The rule collection is likewise `rules` in the URL
  but `responsePolicyRules` in a list response.

- `GCP::SQL::User` — a database user on a Cloud SQL instance. An instance ships
  with no usable login of its own, so this is what makes one reachable by an
  application. `password` is write-only and createOnly: the API never returns
  it, so it cannot reach stored state, and rotating it replaces the user.

  MySQL's `host` is deliberately not modelled: it is part of a user's identity
  rather than a property of it, and a `DeleteRequest` carries no properties for
  the plugin to read it back from, so supporting it means encoding it in the
  native ID. Users created without one get MySQL's default.
- `GCP::SQL::SslCert` — a client certificate for connecting over mutual TLS.
  Addressed by a server-generated `sha1Fingerprint` rather than by the
  `commonName` a forma declares, and the only sqladmin resource whose create
  answers with the resource itself rather than only an Operation. The private
  key is returned exactly once and is dropped rather than persisted: keeping it
  would put a private key in stored state and guarantee drift on every later
  read.
- `GCP::SQL::BackupRun` — one on-demand backup of an instance. Addressed by the
  numeric id sqladmin assigns, which arrives as `backupContext.backupId` on the
  create Operation. Unlike Spanner's and Bigtable's backups it takes no absolute
  expiry, so its fixture cannot rot.

  All three are discovered by walking the instances: discovery lists with no
  properties, so it can name no instance to look in, and sqladmin has no
  wildcard for them.

### Fixed

- A nested Cloud SQL resource no longer takes its native ID from the create
  Operation's `targetLink`. Every sqladmin mutation answers with an Operation
  whose `targetLink` names the **instance**, so a nested resource was stored
  under the instance's native ID — two resources sharing one id — and the next
  sync read the instance and reconciled the nested resource away as absent.
  This affected `GCP::SQL::Database`, which has been registered for some time
  but had no conformance case and so had never exercised the path;
  `testdata/cloudsql-database.pkl` now covers it.
- A deleted `GCP::SQL::BackupRun` is now treated as gone. Cloud SQL does not
  remove a deleted backup run — the record survives as a tombstone and a get
  answers 200 with `status: "DELETED"` rather than 404 — so a backup deleted
  outside formae was reported as still present and never left inventory.
  Discovery also no longer offers long-dead backups as unmanaged resources to
  import.
- Cloud SQL now retries a 409 "another operation was already in progress"
  instead of failing the resource. Operations are serialised per instance and
  every nested type shares its instance's queue, so a mutation issued while
  another is still running is ordinary contention rather than a fault. It joins
  the existing "database is being accessed by other users" case.
- `GCP::SQL::Database` is now discoverable. It had no parent-walking `List`, so
  discovery — which lists with no properties — asked a collection URL with no
  instance in it and found nothing. Every instance-scoped Cloud SQL type now
  shares one walker, since the only thing that differed between them was how an
  item names itself: users and databases by `name`, a certificate by
  `sha1Fingerprint`, a backup run by its server-assigned `id`.

### Added

- `GCP::Spanner::Instance` — the compute and storage a Spanner deployment runs
  on. Project-scoped rather than location-scoped: an instance's region is its
  `config`, not a path segment, and is fixed at creation. `config` is written as
  the bare instance-config id (`regional-europe-central2`) and qualified by the
  plugin, so a forma carries no project id and stays portable between targets.
- `GCP::Spanner::Database` — a database on an instance. Spanner has no `name`
  field on create; the id goes into a `CREATE DATABASE` statement, quoted with
  backticks for GoogleSQL and double quotes for PostgreSQL.
- `GCP::Spanner::BackupSchedule` — a recurring backup of one database, which is
  what extends a database beyond its point-in-time retention window. Sits two
  collections deep and, unlike instances and databases, is synchronous.

  Spanner rejects a wildcard for both nested collections —
  `instances/-/databases` and `databases/-/backupSchedules` answer 400 "Invalid
  List... request" — so databases and schedules are discovered by walking the
  collections above them, following `nextPageToken` at each level.

  Note that Spanner creates a `default_daily_full_backup_schedule` alongside
  every database, so discovery reports one nobody declared, and that
  `clean-environment.sh` now sweeps Spanner instances: they are billed by the
  hour, and the database and backup-schedule fixtures each leave one behind.
- `GCP::ServiceDirectory::Namespace` — the top-level container of a Service
  Directory registry. Location-scoped and free; deleting one deletes every
  service and endpoint under it.
- `GCP::ServiceDirectory::Service` — a named service inside a namespace,
  carrying the annotations clients read when they resolve it. Its `namespace`
  is a path component rather than a body field, so it is dropped from the
  request and lifted back out of the returned path.
- `GCP::ServiceDirectory::Endpoint` — one address and port a service answers
  on, and what a resolve call actually returns. It sits two collections deep
  (`namespaces/{ns}/services/{svc}/endpoints/{ep}`).

  Service Directory rejects a wildcard at every level — `locations/-` answers
  "Unsupported location" and `namespaces/-` "Could not parse namespace name" —
  so services and endpoints are discovered by walking the collections above
  them, following `nextPageToken` at each level. A dropped namespace would
  otherwise hide every service and endpoint under it.

### Changed

- `base.ParentResourceConfig` gained `GrandParentType` /
  `GrandParentPropertyName`, for APIs three collections deep. Read, update and
  delete rebuild the whole path from the native ID, so create was the one
  operation with nothing but the declared properties to route with; it now
  carries the grandparent in `PathContext.CustomSegments[0]`.

### Removed

- `GCP::Compute::DiskAsyncReplication` is withdrawn. It never shipped in a stable
  release, only in 0.1.13-dev.1 and -dev.2. Its two properties are the disks the
  pair joins: both immutable, and both declared as references to the disk
  resources, which is how a forma names them. An extracted forma writes such a
  reference unresolved, so the re-apply compares a reference against the URL in
  state on an immutable path and plans a replacement of the pair already in
  place. Suppressing that comparison fixes the lifecycle and makes the pair
  undiscoverable, because the agent requires both fields on a resource it
  persists; reporting them keeps discovery and brings the replacement back. No
  plugin-side shape satisfies both, so the type is out until the agent can
  resolve a reference at plan time. The work, the four conformance runs behind
  that conclusion, and the two agent-side asks are in the draft PR.

### Fixed

- A `GCP::Compute::Disk` no longer plans a delete-and-recreate of itself. Compute
  reports int64 fields as JSON strings, so `physicalBlockSizeBytes` came back as
  `"4096"` against a declared `4096`; the field is createOnly, so the two forms
  compared unequal and every re-apply planned a replacement. It now carries the
  same `toString` transformation `sizeGb` already had.
- A `GCP::SQL::DatabaseInstance` no longer plans an endless
  `replace /settings/dataDiskSizeGb` against itself. The provisioner parses Cloud
  SQL's string form back to a number when it reads the instance, while the schema
  stringified the declared number on the way out, so the two normalisations
  pointed at each other: state held `10` and the desired side produced `"10"`.
  The field is typed `Int`, so the plugin's parse is the half to keep.
- A `GCP::Filestore::Instance` no longer plans a second copy of its own file
  share. Filestore reports `capacityGb` as a JSON string, so state held `"1024"`
  against a declared `1024`; because a share is a list element, the mismatch did
  not read as one changed field - the declared share matched nothing in state
  and the plan appended it. The declared value is now sent as a string, as
  Compute's int64 fields already are.

### Fixed

- An operation-status poll that cannot be read no longer fails the operation it
  was polling, whatever the reason. The earlier fix listed the transport errors
  it would tolerate, which left an unclassified error (`Unknown`) still failing
  the operation - and a burst of those against compute.googleapis.com turned
  into eleven red conformance jobs in one run, each reporting "failed to get
  operation status" for an operation that had not failed. Only a definitive
  answer - not found, denied, bad request - now ends the poll.
- A Monitoring resource is no longer discovered a second time as an unmanaged
  copy of itself. Monitoring answers `dashboards.create` with
  `projects/{project_number}/...` and `dashboards.list` with
  `projects/{project_id}/...` for the same dashboard, so a native ID taken
  verbatim depended on which call produced it and the managed resource never
  correlated with the discovered one. The project segment is now normalised to
  the configured project.

### Fixed

- A failed operation-status poll no longer fails the operation it was polling. A
  transient transport error (network, timeout, throttling, 5xx) says nothing
  about the operation, but reporting it as a failure made the caller re-issue
  the whole create - which then collided with what the first attempt had already
  built. One network blip while polling an AlloyDB instance create surfaced as
  `PRIMARY_ALREADY_EXISTS`. A definitive answer (not found, denied, bad request)
  still fails.

### Fixed

- `GCP::Dataproc::WorkflowTemplate` is now discoverable. Its list response is
  keyed `templates`, which matches neither `items` nor the collection name, so
  the parser found nothing and the type was never discovered.

### Fixed

- `GCP::AlloyDB::Instance` and `GCP::AlloyDB::User` are now discoverable. Both
  live under a cluster and discovery names none, so each listed a path with an
  empty cluster segment and got a 404. Instances now use the API's `clusters/-`
  wildcard; users.list rejects that wildcard, so that one walks the clusters.

### Fixed

- An update of a resource that uses optimistic locking now reports the updated
  properties. That path returned no `ResourceProperties` at all, so whatever the
  update changed was missing from state until some later sync happened to pick it
  up - a `labels` change on a Dataproc workflow template read as never applied.
- `Update` now unwraps `formae.Value` wrappers the way `Create` already did.

### Fixed

- Sub-resources that live inside a parent object are now discoverable. A policy
  rule, a firewall policy association, a router interface, named set or route
  policy and a log view have no collection URL of their own, and discovery lists
  with no properties, so each `List` returned nothing unless the caller named the
  parent. They now walk the parents first (`securityPolicies`, `firewallPolicies`,
  `routers`, and Logging buckets via the `locations/-` wildcard).
- `GCP::Logging::LogView`'s List override is now installed from the package's
  init rather than its own `init()`. Go runs init functions in filename order and
  `log_view_list.go` sorts before `resources.go`, so the generic registration
  silently replaced it and discovery listed a path that 404s.

### Fixed

- `GCP::Compute::BackendServiceSignedUrlKey` and `GCP::Compute::VpnTunnel` are
  now discoverable. Both declared a *required* write-only field (`keyValue`,
  `sharedSecret`) that the API never reports, so a discovered instance could
  never satisfy the schema and was silently dropped from inventory. Both fields
  are now optional; the create path still rejects a missing value.

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

  Conformance green on all eight steps, with a fixture. (It had none for a while:
  `match` went missing from state after create, which looked like a formae bug.
  It was the missing `Status` read-back described under *Fixed* below.)
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

  Conformance green on all eight steps. (It was initially red on Verify, Extract
  and Update over a missing `match.config`; that turned out to be the missing
  `Status` read-back described under *Fixed* below, not anything about this
  resource.)

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

- `GCP::Compute::RegionSecurityPolicyRule` — the regional twin of the Cloud Armor
  rule above. The four verbs are identical, only the policy sits under
  `regions/{region}`, so `policyRuleKind` gained a `regional` flag: it picks the
  scope segment, makes `region` a path component that never travels in the rule
  body, and passes the region to the operation poll so it hits
  `regions/{r}/operations`. Three kinds now share the one provisioner.

  Conformance green on all eight steps, once the `Status` read-back below was in
  place. All four verbs were probed directly, and a direct `Create` against a
  live regional policy returns the right composite native ID.

- `GCP::Compute::NetworkEndpointGroup` gains `pscTargetService` and the
  `PRIVATE_SERVICE_CONNECT` endpoint type, so the group can front a Google API
  or a published service attachment rather than only a Cloud Run service. This
  is what Terraform calls `google_compute_region_network_endpoint_group` — not a
  separate resource here, since this one is already regional. The API rejects
  `network` and `subnetwork` for a PSC group in regional scope, so it needs no
  VPC: `testdata/network-endpoint-group-psc.pkl` is the only conformance case in
  the suite with no prerequisites at all, and runs in under 90 seconds.

- `GCP::Compute::RegionDiskResourcePolicyAttachment` — binds a snapshot schedule
  to a regional disk, the regional twin of the zonal attachment. Same two verbs
  (`addResourcePolicies` / `removeResourcePolicies`), so
  `DiskResourcePolicyAttachmentProvisioner` gained a `regional` flag rather than
  a second copy: it picks the scope segment, swaps the `zone` property for
  `region`, and puts the region into the operation poll. Conformance green on
  all eight steps.

  `clean-environment.sh` detached policies with `--zone` only, so a regional
  attachment would have pinned its policy forever; the detach pass now uses
  whichever of zone/region the disk actually reports.

- `GCP::Compute::NetworkFirewallPolicyAssociation` — attaches a network firewall
  policy to a VPC network, which is what puts the policy in the data path: a
  policy with rules but no association is inert. Like the rules it is a set of
  verbs on the policy (`addAssociation`, `getAssociation?name=N`,
  `removeAssociation?name=N`), so it is a hand-written provisioner, and a removed
  association answers `getAssociation` with **400, not 404**, so not-found is
  mapped explicitly. Nothing is updatable — an association is a
  (policy, network) pair — so a change replaces it. Conformance green on all
  eight steps.

  `clean-environment.sh` now detaches associations before deleting policies: an
  association pins both its policy and its network, so a killed run used to
  leave all three behind.

- `GCP::Compute::RegionNetworkFirewallPolicyAssociation` — the regional twin of
  the association above, for a `RegionNetworkFirewallPolicy`. The three verbs are
  identical, only the policy sits under `regions/{region}` (the network itself
  stays global), so `FirewallPolicyAssociationProvisioner` gained a `regional`
  flag rather than a second copy. Conformance green on all eight steps, and the
  cleanup script detaches regional associations too.

- `GCP::Compute::RegionCompositeHealthCheck` — completes the health-aggregation
  trio: `RegionHealthAggregationPolicy` decides what "healthy" means,
  `RegionHealthSource` applies that to a backend service, and this reports the
  verdict where a load balancer can act on it. The other two were inert without
  it. `healthDestination` must be a **forwarding rule** — the API rejects a
  backend service with "Unexpected resource collection" — so the fixture builds
  the full internal-load-balancer chain (network, subnet, health check, backend
  service, forwarding rule) without booting a single VM.

  Unlike its two siblings, update is supported, and it needs the fingerprint.
  The API hides that: a patch without one returns **200 with a normal
  operation**, and the *operation* then fails with 412 CONDITION_NOT_MET
  ("missing fingerprint"). Registered with optimistic locking on `fingerprint`;
  conformance green on all eight steps, Update included.

- `GCP::Compute::BackendServiceSignedUrlKey` — a Cloud CDN signed-URL key on a
  backend service. Without a key no signed URL can be issued, so this is what
  makes `enableCDN` usable for private content. Added and removed with the
  `addSignedUrlKey` / `deleteSignedUrlKey` verbs, so it is a hand-written
  provisioner.

  The secret is write-only in the strongest sense: the API reports key *names*
  only, under `cdnPolicy.signedUrlKeyNames`, and omits the block entirely when a
  service has no keys. `Read` therefore reports presence rather than value —
  which is all drift detection can check — and `keyValue` accepts a wrapped
  `formae.Value` so it stays out of plans and state. Nothing is updatable:
  rotating a key removes it and adds the new value. Conformance green on all
  eight steps.

- `GCP::Compute::RouterRoutePolicy` — a BGP route policy on a Cloud Router,
  filtering or rewriting the routes it imports from, or advertises to, its peers.
  Without one a router takes and gives every route as-is. Verb-based
  (`updateRoutePolicy`, which both creates and updates, plus `getRoutePolicy`,
  `deleteRoutePolicy`, `listRoutePolicies`), so it is a hand-written provisioner.
  Three API quirks: `getRoutePolicy` wraps the policy in a `resource` envelope,
  `listRoutePolicies` returns `result` rather than the usual `items`, and an
  update must carry the current fingerprint while a create must not carry one at
  all — so `Update` re-reads the policy for a fresh fingerprint instead of
  trusting the declared forma, where it would go stale after the first change.
  A removed policy answers 400 ("The policy does not exist"), not 404.
  Conformance green on all eight steps, Update included.

- `GCP::Compute::RouterNamedSet` — a reusable list of prefixes on a Cloud
  Router. A route policy term can match against a named set instead of spelling
  out every prefix, so one edit here changes every policy referencing it.

  Same shape as the route policy down to the quirks — the update verb also
  creates, the get verb wraps its payload in a `resource` envelope, the list verb
  returns `result` rather than `items`, and an update needs the current
  fingerprint while a create must not carry one — so
  `RouterRoutePolicyProvisioner` became `RouterSubResourceProvisioner`,
  parameterised by a `routerSubKind` holding the four verb names, the query
  parameter and the native-ID segment. The verbs cannot be derived from one
  noun (`listRoutePolicies` pluralises, `listNamedSets` does not), so a unit
  test pins all six strings per kind. `NAMED_SET_TYPE_PREFIX` must be spelled in
  full — `PREFIX` is rejected — and a prefix element carries its own quotes
  (`'10.0.0.0/8'`). A missing set answers 404 `NAMED_SET_NOT_FOUND` where a
  missing policy answers 400 `does not exist`, so both spellings count as gone.
  Conformance green on all eight steps for both kinds, Update included.

- `GCP::Compute::ProjectMetadataItem` — one key of the project's common instance
  metadata: the project-wide defaults every VM inherits, such as
  `enable-oslogin`, `ssh-keys` or a default `startup-script`. There was no way to
  manage any of that before.

  This is **shared, project-wide state**, and the API has no per-key operation:
  `setCommonInstanceMetadata` replaces the whole list. So every write is
  read-modify-write that touches one key and copies every other key verbatim,
  and the merge carries the resource's whole safety story — it is unit-tested
  against foreign keys, in-place overwrite, absent-key removal, a project with no
  metadata, and junk entries (keyless items, non-maps, keys with no value). A
  stale fingerprint is rejected by the API rather than silently overwriting, so a
  concurrent editor causes one retry, never a lost key; `writeMetadataItem`
  retries once on that error.

  Declare one resource per key — modelling the whole map would have two
  declarations fighting over one list. `List` reports every key, so undeclared
  ones surface as unmanaged, which is honest: they are real project settings.
  Conformance green on all eight steps, and the project's pre-existing
  `enable-oslogin` was verified intact afterwards. The cleanup script removes
  only `formae-plugin-sdk`-prefixed keys.

- `GCP::Compute::RouterInterface` — one entry of `Router.interfaces[]`, where a
  Cloud Router attaches to whatever it peers over. A BGP peer is configured
  against an interface, so this is the first half of making a router speak BGP,
  and `Router` did not model interfaces at all.

  Like Cloud NAT it lives inside the router and is managed by read-modify-write
  through routers.patch, so sibling interfaces — and the router's NATs and BGP
  peers — survive every operation. The merge is unit-tested against sibling
  preservation, in-place overwrite, absent-key removal, an empty router and junk
  entries.

  Every field is createOnly: the API rejects an in-place change ("the following
  field(s) specified in the router interface cannot be updated"), so `Update`
  reports not-updatable and formae replaces instead. Attaching by `subnetwork` +
  `privateIpAddress` — a router appliance interface — needs no VPN tunnel, so the
  fixture sidesteps the exhausted per-region VPN gateway quota entirely and boots
  no VMs. Conformance green on all eight steps, with Replace exercising the
  delete-then-create path.

- `GCP::Compute::DiskAsyncReplication` — the replication link between a primary
  disk and a secondary in another region, which is what cross-region disk
  disaster recovery is. The disks are ordinary `Disk` resources; this models the
  relationship, started and stopped with the startAsyncReplication /
  stopAsyncReplication verbs on the primary.

  The subtlety that shapes the whole implementation: **stopping replication does
  not clear `asyncPrimaryDisk` from the secondary.** Only
  `resourceStatus.asyncPrimaryDisk.state` changes, ACTIVE to STOPPED, so a read
  that keyed on the field being present would report a dead pair as live
  forever. `Read` judges by state, and also refuses a secondary that has been
  re-paired with a different primary. `stopAsyncReplication` is idempotent, so
  deleting twice is not an error. Nothing is updatable — the link is a pair.

  `Disk.asyncPrimaryDisk.disk` was typed `String`, which made it impossible to
  reference the primary through a resolvable; it now accepts one, so formae
  orders the creates. That matters more than convenience here: a secondary
  cannot be paired after creation, so the reference has to be right the first
  time. A disk in active replication also cannot be deleted, so the cleanup
  script stops replication before its disk passes run. Conformance green on all
  eight steps.

- `GCP::ArtifactRegistry::Rule` — a rule gates an operation on its repository
  (denying downloads, for instance). A repository without rules allows whatever
  the caller's IAM permits, so this is how one enforces policy of its own. It is
  the first parented resource in this package, and config-driven rather than
  hand-written, which took three fixes to the generic Artifact Registry plumbing:

  - the path builder now inserts `repositories/{repo}` when a resource is
    nested, so a rule lands on `.../repositories/{repo}/rules/{rule}`;
  - the native-ID extractor keeps that parent segment, since a read URL is
    rebuilt from the id and would otherwise address the location-level
    collection; and
  - `ArtifactRegistryNativeID` gained a `Parser` that restores
    ParentType/ParentResource from a nested id.

  A request transformer drops `repository` and `location` (path components the
  API rejects as body fields) while keeping `name`, which the engine reads to
  fill `?ruleId=`; a response transformer recovers `repository` and `location`
  from the returned path. Rules are synchronous, unlike repositories, so the
  definition carries its own `OperationConfig`. Note the API allows only **one
  DOWNLOAD rule per repository**. Conformance green on all eight steps, Update
  included.

- `GCP::Eventarc::MessageBus` — the hub of an Eventarc Advanced setup, where
  publishers send events and enrollments and pipelines route them onward. A
  `Trigger` wires one source to one destination; a bus is the fan-out point
  between many, and it supports PATCH where Trigger does not.

  **Eventarc Advanced is not available in every region.** `europe-central2` —
  this project's `GCP_LOCATION` — is rejected outright with "region ... is not
  supported in Eventarc Advanced", so the resource declares its own `location`
  and the fixture pins `europe-west1`. A request transformer keeps `location` out
  of the body (it addresses the URL) while keeping `name` for `?messageBusId=`,
  and a response transformer recovers `location` from the returned path so a
  declared location is not reported missing. Because the fixture's region differs
  from `GCP_LOCATION`, the cleanup script names the Advanced regions explicitly.
  Conformance green on all eight steps, Update included.

- `GCP::Eventarc::Pipeline` — where a message bus sends the events it accepts. An
  enrollment decides which events reach a pipeline; the pipeline says where they
  go and how hard to retry.

  Two API facts govern the fixture. First, **only one message bus is allowed per
  project per region** (`MessageBusesPerProjectPerRegion`, limit 1), so the
  pipeline case pins `us-central1` while `eventarc-message-bus` keeps
  `europe-west1` — otherwise whichever ran first would hold the region's only
  slot and fail the other. Second, an `httpEndpoint` destination needs a real
  `networkAttachment`, and a bogus one fails the create *asynchronously*; a
  `messageBus` destination needs nothing but the bus, so that is what the fixture
  uses.

  A forma passes `bus.res.name`, so formae orders the creates; the request
  transformer expands that short name into the full path Eventarc wants and the
  response transformer shortens it back. That symmetry is the point — expanding
  on write without shortening on read made all four comparison steps report drift
  on a pipeline that was in fact correct.

  This resource is slow: roughly four minutes to create and two and a half to
  delete, so the conformance case takes about 25 minutes and needs
  `TIMEOUT=30`. The API also refuses a PATCH while creation is still running.
  Conformance green on all eight steps, Update included.

- `GCP::Compute::TargetVpnGateway` — the classic, route-based VPN gateway.
  `HaVpnGateway` is the modern alternative with an SLA and BGP; this one
  terminates policy- and route-based tunnels and still backs plenty of existing
  setups.

  It is a different collection (`targetVpnGateways`, not `vpnGateways`) drawing
  on a **separate quota**, which is why this case passes in a project whose
  `VPN_GATEWAYS_PER_REGION` is exhausted — the condition that still keeps
  `vpn-tunnel` red. Immutable: PATCH is not a method on it at all (an attempt
  answers with an HTML 404), so any change replaces. Conformance green on all
  eight steps; the cleanup script gained a target-VPN-gateway pass, ordered before
  the network passes since a gateway holds its network.

- `GCP::Workflows::Workflow` — a workflow definition: the YAML program Workflows
  runs to orchestrate calls to other services. **New service namespace** — the
  plugin had no Workflows coverage at all — so this adds
  `pkg/resources/workflows` (API config, LRO operations, native-ID parser) and
  `schema/pkl/workflows`.

  Defining a workflow costs nothing; only executions are billed, and nothing
  executes this one. Editing `sourceContents` produces a new revision rather than
  mutating the old one, which `revisionId` reports, and `serviceAccount` is
  filled in with the project's default compute service account when unset.
  Conformance green on all eight steps, Update included, in under two minutes.

- `GCP::Dataproc::SessionTemplate` — a reusable configuration for a serverless
  Spark session: runtime version, session kind, and the environment a session
  inherits. The template runs nothing and costs nothing; only sessions started
  from it are billed.

  Two API shapes made this more than a copy of the autoscaling policy. Dataproc
  is **split by scope**: session templates live under `locations/{location}` while
  autoscaling policies live under `regions/{region}`, so this definition carries
  its own `APIConfig` and native-ID parser. And there is **no
  `?sessionTemplateId=` parameter** — the API rejects it as an unbindable query
  parameter and takes the id from the body's `name` as a full path — so a request
  transformer expands the declared short name and a response transformer shortens
  it back, keeping declared and observed values comparable. Conformance green on
  all eight steps in 16 seconds, the fastest case in the suite.

- `GCP::Dataproc::WorkflowTemplate` — a Spark job graph plus the cluster to run it
  on. Instantiating one creates a cluster, runs the jobs in dependency order and
  tears it down; defining the template runs nothing and costs nothing.
  Region-scoped, so it shares the existing path builder.

  Two conventions of its own: the create body carries `id` rather than a
  `?workflowTemplateId=` parameter, and an update is a **PUT** that must carry the
  template's current `version`, supplied through optimistic locking. Step ids must
  match `[a-zA-Z0-9][-_a-zA-Z0-9]{1,48}[a-zA-Z0-9]` — a two-character id like
  "s1" is rejected.

  **Conformance is 7/8: Update is red on `labels`.** Create, Verify, Extract,
  Sync, Destroy and out-of-band delete all pass. What is established: the update
  itself works — driving the engine's `Update` directly applies the labels and a
  following read returns them — and the base fix below now attaches the update
  response to stored state, which did not change the outcome. The remaining
  cause is not yet identified, so this is shipped working but with Update
  unverified end to end.

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

- **More resources could be managed but never discovered.** Discovery calls `List`
  with no hints, and several resources answered with an empty list or the wrong
  location:
  - `ArtifactRegistry::Rule` — a rule lives under a repository and Artifact
    Registry has no wildcard for either segment (`repositories/-` answers
    "Repository does not exist"), so listing now walks the repositories first.
    Registered as a List-only override, leaving the generic create, read, update
    and delete in place.
  - `Compute::BackendServiceSignedUrlKey` — now reads the aggregated
    backend-service list, one call carrying every service's `cdnPolicy`, rather
    than requiring a service to be named.
  - `Compute::DiskAsyncReplication` — now reports every pair whose replication is
    ACTIVE. Stopped pairs stay absent deliberately: `Read` treats them as gone,
    so listing them would produce ids that immediately read as not-found.
  - `Eventarc::MessageBus` / `Pipeline` — Advanced runs in a subset of regions, so
    a forma pins one that is rarely the target's, and discovery looked only in the
    target's. `PathContext` gained an `IsList` marker so a path builder can tell a
    collection URL built for listing from one built for create or read; the
    Eventarc builder uses `locations/-` for the Advanced collections when listing.

- **A synchronous update stored properties the read path never returns.** The
  previous fix echoed the update response into state, but some APIs echo fields
  their GET omits — a Storage bucket's PATCH returns `defaultObjectAcl`, which
  made conformance report a property "not expected and not a provider default".
  The update now reads the resource back instead, so state after an update has
  exactly the shape it has after a create or a sync.


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

- **A synchronous resource's update left stale properties in state.**
  `handleSynchronousCreate` attaches the create response as
  `ProgressResult.ResourceProperties`, but the synchronous branch of
  `performUpdate` attached nothing: with no operation to poll there is no
  read-back either, so every changed field stayed at its pre-update value until
  the next sync. `performUpdate` now marshals the update response the same way
  Create does, through the response transformer. Verified no regression by
  re-running `dataproc-sessiontemplate` and `artifactregistry-rule` (both
  synchronous with updates): 8/8 each.

- **Optimistic locking only understood string etags.** The locking value was read
  with `Body[field].(string)`, so a numeric etag — Dataproc's
  `workflowTemplates.version` — silently became `""` and the API rejected the
  update. The raw value now goes into the request body with its own type
  preserved, and a new `lockingValueString` renders it for the query-parameter
  case; unit-tested across string, float64, int, int64, `json.Number`, absent and
  junk inputs.

- `GCP::Compute::RouterNat` now returns the NAT's properties from `Status` after
  a successful operation. It has a bespoke `Status` (its RequestID carries the
  synthetic native ID), so it missed the read-back that `base.StatusWithRead`
  adds elsewhere, meaning a NAT's nested `logConfig` and `subnetworks` would be
  absent from state until the next sync. Unit-tested only: `RouterNat` has no
  conformance fixture, so this is unverified end to end.


- **Hand-written provisioners lost structured properties from post-create state.**
  `base.UnifiedProvisioner.Status` re-reads a resource after a successful
  operation and returns the result in `ProgressResult.ResourceProperties`; a
  provisioner that overrode CRUD and delegated `Status` straight to
  `BaseResource` skipped that read-back. Scalars survived from the declared
  forma, nested objects and arrays did not, and only the next sync filled them
  in — so `SecurityPolicyRule`, `RegionSecurityPolicyRule`,
  `NetworkFirewallPolicyRule` and `RouterRoutePolicy` all failed Verify, Extract
  and Update while passing Sync. That pattern looked like a formae bug and was
  recorded as one; it was ours.

  New `base.StatusWithRead` does the read-back, and all seven hand-written
  provisioners route `Status` through it. Re-running the affected fixtures
  unchanged apart from the fix took each from three failing steps to 8/8.
  **Any new provisioner that overrides CRUD must override `Status` too and route
  it through `StatusWithRead`.**



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
