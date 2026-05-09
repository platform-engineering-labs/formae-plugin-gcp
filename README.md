# GCP Plugin for Formae

[![CI](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/ci.yml)
[![Nightly](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/nightly.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/nightly.yml)

Google Cloud Platform resource plugin for
[formae](https://github.com/platform-engineering-labs/formae). Manage GCP
infrastructure declaratively across Compute, GKE, CloudRun, BigQuery,
Bigtable, Cloud SQL, and Cloud Storage.

## Supported Resources

This plugin supports **47 GCP resource types** across 8 services.

| Service | Resources | Examples |
|---------|-----------|----------|
| Compute | 27 | Network, Subnetwork, Firewall, Instance, Disk, Address, Router, BackendService, ForwardingRule, HealthCheck, UrlMap, TargetHttp(s)Proxy |
| Cloud Run | 6 | Service, Job, Revision, Execution, Task, WorkerPool |
| Bigtable | 5 | Instance, Cluster, Table, Backup, MaterializedView |
| Cloud Storage | 5 | Bucket, BucketAccessControl, ObjectAccessControl, DefaultObjectAccessControl, AnywhereCache |
| BigQuery | 2 | Dataset, Table |
| GKE (Container) | 2 | Cluster, NodePool |
| GKE Hub | 2 | Membership, Feature |
| Cloud SQL | 1 | DatabaseInstance |

See [`schema/pkl/`](schema/pkl/) for the complete list of resource types and
field definitions.

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

Copyright 2026 Platform Engineering Labs Inc.
