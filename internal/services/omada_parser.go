package services

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// ParseOmadaExport detects if the providing JSON is DHCP reservations or Port Forwarding rules,
// and maps them into Edge Nexus models.
func ParseOmadaExport(data []byte) ([]StaticLease, []PortForwardRule, error) {
	// First, try to unmarshal to a generic structure to find result.data
	type genericOmadaResponse struct {
		Result struct {
			Data []json.RawMessage `json:"data"`
		} `json:"result"`
	}

	var parsed genericOmadaResponse
	err := json.Unmarshal(data, &parsed)
	if err != nil {
		// Possibly it's a direct array? Let's check
		var arr []json.RawMessage
		if errArray := json.Unmarshal(data, &arr); errArray == nil {
			parsed.Result.Data = arr
		} else {
			return nil, nil, fmt.Errorf("could not parse Omada JSON format")
		}
	}

	if len(parsed.Result.Data) == 0 {
		return nil, nil, fmt.Errorf("no data found in Omada JSON")
	}

	var dhcpList []StaticLease
	var fwList []PortForwardRule

	// We examine the blocks to determine what they are. Since a given export might theoretically 
	// contain a mix (if combined), or just one type, we check each item.
	for _, itemJSON := range parsed.Result.Data {
		var detect map[string]interface{}
		if err := json.Unmarshal(itemJSON, &detect); err != nil {
			continue
		}

		if _, hasMac := detect["mac"]; hasMac {
			// It's likely a DHCP reservation
			var d struct {
				Name string `json:"name"`
				MAC  string `json:"mac"`
				IP   string `json:"ip"`
			}
			if err := json.Unmarshal(itemJSON, &d); err == nil {
				dhcpList = append(dhcpList, StaticLease{
					Name: d.Name,
					MAC:  d.MAC,
					IP:   d.IP,
				})
			}
		} else if _, hasPort := detect["externalPort"]; hasPort {
			// It's likely a Port Forwarding rule
			var f struct {
				Name         string `json:"name"`
				Protocol     int    `json:"protocol"`
				ExternalPort string `json:"externalPort"`
				ForwardIp    string `json:"forwardIp"`
				ForwardPort  string `json:"forwardPort"`
				Status       bool   `json:"status"`
			}
			if err := json.Unmarshal(itemJSON, &f); err == nil {
				// Protocol mapping: Omada 0=ALL(tcp udp), 1=TCP, 2=UDP
				protoStr := "tcp udp"
				if f.Protocol == 1 {
					protoStr = "tcp"
				} else if f.Protocol == 2 {
					protoStr = "udp"
				}
				
				srcPort, _ := strconv.Atoi(f.ExternalPort)
				destPort, _ := strconv.Atoi(f.ForwardPort)

				fwList = append(fwList, PortForwardRule{
					Name:     f.Name,
					Proto:    protoStr,
					SrcPort:  srcPort,
					DestIP:   f.ForwardIp,
					DestPort: destPort,
					Enabled:  true, // Defaulting to true as per migration, but could map f.Status
				})
			}
		}
	}

	return dhcpList, fwList, nil
}
