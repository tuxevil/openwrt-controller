const fs = require('fs');

let agent = fs.readFileSync('devices/agent.sh', 'utf8');

const replacement = `    # 2.5 BANDWIDTH SENTINEL (Top Talkers)
    STATE_FILE="/tmp/bandwidth_state"
    TOP_TALKERS=$(
        for IFACE in $(ls /sys/class/net | grep -E "wlan|ath|radio|ra|phy"); do
            iw dev "$IFACE" station dump 2>/dev/null | awk '
                /^Station/ { mac=$2 }
                /RX bytes:/ { rx=$3 }
                /TX bytes:/ { tx=$3; print mac, rx, tx }
            '
        done | awk -v state_file="$STATE_FILE" -v ts="$(date +%s)" '
            BEGIN {
                while ((getline < state_file) > 0) {
                    if (NF >= 4) {
                        prev_rx[$1] = $2; prev_tx[$1] = $3; prev_ts[$1] = $4
                    }
                }
                close(state_file)
            }
            {
                mac = $1; rx = $2; tx = $3
                if (mac in prev_rx) {
                    dt = ts - prev_ts[mac]
                    if (dt == 0) dt = 1
                    diff_rx = rx - prev_rx[mac]
                    diff_tx = tx - prev_tx[mac]
                    if (diff_rx < 0) diff_rx = 0
                    if (diff_tx < 0) diff_tx = 0
                    rate_rx = int(diff_rx / dt)
                    rate_tx = int(diff_tx / dt)
                    total_rate = rate_rx + rate_tx
                    
                    rates[mac] = total_rate
                    details_rx[mac] = rate_rx
                    details_tx[mac] = rate_tx
                }
                new_state[mac] = rx " " tx " " ts
            }
            END {
                printf "" > state_file
                for (m in new_state) {
                    print m, new_state[m] > state_file
                }
                close(state_file)
                
                n = 0
                for (m in rates) { arr[n] = m; n++ }
                for (i=0; i<n; i++) {
                    for (j=i+1; j<n; j++) {
                        if (rates[arr[j]] > rates[arr[i]]) {
                            temp = arr[i]; arr[i] = arr[j]; arr[j] = temp
                        }
                    }
                }
                
                printf "["
                limit = (n < 5) ? n : 5
                for (i=0; i<limit; i++) {
                    m = arr[i]
                    if (i > 0) printf ","
                    printf "{\\"mac\\":\\"%s\\",\\"rate_rx\\":%d,\\"rate_tx\\":%d,\\"total_rate\\":%d}", m, details_rx[m], details_tx[m], rates[m]
                }
                printf "]"
            }
        '
    )
    [ -z "$TOP_TALKERS" ] && TOP_TALKERS="[]"

    IFACE_STATS=$(awk '
        BEGIN { printf "{" }
        NR > 2 {
            sub(/:/, "", $1)
            if (count > 0) printf ","
            printf "\\"%s\\":{\\"rx_bytes\\":%s,\\"tx_bytes\\":%s}", $1, $2, $10
            count++
        }
        END { printf "}" }
    ' /proc/net/dev)

    # 3. DESCUBRIMIENTO L2 (Tabla ARP y Bridge)`;

agent = agent.replace('    # 3. DESCUBRIMIENTO L2 (Tabla ARP y Bridge)', replacement);

const payloadReplacement = `"wireless_stations": $WIFI_DATA,
    "top_talkers": $TOP_TALKERS,
    "iface_stats": $IFACE_STATS,
    "arp_table":`;

agent = agent.replace('"wireless_stations": $WIFI_DATA,\n    "arp_table":', payloadReplacement);

fs.writeFileSync('devices/agent.sh', agent);
console.log("Device patched successfully");
