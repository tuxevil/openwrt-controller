#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
AGENT="$SCRIPT_DIR/agent.sh"

grep -q 'DEVICE_TOKEN_FILE=.*\/etc\/nerve-device-token' "$AGENT"
grep -q 'DEVICE_ID_FILE=.*\/etc\/nerve-device-id' "$AGENT"
grep -q 'chmod 600 "\$DEVICE_ID_FILE"' "$AGENT"
grep -q 'chmod 600 "\$DEVICE_TOKEN_FILE"' "$AGENT"
grep -q '"capabilities"' "$AGENT"
grep -q 'CAP_INTERFACES' "$AGENT"
grep -q 'CAP_RADIOS' "$AGENT"
grep -q 'CAP_PACKAGES' "$AGENT"
grep -q 'X-Device-Token:' "$AGENT"
grep -q 'bootstrap_agent()' "$AGENT"
grep -q 'Device token missing; starting bootstrap provisioning' "$AGENT"
grep -q 'exit 1' "$AGENT"

# Runtime device-scoped calls must use the per-device token. The site key is
# reserved for bootstrap and update discovery.
grep -q 'TELEMETRY_HEADERS="-H X-Device-Token:' "$AGENT"
grep -q 'CONFIG_HEADERS="-H X-Device-Token:' "$AGENT"
grep -q 'CONFIG_HTTP_CODE=' "$AGENT"
grep -q 'AGENT_UPDATE_PUBLIC_KEY=' "$AGENT"
grep -q 'signature_algorithm' "$AGENT"
grep -q 'openssl pkeyutl -verify' "$AGENT"
grep -Eq '^AGENT_UPDATE_PUBLIC_KEY="[A-Za-z0-9+/=]+"' "$SCRIPT_DIR/99-nerve-center-bootstrap"
grep -q 'AGENT_UPDATE_PUBLIC_KEY=' "$SCRIPT_DIR/99-nerve-center-bootstrap"
grep -q 'AGENT_UPDATE_SIGNING_KEY' "$SCRIPT_DIR/../docs/AGENT_SIGNING.md"

# Wireless changes must have a durable local journal and must recover before
# the agent makes its first network request.
grep -q 'NERVE_TRANSACTION_ROOT=' "$AGENT"
grep -q 'transaction_recover_pending' "$AGENT"
grep -q 'transaction_begin wireless' "$AGENT"
grep -q 'transaction_mark_pending wireless' "$AGENT"
grep -q 'transaction_commit wireless' "$AGENT"
grep -q 'NERVE_WIFI_HASH_FILE' "$AGENT"
grep -q 'NERVE_WG_HASH_FILE' "$AGENT"
grep -q 'NERVE_OPERATION_STATUS_FILE' "$AGENT"
grep -q 'apply_pending_operation' "$AGENT"
grep -q 'operation_health_check' "$AGENT"
grep -q 'operation_apply_command' "$AGENT"
grep -q 'delete_all' "$AGENT"
grep -q 'apply_operation' "$AGENT"

# The bootstrap must download with the site key, not invent a device token.
grep -q 'X-Site-Key: \$SITE_KEY' "$SCRIPT_DIR/99-nerve-center-bootstrap"

echo "agent provisioning contract passed"
