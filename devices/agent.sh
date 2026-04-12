#!/bin/sh

CONTROLLER_IP="10.0.0.6"
DEVICE_ID=$(cat /sys/class/net/br-lan/address)
TELEMETRY_URL="http://$CONTROLLER_IP:3000/api/telemetry"

while true; do
    BOARD=$(ubus call system board)
    SYS=$(ubus call system info)

    # RECOLECCIÓN WIRELESS (Estructura de Mapa por Interfaz)
    WIFI_DATA="{"
    FIRST_IFACE=1
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

    ARP_TABLE=$(cat /proc/net/arp | awk 'BEGIN{printf "["} NR>1{if(NR>2)printf ","; printf "{\"ip\":\"%s\",\"mac\":\"%s\"}", $1, $4} END{printf "]"}')
    DHCP=$(ubus call dhcp ipv4leases 2>/dev/null || echo "{\"leases\":[]}")

    PAYLOAD=$(cat <<EOF
{
    "device_id": "$DEVICE_ID",
    "board": $BOARD,
    "system": $SYS,
    "wireless_stations": $WIFI_DATA,
    "arp_table": $ARP_TABLE,
    "dhcp": $DHCP
}
EOF
)

    curl -m 5 -s -X POST -H "Content-Type: application/json" -d "$PAYLOAD" "$TELEMETRY_URL" > /dev/null
    sleep 10
done