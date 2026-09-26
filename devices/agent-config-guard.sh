#!/bin/sh
# Local, durable commit-confirmed changes to the agent runtime configuration.
# Does not modify UCI or restart networking. Install with agent-config-guard.init.
set -eu
umask 077
ROOT=${GUARD_ROOT:-/etc/nerve/config-guard}
TARGET=${GUARD_TARGET:-/etc/nerve/agent.conf}
AGENT=${GUARD_AGENT:-/etc/init.d/agent}
BOOT_FILE=${GUARD_BOOT_FILE:-/proc/sys/kernel/random/boot_id}
UPTIME_FILE=${GUARD_UPTIME_FILE:-/proc/uptime}
LOCK=${GUARD_LOCK:-/var/lock/nerve-config-guard.lock}
mkdir -p "$ROOT"
now() { cut -d. -f1 < "$UPTIME_FILE"; }
boot() { cat "$BOOT_FILE"; }
digest() { sha256sum "$1" | awk '{print $1}'; }
valid_id() { case "$1" in ''|*[!a-zA-Z0-9_-]*) return 1;; esac; }
expired() {
    [ "$(cat "$ROOT/active/boot")" = "$(boot)" ] || return 0
    deadline=$(cat "$ROOT/active/deadline")
    case "$deadline" in ''|*[!0-9]*) return 0;; esac
    [ "$(now)" -ge "$deadline" ]
}
restore() {
    # Keep the journal active until both the file and service are recovered.
    # During boot, agent's normal S99 startup will consume the restored file.
    [ "$(digest "$ROOT/active/original")" = "$(cat "$ROOT/active/original.sha256")" ]
    cp -p "$ROOT/active/original" "$TARGET.guard-restore"
    mv -f "$TARGET.guard-restore" "$TARGET"
    sync
    if [ "${1:-}" != boot ]; then "$AGENT" restart 9>&-; fi
    printf '%s\n' RESTORED > "$ROOT/active/result"
    mv "$ROOT/active" "$ROOT/restored-$(cat "$ROOT/active/id")"
    sync
}
check() {
    [ -d "$ROOT/active" ] || return 0
    if expired; then
        if [ "${1:-}" = boot ] && [ "$(cat "$ROOT/active/boot")" != "$(boot)" ]; then
            restore boot
        else
            restore
        fi
    fi
}
locked() {
    (
        flock -x 9
        "$@"
    ) 9>"$LOCK"
}
arm() {
    id=$1; ttl=$2; candidate=$3
    valid_id "$id"
    case "$ttl" in ''|*[!0-9]*) return 1;; esac
    [ "$ttl" -ge 30 ] && [ "$ttl" -le 1800 ] || return 1
    [ ! -e "$ROOT/active" ] && [ ! -e "$ROOT/prepared-$id" ] || return 1
    [ ! -e "$ROOT/restored-$id" ] && [ ! -e "$ROOT/committed-$id" ] || return 1
    [ -f "$TARGET" ] && [ ! -L "$TARGET" ] || return 1
    /bin/sh -n "$candidate"
    mkdir "$ROOT/prepared-$id"
    cp -p "$TARGET" "$ROOT/prepared-$id/original"
    digest "$TARGET" > "$ROOT/prepared-$id/original.sha256"
    cp "$candidate" "$ROOT/prepared-$id/candidate"
    chmod 600 "$ROOT/prepared-$id/candidate"
    digest "$candidate" > "$ROOT/prepared-$id/candidate.sha256"
    boot > "$ROOT/prepared-$id/boot"
    printf '%s\n' "$id" > "$ROOT/prepared-$id/id"
    printf '%s\n' "$(( $(now) + ttl ))" > "$ROOT/prepared-$id/deadline"
    sync
    mv "$ROOT/prepared-$id" "$ROOT/active"
    sync
}
apply() {
    [ "$(cat "$ROOT/active/id")" = "$1" ]
    if expired; then return 1; fi
    [ "$(digest "$TARGET")" = "$(cat "$ROOT/active/original.sha256")" ]
    [ "$(digest "$ROOT/active/candidate")" = "$(cat "$ROOT/active/candidate.sha256")" ]
    cp "$ROOT/active/candidate" "$TARGET.guard-new"
    chmod 600 "$TARGET.guard-new"
    mv -f "$TARGET.guard-new" "$TARGET"
    sync
    "$AGENT" restart 9>&-
}
confirm() {
    [ "$(cat "$ROOT/active/id")" = "$1" ]
    if expired; then return 1; fi
    [ "$(digest "$TARGET")" = "$(cat "$ROOT/active/candidate.sha256")" ]
    printf '%s\n' COMMITTED > "$ROOT/active/result"
    mv "$ROOT/active" "$ROOT/committed-$1"
    sync
}
case "${1:-}" in
    arm) [ "$#" -eq 4 ]; locked arm "$2" "$3" "$4";;
    apply) [ "$#" -eq 2 ]; locked apply "$2";;
    confirm) [ "$#" -eq 2 ]; locked confirm "$2";;
    check) locked check;;
    boot) locked check boot;;
    worker)
        while :; do
            # A failure leaves the durable journal active for the next retry.
            "$0" check || logger -t nerve-config-guard 'Restore failed; retrying'
            sleep 2
        done;;
    *) echo 'Usage: agent-config-guard {arm ID TTL CANDIDATE|apply ID|confirm ID|check|boot|worker}' >&2; exit 2;;
esac
