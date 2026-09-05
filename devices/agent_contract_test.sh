#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
AGENT="$SCRIPT_DIR/agent.sh"

grep -q 'DEVICE_TOKEN_FILE="/etc/nerve-device-token"' "$AGENT"
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
grep -q 'AGENT_UPDATE_PUBLIC_KEY=' "$AGENT"
grep -q 'signature_algorithm' "$AGENT"
grep -q 'openssl pkeyutl -verify' "$AGENT"
grep -q 'AGENT_UPDATE_PUBLIC_KEY="REPLACE_WITH_ED25519_PUBLIC_KEY_BASE64"' "$SCRIPT_DIR/99-nerve-center-bootstrap"
grep -q 'AGENT_UPDATE_PUBLIC_KEY=' "$SCRIPT_DIR/99-nerve-center-bootstrap"

# The bootstrap must download with the site key, not invent a device token.
grep -q 'X-Site-Key: \$SITE_KEY' "$SCRIPT_DIR/99-nerve-center-bootstrap"

echo "agent provisioning contract passed"
