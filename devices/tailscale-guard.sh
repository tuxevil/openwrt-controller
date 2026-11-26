#!/bin/sh
# Fail-safe commit-confirmed manager for Tailscale deployment on OpenWrt.
# Guarantees automatic rollback to pristine network/firewall backups if
# connectivity, reachability, or confirmation fails within the TTL window.
set -eu
umask 077

ROOT=${TS_GUARD_ROOT:-/etc/nerve/tailscale-guard}
NET_CONFIG=${TS_GUARD_NET_CONFIG:-/etc/config/network}
FW_CONFIG=${TS_GUARD_FW_CONFIG:-/etc/config/firewall}
TS_INIT=${TS_GUARD_TS_INIT:-/etc/init.d/tailscale}
FW_RELOAD=${TS_GUARD_FW_RELOAD:-/etc/init.d/firewall}
NET_RELOAD=${TS_GUARD_NET_RELOAD:-/etc/init.d/network}
BOOT_FILE=${TS_GUARD_BOOT_FILE:-/proc/sys/kernel/random/boot_id}
UPTIME_FILE=${TS_GUARD_UPTIME_FILE:-/proc/uptime}
LOCK=${TS_GUARD_LOCK:-/var/lock/nerve-tailscale-guard.lock}
HEALTH_CHECK_TARGET=${TS_GUARD_HEALTH_CHECK_TARGET:-1.1.1.1}
HEALTH_CHECK_GW=${TS_GUARD_HEALTH_CHECK_GW:-10.128.128.1}

mkdir -p "$ROOT"

now() {
    if [ -f "$UPTIME_FILE" ]; then
        cut -d. -f1 < "$UPTIME_FILE"
    else
        date +%s
    fi
}

boot() {
    if [ -f "$BOOT_FILE" ]; then
        cat "$BOOT_FILE"
    else
        echo "default-boot"
    fi
}

digest() {
    if [ -f "$1" ]; then
        sha256sum "$1" | awk '{print $1}'
    else
        echo "none"
    fi
}

valid_id() {
    case "$1" in
        ''|*[!a-zA-Z0-9_-]*) return 1;;
    esac
}

expired() {
    [ "$(cat "$ROOT/active/boot" 2>/dev/null || true)" = "$(boot)" ] || return 0
    deadline=$(cat "$ROOT/active/deadline" 2>/dev/null || true)
    case "$deadline" in
        ''|*[!0-9]*) return 0;;
    esac
    [ "$(now)" -ge "$deadline" ]
}

reload_services() {
    # Reload firewall and network safely
    if [ -x "$FW_RELOAD" ]; then
        "$FW_RELOAD" reload >/dev/null 2>&1 || true
    elif command -v fw4 >/dev/null 2>&1; then
        fw4 reload >/dev/null 2>&1 || true
    elif command -v fw3 >/dev/null 2>&1; then
        fw3 reload >/dev/null 2>&1 || true
    fi

    if [ -x "$NET_RELOAD" ]; then
        "$NET_RELOAD" reload >/dev/null 2>&1 || true
    fi
}

stop_tailscale() {
    if command -v tailscale >/dev/null 2>&1; then
        tailscale down >/dev/null 2>&1 || true
    fi
    if [ -x "$TS_INIT" ]; then
        "$TS_INIT" stop >/dev/null 2>&1 || true
    fi
}

restore() {
    # Restore network and firewall configurations from backups
    if [ -f "$ROOT/active/orig_network" ]; then
        cp -p "$ROOT/active/orig_network" "$NET_CONFIG.restore.$$"
        mv -f "$NET_CONFIG.restore.$$" "$NET_CONFIG"
    fi

    if [ -f "$ROOT/active/orig_firewall" ]; then
        cp -p "$ROOT/active/orig_firewall" "$FW_CONFIG.restore.$$"
        mv -f "$FW_CONFIG.restore.$$" "$FW_CONFIG"
    fi

    stop_tailscale
    reload_services
    sync

    id=$(cat "$ROOT/active/id" 2>/dev/null || echo "unknown")
    printf '%s\n' RESTORED > "$ROOT/active/result"
    mv "$ROOT/active" "$ROOT/restored-$id"
    sync
}

health_check() {
    # In test mode or when TS_GUARD_SKIP_HEALTH=1 is set, pass
    [ "${TS_GUARD_SKIP_HEALTH:-0}" = "1" ] && return 0

    # 1. Verify default gateway / local link ping
    if command -v ping >/dev/null 2>&1; then
        if [ -n "$HEALTH_CHECK_GW" ]; then
            ping -c 1 -W 2 "$HEALTH_CHECK_GW" >/dev/null 2>&1 || return 1
        fi
        # 2. Verify WAN connectivity (1.1.1.1)
        if [ -n "$HEALTH_CHECK_TARGET" ]; then
            ping -c 1 -W 3 "$HEALTH_CHECK_TARGET" >/dev/null 2>&1 || return 1
        fi
    fi
    return 0
}

check() {
    [ -d "$ROOT/active" ] || return 0
    if expired; then
        restore
    fi
}

locked() {
    (
        flock -x 9
        "$@"
    ) 9>"$LOCK"
}

arm() {
    id=$1
    ttl=$2
    valid_id "$id"

    case "$ttl" in
        ''|*[!0-9]*) return 1;;
    esac
    [ "$ttl" -ge 30 ] && [ "$ttl" -le 1800 ] || return 1

    [ ! -e "$ROOT/active" ] || return 1
    [ ! -e "$ROOT/prepared-$id" ] || return 1
    [ ! -e "$ROOT/restored-$id" ] || return 1
    [ ! -e "$ROOT/committed-$id" ] || return 1

    mkdir -p "$ROOT/prepared-$id"

    if [ -f "$NET_CONFIG" ]; then
        cp -p "$NET_CONFIG" "$ROOT/prepared-$id/orig_network"
        digest "$NET_CONFIG" > "$ROOT/prepared-$id/orig_network.sha256"
    fi

    if [ -f "$FW_CONFIG" ]; then
        cp -p "$FW_CONFIG" "$ROOT/prepared-$id/orig_firewall"
        digest "$FW_CONFIG" > "$ROOT/prepared-$id/orig_firewall.sha256"
    fi

    boot > "$ROOT/prepared-$id/boot"
    printf '%s\n' "$id" > "$ROOT/prepared-$id/id"
    printf '%s\n' "$(( $(now) + ttl ))" > "$ROOT/prepared-$id/deadline"
    sync
    mv "$ROOT/prepared-$id" "$ROOT/active"
    sync
}

apply() {
    id=$1
    [ "$(cat "$ROOT/active/id" 2>/dev/null || true)" = "$id" ] || return 1
    if expired; then
        restore
        return 1
    fi

    # Perform post-apply health check
    if ! health_check; then
        # Health check failed immediately after mutation -> Rollback now!
        restore
        return 2
    fi
    return 0
}

confirm() {
    id=$1
    [ "$(cat "$ROOT/active/id" 2>/dev/null || true)" = "$id" ] || return 1
    if expired; then
        restore
        return 1
    fi

    printf '%s\n' COMMITTED > "$ROOT/active/result"
    mv "$ROOT/active" "$ROOT/committed-$id"
    sync
}

rollback() {
    id=$1
    [ "$(cat "$ROOT/active/id" 2>/dev/null || true)" = "$id" ] || return 1
    restore
}

status() {
    if [ -d "$ROOT/active" ]; then
        id=$(cat "$ROOT/active/id" 2>/dev/null || echo "unknown")
        deadline=$(cat "$ROOT/active/deadline" 2>/dev/null || echo "0")
        printf '{"state":"ACTIVE","id":"%s","deadline":%s,"remaining":%d}\n' \
            "$id" "$deadline" "$(( deadline - $(now) ))"
    else
        printf '{"state":"IDLE"}\n'
    fi
}

boot_hook() {
    # Invoked on system boot: check if an uncommitted active transaction was left behind
    [ -d "$ROOT/active" ] || return 0
    restore
}

cmd=${1:-}
shift || true

case "$cmd" in
    arm) locked arm "$@" ;;
    apply) locked apply "$@" ;;
    confirm) locked confirm "$@" ;;
    rollback) locked rollback "$@" ;;
    check) locked check "$@" ;;
    boot) locked boot_hook ;;
    status) status ;;
    *)
        echo "Usage: $0 {arm <id> <ttl>|apply <id>|confirm <id>|rollback <id>|check|boot|status}" >&2
        exit 1
        ;;
esac
