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

while true; do
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

    # 5. CONSTRUCCIÓN DEL PAYLOAD
    PAYLOAD=$(cat <<EOF
{
    "device_id": "$DEVICE_ID",
    "timestamp": $(date +%s),
    "board": $BOARD,
    "system": $SYS_INFO,
    "wireless_stations": $WIFI_DATA,
    "arp_table": $ARP_TABLE,
    "bridge_table": $BRIDGE_TABLE,
    "dhcp": $DHCP_LEASES
}
EOF
)

    # 6. ENVÍO DE TELEMETRÍA (Con X-Site-Key)
    curl -m 5 -s -X POST \
        -H "Content-Type: application/json" \
        -H "X-Site-Key: $SITE_KEY" \
        -d "$PAYLOAD" \
        "$TELEMETRY_URL" > /dev/null

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