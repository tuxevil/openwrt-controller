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
grep -q '"device_change_set":{"version":2,"namespaces":\["system","dhcp","firewall","dropbear","sqm"\],"max_operations":8' "$AGENT"
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
grep -q 'AGENT_VERSION_NUMBER_FILE=' "$AGENT"
grep -q 'not newer than the installed version' "$AGENT"
grep -q 'signature_algorithm' "$AGENT"
grep -q 'openssl pkeyutl -verify' "$AGENT"
grep -q 'CONTROLLER_URL=' "$AGENT"
grep -q 'REQUIRE_TLS=' "$AGENT"
grep -q 'CONTROLLER_CA_FILE=' "$AGENT"
grep -q 'controller_curl()' "$AGENT"
grep -q 'transaction_valid_generation()' "$AGENT"
grep -q 'operation_generation=' "$AGENT"
grep -q '"generation":%s' "$AGENT"
if grep -q 'CONTROLLER_IP' "$AGENT"; then
    echo "signed agent must use a complete CONTROLLER_URL" >&2
    exit 1
fi
if grep -q 'SITE_KEY' "$AGENT"; then
    echo "signed agent must not contain a site-wide credential" >&2
    exit 1
fi
if grep -q 'SIGNATURE_DECODED' "$AGENT"; then
    echo "agent must not move binary signatures through shell variables" >&2
    exit 1
fi
if grep -q 'paste -sd' "$AGENT"; then
    echo "agent telemetry must not depend on the optional paste utility" >&2
    exit 1
fi
grep -q 'join_csv()' "$AGENT"
grep -q 'decode_base64()' "$AGENT"
grep -q 'decode_base64()' "$SCRIPT_DIR/99-nerve-center-bootstrap"
if grep -Eq '^[[:space:]]*(apk|opkg)[[:space:]]+(update|install|add|del|remove)' "$AGENT"; then
    echo "agent must not mutate the package database at runtime" >&2
    exit 1
fi
grep -q '^CONTROLLER_URL="https://' "$SCRIPT_DIR/99-nerve-center-bootstrap"
grep -q '^REQUIRE_TLS="true"$' "$SCRIPT_DIR/99-nerve-center-bootstrap"
grep -q '^ROOT_PASSWORD=""' "$SCRIPT_DIR/99-nerve-center-bootstrap"
grep -q 'controller_curl()' "$SCRIPT_DIR/99-nerve-center-bootstrap"
PAYLOAD_LINE=$(awk '/^[[:space:]]*PAYLOAD=\$\(cat <<EOF$/ {print NR; exit}' "$AGENT")
NEIGHBOR_DEFAULT_LINE=$(awk '/^[[:space:]]*NEIGHBOR_APS="\[\]"$/ {print NR; exit}' "$AGENT")
if [ -z "$PAYLOAD_LINE" ] || [ -z "$NEIGHBOR_DEFAULT_LINE" ] || [ "$NEIGHBOR_DEFAULT_LINE" -ge "$PAYLOAD_LINE" ]; then
    echo "agent must initialize neighbor_aps before the first telemetry payload" >&2
    exit 1
fi
RUNTIME_PREFLIGHT_LINE=$(awk '/Runtime configuration or device identity is missing/ {print NR; exit}' "$AGENT")
for self_test in --self-test-signature --self-test-transaction --self-test-operation --self-test-change-set --self-test-status --self-test-change-set-transition --self-test-runtime-config --self-test-log-collection; do
    SELF_TEST_LINE=$(awk -v flag="$self_test" 'index($0, flag) {print NR; exit}' "$AGENT")
    if [ -z "$RUNTIME_PREFLIGHT_LINE" ] || [ -z "$SELF_TEST_LINE" ] || [ "$SELF_TEST_LINE" -ge "$RUNTIME_PREFLIGHT_LINE" ]; then
        echo "$self_test must dispatch before runtime preflight" >&2
        exit 1
    fi
done
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
grep -q 'apply_change_set' "$AGENT"
grep -q 'apply_pending_change_set' "$AGENT"
grep -q 'change_set_transaction' "$AGENT"
grep -q 'change_set_status_write' "$AGENT"
grep -q 'change_set_content_plan_hash' "$AGENT"

# The bootstrap must download with the short-lived enrollment token, not a
# site-wide API key or an invented device token.
grep -q 'X-Site-Enrollment-Token: \$SITE_ENROLLMENT_TOKEN' "$SCRIPT_DIR/99-nerve-center-bootstrap"
if grep -q 'SITE_KEY' "$SCRIPT_DIR/99-nerve-center-bootstrap"; then
    echo "bootstrap must not persist a site-wide API key" >&2
    exit 1
fi

echo "agent provisioning contract passed"
