# GCP authentication for GitHub Actions (Workload Identity Federation)

Every workflow in this repository that touches real GCP — `ci.yml`,
`nightly.yml`, `debug-conformance.yml`, `probe-permissions.yml` — authenticates
with [`google-github-actions/auth@v3`][auth] using Workload Identity
Federation. No service-account key is ever stored in this repository. GitHub
mints a short-lived OIDC token for the job, GCP exchanges it for an access
token, and the token dies with the job.

This document is what you need to stand that up in a fresh project. Everything
in angle brackets is a placeholder — substitute your own value.

| Placeholder | What it is | How to find it |
|---|---|---|
| `<PROJECT_ID>` | GCP project id | `gcloud config get-value project` |
| `<PROJECT_NUMBER>` | GCP project number (not the id) | `gcloud projects describe <PROJECT_ID> --format='value(projectNumber)'` |
| `<POOL_ID>` | Name you give the identity pool | your choice, e.g. `github` |
| `<PROVIDER_ID>` | Name you give the provider inside the pool | your choice, e.g. `github-provider` |
| `<SA_NAME>` | Name you give the service account | your choice, e.g. `github-deploy` |
| `<GITHUB_ORG>/<GITHUB_REPO>` | The repository being trusted | this repository |

## 1. Enable the APIs the federation itself needs

```bash
gcloud services enable \
  iamcredentials.googleapis.com \
  sts.googleapis.com \
  cloudresourcemanager.googleapis.com \
  --project=<PROJECT_ID>
```

The conformance suite additionally needs every service whose resources it
exercises enabled in the project. It covers roughly fifty GCP services (one per
directory under `pkg/resources/`), and a case whose service is disabled fails
with `SERVICE_DISABLED` rather than a resource-level error. Enable them as you
add cases rather than all at once.

## 2. Create the identity pool and provider

```bash
gcloud iam workload-identity-pools create <POOL_ID> \
  --project=<PROJECT_ID> \
  --location=global \
  --display-name="GitHub Actions"

gcloud iam workload-identity-pools providers create-oidc <PROVIDER_ID> \
  --project=<PROJECT_ID> \
  --location=global \
  --workload-identity-pool=<POOL_ID> \
  --display-name="GitHub" \
  --issuer-uri="https://token.actions.githubusercontent.com" \
  --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository,attribute.repository_owner=assertion.repository_owner" \
  --attribute-condition="assertion.repository_owner == '<GITHUB_ORG>'"
```

The `--attribute-condition` is the part that matters. Without it the provider
will exchange a token minted by **any** GitHub repository in the world for GCP
credentials in your project. Scoping it to your organisation here is a coarse
first gate; step 4 narrows it to a single repository.

`google.subject=assertion.sub` is what makes the per-repository binding in
step 4 possible: GitHub's `sub` claim carries
`repo:<GITHUB_ORG>/<GITHUB_REPO>:ref:refs/heads/<branch>`, and
`attribute.repository` carries just `<GITHUB_ORG>/<GITHUB_REPO>`.

## 3. Create the service account

```bash
gcloud iam service-accounts create <SA_NAME> \
  --project=<PROJECT_ID> \
  --display-name="GitHub Actions conformance"
```

Do **not** create a key for it. The whole point of federation is that there is
no key to leak or rotate.

## 4. Let this repository — and only this repository — impersonate it

```bash
gcloud iam service-accounts add-iam-policy-binding \
  <SA_NAME>@<PROJECT_ID>.iam.gserviceaccount.com \
  --project=<PROJECT_ID> \
  --role="roles/iam.workloadIdentityUser" \
  --member="principalSet://iam.googleapis.com/projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/<POOL_ID>/attribute.repository/<GITHUB_ORG>/<GITHUB_REPO>"
```

`attribute.repository/<GITHUB_ORG>/<GITHUB_REPO>` is the repository-level
condition. A workflow in a different repository under the same organisation
passes the provider's attribute condition from step 2 but fails this binding, so
it cannot impersonate the service account.

To narrow further — for instance to trust only the default branch — bind
`principal://.../subject/repo:<GITHUB_ORG>/<GITHUB_REPO>:ref:refs/heads/main`
instead of the `principalSet` above. Note that the conformance workflows here
are dispatched on feature branches (`debug-conformance.yml` exists precisely to
run against your branch before you open a PR), so a branch-scoped binding would
break them.

## 5. Grant the service account what the suite needs

The conformance suite runs a full create/read/update/delete/discover/extract
lifecycle against live GCP for every case in `testdata/`, so the service account
needs create and delete permission on every resource family it exercises. In
practice that is:

- **Broad resource management.** `roles/editor` covers the bulk of it. It is
  wide, and that is a deliberate tradeoff for a disposable test project — do not
  use this pattern in a project that holds anything you care about.
- **IAM policy changes**, which `roles/editor` deliberately does not include.
  Cases such as `cloudrun-service-iam-member`, `pubsub-topic-iam-member` and
  `pubsub-subscription-iam-member` set IAM bindings on the resources they
  create, and `GCP::IAM::Role` creates custom roles. These need
  `roles/resourcemanager.projectIamAdmin` and `roles/iam.roleAdmin`.
- **Service enablement**, if you want CI rather than a human to turn APIs on:
  `roles/serviceusage.serviceUsageAdmin`.

Rather than trusting a role list to stay accurate as the resource surface grows,
**ask the API what the service account actually holds**. That is what
`.github/workflows/probe-permissions.yml` is for:

```bash
gh workflow run probe-permissions.yml \
  -f permissions=essentialcontacts.contacts.list,run.services.create,pubsub.topics.setIamPolicy
```

It runs `scripts/ci/probe-permissions.sh` as the CI service account and reports
`GRANTED` / `denied` / `INVALID` per permission, with no side effects and no
cost. Run it before writing a batch of resources, not after a batch fails.

## 6. Set the repository secrets

Set these under **Settings → Secrets and variables → Actions**:

| Secret | Value |
|--------|-------|
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | `projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/<POOL_ID>/providers/<PROVIDER_ID>` |
| `GCP_SERVICE_ACCOUNT` | `<SA_NAME>@<PROJECT_ID>.iam.gserviceaccount.com` |
| `GCP_PROJECT_ID` | `<PROJECT_ID>` |
| `GCP_PROJECT_NUMBER` | `<PROJECT_NUMBER>` |
| `GCP_REGION` | Region the fixtures deploy into, e.g. `us-central1` |
| `GCP_ZONE` | Zone within that region, e.g. `us-central1-a` |
| `GCP_LOCATION` | Location for APIs that use `location` rather than `region`/`zone`, e.g. `us-central1` |

`GCP_WORKLOAD_IDENTITY_PROVIDER` is the *full* provider path, not just the
provider id. `gcloud iam workload-identity-pools providers describe
<PROVIDER_ID> --project=<PROJECT_ID> --location=global
--workload-identity-pool=<POOL_ID> --format='value(name)'` prints it.

`GCP_REGION`, `GCP_ZONE` and `GCP_LOCATION` are all separate because GCP is not
consistent about which one an API wants — compute uses region and zone, and
newer APIs use location. A fixture that needs a location will fail with an
empty path segment if `GCP_LOCATION` is unset, so set all three.

## 7. Job-level requirements in the workflow

Every job that authenticates needs `id-token: write`, or GitHub will not mint an
OIDC token for it and `auth@v3` fails before it reaches GCP:

```yaml
    permissions:
      id-token: write
      contents: read
    steps:
      - uses: actions/checkout@v7
      - uses: google-github-actions/auth@v3
        with:
          workload_identity_provider: ${{ secrets.GCP_WORKLOAD_IDENTITY_PROVIDER }}
          service_account: ${{ secrets.GCP_SERVICE_ACCOUNT }}
```

`auth@v3` writes a credential file and exports `GOOGLE_APPLICATION_CREDENTIALS`,
which is how `scripts/ci/setup-credentials.sh` finds it — the script accepts
`GCP_CREDENTIALS_FILE`, `GCP_CREDENTIALS_JSON`, `GOOGLE_APPLICATION_CREDENTIALS`
or Application Default Credentials, and fails the run early if none is present.

## Verifying it works

Cheapest first:

1. `gh workflow run probe-permissions.yml -f permissions=compute.networks.list`
   — proves the whole exchange works and costs nothing. A failure here is
   federation, not a resource.
2. `gh workflow run debug-conformance.yml --ref <your-branch> -f test_cases=disk`
   — one live lifecycle.

## Local development

Federation is CI-only. Locally, point the suite at credentials directly:

```bash
cp .env.example .env
# then set one of:
#   GCP_CREDENTIALS_FILE=/path/to/service-account.json
#   GCP_CREDENTIALS_JSON='{...}'
# or run: gcloud auth application-default login
```

See [CONTRIBUTING.md](../CONTRIBUTING.md) for the rest of the local loop.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `Unable to acquire impersonated credentials` | The step-4 binding is missing, or its `principalSet` names the wrong project *number*, pool or repository. |
| `The audience in ID Token does not match` | `GCP_WORKLOAD_IDENTITY_PROVIDER` is not the full provider path. |
| `Permission 'iam.serviceAccounts.getAccessToken' denied` | `iamcredentials.googleapis.com` is not enabled (step 1). |
| `unable to get local issuer` / no token minted | The job is missing `id-token: write`. |
| `SERVICE_DISABLED` mentioning a quota project | The API for that resource family is not enabled in `<PROJECT_ID>`. |
| A 403 on one resource type while every other type in the same run authenticates fine | Not a credentials problem. Some Google APIs refuse federated principals outright — see PLA-758. |

[auth]: https://github.com/google-github-actions/auth
