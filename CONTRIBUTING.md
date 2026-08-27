# Contributing

This document covers local development for plugin authors. For user-facing
plugin docs (configuration, supported resources, examples), see
[README.md](README.md).

## Prerequisites

- Go 1.25+
- [Pkl CLI](https://pkl-lang.org/main/current/pkl-cli/index.html)
- A GCP project with the relevant APIs enabled (for integration/conformance testing)

## Local Installation

```bash
make install
```

## Building

```bash
make build      # Build plugin binary
make test       # Run unit tests
make lint       # Run linter (requires golangci-lint)
make install    # Build + install locally for testing
```

## Local Testing

```bash
# Copy .env.example and configure your credentials
cp .env.example .env

# Load environment variables
export $(cat .env | xargs)

# Install plugin and schemas locally
make install

# Start formae agent (discovers the plugin)
formae agent start

# Apply example resources
formae apply examples/network.pkl
```

## Test-Time Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `GCP_PROJECT_ID` | GCP project ID | Yes |
| `GCP_PROJECT_NUMBER` | GCP project number | Yes |
| `GCP_REGION` | GCP region (e.g., `europe-central2`) | Yes |
| `GCP_ZONE` | GCP zone (e.g., `europe-central2-b`) | Yes |
| `GCP_CREDENTIALS_FILE` | Path to service account JSON key | No* |
| `GCP_CREDENTIALS_JSON` | Inline service account JSON key | No* |

\*One of `GCP_CREDENTIALS_FILE` or `GCP_CREDENTIALS_JSON` is required for
local development. In CI with Workload Identity Federation, leave both unset
to use Application Default Credentials (ADC).

## Conformance Testing

Run the full conformance test suite (CRUD lifecycle + discovery):

```bash
make conformance-test                  # Latest formae version
make conformance-test VERSION=0.84.0   # Specific version
```

The conformance harness:

1. Calls `setup-credentials` to provision cloud credentials
2. Calls `clean-environment` to remove orphaned resources from previous runs
3. Builds and installs the plugin locally
4. Downloads the requested formae version (defaults to latest)
5. Runs CRUD lifecycle tests for each resource type
6. Runs discovery tests to verify resource detection
7. Calls `clean-environment` again to clean up test resources

### CI Hooks

#### `scripts/ci/setup-credentials.sh`

Provisions credentials for the cloud provider. Called before running
conformance tests. For GCP this means verifying `gcloud auth` or workload
identity.

#### `scripts/ci/clean-environment.sh`

Cleans up test resources before AND after conformance tests:
- Pre-cleanup removes orphaned resources from previous failed runs
- Post-cleanup removes resources created during the test run

The script must be idempotent and delete all resources under the `testdata/`
namespace prefix.

### GitHub Actions

Two workflows run conformance against real GCP, and both authenticate through
Workload Identity Federation (see
[docs/gcp-github-actions-setup.md](docs/gcp-github-actions-setup.md) and the
secrets below):

- **`ci.yml`** runs the whole matrix on push to `main` and on manual dispatch.
  It does **not** run on pull requests: a full matrix takes 80-100 minutes, and
  every conformance workflow here shares the `gcp-conformance-tests`
  serialization group, so a run queued behind another is usually evicted before
  it starts. Pull requests are gated on the fast checks (build, lint, unit
  tests, manifest, schema).
- **`debug-conformance.yml`** runs only the test cases you name, against the ref
  you dispatch it on. This is how you validate a resource change on your branch
  before opening the pull request:

  ```bash
  gh workflow run debug-conformance.yml --ref <your-branch> \
    -f test_cases=bucket,disk
  ```

#### Required GitHub Secrets

| Secret | Description |
|--------|-------------|
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | Full WIF provider path |
| `GCP_SERVICE_ACCOUNT` | Service account email |
| `GCP_PROJECT_ID` | GCP project ID |
| `GCP_PROJECT_NUMBER` | GCP project number |
| `GCP_REGION` | GCP region |
| `GCP_ZONE` | GCP zone |

For detailed WIF setup instructions, see
[docs/gcp-workload-identity-federation-github-actions.md](docs/gcp-workload-identity-federation-github-actions.md).
