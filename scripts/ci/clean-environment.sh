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
#   1b-1d. Autoscalers -> managed instance groups -> instance templates
#      (each depends on the next; templates hold a network reference)
#   1e. Images, snapshots, resource policies (leaf)
#   1f. Log views, logging sinks, logging exclusions, Monitoring services + dashboards
#   1f2. VPN tunnels -> HA/external VPN gateways -> routers (gateway quota is 2/region)
#   1g. SSL policies (after the target proxies that reference them)
#   1h. Network attachments (hold a subnet reference)
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
# Every fixture names what it creates "formae-<something>-<testRunID>", but only
# some use the "formae-plugin-sdk-test-" form. 101 name literals in testdata/ use
# a bare "formae-test-" instead, and a handful use neither ("formae-sp-",
# "formae-thsp-", "formae-rrset-"). The sweeps matched "^formae-plugin-sdk" only,
# so those never got collected - which is how eight SSL certificates reached the
# global cap of ten and how thirteen Service Directory namespaces accumulated in
# europe-central2. Leaked resources exhaust quotas, and a quota failure surfaces
# as an unrelated case failing, so this is a correctness problem, not tidiness.
#
# Matching "^formae-" covers every family, including ones added later - the
# enumeration is what drifted, so there is no enumeration any more.
SWEEP_RE="${SWEEP_RE:-^formae-}"

# Names that match SWEEP_RE but must never be deleted. A "formae-" resource is
# not always a leak: formae-byo-cert is a certificate someone installed in July
# and is still in use. Add a name here rather than narrowing SWEEP_RE.
KEEP_RE="${KEEP_RE:-^(formae-byo-cert)$}"

echo "clean-environment.sh: sweeping names matching '${SWEEP_RE}' (keeping '${KEEP_RE}')"

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
# Network firewall policies are global; a policy with associations must have
# them removed first, but the fixtures create none.
# Cloud Armor policies, regional and global in one pass. The rule fixtures each
# create one as a prerequisite and conformance Destroy only removes the resource
# under test, so every run leaves one behind.
#
# This used to run twice with two filters, and neither reached a regional
# policy: "name~^formae- AND -region:''" matches nothing at all, and
# the global pass explicitly skipped any row that had a region. Regional
# policies were therefore never deleted - nine had piled up in europe-central2
# when this was found, and a rerun of the same CI run collided with its own
# leftover ("The resource ... already exists"). Filter client-side and branch on
# whether the row carries a region.
echo "Cleaning GCP security policies (regional and global)..."
SEC_POLICIES=$(gcloud compute security-policies list --format="value(name,region.basename())" 2>/dev/null | grep -E "$SWEEP_RE" | grep -Ev "$KEEP_RE" || true)
if [ -n "$SEC_POLICIES" ]; then
    echo "$SEC_POLICIES" | while read -r pol region; do
        if [ -n "$region" ]; then
            echo "  Deleting regional security policy: $pol (region: $region)"
            gcloud compute security-policies delete "$pol" --region="$region" --quiet 2>/dev/null || true
        else
            echo "  Deleting global security policy: $pol"
            gcloud compute security-policies delete "$pol" --global --quiet 2>/dev/null || true
        fi
    done
else
    echo "  No security policies found"
fi

# An association pins both its policy and the network it attaches to, so it has
# to be detached before either can be deleted.
echo "Detaching network firewall policy associations..."
NFP_FOR_ASSOC=$(gcloud compute network-firewall-policies list --global --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$NFP_FOR_ASSOC" ]; then
    echo "$NFP_FOR_ASSOC" | while read -r pol; do
        ASSOCS=$(gcloud compute network-firewall-policies describe "$pol" --global --format="value(associations[].name)" 2>/dev/null || true)
        for assoc in $(echo "$ASSOCS" | tr ';,' ' '); do
            [ -z "$assoc" ] && continue
            echo "  Detaching association $assoc from $pol"
            gcloud compute network-firewall-policies associations delete --firewall-policy="$pol" --name="$assoc" --global-firewall-policy --quiet 2>/dev/null || true
        done
    done
else
    echo "  No network firewall policies to check for associations"
fi

# Regional policies keep their associations in a separate collection.
#
# These listings filter client-side: "name~... AND -region:''" matches nothing,
# so every regional pass below was a no-op and the resources leaked. Twelve
# regional network firewall policies had accumulated against a quota of 10.
echo "Detaching regional network firewall policy associations..."
RNFP_FOR_ASSOC=$(gcloud compute network-firewall-policies list --format="value(name,region.basename())" 2>/dev/null | grep -E "$SWEEP_RE" | grep -Ev "$KEEP_RE" || true)
if [ -n "$RNFP_FOR_ASSOC" ]; then
    echo "$RNFP_FOR_ASSOC" | while read -r pol region; do
        [ -z "$region" ] && continue
        ASSOCS=$(gcloud compute network-firewall-policies describe "$pol" --region="$region" --format="value(associations[].name)" 2>/dev/null || true)
        for assoc in $(echo "$ASSOCS" | tr ';,' ' '); do
            [ -z "$assoc" ] && continue
            echo "  Detaching association $assoc from $pol (region: $region)"
            gcloud compute network-firewall-policies associations delete --firewall-policy="$pol" --name="$assoc" --firewall-policy-region="$region" --quiet 2>/dev/null || true
        done
    done
else
    echo "  No regional network firewall policies to check for associations"
fi

echo "Cleaning GCP network firewall policies..."
NET_FW_POLICIES=$(gcloud compute network-firewall-policies list --global --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$NET_FW_POLICIES" ]; then
    echo "$NET_FW_POLICIES" | while read -r pol; do
        echo "  Deleting network firewall policy: $pol"
        gcloud compute network-firewall-policies delete "$pol" --global --quiet 2>/dev/null || true
    done
else
    echo "  No network firewall policies found"
fi

echo "Cleaning GCP regional network firewall policies..."
REGION_FW_POLICIES=$(gcloud compute network-firewall-policies list --format="value(name,region.basename())" 2>/dev/null | grep -E "$SWEEP_RE" | grep -Ev "$KEEP_RE" || true)
if [ -n "$REGION_FW_POLICIES" ]; then
    echo "$REGION_FW_POLICIES" | while read -r pol region; do
        [ -z "$region" ] && continue
        echo "  Deleting regional network firewall policy: $pol (region: $region)"
        gcloud compute network-firewall-policies delete "$pol" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional network firewall policies found"
fi

echo "Cleaning GCP firewalls..."
FIREWALLS=$(gcloud compute firewall-rules list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$FIREWALLS" ]; then
    echo "$FIREWALLS" | while read -r fw; do
        echo "  Deleting firewall: $fw"
        gcloud compute firewall-rules delete "$fw" --quiet 2>/dev/null || true
    done
else
    echo "  No firewalls found"
fi

# --- 1b. Autoscalers (must delete before the MIGs they target) ---
echo "Cleaning GCP autoscalers..."
AUTOSCALERS=$(gcloud compute instance-groups managed list --filter="name~^formae-" --format="value(autoscaler.name,zone)" 2>/dev/null | grep -v '^\s*$' || true)
if [ -n "$AUTOSCALERS" ]; then
    echo "$AUTOSCALERS" | while read -r autoscaler zone; do
        [ -z "$autoscaler" ] && continue
        echo "  Deleting autoscaler: $autoscaler (zone: $zone)"
        gcloud compute instance-groups managed stop-autoscaling "${autoscaler}" --zone="$zone" --quiet 2>/dev/null || true
    done
else
    echo "  No autoscalers found"
fi

# Regional autoscalers are attached to regional MIGs, which report no zone.
echo "Cleaning GCP regional autoscalers..."
REGION_AUTOSCALERS=$(gcloud compute instance-groups managed list --filter="name~^formae- AND -zone:*" --format="value(autoscaler.name,region)" 2>/dev/null | grep -v '^\s*$' || true)
if [ -n "$REGION_AUTOSCALERS" ]; then
    echo "$REGION_AUTOSCALERS" | while read -r autoscaler region; do
        [ -z "$autoscaler" ] && continue
        echo "  Stopping regional autoscaling on: $autoscaler (region: $region)"
        gcloud compute instance-groups managed stop-autoscaling "$autoscaler" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional autoscalers found"
fi

# --- 1c. Managed instance groups (must delete before their instance templates) ---
echo "Cleaning GCP managed instance groups..."
MIGS=$(gcloud compute instance-groups managed list --filter="name~^formae-" --format="value(name,zone)" 2>/dev/null || true)
if [ -n "$MIGS" ]; then
    echo "$MIGS" | while read -r mig zone; do
        echo "  Deleting managed instance group: $mig (zone: $zone)"
        gcloud compute instance-groups managed delete "$mig" --zone="$zone" --quiet 2>/dev/null || true
    done
else
    echo "  No managed instance groups found"
fi

# Regional MIGs report no zone, so the zonal loop above skips them.
echo "Cleaning GCP regional managed instance groups..."
REGION_MIGS=$(gcloud compute instance-groups managed list --filter="name~^formae- AND -zone:*" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$REGION_MIGS" ]; then
    echo "$REGION_MIGS" | while read -r mig region; do
        echo "  Deleting regional managed instance group: $mig (region: $region)"
        gcloud compute instance-groups managed delete "$mig" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional managed instance groups found"
fi

# --- 1d. Instance templates (hold a network reference, so delete before networks) ---
# Regional instance templates are a separate collection from the global ones.
echo "Cleaning GCP regional instance templates..."
REGION_TEMPLATES=$(gcloud compute instance-templates list --format="value(name,region.basename())" 2>/dev/null | grep -E "$SWEEP_RE" | grep -Ev "$KEEP_RE" || true)
if [ -n "$REGION_TEMPLATES" ]; then
    echo "$REGION_TEMPLATES" | while read -r tmpl region; do
        [ -z "$region" ] && continue
        echo "  Deleting regional instance template: $tmpl (region: $region)"
        gcloud compute instance-templates delete "$tmpl" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional instance templates found"
fi

echo "Cleaning GCP instance templates..."
TEMPLATES=$(gcloud compute instance-templates list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$TEMPLATES" ]; then
    echo "$TEMPLATES" | while read -r tmpl; do
        echo "  Deleting instance template: $tmpl"
        gcloud compute instance-templates delete "$tmpl" --quiet 2>/dev/null || true
    done
else
    echo "  No instance templates found"
fi

# --- 1e. Images and resource policies (leaf resources, no dependents) ---
# Machine images pin their source instance, so they go before the instances loop.
echo "Cleaning GCP machine images..."
MACHINE_IMAGES=$(gcloud compute machine-images list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$MACHINE_IMAGES" ]; then
    echo "$MACHINE_IMAGES" | while read -r mi; do
        echo "  Deleting machine image: $mi"
        gcloud compute machine-images delete "$mi" --quiet 2>/dev/null || true
    done
else
    echo "  No machine images found"
fi

echo "Cleaning GCP images..."
IMAGES=$(gcloud compute images list --no-standard-images --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$IMAGES" ]; then
    echo "$IMAGES" | while read -r img; do
        echo "  Deleting image: $img"
        gcloud compute images delete "$img" --quiet 2>/dev/null || true
    done
else
    echo "  No images found"
fi

# Instant snapshots are zonal and pin their source disk, so they go before disks.
# Regional instant snapshots are a separate collection from the zonal ones.
echo "Cleaning GCP regional instant snapshots..."
REGION_ISNAPS=$(gcloud compute instant-snapshots list --filter="name~^formae- AND -zone:*" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$REGION_ISNAPS" ]; then
    echo "$REGION_ISNAPS" | while read -r isnap region; do
        [ -z "$region" ] && continue
        echo "  Deleting regional instant snapshot: $isnap (region: $region)"
        gcloud compute instant-snapshots delete "$isnap" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional instant snapshots found"
fi

echo "Cleaning GCP instant snapshots..."
INSTANT_SNAPSHOTS=$(gcloud compute instant-snapshots list --filter="name~^formae-" --format="value(name,zone)" 2>/dev/null || true)
if [ -n "$INSTANT_SNAPSHOTS" ]; then
    echo "$INSTANT_SNAPSHOTS" | while read -r isnap zone; do
        echo "  Deleting instant snapshot: $isnap (zone: $zone)"
        gcloud compute instant-snapshots delete "$isnap" --zone="$zone" --quiet 2>/dev/null || true
    done
else
    echo "  No instant snapshots found"
fi

echo "Cleaning GCP snapshots..."
SNAPSHOTS=$(gcloud compute snapshots list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$SNAPSHOTS" ]; then
    echo "$SNAPSHOTS" | while read -r snap; do
        echo "  Deleting snapshot: $snap"
        gcloud compute snapshots delete "$snap" --quiet 2>/dev/null || true
    done
else
    echo "  No snapshots found"
fi

# A policy still attached to a disk cannot be deleted, so detach first. A
# regional disk reports a region and no zone, and remove-resource-policies needs
# whichever one it actually has.
echo "Detaching resource policies from test disks..."
POLICY_DISKS=$(gcloud compute disks list --filter="name~^formae-" --format="value(name,zone,region,resourcePolicies[])" 2>/dev/null || true)
if [ -n "$POLICY_DISKS" ]; then
    echo "$POLICY_DISKS" | while read -r dk zone region policies; do
        [ -z "$policies" ] && continue
        if [ -n "$zone" ]; then
            SCOPE_FLAG="--zone=$zone"
        else
            SCOPE_FLAG="--region=$region"
        fi
        for pol in $(echo "$policies" | tr ';,' ' '); do
            [ -z "$pol" ] && continue
            echo "  Detaching $(basename "$pol") from $dk"
            gcloud compute disks remove-resource-policies "$dk" "$SCOPE_FLAG" --resource-policies="$pol" --quiet 2>/dev/null || true
        done
    done
else
    echo "  No disks with attached policies found"
fi

echo "Cleaning GCP resource policies..."
POLICIES=$(gcloud compute resource-policies list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$POLICIES" ]; then
    echo "$POLICIES" | while read -r policy region; do
        echo "  Deleting resource policy: $policy (region: $region)"
        gcloud compute resource-policies delete "$policy" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No resource policies found"
fi

# --- 1f. Logging sinks and Monitoring dashboards (leaf resources) ---
echo "Cleaning GCP logging sinks..."
SINKS=$(gcloud logging sinks list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$SINKS" ]; then
    echo "$SINKS" | while read -r sink; do
        echo "  Deleting log sink: $sink"
        gcloud logging sinks delete "$sink" --quiet 2>/dev/null || true
    done
else
    echo "  No logging sinks found"
fi

echo "Cleaning GCP log scopes..."
LOG_SCOPES=$(gcloud logging scopes list --location=global --filter="name~formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$LOG_SCOPES" ]; then
    echo "$LOG_SCOPES" | while read -r ls_; do
        echo "  Deleting log scope: $ls_"
        gcloud logging scopes delete "$(basename "$ls_")" --location=global --quiet 2>/dev/null || true
    done
else
    echo "  No log scopes found"
fi

echo "Cleaning GCP saved queries..."
SAVED_QUERIES=$(gcloud logging saved-queries list --location=global --filter="name~formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$SAVED_QUERIES" ]; then
    echo "$SAVED_QUERIES" | while read -r sq; do
        echo "  Deleting saved query: $sq"
        gcloud logging saved-queries delete "$sq" --location=global --quiet 2>/dev/null || true
    done
else
    echo "  No saved queries found"
fi

echo "Cleaning GCP log views..."
VIEWS=$(gcloud logging views list --bucket=_Default --location=global --filter="name~formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$VIEWS" ]; then
    echo "$VIEWS" | while read -r view; do
        echo "  Deleting log view: $view"
        gcloud logging views delete "$view" --bucket=_Default --location=global --quiet 2>/dev/null || true
    done
else
    echo "  No log views found"
fi

echo "Cleaning GCP logging exclusions..."
EXCLUSIONS=$(gcloud logging exclusions list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$EXCLUSIONS" ]; then
    echo "$EXCLUSIONS" | while read -r excl; do
        echo "  Deleting log exclusion: $excl"
        gcloud logging exclusions delete "$excl" --quiet 2>/dev/null || true
    done
else
    echo "  No logging exclusions found"
fi

echo "Cleaning GCP custom metric descriptors..."
METRIC_DESCRIPTORS=$(gcloud logging metrics list --format="value(name)" 2>/dev/null >/dev/null; gcloud monitoring metrics-descriptors list --filter="type~custom.googleapis.com/formae" --format="value(type)" 2>/dev/null || true)
if [ -n "$METRIC_DESCRIPTORS" ]; then
    echo "$METRIC_DESCRIPTORS" | while read -r md; do
        echo "  Deleting metric descriptor: $md"
        gcloud monitoring metrics-descriptors delete "$md" --quiet 2>/dev/null || true
    done
else
    echo "  No custom metric descriptors found"
fi

echo "Cleaning GCP monitoring custom services (deletes their SLOs too)..."
SERVICES=$(gcloud monitoring services list --filter="name~formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$SERVICES" ]; then
    echo "$SERVICES" | while read -r svc; do
        echo "  Deleting monitoring service: $svc"
        gcloud monitoring services delete "$svc" --quiet 2>/dev/null || true
    done
else
    echo "  No monitoring services found"
fi

echo "Cleaning GCP monitoring dashboards..."
DASHBOARDS=$(gcloud monitoring dashboards list --filter="displayName~Formae" --format="value(name)" 2>/dev/null || true)
if [ -n "$DASHBOARDS" ]; then
    echo "$DASHBOARDS" | while read -r dash; do
        echo "  Deleting dashboard: $dash"
        gcloud monitoring dashboards delete "$dash" --quiet 2>/dev/null || true
    done
else
    echo "  No monitoring dashboards found"
fi

# --- 1f2. HA VPN: tunnels -> gateways (VPN_GATEWAYS_PER_REGION is only 2, so a
# leftover gateway blocks the next run with QUOTA_EXCEEDED). Routers are cleaned
# with the other regional resources below.
echo "Cleaning GCP VPN tunnels..."
VPN_TUNNELS=$(gcloud compute vpn-tunnels list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$VPN_TUNNELS" ]; then
    echo "$VPN_TUNNELS" | while read -r tun region; do
        echo "  Deleting VPN tunnel: $tun (region: $region)"
        gcloud compute vpn-tunnels delete "$tun" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No VPN tunnels found"
fi

# Classic VPN gateways are a separate collection from the HA ones and hold their
# network, so they go before the network passes.
echo "Cleaning GCP target VPN gateways..."
TARGET_VPN_GWS=$(gcloud compute target-vpn-gateways list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$TARGET_VPN_GWS" ]; then
    echo "$TARGET_VPN_GWS" | while read -r gw region; do
        [ -z "$gw" ] && continue
        echo "  Deleting target VPN gateway: $gw ($region)"
        gcloud compute target-vpn-gateways delete "$gw" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No target VPN gateways found"
fi

echo "Cleaning GCP HA VPN gateways..."
VPN_GATEWAYS=$(gcloud compute vpn-gateways list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$VPN_GATEWAYS" ]; then
    echo "$VPN_GATEWAYS" | while read -r gw region; do
        echo "  Deleting HA VPN gateway: $gw (region: $region)"
        gcloud compute vpn-gateways delete "$gw" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No HA VPN gateways found"
fi

echo "Cleaning GCP external VPN gateways..."
# This collection pushes the filter server-side and the API rejects it
# ("Invalid list filter expression"), so the list came back empty and nothing
# was ever deleted - the gateways then sat on a quota of 5 and every VPN case
# failed. Filter client-side instead. Other compute collections accept it.
EXT_GATEWAYS=$(gcloud compute external-vpn-gateways list --format="value(name)" 2>/dev/null | grep -E "$SWEEP_RE" | grep -Ev "$KEEP_RE" || true)
if [ -n "$EXT_GATEWAYS" ]; then
    echo "$EXT_GATEWAYS" | while read -r egw; do
        echo "  Deleting external VPN gateway: $egw"
        gcloud compute external-vpn-gateways delete "$egw" --quiet 2>/dev/null || true
    done
else
    echo "  No external VPN gateways found"
fi

echo "Cleaning GCP cloud routers..."
ROUTERS=$(gcloud compute routers list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$ROUTERS" ]; then
    echo "$ROUTERS" | while read -r rtr region; do
        echo "  Deleting cloud router: $rtr (region: $region)"
        gcloud compute routers delete "$rtr" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No cloud routers found"
fi

# --- 1g. SSL policies (must delete after the proxies that reference them) ---
# Regional SSL policies are a separate collection from the global ones.
echo "Cleaning GCP regional SSL policies..."
REGION_SSL=$(gcloud compute ssl-policies list --format="value(name,region.basename())" 2>/dev/null | grep -E "$SWEEP_RE" | grep -Ev "$KEEP_RE" || true)
if [ -n "$REGION_SSL" ]; then
    echo "$REGION_SSL" | while read -r pol region; do
        [ -z "$region" ] && continue
        echo "  Deleting regional SSL policy: $pol (region: $region)"
        gcloud compute ssl-policies delete "$pol" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional SSL policies found"
fi

echo "Cleaning GCP SSL policies..."
SSL_POLICIES=$(gcloud compute ssl-policies list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$SSL_POLICIES" ]; then
    echo "$SSL_POLICIES" | while read -r pol; do
        echo "  Deleting SSL policy: $pol"
        gcloud compute ssl-policies delete "$pol" --quiet 2>/dev/null || true
    done
else
    echo "  No SSL policies found"
fi

# --- API Gateway (gateways, then configs, then apis) ---
# A config cannot be deleted while a gateway serves it, and an api cannot be
# deleted while it holds configs, so the hierarchy is torn down from the bottom.
# Every delete is a long-running operation; --async would leave the next delete
# racing the previous one, so these wait.
echo "Cleaning GCP API Gateway gateways..."
for agw_loc in "${GCP_REGION:-}" "${GCP_LOCATION:-}"; do
    [ -z "$agw_loc" ] && continue
    AGW_GW=$(gcloud api-gateway gateways list --location="$agw_loc" \
        --filter="name~formae-|name~formae-test" --format="value(name)" 2>/dev/null || true)
    if [ -n "$AGW_GW" ]; then
        echo "$AGW_GW" | while read -r gw; do
            echo "  Deleting API Gateway gateway: $gw (location: $agw_loc)"
            gcloud api-gateway gateways delete "$gw" --location="$agw_loc" --quiet 2>/dev/null || true
        done
    else
        echo "  No API Gateway gateways found in $agw_loc"
    fi
done

echo "Cleaning GCP API Gateway apis and their configs..."
AGW_APIS=$(gcloud api-gateway apis list --filter="name~formae-|name~formae-test" \
    --format="value(name)" 2>/dev/null || true)
if [ -n "$AGW_APIS" ]; then
    echo "$AGW_APIS" | while read -r agw_api; do
        AGW_CFGS=$(gcloud api-gateway api-configs list --api="$agw_api" --format="value(name)" 2>/dev/null || true)
        if [ -n "$AGW_CFGS" ]; then
            echo "$AGW_CFGS" | while read -r cfg; do
                echo "  Deleting API Gateway config: $cfg (api: $agw_api)"
                gcloud api-gateway api-configs delete "$cfg" --api="$agw_api" --quiet 2>/dev/null || true
            done
        fi
        echo "  Deleting API Gateway api: $agw_api"
        gcloud api-gateway apis delete "$agw_api" --quiet 2>/dev/null || true
    done
else
    echo "  No API Gateway apis found"
fi

# --- Memorystore for Memcached ---
# A memcached instance is billed by node-hour for as long as it exists, and it
# takes 20-30 minutes to create, so a leaked one is both a standing cost and a
# slow thing to notice.
echo "Cleaning GCP Memcache instances..."
for mc_loc in "${GCP_REGION:-}" "${GCP_LOCATION:-}"; do
    [ -z "$mc_loc" ] && continue
    MEMCACHE=$(gcloud memcache instances list --region="$mc_loc" \
        --filter="name~formae-|name~formae-test" --format="value(name)" 2>/dev/null || true)
    if [ -n "$MEMCACHE" ]; then
        echo "$MEMCACHE" | while read -r inst; do
            echo "  Deleting Memcache instance: $inst (region: $mc_loc)"
            gcloud memcache instances delete "$inst" --region="$mc_loc" --quiet 2>/dev/null || true
        done
    else
        echo "  No Memcache instances found in $mc_loc"
    fi
done

# --- Spanner (databases go with their instance) ---
# Unlike almost everything else swept here, a Spanner instance is billed for as
# long as it exists - the smallest regional one is 100 processing units - so a
# leaked instance is a standing cost rather than a tidiness problem. Deleting an
# instance takes its databases with it.
echo "Cleaning GCP Spanner instances..."
SPANNER=$(gcloud spanner instances list --filter="name~formae-|name~formae-test" \
    --format="value(name)" 2>/dev/null || true)
if [ -n "$SPANNER" ]; then
    echo "$SPANNER" | while read -r inst; do
        echo "  Deleting Spanner instance: $inst"
        gcloud spanner instances delete "$inst" --quiet 2>/dev/null || true
    done
else
    echo "  No Spanner instances found"
fi

# --- Service Directory (endpoints, then services, then namespaces) ---
# Deleting a namespace takes its services and endpoints with it, so only the
# namespaces need sweeping. goog-psc-default is Google-managed and does not
# match the test prefix.
echo "Cleaning GCP Service Directory namespaces..."
for sd_loc in "${GCP_REGION:-}" "${GCP_LOCATION:-}"; do
    [ -z "$sd_loc" ] && continue
    SD_NS=$(gcloud service-directory namespaces list --location="$sd_loc" \
        --filter="name~formae-|name~formae-test" --format="value(name)" 2>/dev/null || true)
    if [ -n "$SD_NS" ]; then
        echo "$SD_NS" | while read -r ns; do
            echo "  Deleting Service Directory namespace: $ns (location: $sd_loc)"
            gcloud service-directory namespaces delete "$ns" --location="$sd_loc" --quiet 2>/dev/null || true
        done
    else
        echo "  No Service Directory namespaces found in $sd_loc"
    fi
done

# --- 1h. SSL certificates (must delete after the proxies that reference them) ---
# A project holds at most 10 SSL certificates globally, and every leaked one
# counts. Once the cap is reached, any case that creates a certificate fails with
# "Quota 'SSL_CERTIFICATES' exceeded. Limit: 10.0 globally." rather than anything
# resembling a plugin bug - which is exactly how target-https-proxy failed on
# 2026-08-29, against eight certificates left behind since July. The sweep knew
# about ssl-policies but never about the certificates themselves.
# Certificates are swept only when asked. They are the one resource here whose
# removal is not obviously safe to decide automatically: unlike a namespace or an
# api, a certificate can be one someone installed deliberately, and the cap being
# global means a wrong deletion is felt project-wide. Set
# FORMAE_SWEEP_SSL_CERTIFICATES=1 to include them.
if [ "${FORMAE_SWEEP_SSL_CERTIFICATES:-0}" != "1" ]; then
    echo "Skipping GCP SSL certificates (set FORMAE_SWEEP_SSL_CERTIFICATES=1 to sweep them)"
else

echo "Cleaning GCP regional SSL certificates..."
REGION_CERTS=$(gcloud compute ssl-certificates list --format="value(name,region.basename())" 2>/dev/null | grep -E "$SWEEP_RE" | grep -Ev "$KEEP_RE" || true)
if [ -n "$REGION_CERTS" ]; then
    echo "$REGION_CERTS" | while read -r cert region; do
        [ -z "$region" ] && continue
        echo "  Deleting regional SSL certificate: $cert (region: $region)"
        gcloud compute ssl-certificates delete "$cert" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional SSL certificates found"
fi

echo "Cleaning GCP SSL certificates..."
SSL_CERTS=$(gcloud compute ssl-certificates list --global --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$SSL_CERTS" ]; then
    echo "$SSL_CERTS" | while read -r cert; do
        echo "  Deleting SSL certificate: $cert"
        gcloud compute ssl-certificates delete "$cert" --global --quiet 2>/dev/null || true
    done
else
    echo "  No SSL certificates found"
fi

fi

# --- 1h. Network attachments (hold a subnet reference, so delete before subnets) ---
# Service attachments hold forwarding-rule and PSC-subnet references, so they
# must go before the forwarding rules and subnets below.
echo "Cleaning GCP service attachments..."
SERVICE_ATTACHMENTS=$(gcloud compute service-attachments list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$SERVICE_ATTACHMENTS" ]; then
    echo "$SERVICE_ATTACHMENTS" | while read -r sa region; do
        echo "  Deleting service attachment: $sa (region: $region)"
        gcloud compute service-attachments delete "$sa" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No service attachments found"
fi

echo "Cleaning GCP network attachments..."
NET_ATTACHMENTS=$(gcloud compute network-attachments list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$NET_ATTACHMENTS" ]; then
    echo "$NET_ATTACHMENTS" | while read -r att region; do
        echo "  Deleting network attachment: $att (region: $region)"
        gcloud compute network-attachments delete "$att" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No network attachments found"
fi

# Peerings must be removed before their networks can be deleted.
echo "Cleaning GCP network peerings..."
PEER_NETS=$(gcloud compute networks list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$PEER_NETS" ]; then
    echo "$PEER_NETS" | while read -r net; do
        PEERINGS=$(gcloud compute networks peerings list --network="$net" --format="value(peerings[].name)" 2>/dev/null || true)
        for peering in $PEERINGS; do
            [ -z "$peering" ] && continue
            echo "  Deleting peering $peering on $net"
            gcloud compute networks peerings delete "$peering" --network="$net" --quiet 2>/dev/null || true
        done
    done
else
    echo "  No networks to check for peerings"
fi

# --- 2. Subnetworks (must delete before networks) ---
echo "Cleaning GCP subnetworks..."
SUBNETWORKS=$(gcloud compute networks subnets list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
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
GCE_INSTANCES=$(gcloud compute instances list --filter="name~^formae-" --format="value(name,zone)" 2>/dev/null || true)
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
DISKS=$(gcloud compute disks list --filter="name~^formae-" --format="value(name,zone)" 2>/dev/null || true)
if [ -n "$DISKS" ]; then
    echo "$DISKS" | while read -r disk zone; do
        echo "  Deleting disk: $disk (zone: $zone)"
        gcloud compute disks delete "$disk" --zone="$zone" --quiet 2>/dev/null || true
    done
else
    echo "  No disks found"
fi

# Regional disks report no zone, so the zonal loop above skips them.
echo "Cleaning GCP sole-tenant node templates..."
NODE_TEMPLATES=$(gcloud compute sole-tenancy node-templates list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$NODE_TEMPLATES" ]; then
    echo "$NODE_TEMPLATES" | while read -r nt region; do
        echo "  Deleting node template: $nt (region: $region)"
        gcloud compute sole-tenancy node-templates delete "$nt" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No node templates found"
fi

# A disk in active async replication cannot be deleted, so stop replication on
# any test primary before the disk passes below run.
echo "Stopping async replication on test disks..."
REPL_DISKS=$(gcloud compute disks list --filter="name~^formae- AND -zone:''" --format="value(name,zone)" 2>/dev/null || true)
if [ -n "$REPL_DISKS" ]; then
    echo "$REPL_DISKS" | while read -r dsk zone; do
        [ -z "$zone" ] && continue
        gcloud compute disks stop-async-replication "$dsk" --zone="$zone" --quiet 2>/dev/null || true
    done
else
    echo "  No zonal test disks to check"
fi

echo "Cleaning GCP regional disks..."
REGION_DISKS=$(gcloud compute disks list --filter="name~^formae- AND -zone:*" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$REGION_DISKS" ]; then
    echo "$REGION_DISKS" | while read -r disk region; do
        echo "  Deleting regional disk: $disk (region: $region)"
        gcloud compute disks delete "$disk" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional disks found"
fi

# --- Target TCP proxies (global, must be deleted before their backend services) ---
# Target instances hold a VM reference, so they go before the instances loop
# above can succeed on a re-run; they are zonal.
echo "Cleaning GCP target instances..."
TARGET_INSTANCES=$(gcloud compute target-instances list --filter="name~^formae-" --format="value(name,zone)" 2>/dev/null || true)
if [ -n "$TARGET_INSTANCES" ]; then
    echo "$TARGET_INSTANCES" | while read -r ti zone; do
        echo "  Deleting target instance: $ti (zone: $zone)"
        gcloud compute target-instances delete "$ti" --zone="$zone" --quiet 2>/dev/null || true
    done
else
    echo "  No target instances found"
fi

# Target gRPC proxies hold a url map reference, so they go before the url maps
# loop; the fixture's map and backend service are prerequisites that Destroy
# leaves behind.
echo "Cleaning GCP target gRPC proxies..."
TARGET_GRPC_PROXIES=$(gcloud compute target-grpc-proxies list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$TARGET_GRPC_PROXIES" ]; then
    echo "$TARGET_GRPC_PROXIES" | while read -r tgp; do
        echo "  Deleting target gRPC proxy: $tgp"
        gcloud compute target-grpc-proxies delete "$tgp" --quiet 2>/dev/null || true
    done
else
    echo "  No target gRPC proxies found"
fi

echo "Cleaning GCP target TCP proxies..."
TARGET_TCP_PROXIES=$(gcloud compute target-tcp-proxies list --filter="name~^formae-" --global --format="value(name)" 2>/dev/null || true)
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
GLOBAL_FORWARDING_RULES=$(gcloud compute forwarding-rules list --filter="name~^formae-" --global --format="value(name)" 2>/dev/null || true)
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
REGIONAL_FORWARDING_RULES=$(gcloud compute forwarding-rules list --filter="name~^formae-" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==2' || true)
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
REGIONAL_HTTP_PROXIES=$(gcloud compute target-http-proxies list --filter="name~^formae-" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==2' || true)
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
TARGET_HTTP_PROXIES=$(gcloud compute target-http-proxies list --filter="name~^formae-" --global --format="value(name)" 2>/dev/null || true)
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
URL_MAPS=$(gcloud compute url-maps list --filter="name~^formae-" --global --format="value(name)" 2>/dev/null || true)
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
REGIONAL_URL_MAPS=$(gcloud compute url-maps list --filter="name~^formae-" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==2' || true)
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
BACKEND_SERVICES=$(gcloud compute backend-services list --filter="name~^formae-" --global --format="value(name)" 2>/dev/null || true)
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
REGIONAL_BACKEND_SERVICES=$(gcloud compute backend-services list --filter="name~^formae-" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==2' || true)
if [ -n "$REGIONAL_BACKEND_SERVICES" ]; then
    echo "$REGIONAL_BACKEND_SERVICES" | while read -r bs region; do
        echo "  Deleting regional backend service: $bs (region: $region)"
        gcloud compute backend-services delete "$bs" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No regional backend services found"
fi

# --- Health checks (global) ---
echo "Cleaning GCP legacy HTTPS health checks..."
HTTPS_HCS=$(gcloud compute https-health-checks list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$HTTPS_HCS" ]; then
    echo "$HTTPS_HCS" | while read -r hc; do
        echo "  Deleting legacy HTTPS health check: $hc"
        gcloud compute https-health-checks delete "$hc" --quiet 2>/dev/null || true
    done
else
    echo "  No legacy HTTPS health checks found"
fi

echo "Cleaning GCP legacy HTTP health checks..."
HTTP_HCS=$(gcloud compute http-health-checks list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$HTTP_HCS" ]; then
    echo "$HTTP_HCS" | while read -r hc; do
        echo "  Deleting legacy HTTP health check: $hc"
        gcloud compute http-health-checks delete "$hc" --quiet 2>/dev/null || true
    done
else
    echo "  No legacy HTTP health checks found"
fi

# Global (internet) NEGs are separate from the zonal/regional ones. Deleting one
# takes its attached endpoints with it (verified against the API), so no detach
# pass is needed here.
echo "Cleaning GCP global network endpoint groups..."
GLOBAL_NEGS=$(gcloud compute network-endpoint-groups list --global --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$GLOBAL_NEGS" ]; then
    echo "$GLOBAL_NEGS" | while read -r neg; do
        echo "  Deleting global network endpoint group: $neg"
        gcloud compute network-endpoint-groups delete "$neg" --global --quiet 2>/dev/null || true
    done
else
    echo "  No global network endpoint groups found"
fi

echo "Cleaning GCP health checks..."
HEALTH_CHECKS=$(gcloud compute health-checks list --filter="name~^formae-" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==1{print $1}' || true)
if [ -n "$HEALTH_CHECKS" ]; then
    echo "$HEALTH_CHECKS" | while read -r hc; do
        echo "  Deleting health check: $hc"
        gcloud compute health-checks delete "$hc" --global --quiet 2>/dev/null || true
    done
else
    echo "  No health checks found"
fi

# --- Regional health checks (region-http-lb test) ---
# A composite health check references health sources, so it goes before them.
# Project metadata is shared, project-wide state, so this removes only keys the
# test fixtures own and never touches anything else (enable-oslogin, ssh-keys).
# Artifact Registry repositories (and the rules inside them, which go with the
# repository). Both the repository and rule fixtures leave one behind when a run
# is killed, and neither the repository nor its rules are named by the compute
# passes above.
# Eventarc Advanced message buses. These live in a region Eventarc Advanced
# supports rather than GCP_LOCATION, so the fixture's region is listed explicitly
# instead of being inherited.
# Pipelines hold their destination bus, so they go first. Only one bus is allowed
# per project per region, so a leftover bus fails the next run's create outright.
# Workflows definitions. Free to keep, but they are test debris.
# Dataproc session templates. Free to keep, but they are test debris. Note these
# are location-scoped, unlike autoscaling policies.
echo "Cleaning GCP dataproc session templates..."
STS=$(gcloud dataproc session-templates list --location="${GCP_LOCATION:-europe-central2}" --format="value(name)" 2>/dev/null | grep -E "$SWEEP_RE" | grep -Ev "$KEEP_RE" || true)
if [ -n "$STS" ]; then
    echo "$STS" | while read -r st; do
        [ -z "$st" ] && continue
        echo "  Deleting session template: $(basename "$st")"
        gcloud dataproc session-templates delete "$(basename "$st")" --location="${GCP_LOCATION:-europe-central2}" --quiet 2>/dev/null || true
    done
else
    echo "  No dataproc session templates found"
fi

echo "Cleaning GCP workflows..."
WFS=$(gcloud workflows list --location="${GCP_LOCATION:-europe-central2}" --format="value(name)" 2>/dev/null | grep -E "$SWEEP_RE" | grep -Ev "$KEEP_RE" || true)
if [ -n "$WFS" ]; then
    echo "$WFS" | while read -r wf; do
        [ -z "$wf" ] && continue
        echo "  Deleting workflow: $wf"
        gcloud workflows delete "$wf" --location="${GCP_LOCATION:-europe-central2}" --quiet 2>/dev/null || true
    done
else
    echo "  No workflows found"
fi

# This gcloud has no eventarc message-buses/pipelines/enrollments/
# google-api-sources surface at all, so the sweep that used to live here was a
# silent no-op: the missing subcommand went to /dev/null and every leftover
# survived. clean-eventarc-case.sh already talks REST for exactly this reason,
# so reuse it rather than keeping a second, dead copy.
echo "Cleaning GCP eventarc Advanced resources..."
"$(dirname "$0")/clean-eventarc-case.sh" all

# Analytics Hub and private CA get their own scripts for the same reason:
# gcloud cannot see those resources either, and private CA leaks cost money.
"$(dirname "$0")/clean-analyticshub.sh"

"$(dirname "$0")/clean-privateca.sh"

# Datastream private connections hold a reserved /29 until they are really
# gone, so a leak blocks the next run's peering, not just tidiness.
"$(dirname "$0")/clean-datastream-case.sh" all

# A leaked Filestore instance is the most expensive leak in the suite: a
# BASIC_HDD instance has a 1 TiB minimum and is billed per GiB-month.
"$(dirname "$0")/clean-filestore-case.sh" all

echo "Cleaning GCP artifact registry repositories..."
AR_REPOS=$(gcloud artifacts repositories list --format="value(name)" 2>/dev/null | grep -E '(^|/)formae-' || true)
if [ -n "$AR_REPOS" ]; then
    echo "$AR_REPOS" | while read -r repo; do
        [ -z "$repo" ] && continue
        short=$(basename "$repo")
        loc=$(echo "$repo" | awk -F/ '{for(i=1;i<=NF;i++) if($i=="locations") print $(i+1)}')
        [ -z "$loc" ] && loc="${GCP_LOCATION:-europe-central2}"
        echo "  Deleting artifact repository: $short ($loc)"
        gcloud artifacts repositories delete "$short" --location="$loc" --quiet 2>/dev/null || true
    done
else
    echo "  No artifact registry repositories found"
fi

echo "Cleaning GCP project metadata test keys..."
PMI_KEYS=$(gcloud compute project-info describe --format="value(commonInstanceMetadata.items[].key)" 2>/dev/null | tr ';,' '\n' | grep -E "$SWEEP_RE" | grep -Ev "$KEEP_RE" || true)
if [ -n "$PMI_KEYS" ]; then
    echo "$PMI_KEYS" | while read -r mkey; do
        [ -z "$mkey" ] && continue
        echo "  Removing project metadata key: $mkey"
        gcloud compute project-info remove-metadata --keys="$mkey" --quiet 2>/dev/null || true
    done
else
    echo "  No project metadata test keys found"
fi

echo "Cleaning GCP composite health checks..."
CHCS=$(gcloud compute composite-health-checks list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$CHCS" ]; then
    echo "$CHCS" | while read -r chc region; do
        echo "  Deleting composite health check: $chc (region: $region)"
        gcloud compute composite-health-checks delete "$chc" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No composite health checks found"
fi

# Health sources reference an aggregation policy, so they go first.
echo "Cleaning GCP health sources..."
HEALTH_SOURCES=$(gcloud compute health-sources list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$HEALTH_SOURCES" ]; then
    echo "$HEALTH_SOURCES" | while read -r hs region; do
        echo "  Deleting health source: $hs (region: $region)"
        gcloud compute health-sources delete "$hs" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No health sources found"
fi

echo "Cleaning GCP health aggregation policies..."
HAPS=$(gcloud compute health-aggregation-policies list --filter="name~^formae-" --format="value(name,region)" 2>/dev/null || true)
if [ -n "$HAPS" ]; then
    echo "$HAPS" | while read -r hap region; do
        echo "  Deleting health aggregation policy: $hap (region: $region)"
        gcloud compute health-aggregation-policies delete "$hap" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No health aggregation policies found"
fi

echo "Cleaning GCP regional health checks..."
REGIONAL_HEALTH_CHECKS=$(gcloud compute health-checks list --filter="name~^formae-" --format="value(name,region.basename())" 2>/dev/null | awk 'NF==2' || true)
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
    PSA_NETWORKS=$(gcloud compute networks list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
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
GLOBAL_ADDRESSES=$(gcloud compute addresses list --global --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$GLOBAL_ADDRESSES" ]; then
    echo "$GLOBAL_ADDRESSES" | while read -r addr; do
        echo "  Deleting global address: $addr"
        gcloud compute addresses delete "$addr" --global --quiet 2>/dev/null || true
    done
else
    echo "  No global addresses found"
fi

# --- 6c. Serverless VPC Access connectors (attached to a network, so delete
#         before networks). Connector names are capped at 25 chars, so the
#         testdata name is "formae-test-conn-<runID>", not the long
#         formae-plugin-sdk prefix the other filters use. ---
echo "Cleaning GCP VPC Access connectors..."
VPC_CONNECTORS=$(gcloud compute networks vpc-access connectors list --region="${GCP_REGION:-europe-central2}" --filter="name~formae-test-conn" --format="value(name.basename())" 2>/dev/null || true)
if [ -n "$VPC_CONNECTORS" ]; then
    echo "$VPC_CONNECTORS" | while read -r conn; do
        echo "  Deleting VPC Access connector: $conn"
        gcloud compute networks vpc-access connectors delete "$conn" --region="${GCP_REGION:-europe-central2}" --quiet 2>/dev/null || true
    done
else
    echo "  No VPC Access connectors found"
fi

# --- 7. Networks (after firewalls and subnetworks are deleted) ---
echo "Cleaning GCP networks..."
NETWORKS=$(gcloud compute networks list --filter="name~^formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$NETWORKS" ]; then
    echo "$NETWORKS" | while read -r network; do
        echo "  Deleting network: $network"
        gcloud compute networks delete "$network" --quiet 2>/dev/null || true
    done
else
    echo "  No networks found"
fi

# --- 8. Storage buckets ---
# formae-probe- covers resources created by hand while investigating a failure.
# Those are as disposable as a test's own, but they carry neither test prefix, so
# nothing swept them and they outlived every run - and the credentials used to
# create one often cannot delete it.
echo "Cleaning GCP storage buckets..."
BUCKETS=$(gcloud storage buckets list --filter="name~^formae--test OR name~^formae-probe-" --format="value(name)" 2>/dev/null || true)
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
INSTANCES=$(gcloud bigtable instances list --filter="name~^formae-test-instance" --format="value(name)" 2>/dev/null || true)
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
# AlloyDB bills per instance-minute, so a leaked cluster is expensive, not just
# untidy. Instances must go before their cluster.
# Backups outlive their cluster, so they are cleaned independently.
echo "Cleaning GCP AlloyDB backups..."
# "value(name,region)" yields an empty region column - the region lives only
# inside the resource path - so a delete built from it ran with --region="" and
# failed silently. Parse the region out of the path instead.
ALLOYDB_BACKUPS=$(gcloud alloydb backups list --region=- --filter="name~formae-test OR name~formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$ALLOYDB_BACKUPS" ]; then
    echo "$ALLOYDB_BACKUPS" | while read -r bkp; do
        region=$(echo "$bkp" | sed -E 's#.*/locations/([^/]+)/.*#\1#')
        echo "  Deleting AlloyDB backup: $(basename "$bkp") (region: $region)"
        gcloud alloydb backups delete "$(basename "$bkp")" --region="$region" --quiet 2>/dev/null || true
    done
else
    echo "  No AlloyDB backups found"
fi

echo "Cleaning GCP AlloyDB instances and clusters..."
ALLOYDB_CLUSTERS=$(gcloud alloydb clusters list --region=- --filter="name~formae-test OR name~formae-" --format="value(name)" 2>/dev/null || true)
if [ -n "$ALLOYDB_CLUSTERS" ]; then
    echo "$ALLOYDB_CLUSTERS" | while read -r cluster; do
        region=$(echo "$cluster" | sed -E 's#.*/locations/([^/]+)/.*#\1#')
        cname=$(basename "$cluster")
        INSTS=$(gcloud alloydb instances list --cluster="$cname" --region="$region" --format="value(name)" 2>/dev/null || true)
        if [ -n "$INSTS" ]; then
            echo "$INSTS" | while read -r inst; do
                echo "  Deleting AlloyDB instance: $(basename "$inst") (cluster: $cname)"
                gcloud alloydb instances delete "$(basename "$inst")" --cluster="$cname" --region="$region" --quiet 2>/dev/null || true
            done
        fi
        USERS=$(gcloud alloydb users list --cluster="$cname" --region="$region" --format="value(name)" 2>/dev/null || true)
        if [ -n "$USERS" ]; then
            echo "$USERS" | while read -r usr; do
                echo "  Deleting AlloyDB user: $(basename "$usr") (cluster: $cname)"
                gcloud alloydb users delete "$(basename "$usr")" --cluster="$cname" --region="$region" --quiet 2>/dev/null || true
            done
        fi
        echo "  Deleting AlloyDB cluster: $cname (region: $region)"
        gcloud alloydb clusters delete "$cname" --region="$region" --force --quiet 2>/dev/null || true
    done
else
    echo "  No AlloyDB clusters found"
fi

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
SECRETS=$(gcloud secrets list --filter="name~^formae--test" --format="value(name)" 2>/dev/null || true)
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
SERVICE_ACCOUNTS=$(gcloud iam service-accounts list --filter="email~^formae-" --format="value(email)" 2>/dev/null || true)
if [ -n "$SERVICE_ACCOUNTS" ]; then
    echo "$SERVICE_ACCOUNTS" | while read -r sa; do
        echo "  Deleting service account: $sa"
        gcloud iam service-accounts delete "$sa" --quiet 2>/dev/null || true
    done
else
    echo "  No service accounts found"
fi

# --- 13. Service Directory namespaces (servicedirectory-* tests). Deleting a
#         namespace deletes the services and endpoints under it, so this one
#         sweep covers all three cases. The service and endpoint fixtures build
#         a namespace as a prerequisite and conformance Destroy only removes the
#         resource under test, so those namespaces outlive every run. ---
echo "Cleaning GCP Service Directory namespaces..."
# --filter is server-side here and Service Directory rejects the "~" operator
# ("Failed to parse the filter"), so the prefix is matched with grep instead.
SD_NAMESPACES=$(gcloud service-directory namespaces list --location="${GCP_LOCATION:-${GCP_REGION:-europe-central2}}" --format="value(name.basename())" 2>/dev/null | grep -E "$SWEEP_RE" | grep -Ev "$KEEP_RE" || true)
if [ -n "$SD_NAMESPACES" ]; then
    echo "$SD_NAMESPACES" | while read -r ns; do
        echo "  Deleting Service Directory namespace: $ns"
        gcloud service-directory namespaces delete "$ns" --location="${GCP_LOCATION:-${GCP_REGION:-europe-central2}}" --quiet 2>/dev/null || true
    done
else
    echo "  No Service Directory namespaces found"
fi

# --- 14. Spanner instances (spanner-* tests). Deleting an instance deletes the
#         databases and backup schedules under it, so this one sweep covers all
#         three cases. Unlike most leftovers these are **billed by the hour**:
#         the database and backup-schedule fixtures each build an instance as a
#         prerequisite, and conformance Destroy only removes the resource under
#         test, so every run of those two cases leaves one running. ---
echo "Cleaning GCP Spanner instances..."
SPANNER_INSTANCES=$(gcloud spanner instances list --format="value(name)" 2>/dev/null | grep '^formae-' || true)
if [ -n "$SPANNER_INSTANCES" ]; then
    echo "$SPANNER_INSTANCES" | while read -r inst; do
        echo "  Deleting Spanner instance: $inst"
        gcloud spanner instances delete "$inst" --quiet 2>&1 | tail -1 || true
    done
else
    echo "  No Spanner instances found"
fi

echo ""
echo "clean-environment.sh: Cleanup complete"
