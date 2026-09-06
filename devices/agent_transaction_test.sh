#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
AGENT="$SCRIPT_DIR/agent.sh"
FIXTURE_DIR="$SCRIPT_DIR/test-fixtures/transaction"
ROOT=$(mktemp -d)
trap 'rm -rf "$ROOT"' EXIT

mkdir -p "$ROOT/etc/config"
printf '%s\n' 'wireless.baseline=1' > "$ROOT/etc/config/wireless"

NERVE_TRANSACTION_ROOT="$ROOT/etc/nerve/transactions"
NERVE_CONFIG_ROOT="$ROOT/etc/config"
NERVE_OPERATION_STATUS_FILE="$NERVE_TRANSACTION_ROOT/operation_status"
export NERVE_TRANSACTION_ROOT NERVE_CONFIG_ROOT NERVE_OPERATION_STATUS_FILE
export DEVICE_ID_FILE="$ROOT/device-id" DEVICE_TOKEN_FILE="$ROOT/device-token"

PATH="$FIXTURE_DIR:$PATH" sh "$AGENT" --self-test-transaction

test "$(cat "$NERVE_TRANSACTION_ROOT/self-test-operation/state")" = "COMMITTED"
test ! -e "$NERVE_TRANSACTION_ROOT/active"
test ! -e "$NERVE_TRANSACTION_ROOT/self-test-operation/backup"
test "$(cat "$NERVE_TRANSACTION_ROOT/wifi_config.hash")" = "self-test-hash"
test "$(cat "$NERVE_TRANSACTION_ROOT/last")" = "self-test-operation"

PATH="$FIXTURE_DIR:$PATH" sh "$AGENT" --self-test-operation
test "$(cat "$NERVE_TRANSACTION_ROOT/self-operation/state")" = "COMMITTED"
test ! -e "$NERVE_TRANSACTION_ROOT/self-operation/backup"

printf '%s\n' self-operation > "$NERVE_OPERATION_STATUS_FILE"
printf '%s\n' self-test-operation > "$NERVE_TRANSACTION_ROOT/last"
STATUS_JSON=$(PATH="$FIXTURE_DIR:$PATH" sh "$AGENT" --self-test-status)
case "$STATUS_JSON" in
    *'"id":"self-operation"'*) ;;
    *) echo "typed operation status was overwritten"; exit 1 ;;
esac
rm -f "$NERVE_OPERATION_STATUS_FILE"
STATUS_JSON=$(PATH="$FIXTURE_DIR:$PATH" sh "$AGENT" --self-test-status)
case "$STATUS_JSON" in
    *'"id":"self-test-operation"'*) ;;
    *) echo "fallback transaction status was not reported"; exit 1 ;;
esac

UCI_FIXTURE_STATE="$ROOT/uci-state" UCI_FIXTURE_LOG="$ROOT/uci.log" \
PATH="$FIXTURE_DIR:$PATH" \
SELF_TEST_OPERATION_JSON='{"operation_id":"lease-operation-1","plan_hash":"lease-operation-1","config":"dhcp","commands":[{"action":"ensure_host","config":"dhcp","section":"living room","option":"AA:BB:CC:DD:EE:FF","value":"192.0.2.10"}],"auto_confirm":true}' \
sh "$AGENT" --self-test-operation
UCI_FIXTURE_STATE="$ROOT/uci-state" UCI_FIXTURE_LOG="$ROOT/uci.log" \
PATH="$FIXTURE_DIR:$PATH" \
SELF_TEST_OPERATION_JSON='{"operation_id":"lease-operation-2","plan_hash":"lease-operation-2","config":"dhcp","commands":[{"action":"ensure_host","config":"dhcp","section":"living room","option":"AA:BB:CC:DD:EE:FF","value":"192.0.2.11"}],"auto_confirm":true}' \
sh "$AGENT" --self-test-operation
test "$(grep -c '^add dhcp host$' "$ROOT/uci.log")" = "1"
grep -q '^dhcp.@host\[0\].name=living room$' "$ROOT/uci-state"
grep -q '^dhcp.@host\[0\].ip=192.0.2.11$' "$ROOT/uci-state"
UCI_FIXTURE_STATE="$ROOT/uci-state" UCI_FIXTURE_LOG="$ROOT/uci.log" \
PATH="$FIXTURE_DIR:$PATH" \
SELF_TEST_OPERATION_JSON='{"operation_id":"lease-operation-3","plan_hash":"lease-operation-3","config":"dhcp","commands":[{"action":"ensure_host","config":"dhcp","section":"short-ip","option":"11:22:33:44:55:66","value":"192.0.2.1"}],"auto_confirm":true}' \
sh "$AGENT" --self-test-operation
test "$(grep -c '^add dhcp host$' "$ROOT/uci.log")" = "2"
grep -q '^dhcp.@host\[1\].ip=192.0.2.1$' "$ROOT/uci-state"

UCI_FIXTURE_STATE="$ROOT/firewall-state" UCI_FIXTURE_LOG="$ROOT/firewall.log" \
PATH="$FIXTURE_DIR:$PATH" \
SELF_TEST_OPERATION_JSON='{"operation_id":"firewall-operation","plan_hash":"firewall-operation","config":"firewall","commands":[{"action":"delete_all","config":"firewall","section":"redirect","option":"","value":""}],"auto_confirm":true}' \
sh "$AGENT" --self-test-operation
if grep -q '^firewall.@redirect' "$ROOT/firewall-state"; then
    echo "firewall replacement left an old redirect"
    exit 1
fi

mkdir -p "$NERVE_TRANSACTION_ROOT/retry-operation"
printf '%s\n' retry-operation > "$NERVE_TRANSACTION_ROOT/active"
printf '%s\n' wireless > "$NERVE_TRANSACTION_ROOT/retry-operation/config"
printf '%s\n' 1 > "$NERVE_TRANSACTION_ROOT/retry-operation/backup_exists"
printf '%s\n' ROLLING_BACK > "$NERVE_TRANSACTION_ROOT/retry-operation/state"
printf '%s\n' 'wireless.baseline=1' > "$NERVE_TRANSACTION_ROOT/retry-operation/backup"
printf '%s\n' 'wireless.unconfirmed=1' > "$ROOT/etc/config/wireless"
SELF_TEST_OPERATION_JSON='{"operation_id":"retry-operation","plan_hash":"retry-operation","config":"wireless","commands":[{"action":"set","config":"wireless","section":"wifi0","option":"ssid","value":"retry"}],"auto_confirm":true}' \
PATH="$FIXTURE_DIR:$PATH" sh "$AGENT" --self-test-operation
test "$(cat "$NERVE_TRANSACTION_ROOT/retry-operation/state")" = "COMMITTED"
test ! -e "$NERVE_TRANSACTION_ROOT/active"

mkdir -p "$NERVE_TRANSACTION_ROOT/old-terminal"
printf '%s\n' system > "$NERVE_TRANSACTION_ROOT/old-terminal/config"
printf '%s\n' COMMITTED > "$NERVE_TRANSACTION_ROOT/old-terminal/state"
PATH="$FIXTURE_DIR:$PATH" sh "$AGENT" --prune-transactions
test ! -e "$NERVE_TRANSACTION_ROOT/old-terminal"
test -e "$NERVE_TRANSACTION_ROOT/retry-operation/state"

if UCI_FIXTURE_STATE="$ROOT/mismatch-state" UCI_FIXTURE_LOG="$ROOT/mismatch.log" \
    PATH="$FIXTURE_DIR:$PATH" \
    SELF_TEST_OPERATION_JSON='{"operation_id":"mismatched-operation","plan_hash":"mismatched-operation-hash","config":"system","commands":[{"action":"set","config":"dhcp","section":"lan","option":"start","value":"100"}],"auto_confirm":true}' \
    sh "$AGENT" --self-test-operation; then
    echo "mismatched operation was accepted"
    exit 1
fi
test ! -e "$NERVE_TRANSACTION_ROOT/active"
if grep -q '^set ' "$ROOT/mismatch.log"; then
    echo "mismatched operation reached uci"
    exit 1
fi

if UCI_FIXTURE_STATE="$ROOT/hash-mismatch-state" UCI_FIXTURE_LOG="$ROOT/hash-mismatch.log" \
    PATH="$FIXTURE_DIR:$PATH" \
    SELF_TEST_OPERATION_JSON='{"operation_id":"hash-operation","plan_hash":"different-hash","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"rejected"}],"auto_confirm":true}' \
    sh "$AGENT" --self-test-operation; then
    echo "operation hash mismatch was accepted"
    exit 1
fi
test ! -e "$NERVE_TRANSACTION_ROOT/active"
if grep -q '^set ' "$ROOT/hash-mismatch.log"; then
    echo "hash mismatch reached uci"
    exit 1
fi

mkdir -p "$NERVE_TRANSACTION_ROOT/recovery-operation"
printf '%s\n' recovery-operation > "$NERVE_TRANSACTION_ROOT/active"
printf '%s\n' wireless > "$NERVE_TRANSACTION_ROOT/recovery-operation/config"
printf '%s\n' 1 > "$NERVE_TRANSACTION_ROOT/recovery-operation/backup_exists"
printf '%s\n' PENDING_CONFIRM > "$NERVE_TRANSACTION_ROOT/recovery-operation/state"
printf '%s\n' 'wireless.baseline=1' > "$NERVE_TRANSACTION_ROOT/recovery-operation/backup"
cp "$NERVE_TRANSACTION_ROOT/recovery-operation/backup" "$ROOT/expected-wireless"
printf '%s\n' 'wireless.unconfirmed=1' > "$ROOT/etc/config/wireless"

PATH="$FIXTURE_DIR:$PATH" sh "$AGENT" --recover-transactions

cmp "$ROOT/etc/config/wireless" "$ROOT/expected-wireless"
test "$(cat "$NERVE_TRANSACTION_ROOT/recovery-operation/state")" = "RESTORED"
test ! -e "$NERVE_TRANSACTION_ROOT/active"
test ! -e "$NERVE_TRANSACTION_ROOT/recovery-operation/backup"
test "$(cat "$NERVE_TRANSACTION_ROOT/last")" = "recovery-operation"

mkdir -p "$NERVE_TRANSACTION_ROOT/orphan-operation"
printf '%s\n' wireless > "$NERVE_TRANSACTION_ROOT/orphan-operation/config"
printf '%s\n' 1 > "$NERVE_TRANSACTION_ROOT/orphan-operation/backup_exists"
printf '%s\n' APPLYING > "$NERVE_TRANSACTION_ROOT/orphan-operation/state"
printf '%s\n' 'wireless.baseline=1' > "$NERVE_TRANSACTION_ROOT/orphan-operation/backup"
printf '%s\n' 'wireless.orphan=1' > "$ROOT/etc/config/wireless"

PATH="$FIXTURE_DIR:$PATH" sh "$AGENT" --recover-transactions

cmp "$ROOT/etc/config/wireless" "$ROOT/expected-wireless"
test "$(cat "$NERVE_TRANSACTION_ROOT/orphan-operation/state")" = "RESTORED"
test ! -e "$NERVE_TRANSACTION_ROOT/orphan-operation/backup"

echo "agent durable transaction contract passed"
