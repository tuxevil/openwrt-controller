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

echo 'agent update commit cleanup passed'
