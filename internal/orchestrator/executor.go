package orchestrator

import (
	"database/sql"
	"fmt"
	"os"

	"golang.org/x/crypto/ssh"
	"openwrt-controller/internal/database"
)

var privateKey ssh.Signer

func init() {
	keyBytes, err := os.ReadFile("certs/id_controller")
	if err == nil {
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err == nil {
			privateKey = signer
		}
	}
}

// ExecuteCommand runs a bash command over SSH on the target device
func ExecuteCommand(schema, deviceID string, cmd string) error {
	var targetIP sql.NullString
	err := database.DB.QueryRow(fmt.Sprintf("SELECT last_ip FROM %s.devices WHERE id = $1", schema), deviceID).Scan(&targetIP)
	if err != nil || !targetIP.Valid || targetIP.String == "" {
		return fmt.Errorf("device ip not found")
	}

	if privateKey == nil {
		return fmt.Errorf("ssh private key not loaded")
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(privateKey),
		},
		HostKeyCallback: TofuHostKeyCallback,
	}

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

	out, err := session.CombinedOutput(cmd)
	if err != nil {
		// Detect specifically if nftables is missing (exit code 127 or "command not found" string)
		if e, ok := err.(*ssh.ExitError); ok {
			if e.ExitStatus() == 127 || string(out) == "bash: nft: command not found\n" || string(out) == "ash: nft: not found\n" {
				return fmt.Errorf("Incompatible Engine: nftables not supported on this device")
			}
		}
		return fmt.Errorf("command execution failed: %v, output: %s", err, string(out))
	}

	return nil
}
