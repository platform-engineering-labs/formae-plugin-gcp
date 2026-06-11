# Formae Plugin Makefile
#
# Targets:
#   build   - Build the plugin binary
#   test    - Run tests
#   lint    - Run linter
#   clean   - Remove build artifacts
#   install - Build and install plugin locally (binary + schema + manifest)

# Plugin metadata - extracted from formae-plugin.pkl
PLUGIN_NAME := $(shell pkl eval -x 'name' formae-plugin.pkl 2>/dev/null || echo "example")
PLUGIN_VERSION := $(shell pkl eval -x 'version' formae-plugin.pkl 2>/dev/null || echo "0.0.0")
PLUGIN_NAMESPACE := $(shell pkl eval -x 'namespace' formae-plugin.pkl 2>/dev/null || echo "EXAMPLE")

# Build settings
GO := go
GOFLAGS := -trimpath
BINARY := $(PLUGIN_NAME)

# Installation paths
# NOTE: Directory structure will change from <namespace> to <name> in a future version
PLUGIN_BASE_DIR := $(HOME)/.pel/formae/plugins
INSTALL_DIR := $(PLUGIN_BASE_DIR)/$(PLUGIN_NAME)/v$(PLUGIN_VERSION)

.PHONY: all build schema-version test test-unit test-integration lint lint-reuse add-license verify-schema schema-docs clean install help setup-credentials clean-environment conformance-test conformance-test-crud conformance-test-discovery conformance-test-crud-run conformance-test-discovery-run

all: build

## schema-version: Write schema/pkl/VERSION from manifest (used by build + verify-schema)
schema-version:
	@mkdir -p schema/pkl && echo "$(PLUGIN_VERSION)" > schema/pkl/VERSION

## build: Build the plugin binary and update manifest
build: schema-version
	$(GO) build $(GOFLAGS) -o bin/$(BINARY) .
	@./scripts/update-min-formae-version.sh

## test: Run all tests
test:
	$(GO) test -v ./...

## test-unit: Run unit tests only (tests with //go:build unit tag)
test-unit:
	$(GO) test -v -tags=unit ./...

## test-integration: Run integration tests (requires cloud credentials)
## Add tests with //go:build integration tag
test-integration:
	$(GO) test -v -tags=integration -timeout 50m  ./...

## lint: Run golangci-lint
lint:
	golangci-lint run

## lint-reuse: Check REUSE license compliance
lint-reuse:
	./scripts/lint_reuse.sh

## add-license: Add license headers to source files (idempotent)
add-license:
	./scripts/add_license.sh

## verify-schema: Validate PKL schema files
## Checks that schema files are well-formed and follow formae conventions.
verify-schema: schema-version
	$(GO) run github.com/platform-engineering-labs/formae/pkg/plugin/testutil/cmd/verify-schema --namespace $(PLUGIN_NAMESPACE) ./schema/pkl

## schema-docs: Generate documentation for plugin schema in markdown format
schema-docs:
	go run github.com/platform-engineering-labs/formae/pkg/plugin/testutil/cmd/schema-docs --format markdown ./schema/pkl

## clean: Remove build artifacts
clean:
	rm -rf bin/ dist/

## install: Build and install plugin locally (binary + schema + manifest)
## Installs to ~/.pel/formae/plugins/<name>/v<version>/
install: build
	@echo "Installing $(PLUGIN_NAME) v$(PLUGIN_VERSION) (namespace: $(PLUGIN_NAMESPACE))..."
	rm -rf $(PLUGIN_BASE_DIR)/$(PLUGIN_NAME)
	mkdir -p $(INSTALL_DIR)/schema/pkl
	cp bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	cp -r schema/pkl/* $(INSTALL_DIR)/schema/pkl/
	test -f schema/Config.pkl && cp schema/Config.pkl $(INSTALL_DIR)/schema/ || true
	cp formae-plugin.pkl $(INSTALL_DIR)/
	@echo "Installed to $(INSTALL_DIR)"

## help: Show this help message
help:
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## setup-credentials: Provision cloud provider credentials
## Edit scripts/ci/setup-credentials.sh to configure for your provider.
setup-credentials:
	@./scripts/ci/setup-credentials.sh

## clean-environment: Clean up test resources in cloud environment
## Called before and after conformance tests. Edit scripts/ci/clean-environment.sh
## to configure for your provider.
clean-environment:
	@./scripts/ci/clean-environment.sh

## conformance-test: Run all conformance tests (CRUD + discovery)
## Usage: make conformance-test [TEST=bigquery-dataset] [PARALLEL=1] [TIMEOUT=15]
## Calls clean-environment before and after tests.
##
## Parameters:
##   TEST           - Filter tests by name pattern (e.g., TEST=bigquery-dataset)
##   PARALLEL       - Concurrent tests inside the SDK (default: 1 = sequential)
##   TIMEOUT        - Per-operation timeout in minutes (FORMAE_TEST_TIMEOUT;
##                    SDK default 5). Raise for slow resources, e.g. TIMEOUT=15
##                    for Cloud SQL instances (5-10 min to create).
##   GOTEST_TIMEOUT - Overall `go test` timeout in minutes (default: 60)
##
## The conformance SDK installs the latest released formae via orbital
## unless FORMAE_BINARY is set (e.g. by nightly which builds from source).
conformance-test: install
	@echo "Pre-test cleanup..."
	@./scripts/ci/clean-environment.sh || true
	@echo ""
	@$(MAKE) conformance-test-crud-run conformance-test-discovery-run TEST=$(TEST) PARALLEL=$(PARALLEL) TIMEOUT=$(TIMEOUT); \
	TEST_EXIT=$$?; \
	echo ""; \
	echo "Post-test cleanup..."; \
	./scripts/ci/clean-environment.sh || true; \
	exit $$TEST_EXIT

## conformance-test-crud: Run CRUD tests with cleanup (convenience for local dev)
conformance-test-crud: install
	@echo "Pre-test cleanup..."
	@./scripts/ci/clean-environment.sh || true
	@echo ""
	@$(MAKE) conformance-test-crud-run TEST=$(TEST) PARALLEL=$(PARALLEL) TIMEOUT=$(TIMEOUT); \
	TEST_EXIT=$$?; \
	echo ""; \
	echo "Post-test cleanup..."; \
	./scripts/ci/clean-environment.sh || true; \
	exit $$TEST_EXIT

## conformance-test-discovery: Run discovery tests with cleanup (convenience for local dev)
conformance-test-discovery: install
	@echo "Pre-test cleanup..."
	@./scripts/ci/clean-environment.sh || true
	@echo ""
	@$(MAKE) conformance-test-discovery-run TEST=$(TEST) PARALLEL=$(PARALLEL) TIMEOUT=$(TIMEOUT); \
	TEST_EXIT=$$?; \
	echo ""; \
	echo "Post-test cleanup..."; \
	./scripts/ci/clean-environment.sh || true; \
	exit $$TEST_EXIT

## conformance-test-crud-run: Run only CRUD lifecycle tests (no cleanup)
## Used by CI matrix jobs where cleanup is managed separately.
conformance-test-crud-run:
	@echo "Running CRUD conformance tests..."
	@FORMAE_TEST_FILTER="$(TEST)" FORMAE_TEST_TYPE=crud FORMAE_TEST_PARALLEL="$(PARALLEL)" FORMAE_TEST_TIMEOUT="$(TIMEOUT)" \
		$(GO) test -tags=conformance -v -timeout $(or $(GOTEST_TIMEOUT),60)m ./...

## conformance-test-discovery-run: Run only discovery tests (no cleanup)
## Used by CI matrix jobs where cleanup is managed separately.
conformance-test-discovery-run:
	@echo "Running discovery conformance tests..."
	@FORMAE_TEST_FILTER="$(TEST)" FORMAE_TEST_TYPE=discovery FORMAE_TEST_PARALLEL="$(PARALLEL)" FORMAE_TEST_TIMEOUT="$(TIMEOUT)" \
		$(GO) test -tags=conformance -v -timeout $(or $(GOTEST_TIMEOUT),60)m ./...
