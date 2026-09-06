#!/bin/sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
AGENT="$SCRIPT_DIR/agent.sh"
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/agent-runtime-config.XXXXXX")
trap 'rm -rf "$TEST_ROOT"' EXIT HUP INT TERM

write_config() {
    config_file="$1"
    shift
    printf '%s\n' "$@" > "$config_file"
}

run_config_test() {
    NERVE_CONFIG_FILE="$1" DEVICE_ID_FILE="$TEST_ROOT/device-id" \
        sh "$AGENT" --self-test-runtime-config
}

write_config "$TEST_ROOT/https.conf" \
    'CONTROLLER_URL="https://controller.example.com:8443/api"' \
    'REQUIRE_TLS="true"'
run_config_test "$TEST_ROOT/https.conf"

write_config "$TEST_ROOT/http-required.conf" \
    'CONTROLLER_URL="http://controller.example.com:3000/api"' \
    'REQUIRE_TLS="true"'
if run_config_test "$TEST_ROOT/http-required.conf"; then
    echo "HTTP controller URL was accepted when TLS was required" >&2
    exit 1
fi

write_config "$TEST_ROOT/http-legacy.conf" \
    'CONTROLLER_URL="http://controller.example.com:3000/api"' \
    'REQUIRE_TLS="false"'
run_config_test "$TEST_ROOT/http-legacy.conf"

write_config "$TEST_ROOT/invalid.conf" \
    'CONTROLLER_URL="controller.example.com:8443/api"' \
    'REQUIRE_TLS="true"'
if run_config_test "$TEST_ROOT/invalid.conf"; then
    echo "controller URL without a scheme was accepted" >&2
    exit 1
fi

write_config "$TEST_ROOT/missing-ca.conf" \
    'CONTROLLER_URL="https://controller.example.com:8443/api"' \
    'REQUIRE_TLS="true"' \
    "CONTROLLER_CA_FILE=\"$TEST_ROOT/missing-ca.pem\""
if run_config_test "$TEST_ROOT/missing-ca.conf"; then
    echo "missing controller CA file was accepted" >&2
    exit 1
fi

touch "$TEST_ROOT/ca.pem"
write_config "$TEST_ROOT/ca.conf" \
    'CONTROLLER_URL="https://controller.example.com:8443/api"' \
    'REQUIRE_TLS="true"' \
    "CONTROLLER_CA_FILE=\"$TEST_ROOT/ca.pem\""
run_config_test "$TEST_ROOT/ca.conf"

echo "agent runtime transport contract passed"
