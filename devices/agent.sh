#!/bin/sh

# === CONFIGURACIÓN ===
# LLAVE DE SITIO (Obtenida del Dashboard -> Settings)
SITE_KEY="TU_API_KEY_AQUI"
CONTROLLER_IP="10.0.0.6"
PORT="3000"
BASE_URL="http://$CONTROLLER_IP:$PORT/api"
TELEMETRY_URL="$BASE_URL/telemetry"
CONFIG_URL="$BASE_URL/devices/$DEVICE_ID/config"
# Obtener MAC de la interfaz puente como ID único
DEVICE_ID=$(cat /sys/class/net/br-lan/address | tr '[:lower:]' '[:upper:]')

# Instalar dependencias si faltan (opcional)
# opkg update && opkg install iwinfo curl

T_FAILS=0

while true; do
    # 0. CHECK AUTO-UPDATE
    AGENT_VERSION=$(sha256sum "$0" | awk '{print $1}')
    LATEST_JSON=$(curl -m 5 -s -X GET -H "X-Site-Key: $SITE_KEY" "$BASE_URL/agent/latest")
    
    if [ -n "$LATEST_JSON" ]; then
        LATEST_HASH=$(echo "$LATEST_JSON" | jsonfilter -e '@.version_hash' 2>/dev/null)
        if [ -n "$LATEST_HASH" ] && [ "$LATEST_HASH" != "$AGENT_VERSION" ]; then
            logger -t agent "New agent version found: $LATEST_HASH. Downloading..."
            if curl -m 10 -s -X GET -H "X-Site-Key: $SITE_KEY" "$BASE_URL/agent/latest/raw" -o "$0.tmp"; then
                TMP_HASH=$(sha256sum "$0.tmp" | awk '{print $1}')
                if [ "$TMP_HASH" = "$LATEST_HASH" ]; then
                    logger -t agent "Agent downloaded securely. Updating and restarting."
                    cp "$0" "$0.old"
                    mv "$0.tmp" "$0"
                    chmod +x "$0"
                    exit 0
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

    # 3. DESCUBRIMIENTO L2 (Tabla ARP y Bridge)
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

    # 4. DHCP LEASES
    DHCP_LEASES=$(ubus call dhcp ipv4leases 2>/dev/null || echo "{\"leases\":[]}")

    # 5. LOGS RECIENTES (Últimas 50 líneas de syslog)
    SYS_LOGS=$(logread | tail -n 50 | sed 's/\\/\\\\/g; s/"/\\"/g' | awk '{printf "%s\\n", $0}')

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
    "arp_table": $ARP_TABLE,
    "bridge_table": $BRIDGE_TABLE,
    "dhcp": $DHCP_LEASES,
    "flow_sense": $FLOW_SENSE_DATA,
    "logs": "$SYS_LOGS"
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
                # Exiting triggers procd automatic restart
                exit 1
            fi
        fi
    fi

    # 7. OBTENCIÓN DE CONFIGURACIÓN E INYECCIÓN DE LLAVE SSH
    # El controlador envía la llave pública en la respuesta de configuración
    CONFIG_RESPONSE=$(curl -m 5 -s -X GET \
        -H "X-Site-Key: $SITE_KEY" \
        "$CONFIG_URL")

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

        # 8. CONFIGURACIÓN DE WIREGUARD (SECURE_TUNNEL)
        WG_ENABLED=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.enabled' 2>/dev/null)
        if [ "$WG_ENABLED" = "true" ]; then
            logger -t agent "WIREGUARD: Tunnel config received."
            
            # Check if wireguard is installed, install via apk if missing
            if ! command -v wg > /dev/null; then
                logger -t agent "WIREGUARD: wg tools missing. Installing via apk..."
                apk update && apk add wireguard-tools
            fi
            
            # Check if wg0 already configured in uci
            WG_EXISTS=$(uci -q get network.wg0.proto)
            if [ "$WG_EXISTS" != "wireguard" ]; then
                logger -t agent "WIREGUARD: Configuring wg0 interface..."
                
                WG_PRIV=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.private_key' 2>/dev/null)
                WG_PUB=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.controller_pubkey' 2>/dev/null)
                WG_EP=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.endpoint_ip' 2>/dev/null)
                WG_IP=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.internal_ip' 2>/dev/null)
                WG_ALLOWED=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.allowed_ips' 2>/dev/null)
                
                uci set network.wg0=interface
                uci set network.wg0.proto='wireguard'
                uci set network.wg0.private_key="$WG_PRIV"
                
                # Add an IP address for wireguard interface, we append /24 for subnet
                uci -q delete network.wg0.addresses
                uci add_list network.wg0.addresses="${WG_IP}/24"

                # Define the controller peer
                uci set network.wg0_control=wireguard_wg0
                uci set network.wg0_control.public_key="$WG_PUB"
                uci set network.wg0_control.endpoint_host="${WG_EP%%:*}"
                uci set network.wg0_control.endpoint_port="${WG_EP##*:}"
                uci set network.wg0_control.route_allowed_ips='1'
                uci set network.wg0_control.persistent_keepalive='25'
                
                # Add allowed IPs
                uci -q delete network.wg0_control.allowed_ips
                uci add_list network.wg0_control.allowed_ips="$WG_ALLOWED"

                uci commit network
                
                logger -t agent "WIREGUARD: wg0 committed. Bringing interface up."
                ifup wg0
            fi
        fi
    fi

    sleep 10
done