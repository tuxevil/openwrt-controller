package database

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/hex"
	"strings"
	"sync"
)

//go:embed data/oui.tsv.gz
var embeddedOUIGzip []byte

var (
	ouiOnce  sync.Once
	ouiTable map[string]string
)

// MACVendorInfo describes the IEEE OUI manufacturer and IEEE 802 bit flags for
// a MAC address or 3-octet OUI prefix.
type MACVendorInfo struct {
	MAC                 string `json:"mac"`
	OUI                 string `json:"oui"`
	Vendor              string `json:"vendor"`
	LocallyAdministered bool   `json:"locally_administered"`
	Multicast           bool   `json:"multicast"`
	Source              string `json:"source"`
	Note                string `json:"note,omitempty"`
}

func loadOUITable() map[string]string {
	ouiOnce.Do(func() {
		table := make(map[string]string, 42000)
		gz, err := gzip.NewReader(bytes.NewReader(embeddedOUIGzip))
		if err != nil {
			ouiTable = table
			return
		}
		defer gz.Close()

		scanner := bufio.NewScanner(gz)
		for scanner.Scan() {
			line := scanner.Text()
			tab := strings.IndexByte(line, '\t')
			if tab != 6 || tab+1 >= len(line) {
				continue
			}
			table[line[:6]] = line[tab+1:]
		}
		ouiTable = table
	})
	return ouiTable
}

// LookupMACVendor resolves the IEEE OUI manufacturer for a full MAC address or
// 3-octet OUI prefix (e.g. "90:39:5f:2f:c9:7f", "3E:FB:EF", "90395F") fully
// offline without external network calls.
func LookupMACVendor(rawMAC string) MACVendorInfo {
	cleaned := strings.ToUpper(strings.TrimSpace(rawMAC))
	hexOnly := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(cleaned)

	info := MACVendorInfo{
		MAC:    cleaned,
		Source: "invalid",
	}

	if len(hexOnly) < 6 || (len(hexOnly) != 6 && len(hexOnly) != 12) {
		info.Note = "Invalid MAC address or 3-octet OUI prefix."
		return info
	}

	prefixBytes, err := hex.DecodeString(hexOnly[:6])
	if err != nil || len(prefixBytes) != 3 {
		info.Note = "Invalid hexadecimal characters in MAC/OUI."
		return info
	}

	if len(hexOnly) == 12 {
		if _, err := hex.DecodeString(hexOnly); err != nil {
			info.Note = "Invalid hexadecimal characters in MAC address."
			return info
		}
		info.MAC = hexOnly[0:2] + ":" + hexOnly[2:4] + ":" + hexOnly[4:6] + ":" + hexOnly[6:8] + ":" + hexOnly[8:10] + ":" + hexOnly[10:12]
	} else {
		info.MAC = hexOnly[0:2] + ":" + hexOnly[2:4] + ":" + hexOnly[4:6]
	}

	info.OUI = hexOnly[0:2] + ":" + hexOnly[2:4] + ":" + hexOnly[4:6]
	info.Multicast = (prefixBytes[0] & 0x01) != 0
	info.LocallyAdministered = (prefixBytes[0] & 0x02) != 0

	if info.LocallyAdministered {
		info.Source = "locally_administered"
		info.Note = "Locally administered address (set by software, VM/container, or randomized private Wi-Fi MAC; IEEE has no registered manufacturer)."
		return info
	}

	if vendor, ok := loadOUITable()[hexOnly[:6]]; ok && vendor != "" {
		info.Vendor = vendor
		info.Source = "ieee_oui_offline"
		return info
	}

	info.Source = "unknown"
	info.Note = "OUI prefix not found in local IEEE OUI registry."
	return info
}
