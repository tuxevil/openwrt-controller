package orchestrator

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

func generateHubUCI(mesh models.VPNMesh, nodes []models.VPNMeshNode, me models.VPNMeshNode) string {
	var sb strings.Builder

	// Network interface
	sb.WriteString(fmt.Sprintf("uci set network.wg_mesh='interface'\n"))
	sb.WriteString(fmt.Sprintf("uci set network.wg_mesh.proto='wireguard'\n"))
	sb.WriteString(fmt.Sprintf("uci set network.wg_mesh.private_key='%s'\n", me.PrivateKey))
	sb.WriteString(fmt.Sprintf("uci set network.wg_mesh.listen_port='%d'\n", me.ListenPort))
	sb.WriteString(fmt.Sprintf("uci add_list network.wg_mesh.addresses='%s/24'\n", me.InternalIP))

	// Firewall zone
	sb.WriteString("uci set firewall.wg_mesh='zone'\n")
	sb.WriteString("uci set firewall.wg_mesh.name='wg_mesh'\n")
	sb.WriteString("uci set firewall.wg_mesh.input='ACCEPT'\n")
	sb.WriteString("uci set firewall.wg_mesh.forward='ACCEPT'\n")
	sb.WriteString("uci set firewall.wg_mesh.output='ACCEPT'\n")
	sb.WriteString("uci set firewall.wg_mesh.network='wg_mesh'\n")

	// Allow listen port
	sb.WriteString("uci set firewall.wg_mesh_port='rule'\n")
	sb.WriteString("uci set firewall.wg_mesh_port.name='Allow-WG-Mesh'\n")
	sb.WriteString("uci set firewall.wg_mesh_port.src='wan'\n")
	sb.WriteString("uci set firewall.wg_mesh_port.target='ACCEPT'\n")
	sb.WriteString("uci set firewall.wg_mesh_port.proto='udp'\n")
	sb.WriteString(fmt.Sprintf("uci set firewall.wg_mesh_port.dest_port='%d'\n", me.ListenPort))

	// Add peers (spokes)
	for i, peer := range nodes {
		if peer.ID == me.ID {
			continue
		}
		sb.WriteString(fmt.Sprintf("uci set network.wg_mesh_peer%d='wireguard_wg_mesh'\n", i))
		sb.WriteString(fmt.Sprintf("uci set network.wg_mesh_peer%d.public_key='%s'\n", i, peer.PublicKey))
		sb.WriteString(fmt.Sprintf("uci add_list network.wg_mesh_peer%d.allowed_ips='%s/32'\n", i, peer.InternalIP))
		// Optional: add route
		sb.WriteString(fmt.Sprintf("uci set network.wg_mesh_peer%d.route_allowed_ips='1'\n", i))
	}

	sb.WriteString("uci commit network\n")
	sb.WriteString("uci commit firewall\n")
	sb.WriteString("/etc/init.d/network restart\n")
	sb.WriteString("/etc/init.d/firewall restart\n")

	return sb.String()
}

func generateSpokeUCI(mesh models.VPNMesh, hub models.VPNMeshNode, me models.VPNMeshNode, hubEndpoint string) string {
	var sb strings.Builder

	// Network interface
	sb.WriteString(fmt.Sprintf("uci set network.wg_mesh='interface'\n"))
	sb.WriteString(fmt.Sprintf("uci set network.wg_mesh.proto='wireguard'\n"))
	sb.WriteString(fmt.Sprintf("uci set network.wg_mesh.private_key='%s'\n", me.PrivateKey))
	sb.WriteString(fmt.Sprintf("uci add_list network.wg_mesh.addresses='%s/24'\n", me.InternalIP))

	// Firewall zone
	sb.WriteString("uci set firewall.wg_mesh='zone'\n")
	sb.WriteString("uci set firewall.wg_mesh.name='wg_mesh'\n")
	sb.WriteString("uci set firewall.wg_mesh.input='ACCEPT'\n")
	sb.WriteString("uci set firewall.wg_mesh.forward='ACCEPT'\n")
	sb.WriteString("uci set firewall.wg_mesh.output='ACCEPT'\n")
	sb.WriteString("uci set firewall.wg_mesh.network='wg_mesh'\n")

	// Add hub as peer
	sb.WriteString("uci set network.wg_mesh_hub='wireguard_wg_mesh'\n")
	sb.WriteString(fmt.Sprintf("uci set network.wg_mesh_hub.public_key='%s'\n", hub.PublicKey))
	sb.WriteString(fmt.Sprintf("uci add_list network.wg_mesh_hub.allowed_ips='%s'\n", mesh.Subnet))
	sb.WriteString(fmt.Sprintf("uci set network.wg_mesh_hub.endpoint_host='%s'\n", hubEndpoint))
	sb.WriteString(fmt.Sprintf("uci set network.wg_mesh_hub.endpoint_port='%d'\n", hub.ListenPort))
	sb.WriteString("uci set network.wg_mesh_hub.persistent_keepalive='25'\n")
	sb.WriteString("uci set network.wg_mesh_hub.route_allowed_ips='1'\n")

	sb.WriteString("uci commit network\n")
	sb.WriteString("uci commit firewall\n")
	sb.WriteString("/etc/init.d/network restart\n")
	sb.WriteString("/etc/init.d/firewall restart\n")

	return sb.String()
}

func SyncVPNMesh(ctx context.Context, schema string, meshID string) error {
	meshes, err := database.GetVPNMeshes(schema)
	if err != nil {
		return err
	}
	var mesh models.VPNMesh
	found := false
	for _, m := range meshes {
		if m.ID == meshID {
			mesh = m
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("mesh not found")
	}

	nodes, err := database.GetVPNMeshNodes(schema, meshID)
	if err != nil {
		return err
	}

	var hubNode models.VPNMeshNode
	for _, n := range nodes {
		if n.DeviceID == mesh.HubDeviceID {
			hubNode = n
			break
		}
	}

	if hubNode.ID == "" {
		return fmt.Errorf("hub node not found in mesh")
	}

	hubEndpoint, err := resolveHubPublicEndpoint(ctx, schema, hubNode)
	if err != nil {
		return fmt.Errorf("failed to resolve hub public endpoint: %w", err)
	}

	for _, n := range nodes {
		var script string
		if n.ID == hubNode.ID {
			script = generateHubUCI(mesh, nodes, n)
		} else {
			script = generateSpokeUCI(mesh, hubNode, n, hubEndpoint)
		}
		
		err := ExecuteCommand(schema, n.DeviceID, script)
		if err != nil {
			fmt.Printf("Failed to sync device %s: %v\n", n.DeviceID, err)
		}
	}

	return nil
}

// endpointRegexp matches an IPv4 literal optionally followed by ":port".
// It rejects hostnames, multi-octet groups, port 0, and ports >= 65536.
// The groups are deliberately restrictive so user-controlled input can't
// smuggle shell metacharacters into downstream SSH / UCI scripts.
var endpointRegexp = regexp.MustCompile(`^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])(:([1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]))?$`)

func resolveHubPublicEndpoint(ctx context.Context, schema string, hub models.VPNMeshNode) (string, error) {
	if hub.PublicEndpoint != "" {
		if !endpointRegexp.MatchString(hub.PublicEndpoint) {
			return "", fmt.Errorf("hub public_endpoint %q is not a valid IPv4[:port]", hub.PublicEndpoint)
		}
		return hub.PublicEndpoint, nil
	}

	_ = ctx

	out, err := ExecuteCommandWithOutput(schema, hub.DeviceID, "curl -s --max-time 5 https://ifconfig.me")
	if err != nil {
		return "", fmt.Errorf("ssh to hub failed while resolving public IP: %w", err)
	}
	candidate := strings.TrimSpace(out)
	if !endpointRegexp.MatchString(candidate) {
		return "", fmt.Errorf("hub device returned non-IP output for public endpoint: %q", candidate)
	}
	return candidate, nil
}
