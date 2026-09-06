package services

import "strings"

// WLANPolicyResolution describes the policy the device agent should apply.
// The WLAN row is authoritative; site_configs global_* fields are legacy
// compatibility fields and are reported only when they disagree.
type WLANPolicyResolution struct {
	Security  string
	Password  string
	Conflicts []string
}

// WLANAppliesToDevice applies the existing global/custom targeting contract
// without allowing a custom WLAN to leak to every device.
func WLANAppliesToDevice(globalEnabled bool, targetMode string, assigned bool) bool {
	if strings.EqualFold(strings.TrimSpace(targetMode), "custom") {
		return assigned
	}
	return globalEnabled
}

// ResolveCanonicalWLAN keeps the normalized WLAN row as the single source of
// applied security credentials while making legacy site-level disagreements
// visible to callers.
func ResolveCanonicalWLAN(siteSSID, siteSecurity, sitePassword, wlanSSID, wlanSecurity, wlanPassword string) WLANPolicyResolution {
	resolution := WLANPolicyResolution{
		Security: wlanSecurity,
		Password: wlanPassword,
	}
	if siteSSID == "" || siteSSID != wlanSSID {
		return resolution
	}
	if siteSecurity != "" && siteSecurity != wlanSecurity {
		resolution.Conflicts = append(resolution.Conflicts, "global_encryption")
	}
	if sitePassword != "" && sitePassword != wlanPassword {
		resolution.Conflicts = append(resolution.Conflicts, "global_wpa_key")
	}
	return resolution
}
