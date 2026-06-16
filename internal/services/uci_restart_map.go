package services

// ServiceRestartMap is the canonical UCI namespace → service restart command
// mapping used by uci_bridge.go. The previous codebase kept a near-identical
// copy in api/handlers/uci_ops.go (UciRestartMap) which would drift and
// could lead to e.g. a /wireless push restarting the wrong daemon. We keep
// this map here and expose it via a getter to avoid an import cycle
// (handlers depends on services, not the other way around).
var ServiceRestartMap = map[string]string{
	"network":   "/etc/init.d/network restart",
	"wireless":  "wifi",
	"dhcp":      "/etc/init.d/dnsmasq restart",
	"firewall":  "/etc/init.d/firewall restart",
	"system":    "/etc/init.d/system restart",
	"dropbear":  "/etc/init.d/dropbear restart",
	"uhttpd":    "/etc/init.d/uhttpd restart",
	"openvpn":   "/etc/init.d/openvpn restart",
	"hostblock": "/etc/init.d/hostblock restart",
}

// ServiceRestartMapAlias returns the canonical restart map. Defined as a
// function (not a variable alias) to keep package boundaries clean and
// avoid any subtle mutation issues with shared references.
func ServiceRestartMapAlias() map[string]string {
	return ServiceRestartMap
}
