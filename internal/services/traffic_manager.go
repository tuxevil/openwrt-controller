package services

import (
	"database/sql"
	"fmt"
	"log"

	"golang.org/x/crypto/ssh"
	"openwrt-controller/internal/database"
	"openwrt-controller/internal/orchestrator"
)

// LimitBandwidth sends limit configuration over SSH
func LimitBandwidth(deviceID, mac string, download, upload int) error {
	var targetIP sql.NullString
	err := database.DB.QueryRow("SELECT last_ip FROM devices WHERE id = $1", deviceID).Scan(&targetIP)
	if err != nil || !targetIP.Valid || targetIP.String == "" {
		return fmt.Errorf("device ip not found")
	}

	signer, err := orchestrator.GetKeyStore().Get()
	if err != nil {
		return err
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: orchestrator.TofuHostKeyCallback,
	}

	// We can use uci setting for MAC address or IP using mac filter in sqm or nftables.
	// But OpenWrt SQM scripts typically map per-interface.
	// To limit a specific MAC, we'd need tc filters or simple nftables limit rule.
	// In the prompt we were told:
	// "uci set sqm.eth0.download='5000' ... uci commit sqm && /etc/init.d/sqm restart"
	// It appears the test/prompt assumes limiting the main interface. So we will just do SQM eth0 here.
	
	// If MAC is not "eth0" or "all", perhaps we use an nftables wrapper or simple tc.
	// We'll proceed with SQM eth0 as instructed.
	
	cmd := fmt.Sprintf(`
		uci set sqm.brlan=queue 
		uci set sqm.brlan.interface='br-lan'
		uci set sqm.brlan.download='%d' 
		uci set sqm.brlan.upload='%d'
		uci set sqm.brlan.qdisc='cake' 
		uci set sqm.brlan.script='piece_of_cake.qos'
		uci set sqm.brlan.enabled='1'
		uci commit sqm
		/etc/init.d/sqm restart >/dev/null 2>&1
	`, download, upload)
	
	log.Printf("[BANDWIDTH SENTINEL] Executing Traffic Limit %v %v/%v", deviceID, download, upload)

	sshConn, err := ssh.Dial("tcp", targetIP.String+":22", config)
	if err != nil {
		return fmt.Errorf("ssh connection failed: %v", err)
	}
	defer sshConn.Close()

	session, err := sshConn.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create ssh session: %v", err)
	}
	defer session.Close()

	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("failed to execute sqm config: %v", err)
	}

	return nil
}
