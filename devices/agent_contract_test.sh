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

# The bootstrap must download with the site key, not invent a device token.
grep -q 'X-Site-Key: \$SITE_KEY' "$SCRIPT_DIR/99-nerve-center-bootstrap"

echo "agent provisioning contract passed"
