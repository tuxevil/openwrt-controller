#!/bin/sh
set -eu

agent=${AGENT_UNDER_TEST:-$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/agent.sh}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

cat >"$work/uci" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "-q" ]; then
    shift
fi
case "${1:-}" in
    show)
        cat "$MOCK_UCI_SHOW"
        ;;
    get)
        awk -F= -v key="${2:-}" '$1 == key { sub(/^[^=]*=/, ""); print; found=1; exit } END { exit(found ? 0 : 1) }' "$MOCK_UCI_GET"
        ;;
    set|add_list)
        printf '%s %s\n' "$1" "$2" >> "$MOCK_UCI_LOG"
        ;;
    *)
        exit 0
        ;;
esac
EOF
chmod +x "$work/uci"

cat >"$work/show.txt" <<'EOF'
wireless.radio0=wifi-device
wireless.radio2=wifi-device
wireless.cfg_radio0_2=wifi-iface
wireless.cfg_radio2_2=wifi-iface
wireless.cfg_radio2_3=wifi-iface
wireless.cfg_radio2_4=wifi-iface
EOF

cat >"$work/get.txt" <<'EOF'
wireless.cfg_radio0_2.device=radio0
wireless.cfg_radio0_2.ssid=tuxcave
wireless.cfg_radio2_2.device=radio2
wireless.cfg_radio2_2.ssid=tuxcave
wireless.cfg_radio2_2.macfilter=deny
wireless.cfg_radio2_2.maclist=3c:28:6d:0f:85:db aa:bb:cc:dd:ee:ff
wireless.cfg_radio2_3.device=radio2
wireless.cfg_radio2_3.ssid=Guest WiFi
wireless.cfg_radio2_3.macfilter=allow
wireless.cfg_radio2_3.maclist=11:22:33:44:55:66
wireless.cfg_radio2_4.device=radio2
wireless.cfg_radio2_4.ssid=listonly
wireless.cfg_radio2_4.maclist=77:88:99:aa:bb:cc
EOF

# 1. Preserves macfilter and multi-entry maclist across section rename (cfg_radio2_2 -> cfg_radio2_0).
: >"$work/log.txt"
PATH="$work:$PATH" MOCK_UCI_SHOW="$work/show.txt" MOCK_UCI_GET="$work/get.txt" MOCK_UCI_LOG="$work/log.txt" \
    sh "$agent" --self-test-wireless-macfilter cfg_radio2_0 radio2 tuxcave
cat >"$work/expected.txt" <<'EOF'
set wireless.cfg_radio2_0.macfilter=deny
add_list wireless.cfg_radio2_0.maclist=3c:28:6d:0f:85:db
add_list wireless.cfg_radio2_0.maclist=aa:bb:cc:dd:ee:ff
EOF
diff -u "$work/expected.txt" "$work/log.txt"

# 2. Preserves SSIDs containing spaces.
: >"$work/log.txt"
PATH="$work:$PATH" MOCK_UCI_SHOW="$work/show.txt" MOCK_UCI_GET="$work/get.txt" MOCK_UCI_LOG="$work/log.txt" \
    sh "$agent" --self-test-wireless-macfilter cfg_radio2_9 radio2 "Guest WiFi"
cat >"$work/expected.txt" <<'EOF'
set wireless.cfg_radio2_9.macfilter=allow
add_list wireless.cfg_radio2_9.maclist=11:22:33:44:55:66
EOF
diff -u "$work/expected.txt" "$work/log.txt"

# 3. Empty macfilter field does not collapse unit-separator delimiter before maclist.
: >"$work/log.txt"
PATH="$work:$PATH" MOCK_UCI_SHOW="$work/show.txt" MOCK_UCI_GET="$work/get.txt" MOCK_UCI_LOG="$work/log.txt" \
    sh "$agent" --self-test-wireless-macfilter cfg_radio2_8 radio2 listonly
cat >"$work/expected.txt" <<'EOF'
add_list wireless.cfg_radio2_8.maclist=77:88:99:aa:bb:cc
EOF
diff -u "$work/expected.txt" "$work/log.txt"

# 4. Different radio for the same SSID does not inherit radio2's macfilter.
: >"$work/log.txt"
PATH="$work:$PATH" MOCK_UCI_SHOW="$work/show.txt" MOCK_UCI_GET="$work/get.txt" MOCK_UCI_LOG="$work/log.txt" \
    sh "$agent" --self-test-wireless-macfilter cfg_radio0_0 radio0 tuxcave
test ! -s "$work/log.txt"

echo 'wireless local macfilter preservation passed'
