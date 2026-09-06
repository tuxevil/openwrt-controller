#!/bin/bash
# LAB PROTOTYPE: does native rpcd rollback outlive SSH, and does confirm stop it?
# Intentionally pinned to the disposable WNDR3800. Not a production executor.
# Run from repo root: bash devices/commit-confirmed-hostname-lab.prototype.sh --execute
# Only system.@system[0].hostname changes. Fault modes also kill rpcd or reboot
# this node. No network, DHCP or Wi-Fi configuration is edited.
set -euo pipefail

case ${1:-} in
    --check|--execute|--execute-replay|--execute-rpcd-crash|--execute-reboot) mode=$1 ;;
    *) printf 'Requires --check or an explicit --execute[-replay|-rpcd-crash|-reboot] mode.\n' >&2; exit 2 ;;
esac

for tool in ssh timeout jq; do
    command -v "$tool" >/dev/null
done

target=root@10.128.128.3
original=wndr3800ch
temporary=wndr3800ch-lab
sid=
extra_sid=
baseline=
apply_timeout=20
if [[ $mode != --execute ]]; then apply_timeout=60; fi
ssh_args=(-i certs/id_controller -o IdentitiesOnly=yes -o BatchMode=yes
    -o StrictHostKeyChecking=yes -o UserKnownHostsFile=certs/known_hosts
    -o ConnectTimeout=5 -o ServerAliveInterval=3 -o ServerAliveCountMax=2)

remote() {
    timeout --kill-after=3s 20s ssh "${ssh_args[@]}" "$target" "$@"
}

rpc() {
    local object=$1 method=$2 payload=$3 quoted
    printf -v quoted '%q' "$payload"
    remote "ubus -t 10 call $object $method $quoted"
}

session_payload() {
    jq -cn --arg sid "$sid" '{ubus_rpc_session:$sid}'
}

new_session() {
    sid=$(rpc session create '{"timeout":600}' | jq -er '.ubus_rpc_session | select(test("^[0-9a-f]{32}$"))') || return
    rpc session grant "$(jq -cn --arg sid "$sid" \
        '{ubus_rpc_session:$sid,scope:"uci",objects:[["system","read"],["system","write"]]}')"
}

boot_id() {
    remote 'read -r id < /proc/sys/kernel/random/boot_id; printf "%s\n" "$id"'
}

rpcd_pid() {
    rpc service list '{"name":"rpcd"}' | jq -er '.rpcd.instances.instance1 | select(.running) | .pid'
}

hashes() {
    remote 'sha256sum /etc/config/network /etc/config/dhcp /etc/config/firewall /etc/config/dropbear /etc/config/system /etc/config/wireless'
}

assert_hostname() {
    local expected=$1 configured running
    configured=$(rpc uci get '{"config":"system","section":"@system[0]","option":"hostname"}' | jq -er .value)
    running=$(rpc system board '{}' | jq -er .hostname)
    if [[ $configured != "$expected" || $running != "$expected" ]]; then
        printf 'FAIL hostname: expected=%s configured=%s running=%s\n' "$expected" "$configured" "$running" >&2
        return 1
    fi
    printf 'PASS hostname configured=%s running=%s\n' "$configured" "$running"
}

stage() {
    local name=$1
    rpc uci set "$(jq -cn --arg sid "$sid" --arg name "$name" \
        '{ubus_rpc_session:$sid,config:"system",section:"@system[0]",values:{hostname:$name}}')" || return
    rpc uci changes "$(session_payload)" | jq -e --arg name "$name" \
        '(.changes | keys) == ["system"] and
         (.changes.system | length) == 1 and
         .changes.system[0][0] == "set" and
         .changes.system[0][2] == "hostname" and
         .changes.system[0][3] == $name' >/dev/null
}

apply() {
    rpc uci apply "$(jq -cn --arg sid "$sid" --argjson timeout "$apply_timeout" \
        '{ubus_rpc_session:$sid,rollback:true,timeout:$timeout}')"
}

revert_staging() {
    rpc uci revert "$(jq -cn --arg sid "$sid" \
        '{ubus_rpc_session:$sid,config:"system"}')"
}

restore() {
    local configured
    # Native rollback may already have fired, in which case it returns No data.
    rpc uci rollback "$(session_payload)" >/dev/null 2>&1 || true
    # A crash/reboot destroys rpcd sessions. Recovery must not reuse that SID.
    if ! rpc session access "$(jq -cn --arg sid "$sid" \
        '{ubus_rpc_session:$sid,scope:"uci",object:"system",function:"write"}')" 2>/dev/null | jq -e '.access == true' >/dev/null; then
        new_session || return
    fi
    revert_staging || return
    configured=$(rpc uci get '{"config":"system","section":"@system[0]","option":"hostname"}' | jq -er .value) || return
    case "$configured" in
        "$original") ;;
        "$temporary")
            printf 'RECOVERY: explicitly restoring original; this is not automatic rollback.\n' >&2
            stage "$original" || return
            apply || return
            sleep 3
            assert_hostname "$original" || return
            rpc uci confirm "$(session_payload)" || return
            ;;
        *)
            printf 'STOP: unexpected hostname; refusing to overwrite another writer: %s\n' "$configured" >&2
            return 1
            ;;
    esac
    sleep 3
    assert_hostname "$original"
}

cleanup() {
    local status=$?
    trap - EXIT
    if [[ -n $sid ]]; then
        printf 'Restoring original hostname and removing only the lab session.\n' >&2
        # restore can replace a SID lost in a crash; keep it in this shell.
        if ! restore; then
            printf 'RESTORATION FAILED: inspect node before further work.\n' >&2
            exit 1
        fi
        if ! rpc session destroy "$(session_payload)"; then
            printf 'Lab session cleanup failed.\n' >&2
            exit 1
        fi
        if [[ -n $extra_sid ]]; then
            if ! rpc session destroy "$(jq -cn --arg sid "$extra_sid" '{ubus_rpc_session:$sid}')"; then
                printf 'Extra lab session cleanup failed.\n' >&2
                exit 1
            fi
        fi
        if [[ $(hashes) != "$baseline" ]]; then
            printf 'FAIL: configuration hashes differ from baseline; do not overwrite blindly.\n' >&2
            hashes
            exit 1
        fi
        printf 'PASS all six configuration files match baseline byte-for-byte.\n'
    fi
    exit "$status"
}
trap cleanup EXIT

printf 'Preflight: fixed target, identity, staging and native transaction artifacts.\n'
remote 'set -e
    read -r mac < /sys/class/net/eth0/address
    test "$mac" = "04:a1:51:96:a6:4d"
    test "$(uci -q get network.lan.ipaddr)" = "10.128.128.3/24"
    test "$(uci -q get system.@system[0].hostname)" = "wndr3800ch"
    pending=$(uci changes)
    test -z "$pending"
    for path in /var/run/rpcd/snapshot-files/* /var/run/rpcd/snapshot-delta/* /var/run/rpcd/uci-*; do
        if [ -e "$path" ]; then
            printf "STOP: existing rpcd transaction or session staging: %s\n" "$path" >&2
            exit 1
        fi
    done
    ls -d /tmp'
assert_hostname "$original"
baseline=$(hashes)

printf 'Fixture: isolated UCI edit/revert, no commit or service reload.\n'
read -r -d '' fixture_script <<'FIXTURE' || true
set -eu
fixture=$(mktemp -d /tmp/nerve-hostname-fixture.XXXXXX)
trap 'rm -rf "$fixture"' EXIT
mkdir "$fixture/delta"
cp /etc/config/system "$fixture/system"
uci -c "$fixture" -t "$fixture/delta" set system.@system[0].hostname=wndr3800ch-lab
test "$(uci -c "$fixture" -t "$fixture/delta" get system.@system[0].hostname)" = wndr3800ch-lab
uci -c "$fixture" -t "$fixture/delta" changes system
uci -c "$fixture" -t "$fixture/delta" revert system
test "$(uci -c "$fixture" -t "$fixture/delta" get system.@system[0].hostname)" = wndr3800ch
cmp "$fixture/system" /etc/config/system
FIXTURE
remote 'sh -n' <<< "$fixture_script"
remote 'sh -s' <<< "$fixture_script"
test "$(hashes)" = "$baseline"
if [[ $mode == --check ]]; then exit 0; fi

new_session

if [[ $mode == --execute-replay ]]; then
    printf 'REPLAY: duplicate apply, foreign confirm, correct confirm, duplicate confirm.\n'
    extra_sid=$(rpc session create '{"timeout":300}' | jq -er '.ubus_rpc_session | select(test("^[0-9a-f]{32}$"))')
    stage "$temporary"
    apply
    sleep 3
    assert_hostname "$temporary"
    changed=$(hashes)
    if reply=$(apply 2>&1); then
        printf 'FAIL: duplicate apply accepted during pending rollback.\n' >&2
        exit 1
    else
        [[ $reply == *'Permission denied'* ]]
        printf 'OBSERVED duplicate apply: Permission denied.\n'
    fi
    if reply=$(rpc uci confirm "$(jq -cn --arg sid "$extra_sid" '{ubus_rpc_session:$sid}')" 2>&1); then
        printf 'FAIL: foreign session confirmed another session transaction.\n' >&2
        exit 1
    else
        [[ $reply == *'Permission denied'* ]]
        printf 'PASS foreign-session confirmation rejected.\n'
    fi
    assert_hostname "$temporary"
    rpc uci confirm "$(session_payload)"
    if reply=$(rpc uci confirm "$(session_payload)" 2>&1); then
        printf 'OBSERVED duplicate confirm: accepted.\n'
    else
        status=$?
        # This firmware's ubus reports No response, including the full request
        # (and its SID) in stderr. Record only the classification, not that text.
        [[ $reply == *'(No response)'* ]]
        printf 'OBSERVED duplicate confirm: No response (exit %s); no prior terminal result returned.\n' "$status"
    fi
    sleep "$((apply_timeout + 5))"
    assert_hostname "$temporary"
    test "$(hashes)" = "$changed"
    printf 'PASS duplicate/foreign requests did not change the applied configuration.\n'
    exit 0
fi

if [[ $mode == --execute-rpcd-crash || $mode == --execute-reboot ]]; then
    before_boot=$(boot_id)
    before_pid=$(rpcd_pid)
    [[ $before_pid =~ ^[0-9]+$ ]]
    stage "$temporary"
    started=$SECONDS
    apply
    sleep 3
    assert_hostname "$temporary"
    remote 'test -f /var/run/rpcd/snapshot-files/system'
    if [[ $mode == --execute-rpcd-crash ]]; then
        printf 'FAULT: SIGKILL only verified rpcd PID %s on the lab node.\n' "$before_pid"
        remote "set -e; read -r comm < /proc/$before_pid/comm; test \"\$comm\" = rpcd; kill -KILL $before_pid"
        returned=0
        for ((attempt=0; attempt<20; attempt++)); do
            if after_pid=$(rpcd_pid 2>/dev/null) && [[ $after_pid != "$before_pid" ]] && rpc uci configs '{}' >/dev/null 2>&1; then
                returned=1
                printf 'OBSERVED rpcd restarted: PID %s -> %s.\n' "$before_pid" "$after_pid"
                break
            fi
            sleep 2
        done
        test "$returned" = 1
        test "$(boot_id)" = "$before_boot"
    else
        printf 'FAULT: requesting exactly one reboot of the lab node.\n'
        if ! rpc system reboot '{}'; then
            printf 'Reboot acknowledgment lost/failed; observing boot ID without retry.\n' >&2
        fi
        returned=0
        until ((SECONDS - started >= 240)); do
            if after_boot=$(boot_id 2>/dev/null) && [[ $after_boot != "$before_boot" ]] && rpc uci configs '{}' >/dev/null 2>&1; then
                returned=1
                printf 'OBSERVED new boot and rpcd available at t=%ss.\n' "$((SECONDS - started))"
                break
            fi
            sleep 3
        done
        test "$returned" = 1
    fi
    while ((SECONDS - started < apply_timeout + 15)); do sleep 2; done
    printf 'VERDICT at t=%ss: checking original hostname without manual recovery.\n' "$((SECONDS - started))"
    assert_hostname "$original"
    printf 'PASS original configuration recovered after fault.\n'
    exit 0
fi

printf 'TEST 1: apply hostname, close SSH, withhold confirmation for 20 seconds.\n'
stage "$temporary"
apply
sleep 3
assert_hostname "$temporary"
sleep 23
assert_hostname "$original"
printf 'PASS unconfirmed change rolled back after originating SSH session ended.\n'
revert_staging

printf 'TEST 2: confirm from a new SSH connection, then wait past rollback deadline.\n'
stage "$temporary"
apply
sleep 3
assert_hostname "$temporary"
rpc uci confirm "$(session_payload)"
sleep 23
assert_hostname "$temporary"
printf 'PASS confirmed change persisted beyond rollback deadline.\n'
# EXIT cleanup restores and verifies baseline, including on assertion failures.
