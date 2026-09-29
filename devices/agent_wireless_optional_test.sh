#!/bin/sh
set -eu

agent=${AGENT_UNDER_TEST:-$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/agent.sh}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

cat >"$work/jsonfilter" <<'EOF'
#!/bin/sh
# A missing optional JSON path has jsonfilter's real exit status.
exit 1
EOF
cat >"$work/ubus" <<'EOF'
#!/bin/sh
printf '{}\n'
EOF
chmod +x "$work/jsonfilter" "$work/ubus"

awk '/^[[:space:]]*(R_BAND|R_HW|R_CHAN|W_KEY|W_ROAMING|W_80211K|W_80211V|W_MFP|W_AUTH_SERVER|W_AUTH_SECRET|W_DYN_VLAN)=/ { print }' "$agent" >"$work/optional-read.sh"
test "$(wc -l <"$work/optional-read.sh")" -eq 11

PATH="$work:$PATH" sh -c '
    set -e
    CONFIG_RESPONSE="{}"
    RADIO=radio0
    i=0
    . "$1"
    test -z "$W_80211K" && test -z "$W_ROAMING" && test -z "$W_MFP"
' sh "$work/optional-read.sh"

echo 'wireless optional-field reads passed'
