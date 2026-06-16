package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"

	"openwrt-controller/internal/orchestrator"
)

// interfaceNameRegex constrains tcpdump's -i argument to a sane Linux
// network-interface name. The previous code interpolated user input
// directly into the SSH command, which permitted trivial shell injection.
var interfaceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,15}$`)

func CapturePacketHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)
	deviceID := r.PathValue("device_id")

	var req struct {
		Interface   string `json:"interface"`
		PacketCount int    `json:"packet_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Interface == "" {
		req.Interface = "br-lan"
	}
	if !interfaceNameRegex.MatchString(req.Interface) {
		http.Error(w, "invalid interface name", http.StatusBadRequest)
		return
	}
	if req.PacketCount <= 0 || req.PacketCount > 20000 {
		req.PacketCount = 5000 // default safe limit
	}

	// 1. Run tcpdump remotely. Save to /tmp to avoid flash wear.
	// We capture up to PacketCount packets. Wait for it to finish.
	// Then base64 encode the pcap file, cat it, and rm it.
	cmd := fmt.Sprintf("tcpdump -i %s -c %d -w /tmp/capture.pcap 2>/dev/null && base64 /tmp/capture.pcap && rm -f /tmp/capture.pcap", req.Interface, req.PacketCount)

	out, err := orchestrator.ExecuteCommandWithOutput(schema, deviceID, cmd)
	if err != nil {
		http.Error(w, fmt.Sprintf("Capture failed: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// The output might contain some bash warnings or newlines. Base64 ignores newlines usually,
	// but we should trim.
	b64str := strings.TrimSpace(out)

	pcapData, err := base64.StdEncoding.DecodeString(b64str)
	if err != nil {
		http.Error(w, "Failed to decode pcap file from device", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.tcpdump.pcap")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"capture_%s_%s.pcap\"", deviceID, req.Interface))
	w.Write(pcapData)
}

func RunIperfHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)
	deviceID := r.PathValue("device_id")

	var req struct {
		TargetIP string `json:"target_ip"`
		TimeSecs int    `json:"time_secs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.TargetIP == "" {
		http.Error(w, "target_ip is required", http.StatusBadRequest)
		return
	}
	// iperf3 is invoked with -c <ip>; we refuse anything that isn't a
	// literal IPv4/IPv6 address to prevent shell injection via the body.
	if ip := net.ParseIP(req.TargetIP); ip == nil {
		http.Error(w, "target_ip must be a valid IP address", http.StatusBadRequest)
		return
	}
	if req.TimeSecs <= 0 || req.TimeSecs > 60 {
		req.TimeSecs = 10
	}

	cmd := fmt.Sprintf("iperf3 -c %s -t %d --json", req.TargetIP, req.TimeSecs)
	out, err := orchestrator.ExecuteCommandWithOutput(schema, deviceID, cmd)
	if err != nil {
		http.Error(w, fmt.Sprintf("Iperf failed: %s\n%s", err.Error(), out), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(out))
}
