# GCP Plugin for Formae

[![CI](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/ci.yml)
[![Nightly](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/nightly.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-gcp/actions/workflows/nightly.yml)

Formae plugin for managing Google Cloud Platform resources.

> **Note:** Don't use GitHub's "Use this template" button. Instead, use the Formae CLI
> which will prompt for your plugin details and set everything up correctly:
>
> ```bash
> formae plugin init my-plugin
> ```

## Quick Start

1. **Create plugin**: `formae plugin init <name>` (prompts for namespace, license, etc.)
2. **Define resources** in `schema/pkl/*.pkl`
3. **Implement CRUD operations** in `plugin.go`
4. **Build and test**: `make build && make test`

## Project Structure

```
.
├── formae-plugin.pkl      # Plugin manifest (name, version, namespace)
├── plugin.go              # Your ResourcePlugin implementation
├── main.go                # Entry point (don't modify)
├── schema/pkl/            # Pkl resource schemas
│   ├── PklProject
│   └── example.pkl
├── examples/              # Usage examples
├── scripts/
│   ├── ci/                # CI hook scripts
│   │   ├── setup-credentials.sh
│   │   └── clean-environment.sh
│   └── run-conformance-tests.sh
├── go.mod
├── Makefile
└── README.md
```

## What You Implement

You only implement the `ResourcePlugin` interface in `plugin.go`:

```go
type Plugin struct{}

// Configuration
func (p *Plugin) RateLimit() plugin.RateLimitConfig { ... }
func (p *Plugin) DiscoveryFilters() []plugin.MatchFilter { ... }
func (p *Plugin) LabelConfig() plugin.LabelConfig { ... }

// CRUD Operations
func (p *Plugin) Create(ctx, req) (*CreateResult, error) { ... }
func (p *Plugin) Read(ctx, req) (*ReadResult, error) { ... }
func (p *Plugin) Update(ctx, req) (*UpdateResult, error) { ... }
func (p *Plugin) Delete(ctx, req) (*DeleteResult, error) { ... }
func (p *Plugin) Status(ctx, req) (*StatusResult, error) { ... }
func (p *Plugin) List(ctx, req) (*ListResult, error) { ... }
```

**The SDK handles everything else:**
- Plugin identity (name, version, namespace) → read from `formae-plugin.pkl`
- Schema extraction → auto-discovered from `schema/pkl/`
- Resource descriptors → generated from Pkl schemas

## Development

### Prerequisites

- Go 1.25+
- [Pkl CLI](https://pkl-lang.org/main/current/pkl-cli/index.html)

### Building

```bash
make build      # Build plugin binary
make test       # Run unit tests
make lint       # Run linter (requires golangci-lint)
make install    # Build + install locally for testing
```

### Local Testing

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
formae apply examples/basic/main.pkl
```

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `GCP_PROJECT_ID` | GCP project ID | Yes |
| `GCP_PROJECT_NUMBER` | GCP project number | Yes |
| `GCP_REGION` | GCP region (e.g., `europe-central2`) | Yes |
| `GCP_ZONE` | GCP zone (e.g., `europe-central2-b`) | Yes |
| `GCP_CREDENTIALS_FILE` | Path to service account JSON key | No* |
| `GCP_CREDENTIALS_JSON` | Inline service account JSON key | No* |

*One of `GCP_CREDENTIALS_FILE` or `GCP_CREDENTIALS_JSON` is required for local development. In CI with Workload Identity Federation, leave both unset to use Application Default Credentials (ADC).

#### Credentials Handling

**Important:** Credentials are read from environment variables, NOT from the target configuration. This prevents sensitive credential data from being stored in the formae database.

Priority order:
1. `GCP_CREDENTIALS_JSON` - Inline JSON credentials (highest priority)
2. `GCP_CREDENTIALS_FILE` - Path to service account JSON key file
3. Application Default Credentials (ADC) - Used if neither env var is set

For local development:
```bash
# Option 1: Use a credentials file
export GCP_CREDENTIALS_FILE=/path/to/service-account.json

# Option 2: Use inline JSON (useful for CI/CD secrets)
export GCP_CREDENTIALS_JSON='{"type":"service_account",...}'
```

For CI with Workload Identity Federation, leave both unset to use ADC.

### Conformance Testing

Run the full conformance test suite (CRUD lifecycle + discovery) against a specific formae version:

```bash
# Run conformance tests with latest stable version
make conformance-test

# Run conformance tests with a specific version
make conformance-test
```

The conformance tests:
1. Call `setup-credentials` to provision cloud credentials
2. Call `clean-environment` to remove orphaned resources from previous runs
3. Build and install the plugin locally
4. Download the specified formae version (defaults to latest)
5. Run CRUD lifecycle tests for each resource type
6. Run discovery tests to verify resource detection
7. Call `clean-environment` to clean up test resources

### CI Hooks

The template includes hook scripts that you customize for your cloud provider:

#### `scripts/ci/setup-credentials.sh`

Provisions credentials for your cloud provider. Called before running conformance tests.

**Examples:**
- AWS: Verify `AWS_ACCESS_KEY_ID` is set or use OIDC
- OpenStack: Source your RC file and verify required env vars
- Azure: Run `az login` or verify OIDC credentials
- GCP: Run `gcloud auth` or verify workload identity

#### `scripts/ci/clean-environment.sh`

Cleans up test resources in your cloud environment. Called before AND after conformance tests to:
- Remove orphaned resources from previous failed runs (pre-cleanup)
- Clean up resources created during the test run (post-cleanup)

The script should be idempotent and delete all resources under the /testdata folder

#### GitHub Actions

The `.github/workflows/ci.yml` workflow includes a `conformance-tests` job that is
disabled by default. To enable it:

1. Configure Workload Identity Federation in GCP (see [docs/gcp-github-actions-setup.md](docs/gcp-github-actions-setup.md))
2. Add the required GitHub secrets (see below)
3. Set `run_conformance` to `true` when triggering the workflow

#### Required GitHub Secrets

| Secret | Description |
|--------|-------------|
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | Full WIF provider path |
| `GCP_SERVICE_ACCOUNT` | Service account email |
| `GCP_PROJECT_ID` | GCP project ID |
| `GCP_PROJECT_NUMBER` | GCP project number |
| `GCP_REGION` | GCP region |
| `GCP_ZONE` | GCP zone |

For detailed WIF setup instructions, see [docs/gcp-workload-identity-federation-github-actions.md](docs/gcp-workload-identity-federation-github-actions.md).

## Defining Resources (Pkl)

Create resource classes in `schema/pkl/`:

```pkl
@formae.ResourceHint {
    type = "MYPROVIDER::Service::Resource"
    identifier = "$.Id"
}
class MyResource extends formae.Resource {
    @formae.FieldHint {}
    name: String

    @formae.FieldHint { createOnly = true }
    region: String?
}
```

## Plugin Manifest

All plugin metadata lives in `formae-plugin.pkl`:

```pkl
amends "@formae/plugin-manifest.pkl"

name = "myprovider"           # Plugin identifier
version = "1.0.0"             # Semantic version
description = "My cloud provider plugin"

spec {
    protocolVersion = 1       # SDK protocol version
    namespace = "MYPROVIDER"  # Resource type prefix
    capabilities { "create"; "read"; "update"; "delete"; "list"; "discovery" }
}
```

## Async (long-running) Operations

All plugin operations return the `ProgressResult` struct. For async (long-running) operations
return `InProgress` with a `RequestID`. The formae agent will call the `Status` method on
a regular interval to request the status of the operation.

```go
func (p *Plugin) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
    operationID := startAsyncCreate(...)

    return &resource.CreateResult{
        ProgressResult: &resource.ProgressResult{
            Operation:       resource.OperationCreate,
            OperationStatus: resource.OperationStatusInProgress,
            RequestID:       operationID,
        },
    }, nil
}

func (p *Plugin) Status(ctx context.Context, req *resource.StatusRequest) (*resource.StatusResult, error) {
    status := checkOperation(req.RequestID)
    if status.Complete {
        return &resource.StatusResult{
            ProgressResult: &resource.ProgressResult{
                OperationStatus: resource.OperationStatusSuccess,
                NativeID:        status.ResourceID,
            },
        }, nil
    }
    // Still in progress - return InProgress status
}
```

## License

This template is licensed under FSL-1.1-ALv2 - See [LICENSE](LICENSE)

When creating your own plugin, choose an appropriate license for your project.
Common choices include:
- **MIT** - Most permissive
- **Apache-2.0** - Permissive with patent grant (recommended)
- **MPL-2.0** - Weak copyleft
- **FSL-1.1-ALv2** - Functional Source License

Replace the LICENSE file with your chosen license when you create your plugin.
