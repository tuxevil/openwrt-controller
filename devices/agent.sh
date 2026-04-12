#!/bin/sh

# === CONFIGURACIÓN ===
CONTROLLER_IP="10.0.0.6"
PORT="3000"
# Obtener MAC de la interfaz puente como ID único
DEVICE_ID=$(cat /sys/class/net/br-lan/address | tr '[:lower:]' '[:upper:]')
# LLAVE DE SITIO (Obtenida del Dashboard -> Settings)
SITE_KEY="TU_API_KEY_AQUI"

BASE_URL="http://$CONTROLLER_IP:$PORT/api"
TELEMETRY_URL="$BASE_URL/telemetry"
CONFIG_URL="$BASE_URL/devices/$DEVICE_ID/config"

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
                if (count > 0) printf ","
                printf "{\"mac\":\"%s\",\"signal\":%d,\"noise\":%d", $1, $2, $5
                count++
            }
            /RX:/ { printf ",\"rx_rate\":\"%s\"", $2 }
            /TX:/ { printf ",\"tx_rate\":\"%s\"}", $2 }
            END { printf "]" }
        ')
        
        [ "$ASSOCLIST" = "[" ] && ASSOCLIST="[]"
        WIFI_DATA="$WIFI_DATA \"$IFACE\": $ASSOCLIST"
        FIRST_IFACE=0
    done
    WIFI_DATA="$WIFI_DATA }"

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

    # 5. LOGS RECIENTES (Últimas 20 líneas de syslog)
    SYS_LOGS=$(logread | tail -n 50 | sed 's/\\/\\\\/g; s/"/\\"/g' | awk '{printf "%s\\n", $0}')

    # 6. CONSTRUCCIÓN DEL PAYLOAD
    PAYLOAD=$(cat <<EOF
{
    "device_id": "$DEVICE_ID",
    "agent_version": "$AGENT_VERSION",
    "timestamp": $(date +%s),
    "board": $BOARD,
    "system": $SYS_INFO,
    "wireless_stations": $WIFI_DATA,
    "arp_table": $ARP_TABLE,
    "bridge_table": $BRIDGE_TABLE,
    "dhcp": $DHCP_LEASES,
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
        PUBKEY=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.ssh_pubkey')
        
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
    fi

    sleep 10
done