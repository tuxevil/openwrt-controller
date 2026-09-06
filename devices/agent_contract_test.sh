#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
AGENT="$SCRIPT_DIR/agent.sh"

grep -q 'DEVICE_TOKEN_FILE=.*\/etc\/nerve-device-token' "$AGENT"
grep -q 'DEVICE_ID_FILE=.*\/etc\/nerve-device-id' "$AGENT"
grep -q 'ENROLLMENT_TOKEN_FILE=' "$AGENT"
grep -q 'device-enrollment' "$AGENT"
grep -q 'chmod 600 "\$DEVICE_ID_FILE"' "$AGENT"
grep -q 'chmod 600 "\$DEVICE_TOKEN_FILE"' "$AGENT"
grep -q '"capabilities"' "$AGENT"
grep -q 'CAP_INTERFACES' "$AGENT"
grep -q 'CAP_RADIOS' "$AGENT"
grep -q 'CAP_PACKAGES' "$AGENT"
grep -q 'X-Device-Token:' "$AGENT"
grep -q 'bootstrap_agent()' "$AGENT"
grep -q 'site enrollment token is missing' "$AGENT"
grep -q 'exit 1' "$AGENT"

# Runtime device-scoped calls must use the per-device token. The site key is
# reserved for bootstrap and update discovery.
grep -q 'TELEMETRY_HEADERS="-H X-Device-Token:' "$AGENT"
grep -q 'CONFIG_HEADERS="-H X-Device-Token:' "$AGENT"
grep -q 'CONFIG_HTTP_CODE=' "$AGENT"
grep -q 'AGENT_UPDATE_PUBLIC_KEY_FILE=' "$AGENT"
grep -q 'signature_algorithm' "$AGENT"
grep -q 'openssl pkeyutl -verify' "$AGENT"
if grep -q 'SITE_KEY' "$AGENT"; then
    echo "signed agent must not contain a site-wide credential" >&2
    exit 1
fi
if grep -q 'SIGNATURE_DECODED' "$AGENT"; then
    echo "agent must not move binary signatures through shell variables" >&2
    exit 1
fi
if grep -q 'elif \[ -z "\$LATEST_SIGNATURE" \]' "$AGENT"; then
    echo "agent must reject unsigned updates after trust is configured" >&2
    exit 1
fi
grep -Eq '^AGENT_UPDATE_PUBLIC_KEY="[A-Za-z0-9+/=]+"' "$SCRIPT_DIR/99-nerve-center-bootstrap"
grep -q 'AGENT_UPDATE_PUBLIC_KEY=' "$SCRIPT_DIR/99-nerve-center-bootstrap"
grep -q 'verify_agent_artifact' "$SCRIPT_DIR/99-nerve-center-bootstrap"
grep -q 'if ! chmod 755 "${AGENT_DEST}.tmp"' "$SCRIPT_DIR/99-nerve-center-bootstrap"
grep -q 'if ! mv "${AGENT_DEST}.tmp" "$AGENT_DEST"' "$SCRIPT_DIR/99-nerve-center-bootstrap"
grep -q 'Could not start the agent service' "$SCRIPT_DIR/99-nerve-center-bootstrap"
if grep -q 'sed -i' "$SCRIPT_DIR/99-nerve-center-bootstrap"; then
    echo "bootstrap must not mutate the signed agent artifact" >&2
    exit 1
fi
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

# The bootstrap must download with the short-lived enrollment token, not a
# site-wide API key or an invented device token.
grep -q 'X-Site-Enrollment-Token: \$SITE_ENROLLMENT_TOKEN' "$SCRIPT_DIR/99-nerve-center-bootstrap"
if grep -q 'SITE_KEY' "$SCRIPT_DIR/99-nerve-center-bootstrap"; then
    echo "bootstrap must not persist a site-wide API key" >&2
    exit 1
fi

echo "agent provisioning contract passed"
