package services

import (
	"fmt"
)

// AddWLANConfig injects the 802.11r parameters if roaming is enabled
// and applies the configuration using RunMassCommand
func AddWLANConfig(siteID, ssid, security, password string, roaming, k, v bool) []DeviceResult {
	cmd := fmt.Sprintf("uci add wireless wifi-iface && uci set wireless.@wifi-iface[-1].device='radio0' && uci set wireless.@wifi-iface[-1].network='lan' && uci set wireless.@wifi-iface[-1].mode='ap' && uci set wireless.@wifi-iface[-1].ssid='%s' && uci set wireless.@wifi-iface[-1].encryption='%s' && uci set wireless.@wifi-iface[-1].key='%s' ", ssid, security, password)

	if roaming {
		cmd += "&& uci set wireless.@wifi-iface[-1].ieee80211r='1' "
		cmd += "&& uci set wireless.@wifi-iface[-1].mobility_domain='4F57' "
		cmd += "&& uci set wireless.@wifi-iface[-1].ft_over_ds='0' "
		cmd += "&& uci set wireless.@wifi-iface[-1].ft_psk_generate_local='1' "
	}
	
	if k {
		cmd += "&& uci set wireless.@wifi-iface[-1].ieee80211k='1' "
	}
	
	if v {
		// 802.11v in OpenWrt is BSS Transition + WNM Sleep Mode
		cmd += "&& uci set wireless.@wifi-iface[-1].bss_transition='1' "
		cmd += "&& uci set wireless.@wifi-iface[-1].wnm_sleep_mode='1' "
		cmd += "&& uci set wireless.@wifi-iface[-1].time_advertisement='2' "
		cmd += "&& uci set wireless.@wifi-iface[-1].time_zone='<-05>5' "
	}

	cmd += "&& uci commit wireless && /sbin/wifi reload"

	return RunMassCommand(siteID, cmd)
}
