package handlers

import (
	"strconv"
	"strings"
	"testing"
)

func TestParseWirelessBandsAndRoaming(t *testing.T) {
	rawWireless := `wireless.radio0=wifi-device
wireless.radio0.type='mac80211'
wireless.radio0.band='5g'
wireless.radio0.channel='36'
wireless.radio1=wifi-device
wireless.radio1.type='mac80211'
wireless.radio1.band='2g'
wireless.radio1.channel='11'
wireless.default_radio0=wifi-iface
wireless.default_radio0.device='radio0'
wireless.default_radio0.network='lan'
wireless.default_radio0.mode='ap'
wireless.default_radio0.ssid='tuxcave2'
wireless.default_radio0.encryption='sae-mixed'
wireless.default_radio0.key='Zxcvb123!@'
wireless.default_radio0.ieee80211r='1'
wireless.default_radio0.ieee80211k='1'
wireless.default_radio0.ieee80211v='1'
wireless.default_radio1=wifi-iface
wireless.default_radio1.device='radio1'
wireless.default_radio1.network='lan'
wireless.default_radio1.mode='ap'
wireless.default_radio1.ssid='tuxcave2'
wireless.default_radio1.encryption='psk2'
wireless.default_radio1.key='Zxcvb123!@'
wireless.default_radio1.ieee80211r='1'
wireless.legacy_radio1=wifi-iface
wireless.legacy_radio1.device='radio1'
wireless.legacy_radio1.network='lan'
wireless.legacy_radio1.mode='ap'
wireless.legacy_radio1.ssid='Pallatanga'
wireless.legacy_radio1.encryption='psk2'
wireless.legacy_radio1.key='Pallatanga.2877'
`

	wirelessSecs := parseUciShow(rawWireless, "wireless")
	if len(wirelessSecs) == 0 {
		t.Fatalf("expected non-empty parsed wireless sections")
	}

	radioBands := make(map[string]string)
	for _, sec := range wirelessSecs {
		if sec.Type == "wifi-device" {
			band := "2.4GHz"
			if bVal, ok := sec.Options["band"].(string); ok {
				if strings.Contains(bVal, "5g") {
					band = "5GHz"
				} else if strings.Contains(bVal, "2g") {
					band = "2.4GHz"
				}
			} else if chVal, ok := sec.Options["channel"].(string); ok {
				if ch, err := strconv.Atoi(chVal); err == nil && ch >= 36 {
					band = "5GHz"
				}
			}
			radioBands[sec.ID] = band
		}
	}

	if radioBands["radio0"] != "5GHz" {
		t.Errorf("expected radio0 to be 5GHz, got %s", radioBands["radio0"])
	}
	if radioBands["radio1"] != "2.4GHz" {
		t.Errorf("expected radio1 to be 2.4GHz, got %s", radioBands["radio1"])
	}

	type wlanAggregator struct {
		ssid           string
		encryption     string
		key            string
		bands          map[string]bool
		roamingEnabled bool
		ieee80211k     bool
		ieee80211v     bool
	}
	wlanMap := make(map[string]*wlanAggregator)

	for _, sec := range wirelessSecs {
		if sec.Type == "wifi-iface" {
			ssidVal, hasSSID := sec.Options["ssid"]
			if !hasSSID {
				continue
			}
			s, ok := ssidVal.(string)
			if !ok || s == "" {
				continue
			}

			key := ""
			if kVal, ok := sec.Options["key"].(string); ok {
				key = kVal
			}
			enc := "psk2"
			if eVal, ok := sec.Options["encryption"].(string); ok && eVal != "" {
				enc = eVal
			}

			rVal, _ := sec.Options["ieee80211r"].(string)
			roam := (rVal == "1" || rVal == "true")
			kVal, _ := sec.Options["ieee80211k"].(string)
			k := (kVal == "1" || kVal == "true")
			vVal, _ := sec.Options["ieee80211v"].(string)
			v := (vVal == "1" || vVal == "true")

			devName, _ := sec.Options["device"].(string)
			band := radioBands[devName]
			if band == "" {
				band = "2.4GHz"
			}

			agg, exists := wlanMap[s]
			if !exists {
				agg = &wlanAggregator{
					ssid:           s,
					encryption:     enc,
					key:            key,
					bands:          make(map[string]bool),
					roamingEnabled: roam,
					ieee80211k:     k,
					ieee80211v:     v,
				}
				wlanMap[s] = agg
			} else {
				if key != "" && agg.key == "" {
					agg.key = key
				}
				if enc != "" && (agg.encryption == "psk2" || agg.encryption == "") {
					agg.encryption = enc
				}
				if roam {
					agg.roamingEnabled = true
				}
				if k {
					agg.ieee80211k = true
				}
				if v {
					agg.ieee80211v = true
				}
			}
			agg.bands[band] = true
		}
	}

	tuxcave2 := wlanMap["tuxcave2"]
	if tuxcave2 == nil {
		t.Fatalf("expected tuxcave2 in wlanMap")
	}
	if !tuxcave2.bands["2.4GHz"] || !tuxcave2.bands["5GHz"] {
		t.Errorf("expected tuxcave2 to have both 2.4GHz and 5GHz bands")
	}
	if !tuxcave2.roamingEnabled || !tuxcave2.ieee80211k || !tuxcave2.ieee80211v {
		t.Errorf("expected tuxcave2 to have roaming flags enabled")
	}

	pallatanga := wlanMap["Pallatanga"]
	if pallatanga == nil {
		t.Fatalf("expected Pallatanga in wlanMap")
	}
	if !pallatanga.bands["2.4GHz"] || pallatanga.bands["5GHz"] {
		t.Errorf("expected Pallatanga to have only 2.4GHz band")
	}
}

func TestParseUsteerSettings(t *testing.T) {
	rawUsteer := `usteer.@usteer[0]=usteer
usteer.@usteer[0].network='lan'
usteer.@usteer[0].min_connect_snr='-76'
usteer.@usteer[0].min_snr='-76'
usteer.@usteer[0].signal_diff_threshold='8'
usteer.@usteer[0].roam_trigger_snr='-73'
usteer.@usteer[0].roam_scan_snr='-70'
`
	usteerSecs := parseUciShow(rawUsteer, "usteer")
	if len(usteerSecs) == 0 {
		t.Fatalf("expected parsed usteer sections")
	}

	sec := usteerSecs[0]
	if sec.Options["min_connect_snr"] != "-76" {
		t.Errorf("expected min_connect_snr=-76, got %v", sec.Options["min_connect_snr"])
	}
	if sec.Options["signal_diff_threshold"] != "8" {
		t.Errorf("expected signal_diff_threshold=8, got %v", sec.Options["signal_diff_threshold"])
	}
}
