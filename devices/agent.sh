#!/bin/sh

# === CONFIGURACIÓN ===
# LLAVE DE SITIO (Obtenida del Dashboard -> Settings)
SITE_KEY="TU_API_KEY_AQUI"
CONTROLLER_IP="REPLACE_WITH_CONTROLLER_IP"
PORT="3000"
BASE_URL="http://$CONTROLLER_IP:$PORT/api"
TELEMETRY_URL="$BASE_URL/telemetry"
# Obtener MAC de la interfaz puente como ID único
DEVICE_ID=$(cat /sys/class/net/br-lan/address 2>/dev/null | tr '[:lower:]' '[:upper:]' || cat /sys/class/net/eth0/address 2>/dev/null | tr '[:lower:]' '[:upper:]')
CONFIG_URL="$BASE_URL/devices/$DEVICE_ID/config"

# logd is a local dependency, not part of the telemetry heartbeat.  On some
# OpenWrt builds logread can remain blocked on the logd socket; running it in
# the telemetry pipeline would then stop the agent before the POST forever.
# Keep the collection bounded and let telemetry continue when logd is stuck.
collect_recent_logs() {
    local log_file="/tmp/nerve-agent-logread.$$"
    local log_pid
    local waited=0

    rm -f "$log_file"
    logread -l 20 >"$log_file" 2>/dev/null &
    log_pid=$!

    while kill -0 "$log_pid" 2>/dev/null; do
        if [ "$waited" -ge 2 ]; then
            kill "$log_pid" 2>/dev/null
            wait "$log_pid" 2>/dev/null
            rm -f "$log_file"
            return 0
        fi
        sleep 1
        waited=$((waited + 1))
    done

    wait "$log_pid" 2>/dev/null
    sed 's/\\/\\\\/g; s/"/\\"/g' "$log_file" | awk '{printf "%s\\n", $0}'
    rm -f "$log_file"
}

# Local/CI seam for the bounded log collector.  It does not start the agent
# loop or touch device configuration.
if [ "${1:-}" = "--self-test-log-collection" ]; then
    collect_recent_logs
    exit 0
fi

# Instalar dependencias si faltan (opcional)
if ! command -v tcpdump >/dev/null 2>&1; then
    logger -t agent "Installing missing tcpdump..."
    if command -v apk >/dev/null 2>&1; then
        apk update
        apk add tcpdump iperf3 sqm-scripts kmod-sched-cake tailscale
        apk search -e libndpi | grep -q libndpi && apk add libndpi || true
        if apk info -e wpad-basic-wolfssl >/dev/null 2>&1 || apk info -e wpad-basic-mbedtls >/dev/null 2>&1; then
            apk del wpad-basic-wolfssl wpad-basic-mbedtls 2>/dev/null || true
            apk add wpad-mesh-wolfssl || true
        fi
    elif command -v opkg >/dev/null 2>&1; then
        opkg update
        if opkg list-installed | grep -q "wpad-basic"; then opkg remove wpad-basic-wolfssl wpad-basic-mbedtls; opkg install wpad-mesh-wolfssl; fi
        opkg install tcpdump iperf3 sqm-scripts kmod-sched-cake tailscale
        opkg list libndpi | grep -q libndpi && opkg install libndpi || true
    fi
fi
# apk update && apk add iwinfo curl

T_FAILS=0

while true; do
    # 0. CHECK AUTO-UPDATE
    # Reconstruct default config before hashing to match the raw database version_hash
    AGENT_VERSION=$(sed -e 's|^SITE_KEY=.*|SITE_KEY="TU_API_KEY_AQUI"|' \
                        -e 's|^CONTROLLER_IP=.*|CONTROLLER_IP="REPLACE_WITH_CONTROLLER_IP"|' \
                        -e 's|^PORT=.*|PORT="3000"|' "$0" | sha256sum | awk '{print $1}')
    LATEST_JSON=$(curl -m 5 -s -X GET -H "X-Site-Key: $SITE_KEY" "$BASE_URL/agent/latest")
    
    if [ -n "$LATEST_JSON" ]; then
        LATEST_HASH=$(echo "$LATEST_JSON" | jsonfilter -e '@.version_hash' 2>/dev/null)
        if [ -n "$LATEST_HASH" ] && [ "$LATEST_HASH" != "$AGENT_VERSION" ]; then
            logger -t agent "New agent version found: $LATEST_HASH. Downloading..."
            if curl -m 10 -s -X GET -H "X-Site-Key: $SITE_KEY" "$BASE_URL/agent/latest/raw" -o "$0.tmp"; then
                TMP_HASH=$(sha256sum "$0.tmp" | awk '{print $1}')
                if [ "$TMP_HASH" = "$LATEST_HASH" ]; then
                    logger -t agent "Agent downloaded securely. Updating and restarting."
                    
                    # Preserve config from the current agent script
                    CURRENT_SITE_KEY=$(grep -E "^SITE_KEY=" "$0" | cut -d'"' -f2)
                    CURRENT_CONTROLLER_IP=$(grep -E "^CONTROLLER_IP=" "$0" | cut -d'"' -f2)
                    CURRENT_PORT=$(grep -E "^PORT=" "$0" | cut -d'"' -f2)
                    
                    # Replace default config in the new agent script if they were set in the old one
                    [ -n "$CURRENT_SITE_KEY" ] && sed -i "s|^SITE_KEY=.*|SITE_KEY=\"$CURRENT_SITE_KEY\"|" "$0.tmp"
                    [ -n "$CURRENT_CONTROLLER_IP" ] && sed -i "s|^CONTROLLER_IP=.*|CONTROLLER_IP=\"$CURRENT_CONTROLLER_IP\"|" "$0.tmp"
                    [ -n "$CURRENT_PORT" ] && sed -i "s|^PORT=.*|PORT=\"$CURRENT_PORT\"|" "$0.tmp"
                    
                    cp "$0" "$0.old"
                    mv "$0.tmp" "$0"
                    chmod +x "$0"
                    logger -t agent "Agent updated. Reloading in-process to preserve procd respawn budget."
                    # Use exec to re-exec the new script in the same PID.
                    # procd never sees a process exit, so the crash counter is preserved
                    # across self-updates (5 clean exits within 1h would otherwise mark
                    # the instance as crashed and procd would stop respawning it).
                    exec /bin/sh "$0" || {
                        logger -t agent "exec failed; falling back to exit for procd restart"
                        exit 0
                    }
                else
                    logger -t agent "Hash mismatch on new agent. Aborting update."
                    rm -f "$0.tmp"
                fi
            fi
        fi
    fi
    # 1. INFORMACIÓN BÁSICA DEL SISTEMA
    BOARD=$(ubus call system board 2>/dev/null || echo "{}")
    SYS_INFO=$(ubus call system info 2>/dev/null || echo "{}")

    # 2. RECOLECCIÓN WIRELESS AVANZADA (Parser de 3 líneas para iwinfo)
    WIFI_DATA="{"
    FIRST_IFACE=1
    # Escaneamos interfaces incluyendo phy (común en drivers ath9k/WNDR3700)
    for IFACE in $(ls /sys/class/net | grep -E "wlan|ath|radio|ra|phy"); do
        [ $FIRST_IFACE -eq 0 ] && WIFI_DATA="$WIFI_DATA,"
        
        ASSOCLIST=$(iwinfo "$IFACE" assoclist 2>/dev/null | awk '
            BEGIN { printf "[" }
            /^[0-9A-F:]+/ { 
                if (count > 0) printf "},"
                printf "{\"mac\":\"%s\",\"signal\":%d,\"noise\":%d,\"inactive\":%d", $1, $2, $5, $9
                count++
            }
            /RX:/ { 
                mcs="null"; mhz="unknown"; pkts="0"
                if ($4 == "MCS") { mcs=$5; sub(/,/, "", mcs); mhz=$6; pkts=$7 }
                else { pkts=$4 }
                printf ",\"rx_rate\":\"%s\",\"rx_mcs\":%s,\"rx_mhz\":\"%s\",\"rx_pkts\":%d", $2, mcs, mhz, pkts
            }
            /TX:/ { 
                mcs="null"; mhz="unknown"; pkts="0"
                if ($4 == "MCS") { mcs=$5; sub(/,/, "", mcs); mhz=$6; pkts=$7 }
                else { pkts=$4 }
                printf ",\"tx_rate\":\"%s\",\"tx_mcs\":%s,\"tx_mhz\":\"%s\",\"tx_pkts\":%d", $2, mcs, mhz, pkts
            }
            /expected throughput:/ {
                printf ",\"expected_throughput\":\"%s\"", $3
            }
            END { 
                if (count > 0) printf "}"
                printf "]"
            }
        ')
        
        [ "$ASSOCLIST" = "[" ] && ASSOCLIST="[]"
        WIFI_DATA="$WIFI_DATA \"$IFACE\": $ASSOCLIST"
        FIRST_IFACE=0
    done
    WIFI_DATA="$WIFI_DATA }"

    # 2.5 BANDWIDTH SENTINEL (Top Talkers)
    STATE_FILE="/tmp/bandwidth_state"
    TOP_TALKERS=$(
        for IFACE in $(ls /sys/class/net | grep -E "wlan|ath|radio|ra|phy"); do
            iw dev "$IFACE" station dump 2>/dev/null | awk '
                /^Station/ { mac=$2 }
                /rx bytes:/  { rx=$3 }  # lowercase - this is what iw outputs
                /tx bytes:/  { tx=$3; if (mac != "") print mac, rx, tx }
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
                    printf "{\"mac\":\"%s\",\"rate_rx\":%d,\"rate_tx\":%d,\"total_rate\":%d}", m, details_rx[m], details_tx[m], rates[m]
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
            printf "\"%s\":{\"rx_bytes\":%s,\"tx_bytes\":%s}", $1, $2, $10
            count++
        }
        END { printf "}" }
    ' /proc/net/dev)

    # 3. DESCUBRIMIENTO L2 / ECHO_LOCATION (Tabla ARP, Bridge, LLDP, Port Status)
    ARP_TABLE=$(cat /proc/net/arp | awk '
        BEGIN { printf "[" }
        NR > 1 {
            if (NR > 2) printf ","
            printf "{\"ip\":\"%s\",\"mac\":\"%s\",\"device\":\"%s\"}", $1, $4, $6
        }
        END { printf "]" }
    ')

    BRIDGE_TABLE=$(brctl showmacs br-lan 2>/dev/null | awk '
        BEGIN { printf "[" }
        NR > 1 {
            if (NR > 2) printf ","
            printf "{\"port\":\"%s\",\"mac\":\"%s\",\"is_local\":\"%s\"}", $1, $2, $3
        }
        END { printf "]" }
    ')

    BR_STATUS=$(ubus call network.device status '{"name":"br-lan"}' 2>/dev/null)
    [ -z "$BR_STATUS" ] && BR_STATUS="{}"
    
    LLDP_INFO=$(lldpctl -f json 2>/dev/null)
    [ -z "$LLDP_INFO" ] && LLDP_INFO="{}"

    NEIGHBOR_STATS="{\"arp_table\": $ARP_TABLE, \"bridge_table\": $BRIDGE_TABLE, \"br_status\": $BR_STATUS, \"lldp_info\": $LLDP_INFO}"

    # 4. DHCP LEASES
    DHCP_LEASES=$(ubus call dhcp ipv4leases 2>/dev/null || echo "{\"leases\":[]}")

    # 5. LOGS RECIENTES (Últimas 20 líneas de syslog)
    # El colector tiene un límite de 2 s: logd nunca puede impedir el POST
    # de telemetría ni marcar el nodo como offline.
    SYS_LOGS=$(collect_recent_logs)

    # 5.5 FLOW_SENSE – Top 20 destinos activos desde /proc/net/nf_conntrack (ZERO CPU overhead)
    # Lee únicamente conexiones ESTABLISHED, extrae dst_ip y dst_port, agrupa y cuenta.
    # Sin exports IPFIX, sin fprobe, sin softflowd — puro awk sobre el pseudo-filesystem del kernel.
    FLOW_SENSE_DATA="[]"
    if [ -f /proc/net/nf_conntrack ]; then
        FLOW_SENSE_DATA=$(awk '
            /ESTABLISHED/ {
                proto = ""
                src = ""; dst = ""; dport = "0"
                # Determine protocol from field 1 (tcp/udp/icmp)
                proto = $3
                # Walk fields looking for dst= and dport=
                for (i = 1; i <= NF; i++) {
                    n = split($i, kv, "=")
                    if (n == 2) {
                        if (kv[1] == "dst" && dst == "") dst = kv[2]      # first dst = src-side reply dst
                        if (kv[1] == "src" && src == "") src = kv[2]      # first src = originator
                        if (kv[1] == "dport" && dport == "0") dport = kv[2]
                    }
                }
                # Skip loopback and link-local
                if (dst ~ /^127\./ || dst ~ /^169\.254/) next
                key = proto ":" dst ":" dport
                count[key]++
                # Store first-seen src for context
                if (!(key in srcs)) srcs[key] = src
            }
            END {
                # Sort by count descending (bubble sort, max 20 top entries)
                n = 0
                for (k in count) { keys[n] = k; n++ }
                for (i = 0; i < n; i++) {
                    for (j = i+1; j < n; j++) {
                        if (count[keys[j]] > count[keys[i]]) {
                            t = keys[i]; keys[i] = keys[j]; keys[j] = t
                        }
                    }
                }
                limit = (n < 20) ? n : 20
                printf "["
                for (i = 0; i < limit; i++) {
                    k = keys[i]
                    split(k, parts, ":")
                    if (i > 0) printf ","
                    printf "{\"proto\":\"%s\",\"dst\":\"%s\",\"dport\":%s,\"conns\":%d,\"sample_src\":\"%s\"}",
                        parts[1], parts[2], parts[3], count[k], srcs[k]
                }
                printf "]"
            }
        ' /proc/net/nf_conntrack 2>/dev/null)
        [ -z "$FLOW_SENSE_DATA" ] && FLOW_SENSE_DATA="[]"
    fi

    # 6. CONSTRUCCIÓN DEL PAYLOAD
    PAYLOAD=$(cat <<EOF
{
    "device_id": "$DEVICE_ID",
    "agent_version": "$AGENT_VERSION",
    "timestamp": $(date +%s),
    "board": $BOARD,
    "system": $SYS_INFO,
    "wireless_stations": $WIFI_DATA,
    "top_talkers": $TOP_TALKERS,
    "iface_stats": $IFACE_STATS,
    "neighbor_stats": $NEIGHBOR_STATS,
    "dhcp": $DHCP_LEASES,
    "flow_sense": $FLOW_SENSE_DATA,
    "logs": "$SYS_LOGS",
    "survey_id": "$SURVEY_ID",
    "neighbor_aps": $NEIGHBOR_APS
}
EOF
)

    # 6. ENVÍO DE TELEMETRÍA (Con X-Site-Key y comprobación de rollback)
    HTTP_CODE=$(curl -m 5 -s -X POST \
        -H "Content-Type: application/json" \
        -H "X-Site-Key: $SITE_KEY" \
        -d "$PAYLOAD" \
        "$TELEMETRY_URL" -w "%{http_code}" -o /dev/null)

    if [ "$HTTP_CODE" = "202" ]; then
        T_FAILS=0
    else
        T_FAILS=$((T_FAILS+1))
        logger -t agent "Telemetry failed ($HTTP_CODE). Fail count: $T_FAILS"

        if [ $T_FAILS -ge 3 ]; then
            logger -t agent "Telemetry failed 3 times. Initiating rollback."
            if [ -f "$0.old" ]; then
                mv "$0.old" "$0"
                # Re-exec the rolled-back script in-place so procd does not count
                # this as a crash. Belt-and-suspenders: the init.d/agent script
                # uses generous respawn thresholds too.
                exec /bin/sh "$0" || exit 1
            fi
        fi
    fi

    # 7. OBTENCIÓN DE CONFIGURACIÓN E INYECCIÓN DE LLAVE SSH
    # El controlador envía la llave pública en la respuesta de configuración
    CONFIG_RESPONSE=$(curl -m 5 -s -X GET \
        -H "X-Site-Key: $SITE_KEY" \
        "$CONFIG_URL")

    # 7.0 WIFI_SURVEY: detect survey mode from controller. When active:
    #   - telemetry interval drops to 2s (vs 10s normal)
    #   - payload includes "survey_id" so the backend tags per-station signal
    #     samples with the active survey and writes them to InfluxDB.
    #   - "neighbor_aps" snapshot is included so the dashboard can show
    #     "what other APs this device can hear from this location".
    SURVEY_MODE=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.survey_mode' 2>/dev/null)
    SURVEY_ID=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.survey_id' 2>/dev/null)
    [ "$SURVEY_MODE" != "true" ] && SURVEY_MODE="false"
    [ -z "$SURVEY_ID" ] && SURVEY_ID=""

    if [ "$SURVEY_MODE" = "true" ]; then
        # Build a compact neighbor_aps snapshot. iwinfo scan returns a
        # human-readable table; we only need BSSID,SSID,channel,signal per row.
        NEIGHBOR_APS="[]"
        FIRST_IF=1
        NEIGHBOR_APS="["
        for IFACE in $(ls /sys/class/net | grep -E "wlan|ath|radio|ra|phy"); do
            iwinfo "$IFACE" scan 2>/dev/null | awk -v iface="$IFACE" '
                /Address:/ { bssid = $2 }
                /ESSID:/   { essid = ""; for (i=2; i<=NF; i++) essid = essid (i==2?"":" ") $i; gsub(/"/, "", essid) }
                /Channel:/ { chan = $2 }
                /Signal:/  { sig = $2 " " $3
                              if (count > 0) printf ","
                              printf "{\"iface\":\"%s\",\"bssid\":\"%s\",\"ssid\":\"%s\",\"channel\":%s,\"signal\":\"%s\"}", iface, bssid, essid, chan, sig
                              count++
                              bssid=""; essid=""; chan=""; sig=""
                            }
                END { exit }
            '
        done | awk 'BEGIN{first=1} { if(NR>0){ if(!first)printf ","; printf "%s",$0; first=0} } END{print ""}'
        # Wrap properly: prefix and suffix with brackets
        if [ -n "$(echo "$NEIGHBOR_APS" | tr -d '[:space:]')" ]; then
            NEIGHBOR_APS="[${NEIGHBOR_APS}]"
        else
            NEIGHBOR_APS="[]"
        fi
        # Cap neighbor_aps to 64 entries to keep payload small (and prevent
        # a busy AP environment from ballooning telemetry).
        NEIGHBOR_APS=$(echo "$NEIGHBOR_APS" | tr ',' '\n' | head -n 64 | tr '\n' ',' | sed 's/,$//')
        NEIGHBOR_APS="[$NEIGHBOR_APS]"
    else
        NEIGHBOR_APS="[]"
    fi

    if [ -n "$CONFIG_RESPONSE" ]; then
        # Extraer llave pública usando jsonfilter (nativo en OpenWrt)
        PUBKEY=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.ssh_pubkey' 2>/dev/null)
        
        if [ -n "$PUBKEY" ]; then
            # Crear archivo si no existe y asegurar permisos
            touch /etc/dropbear/authorized_keys
            chmod 600 /etc/dropbear/authorized_keys
            
            # Inyectar solo si no está ya presente
            grep -q "$PUBKEY" /etc/dropbear/authorized_keys || {
                echo "$PUBKEY" >> /etc/dropbear/authorized_keys
                logger -t agent "SSH_KEY_INJECTED: Master controller key added."
            }
        fi

        # 7.5 CONFIGURACIÓN WIRELESS CENTRALIZADA
        NEW_WIFI_HASH=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireless' 2>/dev/null | sha256sum | awk '{print $1}')
        OLD_WIFI_HASH=$(cat /tmp/wifi_config.hash 2>/dev/null)
        
        if [ -n "$NEW_WIFI_HASH" ] && [ "$NEW_WIFI_HASH" != "$OLD_WIFI_HASH" ]; then
            logger -t agent "DEBUG: NEW=$NEW_WIFI_HASH OLD=$OLD_WIFI_HASH URL=$CONFIG_URL RES_LEN=${#CONFIG_RESPONSE}"
            logger -t agent "WLAN config changed. Re-provisioning radios..."
            
            WLAN_COUNT=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireless.wlans[@]' 2>/dev/null | wc -l 2>/dev/null || echo 0)
            
            if [ "$WLAN_COUNT" -gt 0 ]; then
                while uci -q delete wireless.@wifi-iface[0]; do :; done
                for CFG in $(uci show wireless | grep -o 'wireless\.cfg_radio[0-9]_[0-9]*' | cut -d. -f2 | sort -u); do uci delete wireless.$CFG; done
                
                for RADIO in $(uci -q show wireless | grep "=wifi-device" | cut -d'.' -f2 | cut -d'=' -f1); do
                    M_BAND=""
                    R_BAND=$(ubus call network.wireless status | jsonfilter -e "@.$RADIO.config.band" 2>/dev/null)
                    if [ -n "$R_BAND" ]; then
                        if [ "$R_BAND" = "2g" ] || [ "$R_BAND" = "2g-5g" ]; then M_BAND="2.4GHz"
                        elif [ "$R_BAND" = "5g" ]; then M_BAND="5GHz"
                        fi
                    fi
                    
                    if [ -z "$M_BAND" ]; then
                        R_HW=$(ubus call network.wireless status | jsonfilter -e "@.$RADIO.config.hwmode" 2>/dev/null)
                        if [ "$R_HW" = "11a" ] || [ "$R_HW" = "11ac" ] || [ "$R_HW" = "11ax" ]; then M_BAND="5GHz"
                        elif [ "$R_HW" = "11g" ] || [ "$R_HW" = "11b" ] || [ "$R_HW" = "11n" ]; then M_BAND="2.4GHz"
                        fi
                    fi
                    
                    if [ -z "$M_BAND" ]; then
                        R_CHAN=$(ubus call network.wireless status | jsonfilter -e "@.$RADIO.config.channel" 2>/dev/null)
                        if [ "$R_CHAN" != "auto" ] && [ "$R_CHAN" -gt 14 ]; then M_BAND="5GHz"
                        else M_BAND="2.4GHz"
                        fi
                    fi

                    i=0
                    while [ $i -lt "$WLAN_COUNT" ]; do
                        W_SSID=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].ssid" 2>/dev/null)
                        W_SEC=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].security" 2>/dev/null)
                        W_KEY=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].key" 2>/dev/null)
                        W_BAND=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].band" 2>/dev/null)
                        W_ROAMING=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].ieee80211r" 2>/dev/null)
                        W_80211K=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].ieee80211k" 2>/dev/null)
                        W_80211V=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].ieee80211v" 2>/dev/null)
                        W_MFP=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].ieee80211w" 2>/dev/null)

                        W_AUTH_SERVER=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].auth_server" 2>/dev/null)

                        W_AUTH_SECRET=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].auth_secret" 2>/dev/null)

                        W_DYN_VLAN=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].dynamic_vlan" 2>/dev/null)
                        
                        if [ "$W_BAND" = "both" ] || [ "$W_BAND" = "$M_BAND" ]; then
                            SECTION="cfg_${RADIO}_${i}"
                            uci set wireless.$SECTION=wifi-iface
                            uci set wireless.$SECTION.device="$RADIO"
                            uci set wireless.$SECTION.network='lan'
                            uci set wireless.$SECTION.mode='ap'
                            uci set wireless.$SECTION.ssid="$W_SSID"
                            uci set wireless.$SECTION.encryption="$W_SEC"
                            [ -n "$W_KEY" ] && uci set wireless.$SECTION.key="$W_KEY"
                            

                            [ -n "$W_MFP" ] && uci set wireless.$SECTION.ieee80211w="$W_MFP"

                            if [ -n "$W_AUTH_SERVER" ] && [ "$W_AUTH_SERVER" != "null" ]; then
								if [ "$W_AUTH_SERVER" = "AUTO" ]; then W_AUTH_SERVER="$CONTROLLER_IP"; fi

                                uci set wireless.$SECTION.auth_server="$W_AUTH_SERVER"

                                uci set wireless.$SECTION.auth_secret="$W_AUTH_SECRET"

                            fi

                            if [ "$W_DYN_VLAN" = "1" ] || [ "$W_DYN_VLAN" = "2" ]; then

                                uci set wireless.$SECTION.dynamic_vlan="$W_DYN_VLAN"

                                uci set wireless.$SECTION.vlan_naming="1"

                                uci set wireless.$SECTION.vlan_bridge="br-vlan"

                            fi
                            
                            if [ "$W_ROAMING" = "1" ] || [ "$W_ROAMING" = "true" ]; then
                                uci set wireless.$SECTION.ieee80211r='1'
                                uci set wireless.$SECTION.ft_over_ds='0'
                                uci set wireless.$SECTION.ft_psk_generate_local='1'
                                uci set wireless.$SECTION.mobility_domain='1234'
                            fi
                            if [ "$W_80211K" = "1" ] || [ "$W_80211K" = "true" ]; then
                                uci set wireless.$SECTION.ieee80211k='1'
                            fi
                            if [ "$W_80211V" = "1" ] || [ "$W_80211V" = "true" ]; then
                                uci set wireless.$SECTION.bss_transition='1'
                                uci set wireless.$SECTION.wnm_sleep_mode='1'
                                uci set wireless.$SECTION.time_advertisement='2'
                                uci set wireless.$SECTION.time_zone='<-05>5'
                            fi
                        fi
                        i=$((i+1))
                    done
                done
                uci commit wireless
                wifi reload
                echo "$NEW_WIFI_HASH" > /tmp/wifi_config.hash
                logger -t agent "WLAN config applied successfully."
            else
                logger -t agent "WLAN config empty. Skipping."
            fi
        fi

        # 8. CONFIGURACIÓN DE WIREGUARD (SECURE_TUNNEL)
        WG_ENABLED=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.enabled' 2>/dev/null)
        if [ "$WG_ENABLED" = "true" ]; then
            logger -t agent "WIREGUARD: Tunnel config received."
            
            # Check if wireguard is installed, install via apk if missing
            if ! command -v wg > /dev/null; then
                logger -t agent "WIREGUARD: wg tools missing. Installing via apk..."
                apk update && apk add wireguard-tools
            fi
            
            # Check if wg_nerve already configured in uci
            WG_EXISTS=$(uci -q get network.wg_nerve.proto)
            if [ "$WG_EXISTS" != "wireguard" ]; then
                logger -t agent "WIREGUARD: Configuring wg_nerve interface..."
                
                WG_PRIV=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.private_key' 2>/dev/null)
                WG_PUB=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.controller_pubkey' 2>/dev/null)
                WG_EP=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.endpoint_ip' 2>/dev/null)
                WG_IP=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.internal_ip' 2>/dev/null)
                WG_ALLOWED=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.allowed_ips' 2>/dev/null)
                
                uci set network.wg_nerve=interface
                uci set network.wg_nerve.proto='wireguard'
                uci set network.wg_nerve.private_key="$WG_PRIV"
                
                # Add an IP address for wireguard interface, we append /24 for subnet
                uci -q delete network.wg_nerve.addresses
                uci add_list network.wg_nerve.addresses="${WG_IP}/24"

                # Define the controller peer
                uci set network.wg_nerve_peer=wireguard_wg_nerve
                uci set network.wg_nerve_peer.public_key="$WG_PUB"
                uci set network.wg_nerve_peer.endpoint_host="${WG_EP%%:*}"
                uci set network.wg_nerve_peer.endpoint_port="${WG_EP##*:}"
                uci set network.wg_nerve_peer.route_allowed_ips='1'
                uci set network.wg_nerve_peer.persistent_keepalive='25'
                
                # Add allowed IPs
                uci -q delete network.wg_nerve_peer.allowed_ips
                uci add_list network.wg_nerve_peer.allowed_ips="$WG_ALLOWED"

                uci commit network
                
                logger -t agent "WIREGUARD: wg_nerve committed. Bringing interface up."
                ifup wg_nerve
            fi
        else
            WG_EXISTS=$(uci -q get network.wg_nerve.proto)
            if [ "$WG_EXISTS" = "wireguard" ]; then
                logger -t agent "WIREGUARD: Disabling and deleting wg_nerve interface..."
                ifdown wg_nerve 2>/dev/null || true
                uci -q delete network.wg_nerve
                uci -q delete network.wg_nerve_peer
                uci commit network
            fi
        fi
    fi

    # 8.5 TAILSCALE / HEADSCALE ZERO TRUST OVERLAY

    if [ -n "$CONFIG_RESPONSE" ]; then

        TS_ENABLED=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.tailscale.enabled' 2>/dev/null)

        TS_KEY=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.tailscale.auth_key' 2>/dev/null)

        if [ "$TS_ENABLED" = "true" ] && [ -n "$TS_KEY" ]; then

            TS_STATUS=$(/etc/init.d/tailscale status 2>/dev/null)

            if ! echo "$TS_STATUS" | grep -q "running"; then

                logger -t agent "TAILSCALE: Starting and authenticating Zero Trust Overlay..."

                /etc/init.d/tailscale enable

                /etc/init.d/tailscale start

                sleep 2

                tailscale up --authkey "$TS_KEY" --accept-routes --reset

            fi

        elif [ "$TS_ENABLED" = "false" ]; then

            TS_STATUS=$(/etc/init.d/tailscale status 2>/dev/null)

            if echo "$TS_STATUS" | grep -q "running"; then

                logger -t agent "TAILSCALE: Disabling overlay network..."

                tailscale logout 2>/dev/null

                /etc/init.d/tailscale stop

                /etc/init.d/tailscale disable

            fi

        fi

    fi

    # 9. [THREAT_SHIELD] — nftables IP reputation enforcement
    if [ -n "$CONFIG_RESPONSE" ]; then
        TS_ENABLED=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.threat_shield' 2>/dev/null)
    fi

    if [ "$TS_ENABLED" = "true" ]; then
        TS_LIST_FILE="/tmp/ts_raw.txt"
        TS_NFT_FILE="/tmp/ts.nft"
        TS_STAMP_FILE="/tmp/ts.stamp"

        # Refresh blocklist if older than 6 hours (21600s) or missing
        TS_REFRESH=0
        if [ ! -f "$TS_STAMP_FILE" ]; then
            TS_REFRESH=1
        else
            TS_STAMP=$(cat "$TS_STAMP_FILE" 2>/dev/null || echo 0)
            TS_NOW=$(date +%s)
            TS_AGE=$((TS_NOW - TS_STAMP))
            [ "$TS_AGE" -gt 21600 ] && TS_REFRESH=1
        fi

        if [ "$TS_REFRESH" = "1" ]; then
            logger -t threat_shield "Downloading reputation blocklist..."
            if curl -m 60 -s -H "X-Site-Key: $SITE_KEY" \
                    "$BASE_URL/threat-shield/list" \
                    -o "$TS_LIST_FILE.tmp" 2>/dev/null; then
                TS_COUNT=$(wc -l < "$TS_LIST_FILE.tmp" 2>/dev/null || echo 0)
                if [ "$TS_COUNT" -gt 10 ]; then
                    mv "$TS_LIST_FILE.tmp" "$TS_LIST_FILE"
                    date +%s > "$TS_STAMP_FILE"
                    logger -t threat_shield "Blocklist updated: $TS_COUNT entries"

                    # Ensure table, set, and chains exist (idempotent)
                    nft list table inet threat_shield > /dev/null 2>&1 || \
                        nft add table inet threat_shield
                    nft list set inet threat_shield denylist > /dev/null 2>&1 || \
                        nft add set inet threat_shield denylist \
                            '{ type ipv4_addr; flags interval; auto-merge; }'
                    nft list chain inet threat_shield forward > /dev/null 2>&1 || {
                        nft add chain inet threat_shield forward \
                            '{ type filter hook forward priority -1; }'
                        nft add rule inet threat_shield forward \
                            ip daddr @denylist counter drop
                    }
                    nft list chain inet threat_shield input > /dev/null 2>&1 || {
                        nft add chain inet threat_shield input \
                            '{ type filter hook input priority -1; }'
                        nft add rule inet threat_shield input \
                            ip saddr @denylist counter drop
                    }

                    # Build atomic nft reload script from the list
                    awk '
                        BEGIN { print "flush set inet threat_shield denylist" }
                        NF && !/^#/ {
                            gsub(/[;, \t].*/, "")
                            if ($1 ~ /^[0-9]/) printf "add element inet threat_shield denylist { %s }\n", $1
                        }
                    ' "$TS_LIST_FILE" > "$TS_NFT_FILE"

                    # Apply atomically
                    nft -f "$TS_NFT_FILE" 2>/dev/null && \
                        logger -t threat_shield "Denylist applied to nftables"
                else
                    rm -f "$TS_LIST_FILE.tmp"
                    logger -t threat_shield "Blocklist download empty or too small, skipping"
                fi
            else
                logger -t threat_shield "Blocklist download failed"
            fi
        fi

        # Collect drop counters from nftables
        TS_DROPS_FWD=$(nft list chain inet threat_shield forward 2>/dev/null | \
            awk '/counter/{for(i=1;i<=NF;i++) if($i=="packets") print $(i+1)}' | head -1)
        TS_DROPS_IN=$(nft list chain inet threat_shield input 2>/dev/null | \
            awk '/counter/{for(i=1;i<=NF;i++) if($i=="packets") print $(i+1)}' | head -1)
        TS_DROPS_TOTAL=$(( ${TS_DROPS_FWD:-0} + ${TS_DROPS_IN:-0} ))
        TS_LOADED=$(wc -l < "$TS_LIST_FILE" 2>/dev/null || echo 0)

    else
        # Disable: remove threat_shield table if it exists
        nft list table inet threat_shield > /dev/null 2>&1 && \
            nft delete table inet threat_shield 2>/dev/null || true
        TS_DROPS_TOTAL=0
        TS_LOADED=0
        rm -f /tmp/ts.stamp 2>/dev/null
    fi

    # WIFI_SURVEY: tighten the loop to 2s when an active survey exists.
    # The controller toggles survey_mode via /api/devices/{id}/config; the
    # next loop iteration picks up the new interval.
    if [ "$SURVEY_MODE" = "true" ]; then
        sleep 2
    else
        sleep 10
    fi
done
