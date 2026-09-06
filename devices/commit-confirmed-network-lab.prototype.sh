#!/bin/bash
# LAB ONLY: one native network apply on the explicitly disposable WNDR3800.
# Question: does rpcd restore management access without SSH or confirmation?
# From repo root: bash devices/commit-confirmed-network-lab.prototype.sh --check
# After operator approval: repeat with --execute (one apply, never retried).
set -euo pipefail

case ${1:-} in
    --check|--execute) mode=$1 ;;
    *) printf 'Usage: %s --check|--execute\n' "$0" >&2; exit 2 ;;
esac
for tool in ssh timeout jq; do command -v "$tool" >/dev/null; done

sid=
baseline=
apply_sent=0
automatic_restore=0
started=0
ssh_args=(-i certs/id_controller -o IdentitiesOnly=yes -o BatchMode=yes
    -o StrictHostKeyChecking=yes -o UserKnownHostsFile=certs/known_hosts
    -o ConnectTimeout=3 -o ServerAliveInterval=2 -o ServerAliveCountMax=2)

remote() {
    timeout --kill-after=2s 12s ssh "${ssh_args[@]}" root@10.128.128.3 "$@"
}
rpc() {
    local quoted
    printf -v quoted '%q' "$3"
    remote "ubus -t 8 call $1 $2 $quoted"
}
session_payload() {
    jq -cn --arg sid "$sid" '{ubus_rpc_session:$sid}'
}
hashes() {
    remote 'sha256sum /etc/config/network /etc/config/dhcp /etc/config/firewall /etc/config/dropbear /etc/config/system /etc/config/wireless'
}
boot_id() {
    remote 'read -r id < /proc/sys/kernel/random/boot_id; printf "%s\n" "$id"'
}
rpcd_pid() {
    rpc service list '{"name":"rpcd"}' | jq -er '.rpcd.instances.instance1 | select(.running) | .pid'
}
lan_ok() {
    jq -e '.up == true and .proto == "static" and
        any(."ipv4-address"[]?; .address == "10.128.128.3" and .mask == 24) and
        any(.route[]?; .target == "0.0.0.0" and .mask == 0 and .nexthop == "10.128.128.1")' >/dev/null
}

cleanup() {
    local status=$? current
    trap - EXIT
    if [[ -z $sid ]]; then exit "$status"; fi
    if ((apply_sent && !automatic_restore)); then
        # Never let a local assertion failure trigger early rollback and turn
        # manual recovery into apparent evidence of the native timer working.
        while ((SECONDS - started < 75)); do sleep 2; done
        printf 'Experiment did not prove recovery; attempting only its own native rollback for cleanup.\n' >&2
        rpc uci rollback "$(session_payload)" >/dev/null 2>&1 || true
    fi
    if ! rpc uci revert "$(jq -cn --arg sid "$sid" '{ubus_rpc_session:$sid,config:"network"}')"; then
        printf 'CLEANUP FAILED: lab staging remains; do not reapply or reboot blindly.\n' >&2
        exit 1
    fi
    if ! rpc session destroy "$(session_payload)"; then
        printf 'CLEANUP FAILED: lab session could not be destroyed.\n' >&2
        exit 1
    fi
    if ! current=$(hashes) || [[ $current != "$baseline" ]]; then
        printf 'FAIL: configuration differs from baseline; no forced file overwrite.\n' >&2
        exit 1
    fi
    printf 'PASS cleanup: lab session removed; all six config files match baseline.\n'
    exit "$status"
}
trap cleanup EXIT

printf 'Preflight: pinned identity, static LAN, no pending writes or native apply.\n'
remote 'set -eu
    read -r mac < /sys/class/net/eth0/address
    test "$mac" = "04:a1:51:96:a6:4d"
    test "$(uci -q get system.@system[0].hostname)" = "wndr3800ch"
    test "$(uci -q get network.lan.proto)" = "static"
    test "$(uci -q get network.lan.ipaddr)" = "10.128.128.3/24"
    test "$(uci -q get dhcp.lan.ignore)" = "1"
    pending=$(uci changes)
    test -z "$pending"
    for path in /var/run/rpcd/snapshot-files/* /var/run/rpcd/snapshot-delta/* /var/run/rpcd/uci-*; do
        if [ -e "$path" ]; then
            printf "STOP: existing rpcd transaction/staging: %s\n" "$path" >&2
            exit 1
        fi
    done
    ls -d /tmp'
rpc network.interface.lan status '{}' | lan_ok
baseline=$(hashes)
original_boot=$(boot_id)
original_rpcd=$(rpcd_pid)

# Validate the exact mutation and revert using a separate config/delta path.
read -r -d '' fixture_script <<'FIXTURE' || true
set -eu
fixture=$(mktemp -d /tmp/nerve-network-fixture.XXXXXX)
trap 'rm -rf "$fixture"' EXIT
mkdir "$fixture/delta"
cp /etc/config/network "$fixture/network"
uci -c "$fixture" -t "$fixture/delta" set network.lan.proto=none
test "$(uci -c "$fixture" -t "$fixture/delta" get network.lan.proto)" = none
uci -c "$fixture" -t "$fixture/delta" changes network
uci -c "$fixture" -t "$fixture/delta" revert network
test "$(uci -c "$fixture" -t "$fixture/delta" get network.lan.proto)" = static
cmp "$fixture/network" /etc/config/network
FIXTURE
printf 'Fixture: shell syntax, isolated proto change, revert and byte comparison.\n'
remote 'sh -n' <<< "$fixture_script"
remote 'sh -s' <<< "$fixture_script"
test "$(hashes)" = "$baseline"
printf 'PLAN: only network.lan.proto static -> none, native rollback=true, timeout=60, no confirm.\n'
if [[ $mode == --check ]]; then exit 0; fi

sid=$(rpc session create '{"timeout":300}' | jq -er '.ubus_rpc_session | select(test("^[0-9a-f]{32}$"))')
rpc session grant "$(jq -cn --arg sid "$sid" \
    '{ubus_rpc_session:$sid,scope:"uci",objects:[["network","read"],["network","write"]]}')"
rpc uci set "$(jq -cn --arg sid "$sid" \
    '{ubus_rpc_session:$sid,config:"network",section:"lan",values:{proto:"none"}}')"
rpc uci changes "$(session_payload)" | jq -e \
    '(.changes | keys) == ["network"] and .changes.network == [["set","lan","proto","none"]]' >/dev/null
test "$(hashes)" = "$baseline"

printf 'APPLY ONCE: no confirmation or recovery commands during the observation window.\n'
started=$SECONDS
apply_sent=1
if rpc uci apply "$(jq -cn --arg sid "$sid" '{ubus_rpc_session:$sid,rollback:true,timeout:60}')"; then
    printf 'Native apply acknowledged.\n'
else
    printf 'Apply acknowledgment lost/failed; observing outcome without retry.\n' >&2
fi

lost=0
recovered=0
failed_probes=0
while ((SECONDS - started < 150)); do
    if remote 'true' >/dev/null 2>&1; then
        if ((lost)) && state=$(rpc network.interface.lan status '{}' 2>/dev/null) && lan_ok <<< "$state"; then
            recovered=1
            printf 'OBSERVED t=%ss: SSH reachable again, static LAN address/default route restored.\n' "$((SECONDS - started))"
            break
        fi
    else
        failed_probes=$((failed_probes + 1))
        if ((failed_probes == 1)); then
            printf 'OBSERVED t=%ss: SSH management access unavailable.\n' "$((SECONDS - started))"
        fi
        if ((failed_probes >= 2)); then lost=1; fi
    fi
    sleep 2
done

if ((!lost || !recovered)); then
    printf 'FAIL/INCONCLUSIVE: access_loss=%s recovery=%s; not evidence of successful network rollback.\n' "$lost" "$recovered" >&2
    exit 1
fi
test "$(hashes)" = "$baseline"
test "$(boot_id)" = "$original_boot"
test "$(rpcd_pid)" = "$original_rpcd"
automatic_restore=1
printf 'PASS: loss and automatic recovery observed; same boot/rpcd PID, six config hashes unchanged.\n'
# EXIT removes only this session and its restored pending delta. No confirm is sent.
