#!/bin/sh
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
GUARD=$HERE/tailscale-guard.sh
LAB=$(mktemp -d)
trap 'rm -rf "$LAB"' EXIT

export TS_GUARD_ROOT=$LAB/journal
export TS_GUARD_NET_CONFIG=$LAB/network
export TS_GUARD_FW_CONFIG=$LAB/firewall
export TS_GUARD_TS_INIT=$LAB/init_tailscale
export TS_GUARD_FW_RELOAD=$LAB/init_firewall
export TS_GUARD_NET_RELOAD=$LAB/init_network
export TS_GUARD_BOOT_FILE=$LAB/boot
export TS_GUARD_UPTIME_FILE=$LAB/uptime
export TS_GUARD_LOCK=$LAB/lock
export TS_GUARD_SKIP_HEALTH=1

# Dummy init scripts
printf '#!/bin/sh\nexit 0\n' > "$TS_GUARD_TS_INIT"
printf '#!/bin/sh\nexit 0\n' > "$TS_GUARD_FW_RELOAD"
printf '#!/bin/sh\nexit 0\n' > "$TS_GUARD_NET_RELOAD"
chmod +x "$TS_GUARD_TS_INIT" "$TS_GUARD_FW_RELOAD" "$TS_GUARD_NET_RELOAD"

printf 'old-boot-uuid\n' > "$TS_GUARD_BOOT_FILE"
printf '100.00 0.00\n' > "$TS_GUARD_UPTIME_FILE"

# Initial configs
printf 'config interface "lan"\n' > "$TS_GUARD_NET_CONFIG"
printf 'config zone "lan"\n' > "$TS_GUARD_FW_CONFIG"

chmod +x "$GUARD"

# 1. Test arm and status
"$GUARD" arm test1 60
STATUS=$("$GUARD" status)
echo "$STATUS" | grep -q '"state":"ACTIVE"'
echo "$STATUS" | grep -q '"id":"test1"'

# 2. Modify config as if candidate was applied
printf 'config interface "tailscale"\n' >> "$TS_GUARD_NET_CONFIG"
"$GUARD" apply test1

# 3. Simulate TTL expiry
printf '165.00 0.00\n' > "$TS_GUARD_UPTIME_FILE"

# Confirm should fail because TTL expired
if "$GUARD" confirm test1; then
    echo "Accepted expired confirmation!" >&2
    exit 1
fi

# Check should have triggered restore
"$GUARD" check
grep -q -v 'config interface "tailscale"' "$TS_GUARD_NET_CONFIG"
test "$(cat "$TS_GUARD_ROOT/restored-test1/result")" = RESTORED

# 4. Test successful arm -> apply -> confirm cycle
printf '200.00 0.00\n' > "$TS_GUARD_UPTIME_FILE"
"$GUARD" arm test2 60
printf 'config interface "tailscale"\n' >> "$TS_GUARD_NET_CONFIG"
"$GUARD" apply test2
"$GUARD" confirm test2
test "$(cat "$TS_GUARD_ROOT/committed-test2/result")" = COMMITTED
grep -q 'config interface "tailscale"' "$TS_GUARD_NET_CONFIG"

# 5. Test explicit rollback
printf '300.00 0.00\n' > "$TS_GUARD_UPTIME_FILE"
"$GUARD" arm test3 60
printf 'config interface "broken"\n' >> "$TS_GUARD_NET_CONFIG"
"$GUARD" rollback test3
grep -q -v 'config interface "broken"' "$TS_GUARD_NET_CONFIG"
test "$(cat "$TS_GUARD_ROOT/restored-test3/result")" = RESTORED

# 6. Test boot recovery (unexpected reboot while active)
printf '400.00 0.00\n' > "$TS_GUARD_UPTIME_FILE"
"$GUARD" arm test4 60
printf 'config interface "unconfirmed"\n' >> "$TS_GUARD_NET_CONFIG"
# Machine boots with new boot id
printf 'new-boot-uuid\n' > "$TS_GUARD_BOOT_FILE"
"$GUARD" boot
grep -q -v 'config interface "unconfirmed"' "$TS_GUARD_NET_CONFIG"
test "$(cat "$TS_GUARD_ROOT/restored-test4/result")" = RESTORED

# 7. Test health check failure triggers immediate rollback
export TS_GUARD_SKIP_HEALTH=0
export TS_GUARD_HEALTH_CHECK_TARGET="192.0.2.1" # Non-routable TEST-NET-1 IP
export TS_GUARD_HEALTH_CHECK_GW="192.0.2.1"
printf '500.00 0.00\n' > "$TS_GUARD_UPTIME_FILE"
"$GUARD" arm test5 60
printf 'config interface "break-ping"\n' >> "$TS_GUARD_NET_CONFIG"
if "$GUARD" apply test5; then
    echo "Apply should have failed and rolled back due to health check" >&2
    exit 1
fi
grep -q -v 'config interface "break-ping"' "$TS_GUARD_NET_CONFIG"
test "$(cat "$TS_GUARD_ROOT/restored-test5/result")" = RESTORED

echo "PASS: tailscale-guard unit tests passed completely."
