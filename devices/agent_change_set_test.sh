#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
AGENT="$SCRIPT_DIR/agent.sh"
FIXTURE_DIR="$SCRIPT_DIR/test-fixtures/transaction"
ROOT=$(mktemp -d)
trap 'rm -rf "$ROOT"' EXIT

mkdir -p "$ROOT/etc/config"
printf '%s\n' self-test-device > "$ROOT/device-id"
printf '%s\n' 'system.@system[0].hostname=baseline' > "$ROOT/uci-state"

agent_self_test() {
    UCI_FIXTURE_STATE="$ROOT/uci-state" \
    UCI_FIXTURE_LOG="$ROOT/uci.log" \
    NERVE_TRANSACTION_ROOT="${TEST_TRANSACTION_ROOT:-$ROOT/etc/nerve/transactions}" \
    NERVE_CONFIG_ROOT="${TEST_CONFIG_ROOT:-$ROOT/etc/config}" \
    NERVE_OPERATION_STATUS_FILE="${TEST_OPERATION_STATUS_FILE:-${TEST_TRANSACTION_ROOT:-$ROOT/etc/nerve/transactions}/operation_status}" \
    DEVICE_ID_FILE="$ROOT/device-id" \
    DEVICE_TOKEN_FILE="$ROOT/device-token" \
    PATH="$FIXTURE_DIR:$PATH" \
    "$@"
}

agent_self_test sh "$AGENT" --self-test-change-set
test "$(grep -c '^set ' "$ROOT/uci.log")" = 1
STATUS_JSON=$(agent_self_test sh "$AGENT" --self-test-status | tr -d '\n')
case "$STATUS_JSON" in
    '{"change_set_id":"self-change-set","device_id":"self-test-device","plan_hash":"'*'","generation":42,"state":"COMMITTED"}') ;;
    *) echo "valid changeset status was not committed" >&2; exit 1 ;;
esac
for manifest_file in change_set_id change_set_device_id change_set_plan_hash change_set_generation change_set_operation_id change_set_commands change_set_observed_state_hash change_set_health_checks change_set_confirmation_policy; do
    test -s "$ROOT/etc/nerve/transactions/self-change-set/$manifest_file"
done
test "$(cat "$ROOT/etc/nerve/transactions/self-change-set/change_set_operation_id")" = self-operation-entry

agent_self_test sh "$AGENT" --self-test-change-set
test "$(grep -c '^set ' "$ROOT/uci.log")" = 1
test ! -e "$ROOT/etc/nerve/transactions/operation_status"

SELF_PLAN_HASH=$(cat "$ROOT/etc/nerve/transactions/self-test-change-set.json" | PATH="$FIXTURE_DIR:$PATH" jsonfilter -e '@.plan_hash')
MALFORMED_OBSERVED_HASH=$(UCI_FIXTURE_STATE="$ROOT/uci-state" UCI_FIXTURE_LOG="$ROOT/uci.log" "$FIXTURE_DIR/uci" show system | sha256sum | awk '{print $1}')
MALFORMED_HEALTH_CHANGE_SET=$(printf '{"change_set_id":"self-change-set","device_id":"self-test-device","plan_hash":"%s","generation":42,"operations":[{"operation_id":"self-operation-entry","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"must-reject"}],"observed_state_hash":"%s"}],"health_checks":"not-an-array","confirmation_policy":"local_auto"}' "$SELF_PLAN_HASH" "$MALFORMED_OBSERVED_HASH")
if agent_self_test env SELF_TEST_CHANGE_SET_JSON="$MALFORMED_HEALTH_CHANGE_SET" sh "$AGENT" --self-test-change-set; then
    echo "malformed health checks were accepted" >&2
    exit 1
fi
test "$(grep -c '^set ' "$ROOT/uci.log")" = 1

STALE_OBSERVED_HASH=$(UCI_FIXTURE_STATE="$ROOT/uci-state" UCI_FIXTURE_LOG="$ROOT/uci.log" "$FIXTURE_DIR/uci" show system | sha256sum | awk '{print $1}')
STALE_PLAN_HASH=$(printf '{"operations":[{"config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"stale-router"}],"observed_state_hash":"%s"}],"health_checks":null,"confirmation_policy":"local_auto"}' "$STALE_OBSERVED_HASH" | sha256sum | awk '{print $1}')
STALE_CHANGE_SET=$(printf '{"change_set_id":"stale-generation-change-set","device_id":"self-test-device","plan_hash":"%s","generation":41,"operations":[{"operation_id":"stale-generation-entry","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"stale-router"}],"observed_state_hash":"%s"}],"confirmation_policy":"local_auto"}' "$STALE_PLAN_HASH" "$STALE_OBSERVED_HASH")
if agent_self_test env SELF_TEST_CHANGE_SET_JSON="$STALE_CHANGE_SET" sh "$AGENT" --self-test-change-set; then
    echo "older changeset generation was accepted" >&2
    exit 1
fi
test "$(grep -c '^set ' "$ROOT/uci.log")" = 1
grep -q '"change_set_id":"self-change-set"' "$ROOT/etc/nerve/transactions/change_set_status"
grep -q '"state":"COMMITTED"' "$ROOT/etc/nerve/transactions/change_set_status"

printf '%s\n' "{\"change_set_id\":\"self-change-set\",\"device_id\":\"self-test-device\",\"plan_hash\":\"$SELF_PLAN_HASH\",\"generation\":42,\"state\":\"RESTORED\"}" > "$ROOT/etc/nerve/transactions/change_set_status"
agent_self_test sh "$AGENT" --self-test-change-set
test "$(grep -c '^set ' "$ROOT/uci.log")" = 1
printf '%s\n' COMMITTED > "$ROOT/etc/nerve/transactions/self-change-set/state"
printf '%s\n' "{\"change_set_id\":\"self-change-set\",\"device_id\":\"self-test-device\",\"plan_hash\":\"$SELF_PLAN_HASH\",\"generation\":42,\"state\":\"REJECTED\"}" > "$ROOT/etc/nerve/transactions/change_set_status"
agent_self_test sh "$AGENT" --self-test-change-set
test "$(grep -c '^set ' "$ROOT/uci.log")" = 1

TERMINAL_REPLAY_CHANGE_SET=$(printf '{"change_set_id":"self-change-set","device_id":"other-device","plan_hash":"%s","generation":42,"operations":[{"operation_id":"self-operation-entry","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"replayed-device"}],"observed_state_hash":"%s"}],"confirmation_policy":"local_auto"}' "$SELF_PLAN_HASH" "$MALFORMED_OBSERVED_HASH")
if agent_self_test env SELF_TEST_CHANGE_SET_JSON="$TERMINAL_REPLAY_CHANGE_SET" sh "$AGENT" --self-test-change-set; then
    echo "terminal changeset replay bypassed device identity validation" >&2
    exit 1
fi
test "$(grep -c '^set ' "$ROOT/uci.log")" = 1

TAMPERED_TERMINAL_CHANGE_SET=$(printf '{"change_set_id":"self-change-set","device_id":"self-test-device","plan_hash":"%s","generation":42,"operations":[{"operation_id":"self-operation-entry","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"replayed-content"}],"observed_state_hash":"%s"}],"confirmation_policy":"local_auto"}' "$SELF_PLAN_HASH" "$MALFORMED_OBSERVED_HASH")
if agent_self_test env SELF_TEST_CHANGE_SET_JSON="$TAMPERED_TERMINAL_CHANGE_SET" sh "$AGENT" --self-test-change-set; then
    echo "terminal changeset replay bypassed content validation" >&2
    exit 1
fi
test "$(grep -c '^set ' "$ROOT/uci.log")" = 1

rm -f "$ROOT/etc/nerve/transactions/change_set_status"
agent_self_test sh "$AGENT" --recover-transactions
grep -q '"change_set_id":"self-change-set"' "$ROOT/etc/nerve/transactions/change_set_status"
grep -q '"state":"COMMITTED"' "$ROOT/etc/nerve/transactions/change_set_status"

agent_self_test sh "$AGENT" --self-test-change-set-transition

rm -f "$ROOT/etc/nerve/transactions/change_set_status"
printf '%s\n' self-change-set > "$ROOT/etc/nerve/transactions/active"
agent_self_test sh "$AGENT" --recover-transactions
grep -q '"state":"COMMITTED"' "$ROOT/etc/nerve/transactions/change_set_status"

agent_self_test env SELF_TEST_OPERATION_JSON='{"operation_id":"after-change-set","plan_hash":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"after-change-set"}],"auto_confirm":true}' sh "$AGENT" --self-test-operation
test "$(cat "$ROOT/etc/nerve/transactions/operation_status")" = after-change-set

if agent_self_test env SELF_TEST_CHANGE_SET_JSON='{"change_set_id":"stale-change-set","device_id":"self-test-device","plan_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","generation":43,"operations":[{"operation_id":"stale-entry","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"rejected"}],"observed_state_hash":"0000000000000000000000000000000000000000000000000000000000000000"}],"confirmation_policy":"local_auto"}' sh "$AGENT" --self-test-change-set; then
    echo "stale changeset was applied" >&2
    exit 1
fi
test "$(grep -c '^set ' "$ROOT/uci.log")" = 2
grep -q '"change_set_id":"stale-change-set"' "$ROOT/etc/nerve/transactions/change_set_status"
grep -q '"state":"REJECTED"' "$ROOT/etc/nerve/transactions/change_set_status"

if agent_self_test env SELF_TEST_CHANGE_SET_JSON='{"change_set_id":"unsupported-change-set","device_id":"self-test-device","plan_hash":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","generation":44,"operations":[{"operation_id":"unsupported-entry","config":"dhcp","commands":[{"action":"set","config":"dhcp","section":"lan","option":"start","value":"100"}],"observed_state_hash":"0000000000000000000000000000000000000000000000000000000000000000"}],"confirmation_policy":"local_auto"}' sh "$AGENT" --self-test-change-set; then
    echo "unsupported changeset namespace was applied" >&2
    exit 1
fi
test "$(grep -c '^set ' "$ROOT/uci.log")" = 2
grep -q '"change_set_id":"unsupported-change-set"' "$ROOT/etc/nerve/transactions/change_set_status"
grep -q '"failure":"unsupported changeset namespace"' "$ROOT/etc/nerve/transactions/change_set_status"

if agent_self_test env SELF_TEST_CHANGE_SET_JSON='{"change_set_id":"wrong-device-change-set","device_id":"other-device","plan_hash":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","generation":45,"operations":[{"operation_id":"wrong-device-entry","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"rejected"}],"observed_state_hash":"0000000000000000000000000000000000000000000000000000000000000000"}],"confirmation_policy":"local_auto"}' sh "$AGENT" --self-test-change-set; then
    echo "wrong-device changeset was applied" >&2
    exit 1
fi
grep -q '"change_set_id":"wrong-device-change-set"' "$ROOT/etc/nerve/transactions/change_set_status"
grep -q '"failure":"changeset device identity mismatch"' "$ROOT/etc/nerve/transactions/change_set_status"

FAILURE_OBSERVED_HASH=$(UCI_FIXTURE_STATE="$ROOT/uci-state" UCI_FIXTURE_LOG="$ROOT/uci.log" "$FIXTURE_DIR/uci" show system | sha256sum | awk '{print $1}')
FAILURE_PLAN_HASH=$(printf '{"operations":[{"config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"must-reject"}],"observed_state_hash":"%s"}],"health_checks":null,"confirmation_policy":"local_auto"}' "$FAILURE_OBSERVED_HASH" | sha256sum | awk '{print $1}')
FAILURE_CHANGE_SET=$(printf '{"change_set_id":"failure-change-set","device_id":"self-test-device","plan_hash":"%s","generation":50,"operations":[{"operation_id":"failure-entry","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"must-reject"}],"observed_state_hash":"%s"}],"confirmation_policy":"local_auto"}' "$FAILURE_PLAN_HASH" "$FAILURE_OBSERVED_HASH")
if agent_self_test env UCI_FIXTURE_FAIL_COMMIT=1 SELF_TEST_CHANGE_SET_JSON="$FAILURE_CHANGE_SET" sh "$AGENT" --self-test-change-set; then
    echo "changeset commit failure was accepted" >&2
    exit 1
fi
test "$(cat "$ROOT/etc/nerve/transactions/failure-change-set/state")" = "RECOVERY_REQUIRED"
grep -q '"state":"RECOVERY_REQUIRED"' "$ROOT/etc/nerve/transactions/change_set_status"
grep -q '"failure":"transaction recovery failed"' "$ROOT/etc/nerve/transactions/change_set_status"
if agent_self_test sh "$AGENT" --recover-transactions; then
    echo "recovery-required changeset was recovered destructively" >&2
    exit 1
fi
rm -f "$ROOT/etc/nerve/transactions/change_set_status"
if agent_self_test sh "$AGENT" --recover-transactions; then
    echo "recovery-required changeset was recovered after status loss" >&2
    exit 1
fi
grep -q '"state":"RECOVERY_REQUIRED"' "$ROOT/etc/nerve/transactions/change_set_status"

RECOVERY_OBSERVED_HASH=$(UCI_FIXTURE_STATE="$ROOT/uci-state" UCI_FIXTURE_LOG="$ROOT/uci.log" "$FIXTURE_DIR/uci" show system | sha256sum | awk '{print $1}')
RECOVERY_PLAN_HASH=$(printf '{"operations":[{"config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"recovery-blocked"}],"observed_state_hash":"%s"}],"health_checks":null,"confirmation_policy":"local_auto"}' "$RECOVERY_OBSERVED_HASH" | sha256sum | awk '{print $1}')
RECOVERY_CHANGE_SET=$(printf '{"change_set_id":"recovery-blocked-change-set","device_id":"self-test-device","plan_hash":"%s","generation":51,"operations":[{"operation_id":"recovery-blocked-entry","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"recovery-blocked"}],"observed_state_hash":"%s"}],"confirmation_policy":"local_auto"}' "$RECOVERY_PLAN_HASH" "$RECOVERY_OBSERVED_HASH")
if agent_self_test env SELF_TEST_CHANGE_SET_JSON="$RECOVERY_CHANGE_SET" sh "$AGENT" --self-test-change-set; then
    echo "changeset was accepted while recovery was required" >&2
    exit 1
fi
test "$(grep -c '^set ' "$ROOT/uci.log")" = 3
rm -f "$ROOT/etc/nerve/transactions/active"
if agent_self_test env SELF_TEST_OPERATION_JSON='{"operation_id":"recovery-fenced-operation","plan_hash":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"must-not-apply"}],"auto_confirm":true}' sh "$AGENT" --self-test-operation; then
    echo "standalone operation bypassed recovery-required fence" >&2
    exit 1
fi
test "$(grep -c '^set ' "$ROOT/uci.log")" = 3

UNKNOWN_ROOT="$ROOT/unknown-transactions"
mkdir -p "$UNKNOWN_ROOT/unknown-journal"
printf '%s\n' CORRUPT > "$UNKNOWN_ROOT/unknown-journal/state"
if TEST_TRANSACTION_ROOT="$UNKNOWN_ROOT" TEST_CONFIG_ROOT="$ROOT/unknown-config" agent_self_test env SELF_TEST_OPERATION_JSON='{"operation_id":"unknown-state-operation","plan_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"must-not-apply"}],"auto_confirm":true}' sh "$AGENT" --self-test-operation; then
    echo "unknown journal state was accepted" >&2
    exit 1
fi
test "$(grep -c '^set ' "$ROOT/uci.log")" = 3
rm -rf "$UNKNOWN_ROOT"

ORPHAN_PLAN_HASH=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
ORPHAN_ROOT="$ROOT/orphan-transactions"
mkdir -p "$ORPHAN_ROOT"
printf '%s\n' "{\"change_set_id\":\"orphan-change-set\",\"device_id\":\"self-test-device\",\"plan_hash\":\"$ORPHAN_PLAN_HASH\",\"generation\":60,\"state\":\"APPLYING\"}" > "$ORPHAN_ROOT/change_set_status"
rm -rf "$ORPHAN_ROOT/orphan-change-set"
if TEST_TRANSACTION_ROOT="$ORPHAN_ROOT" agent_self_test sh "$AGENT" --recover-transactions; then
    echo "orphan applying status was not fenced" >&2
    exit 1
fi
grep -q '"state":"RECOVERY_REQUIRED"' "$ORPHAN_ROOT/change_set_status"

PRUNE_ROOT="$ROOT/prune-replay"
mkdir -p "$PRUNE_ROOT/etc/config"
printf '%s\n' self-test-device > "$PRUNE_ROOT/device-id"
printf '%s\n' 'system.@system[0].hostname=baseline' > "$PRUNE_ROOT/uci-state"
agent_prune_self_test() {
    UCI_FIXTURE_STATE="$PRUNE_ROOT/uci-state" \
    UCI_FIXTURE_LOG="$PRUNE_ROOT/uci.log" \
    NERVE_TRANSACTION_ROOT="$PRUNE_ROOT/etc/nerve/transactions" \
    NERVE_CONFIG_ROOT="$PRUNE_ROOT/etc/config" \
    NERVE_OPERATION_STATUS_FILE="$PRUNE_ROOT/etc/nerve/transactions/operation_status" \
    DEVICE_ID_FILE="$PRUNE_ROOT/device-id" \
    DEVICE_TOKEN_FILE="$PRUNE_ROOT/device-token" \
    PATH="$FIXTURE_DIR:$PATH" \
    "$@"
}
agent_prune_self_test sh "$AGENT" --self-test-change-set
test "$(grep -c '^set system' "$PRUNE_ROOT/uci.log")" = 1
agent_prune_self_test env SELF_TEST_OPERATION_JSON='{"operation_id":"legacy-after-change-set","plan_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","config":"wireless","commands":[{"action":"set","config":"wireless","section":"wifi0","option":"ssid","value":"legacy"}],"auto_confirm":true}' sh "$AGENT" --self-test-operation
agent_prune_self_test sh "$AGENT" --prune-transactions
test ! -e "$PRUNE_ROOT/etc/nerve/transactions/self-change-set/state"
agent_prune_self_test sh "$AGENT" --self-test-change-set
test "$(grep -c '^set system' "$PRUNE_ROOT/uci.log")" = 1

echo "agent changeset contract passed"
