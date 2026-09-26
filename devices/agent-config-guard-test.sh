#!/bin/sh
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
GUARD=$HERE/agent-config-guard.sh
LAB=$(mktemp -d)
trap 'rm -rf "$LAB"' EXIT
export GUARD_ROOT=$LAB/journal GUARD_TARGET=$LAB/agent.conf
export GUARD_AGENT=$LAB/service GUARD_BOOT_FILE=$LAB/boot
export GUARD_UPTIME_FILE=$LAB/uptime GUARD_LOCK=$LAB/lock
printf '#!/bin/sh\nexit 0\n' > "$GUARD_AGENT"
chmod +x "$GUARD_AGENT"
printf 'old-boot\n' > "$GUARD_BOOT_FILE"
printf '100.00 0.00\n' > "$GUARD_UPTIME_FILE"
printf 'CONTROLLER_URL="http://old.invalid/api"\n' > "$GUARD_TARGET"
cp "$GUARD_TARGET" "$LAB/original"
chmod 600 "$GUARD_TARGET"
printf 'CONTROLLER_URL="https://new.invalid/api"\n' > "$LAB/candidate"

"$GUARD" arm timeout 30 "$LAB/candidate"
"$GUARD" apply timeout
cmp "$GUARD_TARGET" "$LAB/candidate"
printf '131.00 0.00\n' > "$GUARD_UPTIME_FILE"
if "$GUARD" confirm timeout; then echo 'Accepted expired confirmation' >&2; exit 1; fi
"$GUARD" check
cmp "$GUARD_TARGET" "$LAB/original"
test "$(cat "$GUARD_ROOT/restored-timeout/result")" = RESTORED

"$GUARD" arm reboot 30 "$LAB/candidate"
"$GUARD" apply reboot
printf 'new-boot\n' > "$GUARD_BOOT_FILE"
printf '1.00 0.00\n' > "$GUARD_UPTIME_FILE"
"$GUARD" boot
cmp "$GUARD_TARGET" "$LAB/original"

"$GUARD" arm success 30 "$LAB/candidate"
if "$GUARD" arm duplicate 30 "$LAB/candidate"; then exit 1; fi
"$GUARD" apply success
if "$GUARD" confirm wrong-id; then exit 1; fi
"$GUARD" confirm success
printf '1000.00 0.00\n' > "$GUARD_UPTIME_FILE"
"$GUARD" check
cmp "$GUARD_TARGET" "$LAB/candidate"
test "$(cat "$GUARD_ROOT/committed-success/result")" = COMMITTED
test ! -e "$GUARD_ROOT/active"

"$GUARD" arm retry 30 "$LAB/original"
"$GUARD" apply retry
printf '#!/bin/sh\nexit 1\n' > "$GUARD_AGENT"
printf '2000.00 0.00\n' > "$GUARD_UPTIME_FILE"
if "$GUARD" check; then echo 'Service failure was ignored' >&2; exit 1; fi
test -d "$GUARD_ROOT/active"
printf '#!/bin/sh\nexit 0\n' > "$GUARD_AGENT"
"$GUARD" check
test ! -e "$GUARD_ROOT/active"
cmp "$GUARD_TARGET" "$LAB/candidate"

"$GUARD" arm corrupt 30 "$LAB/original"
printf '# tampered\n' >> "$GUARD_ROOT/active/candidate"
if "$GUARD" apply corrupt; then echo 'Corrupt candidate was accepted' >&2; exit 1; fi
cmp "$GUARD_TARGET" "$LAB/candidate"
printf '3000.00 0.00\n' > "$GUARD_UPTIME_FILE"
"$GUARD" check
"$GUARD" arm abort 30 "$LAB/original"
"$GUARD" apply abort
if "$GUARD" rollback wrong-id; then exit 1; fi
"$GUARD" rollback abort
cmp "$GUARD_TARGET" "$LAB/candidate"
test "$(cat "$GUARD_ROOT/restored-abort/result")" = RESTORED
echo 'PASS: timeout, expired/wrong confirmation, boot recovery, duplicate fencing, commit, restore retry, candidate integrity, fenced explicit rollback.'
