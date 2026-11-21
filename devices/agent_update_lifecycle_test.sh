#!/bin/sh
set -eu

agent=${AGENT_UNDER_TEST:-$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/agent.sh}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

awk '
    /^    if \[ "\$HTTP_CODE" = "202" \]; then$/ { reading = 1 }
    reading && /^    else$/ { print "    fi"; exit }
    reading { print }
' "$agent" >"$work/agent.sh"
test "$(wc -l <"$work/agent.sh")" -gt 3
sh -n "$work/agent.sh"

printf 'previous agent\n' >"$work/agent.sh.old"
printf '1\n' >"$work/version.old"
printf '2\n' >"$work/version"
HTTP_CODE=202 AGENT_VERSION_NUMBER_FILE="$work/version" sh "$work/agent.sh"
test ! -e "$work/agent.sh.old"
test ! -e "$work/version.old"
test "$(cat "$work/version")" = 2

# Regression test: refusing a malformed or non-newer signed update must not
# skip telemetry or sleep (no tight CPU spin loop).
awk '
    /^    # 0\. CHECK AUTO-UPDATE$/ { reading = 1 }
    reading && /^    # 1\. INFORMACIÓN BÁSICA DEL SISTEMA$/ { exit }
    reading { print }
' "$agent" >"$work/update_check.sh"
test "$(wc -l <"$work/update_check.sh")" -gt 5

cat >"$work/run_downgrade_check.sh" <<'EOF'
#!/bin/sh
set -eu
WORK_DIR="$1"
AGENT_VERSION_NUMBER=2
AGENT_VERSION_NUMBER_FILE="$WORK_DIR/version"
DEVICE_TOKEN="tok"
BASE_URL="https://controller.invalid/api"
AGENT_UPDATE_PUBLIC_KEY="dummy"
LOG_FILE="$WORK_DIR/agent.log"
TELEMETRY_REACHED_FILE="$WORK_DIR/telemetry_reached"

logger() {
    printf '%s\n' "$*" >>"$LOG_FILE"
}
controller_curl() {
    case "$*" in
        */agent/latest)
            printf '%s\n' '{"version_hash":"different-hash","version_number":2,"signature":"sig","signature_algorithm":"Ed25519"}'
            return 0
            ;;
        */agent/latest/raw*)
            echo "unexpected download on downgrade" >&2
            return 1
            ;;
    esac
    return 1
}
jsonfilter() {
    key=""
    while [ $# -gt 0 ]; do
        if [ "$1" = "-e" ]; then
            key="$2"
            shift 2
        else
            shift
        fi
    done
    input=$(cat)
    case "$key" in
        @.version_hash) printf 'different-hash\n' ;;
        @.version_number) printf '2\n' ;;
        @.signature) printf 'sig\n' ;;
        @.signature_algorithm) printf 'Ed25519\n' ;;
    esac
}
while true; do
EOF
cat "$work/update_check.sh" >>"$work/run_downgrade_check.sh"
cat >>"$work/run_downgrade_check.sh" <<'EOF'
    printf 'ok\n' >"$TELEMETRY_REACHED_FILE"
    break
done
EOF

# Run with a short timeout so an infinite spin loop fails fast instead of hanging.
timeout 3 sh "$work/run_downgrade_check.sh" "$work"
test -f "$work/telemetry_reached"
grep -q "refusing downgrade" "$work/agent.log"

echo 'agent update commit cleanup passed'
