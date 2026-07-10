#!/bin/bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Clean Environment Hook
# ======================
# This script is called before AND after conformance tests to clean up
# test resources in your cloud environment.
#
# Purpose:
# - Before tests: Remove orphaned resources from previous failed runs
# - After tests: Clean up resources created during the test run
#
# The script should be idempotent - safe to run multiple times.
# It should delete all resources matching the test resource prefix.
#
# Deletion order matters due to dependencies:
#   1. Firewalls (depend on networks)
#   2. Subnetworks (depend on networks)
#   3. Disks, Cloud Run services, BigQuery tables (leaf resources)
#   4. BigQuery datasets (tables must be deleted first)
#   5. Networks (firewalls and subnetworks must be deleted first)
#   6. Storage buckets
#   7. Bigtable instances
#
# Exit with non-zero status only for unexpected errors.
# Missing resources (already cleaned) should not cause failures.

set -euo pipefail

# Prefix used for test resources - should match what conformance tests create
TEST_PREFIX="${TEST_PREFIX:-formae-plugin-sdk-test-}"

echo "clean-environment.sh: Cleaning resources with prefix '${TEST_PREFIX}'"

# Helper: list and delete resources with a consistent pattern
cleanup_resources() {
    local label="$1"
    local list_cmd="$2"
    local delete_cmd="$3"

    echo "Cleaning ${label}..."
    local items
    items=$(eval "$list_cmd" 2>/dev/null || true)
    if [ -n "$items" ]; then
        echo "$items" | while IFS=$'\t' read -r line; do
            echo "  Deleting: $line"
            eval "$delete_cmd" 2>/dev/null || true
        done
    else
        echo "  No ${label} found"
    fi
}

# --- 1. Firewalls (must delete before networks) ---
echo "Cleaning GCP firewalls..."
FIREWALLS=$(gcloud compute firewall-rules list --filter="name~^formae-plugin-sdk" --format="value(name)" 2>/dev/null || true)
if [ -n "$FIREWALLS" ]; then
    echo "$FIREWALLS" | while read -r fw; do
        echo "  Deleting firewall: $fw"
        gcloud compute firewall-rules delete "$fw" --quiet 2>/dev/null || true
    done
else
    echo "  No firewalls found"
fi

# --- 2. Subnetworks (must delete before networks) ---
echo "Cleaning GCP subnetworks..."
SUBNETWORKS=$(gcloud compute networks subnets list --filter="name~^formae-plugin-sdk" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$SUBNETWORKS" ]; then
    echo "$SUBNETWORKS" | while read -r subnet region; do
        echo "  Deleting subnetwork: $subnet (region: $region)"
        gcloud compute networks subnets delete "$subnet" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No subnetworks found"
fi

# --- 3. Compute instances (must be deleted before their disks) ---
echo "Cleaning GCP compute instances..."
GCE_INSTANCES=$(gcloud compute instances list --filter="name~^formae-plugin-sdk" --format="value(name,zone)" 2>/dev/null || true)
if [ -n "$GCE_INSTANCES" ]; then
    echo "$GCE_INSTANCES" | while read -r vm zone; do
        echo "  Deleting instance: $vm (zone: $zone)"
        gcloud compute instances delete "$vm" --zone="$zone" --quiet 2>/dev/null || true
    done
else
    echo "  No compute instances found"
fi

# --- 4. Disks ---
echo "Cleaning GCP disks..."
DISKS=$(gcloud compute disks list --filter="name~^formae-plugin-sdk" --format="value(name,zone)" 2>/dev/null || true)
if [ -n "$DISKS" ]; then
    echo "$DISKS" | while read -r disk zone; do
        echo "  Deleting disk: $disk (zone: $zone)"
        gcloud compute disks delete "$disk" --zone="$zone" --quiet 2>/dev/null || true
    done
else
    echo "  No disks found"
fi

# --- Target TCP proxies (global, must be deleted before their backend services) ---
echo "Cleaning GCP target TCP proxies..."
TARGET_TCP_PROXIES=$(gcloud compute target-tcp-proxies list --filter="name~^formae-plugin-sdk" --global --format="value(name)" 2>/dev/null || true)
if [ -n "$TARGET_TCP_PROXIES" ]; then
    echo "$TARGET_TCP_PROXIES" | while read -r ttp; do
        echo "  Deleting target TCP proxy: $ttp"
        gcloud compute target-tcp-proxies delete "$ttp" --global --quiet 2>/dev/null || true
    done
else
    echo "  No target TCP proxies found"
fi

# --- Global forwarding rules (must be deleted before their target proxies) ---
echo "Cleaning GCP global forwarding rules..."
GLOBAL_FORWARDING_RULES=$(gcloud compute forwarding-rules list --filter="name~^formae-plugin-sdk" --global --format="value(name)" 2>/dev/null || true)
if [ -n "$GLOBAL_FORWARDING_RULES" ]; then
    echo "$GLOBAL_FORWARDING_RULES" | while read -r fr; do
        echo "  Deleting global forwarding rule: $fr"
        gcloud compute forwarding-rules delete "$fr" --global --quiet 2>/dev/null || true
    done
else
    echo "  No global forwarding rules found"
fi

# --- Regional forwarding rules (region-http-lb test) ---
echo "Cleaning GCP regional forwarding rules..."
REGIONAL_FORWARDING_RULES=$(gcloud compute forwarding-rules list --filter="name~^formae-plugin-sdk" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==2' || true)
if [ -n "$REGIONAL_FORWARDING_RULES" ]; then
    echo "$REGIONAL_FORWARDING_RULES" | while read -r fr region; do
        echo "  Deleting regional forwarding rule: $fr (region: $region)"
        gcloud compute forwarding-rules delete "$fr" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional forwarding rules found"
fi

# --- Regional target HTTP proxies (region-http-lb test) ---
echo "Cleaning GCP regional target HTTP proxies..."
REGIONAL_HTTP_PROXIES=$(gcloud compute target-http-proxies list --filter="name~^formae-plugin-sdk" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==2' || true)
if [ -n "$REGIONAL_HTTP_PROXIES" ]; then
    echo "$REGIONAL_HTTP_PROXIES" | while read -r thp region; do
        echo "  Deleting regional target HTTP proxy: $thp (region: $region)"
        gcloud compute target-http-proxies delete "$thp" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional target HTTP proxies found"
fi

# --- Target HTTP proxies (global, must be deleted before their url maps) ---
echo "Cleaning GCP target HTTP proxies..."
TARGET_HTTP_PROXIES=$(gcloud compute target-http-proxies list --filter="name~^formae-plugin-sdk" --global --format="value(name)" 2>/dev/null || true)
if [ -n "$TARGET_HTTP_PROXIES" ]; then
    echo "$TARGET_HTTP_PROXIES" | while read -r thp; do
        echo "  Deleting target HTTP proxy: $thp"
        gcloud compute target-http-proxies delete "$thp" --global --quiet 2>/dev/null || true
    done
else
    echo "  No target HTTP proxies found"
fi

# --- URL maps (global, must be deleted before their backend services) ---
echo "Cleaning GCP URL maps..."
URL_MAPS=$(gcloud compute url-maps list --filter="name~^formae-plugin-sdk" --global --format="value(name)" 2>/dev/null || true)
if [ -n "$URL_MAPS" ]; then
    echo "$URL_MAPS" | while read -r um; do
        echo "  Deleting URL map: $um"
        gcloud compute url-maps delete "$um" --global --quiet 2>/dev/null || true
    done
else
    echo "  No URL maps found"
fi

# --- Regional URL maps (region-http-lb test) ---
echo "Cleaning GCP regional URL maps..."
REGIONAL_URL_MAPS=$(gcloud compute url-maps list --filter="name~^formae-plugin-sdk" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==2' || true)
if [ -n "$REGIONAL_URL_MAPS" ]; then
    echo "$REGIONAL_URL_MAPS" | while read -r um region; do
        echo "  Deleting regional URL map: $um (region: $region)"
        gcloud compute url-maps delete "$um" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional URL maps found"
fi

# --- Backend services (global, must be deleted before their health checks) ---
echo "Cleaning GCP backend services..."
BACKEND_SERVICES=$(gcloud compute backend-services list --filter="name~^formae-plugin-sdk" --global --format="value(name)" 2>/dev/null || true)
if [ -n "$BACKEND_SERVICES" ]; then
    echo "$BACKEND_SERVICES" | while read -r bs; do
        echo "  Deleting backend service: $bs"
        gcloud compute backend-services delete "$bs" --global --quiet 2>/dev/null || true
    done
else
    echo "  No backend services found"
fi

# --- Regional backend services (region-http-lb test) ---
echo "Cleaning GCP regional backend services..."
REGIONAL_BACKEND_SERVICES=$(gcloud compute backend-services list --filter="name~^formae-plugin-sdk" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==2' || true)
if [ -n "$REGIONAL_BACKEND_SERVICES" ]; then
    echo "$REGIONAL_BACKEND_SERVICES" | while read -r bs region; do
        echo "  Deleting regional backend service: $bs (region: $region)"
        gcloud compute backend-services delete "$bs" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional backend services found"
fi

# --- Health checks (global) ---
echo "Cleaning GCP health checks..."
HEALTH_CHECKS=$(gcloud compute health-checks list --filter="name~^formae-plugin-sdk" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==1{print $1}' || true)
if [ -n "$HEALTH_CHECKS" ]; then
    echo "$HEALTH_CHECKS" | while read -r hc; do
        echo "  Deleting health check: $hc"
        gcloud compute health-checks delete "$hc" --global --quiet 2>/dev/null || true
    done
else
    echo "  No health checks found"
fi

# --- Regional health checks (region-http-lb test) ---
echo "Cleaning GCP regional health checks..."
REGIONAL_HEALTH_CHECKS=$(gcloud compute health-checks list --filter="name~^formae-plugin-sdk" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==2' || true)
if [ -n "$REGIONAL_HEALTH_CHECKS" ]; then
    echo "$REGIONAL_HEALTH_CHECKS" | while read -r hc region; do
        echo "  Deleting regional health check: $hc (region: $region)"
        gcloud compute health-checks delete "$hc" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional health checks found"
fi

# --- 4. Cloud Run services ---
echo "Cleaning GCP Cloud Run services..."
SERVICES=$(gcloud run services list --filter="metadata.name~^formae-test" --format="value(metadata.name,region)" 2>/dev/null || true)
if [ -n "$SERVICES" ]; then
    echo "$SERVICES" | while read -r svc region; do
        echo "  Deleting Cloud Run service: $svc (region: $region)"
        gcloud run services delete "$svc" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No Cloud Run services found"
fi

# --- 4b. Cloud Run jobs ---
echo "Cleaning GCP Cloud Run jobs..."
JOBS=$(gcloud run jobs list --filter="metadata.name~^formae-test" --format="value(metadata.name,region)" 2>/dev/null || true)
if [ -n "$JOBS" ]; then
    echo "$JOBS" | while read -r job region; do
        echo "  Deleting Cloud Run job: $job (region: $region)"
        gcloud run jobs delete "$job" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No Cloud Run jobs found"
fi

# --- 4c. Cloud Run worker pools ---
echo "Cleaning GCP Cloud Run worker pools..."
WORKER_POOLS=$(gcloud run worker-pools list --filter="metadata.name~^formae-test" --format="value(metadata.name,region)" 2>/dev/null || true)
if [ -n "$WORKER_POOLS" ]; then
    echo "$WORKER_POOLS" | while read -r wp region; do
        echo "  Deleting Cloud Run worker pool: $wp (region: $region)"
        gcloud run worker-pools delete "$wp" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No Cloud Run worker pools found"
fi

# --- 5. BigQuery tables (must delete before datasets) ---
echo "Cleaning GCP BigQuery tables..."
DATASETS=$(bq ls --format=json --project_id="${GCP_PROJECT_ID:-}" 2>/dev/null | grep -o '"formae_plugin_sdk_test_[^"]*"' | tr -d '"' || true)
if [ -n "$DATASETS" ]; then
    for ds in $DATASETS; do
        TABLES=$(bq ls --format=json "${GCP_PROJECT_ID}:${ds}" 2>/dev/null | grep -o '"formae_plugin_sdk_test_[^"]*"' | tr -d '"' || true)
        if [ -n "$TABLES" ]; then
            for tbl in $TABLES; do
                echo "  Deleting table: ${ds}.${tbl}"
                bq rm -f -t "${GCP_PROJECT_ID}:${ds}.${tbl}" 2>/dev/null || true
            done
        fi
    done
else
    echo "  No BigQuery tables found"
fi

# --- 6. BigQuery datasets ---
echo "Cleaning GCP BigQuery datasets..."
DATASETS=$(bq ls --format=json --project_id="${GCP_PROJECT_ID:-}" 2>/dev/null | grep -o '"formae_plugin_sdk_test_[^"]*"' | tr -d '"' || true)
if [ -n "$DATASETS" ]; then
    for ds in $DATASETS; do
        echo "  Deleting dataset: $ds"
        bq rm -r -f -d "${GCP_PROJECT_ID}:${ds}" 2>/dev/null || true
    done
else
    echo "  No BigQuery datasets found"
fi

# --- 6b. Private Service Access (servicenetworking connections + peering
#         addresses) must be torn down before their networks. The connection
#         is a VPC peering keyed by (service, network); delete it via the
#         servicenetworking REST API (force=true removes the peering), then the
#         reserved VPC_PEERING global addresses it consumed. ---
echo "Cleaning GCP Private Service Access connections..."
PSA_PROJECT="${GCP_PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || true)}"
PSA_TOKEN="$(gcloud auth print-access-token 2>/dev/null || true)"
if [ -n "$PSA_PROJECT" ] && [ -n "$PSA_TOKEN" ]; then
    PSA_NETWORKS=$(gcloud compute networks list --filter="name~^formae-plugin-sdk" --format="value(name)" 2>/dev/null || true)
    if [ -n "$PSA_NETWORKS" ]; then
        echo "$PSA_NETWORKS" | while read -r net; do
            echo "  Deleting PSA connection on network: $net"
            curl -s -X DELETE -H "Authorization: Bearer ${PSA_TOKEN}" \
                "https://servicenetworking.googleapis.com/v1/services/servicenetworking.googleapis.com/connections/-?force=true&consumerNetwork=projects/${PSA_PROJECT}/global/networks/${net}" \
                >/dev/null 2>&1 || true
        done
    fi
else
    echo "  Skipping PSA cleanup (no project or access token available)"
fi

echo "Cleaning GCP global (VPC_PEERING) addresses..."
GLOBAL_ADDRESSES=$(gcloud compute addresses list --global --filter="name~^formae-plugin-sdk" --format="value(name)" 2>/dev/null || true)
if [ -n "$GLOBAL_ADDRESSES" ]; then
    echo "$GLOBAL_ADDRESSES" | while read -r addr; do
        echo "  Deleting global address: $addr"
        gcloud compute addresses delete "$addr" --global --quiet 2>/dev/null || true
    done
else
    echo "  No global addresses found"
fi

# --- 7. Networks (after firewalls and subnetworks are deleted) ---
echo "Cleaning GCP networks..."
NETWORKS=$(gcloud compute networks list --filter="name~^formae-plugin-sdk" --format="value(name)" 2>/dev/null || true)
if [ -n "$NETWORKS" ]; then
    echo "$NETWORKS" | while read -r network; do
        echo "  Deleting network: $network"
        gcloud compute networks delete "$network" --quiet 2>/dev/null || true
    done
else
    echo "  No networks found"
fi

# --- 8. Storage buckets ---
echo "Cleaning GCP storage buckets..."
BUCKETS=$(gcloud storage buckets list --filter="name~^formae-plugin-sdk-test" --format="value(name)" 2>/dev/null || true)
if [ -n "$BUCKETS" ]; then
    echo "$BUCKETS" | while read -r bucket; do
        echo "  Deleting bucket: $bucket"
        gcloud storage rm -r "gs://$bucket" --quiet 2>/dev/null || true
    done
else
    echo "  No buckets found"
fi

# --- 9. Bigtable instances ---
echo "Cleaning GCP Bigtable instances..."
INSTANCES=$(gcloud bigtable instances list --filter="name~formae-test-instance" --format="value(name)" 2>/dev/null || true)
if [ -n "$INSTANCES" ]; then
    echo "$INSTANCES" | while read -r instance; do
        echo "  Deleting Bigtable instance: $instance"
        gcloud bigtable instances delete "$instance" --quiet 2>/dev/null || true
    done
else
    echo "  No Bigtable instances found"
fi

# --- 10. Cloud SQL instances ---
# Use the sqladmin REST API directly rather than `gcloud sql instances delete`:
# some gcloud versions add a final-backup parameter that sqladmin rejects
# ("Final Backup Retention Days can not be set if enable_final_backup is disabled"),
# and gcloud's credential token type is not always accepted by sqladmin. The REST
# DELETE defaults to no final backup and works with a plain bearer token. Instances
# still in PENDING_CREATE are accepted (the delete queues behind the create).
echo "Cleaning GCP Cloud SQL instances..."
SQL_PROJECT="${GCP_PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || true)}"
SQL_TOKEN="$(gcloud auth print-access-token 2>/dev/null || true)"
if [ -n "$SQL_PROJECT" ] && [ -n "$SQL_TOKEN" ]; then
    SQL_API="https://sqladmin.googleapis.com/v1/projects/${SQL_PROJECT}/instances"
    SQL_INSTANCES=$(curl -s -H "Authorization: Bearer ${SQL_TOKEN}" "$SQL_API" \
        | grep -oE '"name": *"formae-test-sql[^"]*"' \
        | sed -E 's/.*"(formae-test-sql[^"]*)".*/\1/' || true)
    if [ -n "$SQL_INSTANCES" ]; then
        echo "$SQL_INSTANCES" | while read -r instance; do
            echo "  Deleting Cloud SQL instance: $instance"
            curl -s -X DELETE -H "Authorization: Bearer ${SQL_TOKEN}" "${SQL_API}/${instance}" >/dev/null 2>&1 || true
        done
    else
        echo "  No Cloud SQL instances found"
    fi
else
    echo "  Skipping Cloud SQL cleanup (no project or access token available)"
fi

# --- 11. Secret Manager secrets (secret + secret-version tests) ---
# Not cleaned before => leaked secrets cause AlreadyExists on re-run. Deleting a
# secret removes its versions, so this covers both test resource types.
echo "Cleaning GCP Secret Manager secrets..."
SECRETS=$(gcloud secrets list --filter="name~^formae-plugin-sdk-test" --format="value(name)" 2>/dev/null || true)
if [ -n "$SECRETS" ]; then
    echo "$SECRETS" | while read -r secret; do
        echo "  Deleting secret: $secret"
        gcloud secrets delete "$secret" --quiet 2>/dev/null || true
    done
else
    echo "  No secrets found"
fi

# --- 12. IAM service accounts (iam-service-account test) ---
echo "Cleaning GCP IAM service accounts..."
SA_PROJECT="${GCP_PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || true)}"
SERVICE_ACCOUNTS=$(gcloud iam service-accounts list --filter="email~^formae-plugin-sdk-test" --format="value(email)" 2>/dev/null || true)
if [ -n "$SERVICE_ACCOUNTS" ]; then
    echo "$SERVICE_ACCOUNTS" | while read -r sa; do
        echo "  Deleting service account: $sa"
        gcloud iam service-accounts delete "$sa" --quiet 2>/dev/null || true
    done
else
    echo "  No service accounts found"
fi

echo ""
echo "clean-environment.sh: Cleanup complete"
