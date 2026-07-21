package orchestrator

import (
	"bytes"
	"database/sql"
	"fmt"
	"strings"

	"openwrt-controller/internal/database"

	"golang.org/x/crypto/ssh"
)

// getSigner returns the controller's SSH signer from the process-wide
// KeyStore. It exists so the per-command helpers below don't all have to
// re-implement the same nil check.
func getSigner() (ssh.Signer, error) {
	ks := GetKeyStore()
	if ks == nil {
		return nil, fmt.Errorf("controller SSH key not loaded (call orchestrator.LoadKeyStore in main)")
	}
	return ks.Get()
}

// ExecuteCommand runs a bash command over SSH on the target device
func ExecuteCommand(schema, deviceID string, cmd string) error {
	sqlSchema, err := database.SafeSQLSchemaIdent(schema)
	if err != nil {
		return fmt.Errorf("invalid tenant schema: %w", err)
	}
	var targetIP sql.NullString
	err = database.DB.QueryRow(fmt.Sprintf("SELECT last_ip FROM %s.devices WHERE id = $1", sqlSchema), deviceID).Scan(&targetIP)
	if err != nil || !targetIP.Valid || targetIP.String == "" {
		return fmt.Errorf("device ip not found")
	}

	signer, err := getSigner()
	if err != nil {
		return err
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
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

	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = &out
	session.Stdin = strings.NewReader(cmd)
	err = session.Run("sh")
	if err != nil {
		// Detect specifically if nftables is missing (exit code 127 or "command not found" string)
		if e, ok := err.(*ssh.ExitError); ok {
			if e.ExitStatus() == 127 || out.String() == "bash: nft: command not found\n" || out.String() == "ash: nft: not found\n" {
				return fmt.Errorf("Incompatible Engine: nftables not supported on this device")
			}
		}
		return fmt.Errorf("command execution failed: %v, output: %s", err, out.String())
	}

	return nil
}

// ExecuteCommandWithOutput runs a bash command over SSH and returns the stdout/stderr
func ExecuteCommandWithOutput(schema, deviceID string, cmd string) (string, error) {
	sqlSchema, err := database.SafeSQLSchemaIdent(schema)
	if err != nil {
		return "", fmt.Errorf("invalid tenant schema: %w", err)
	}
	var targetIP sql.NullString
	err = database.DB.QueryRow(fmt.Sprintf("SELECT last_ip FROM %s.devices WHERE id = $1", sqlSchema), deviceID).Scan(&targetIP)
	if err != nil || !targetIP.Valid || targetIP.String == "" {
		return "", fmt.Errorf("device ip not found")
	}

	signer, err := getSigner()
	if err != nil {
		return "", err
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: TofuHostKeyCallback,
	}

	sshConn, err := ssh.Dial("tcp", targetIP.String+":22", config)
	if err != nil {
		return "", fmt.Errorf("ssh connection failed: %v", err)
	}
	defer sshConn.Close()

	session, err := sshConn.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create ssh session: %v", err)
	}
	defer session.Close()

	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = &out
	session.Stdin = strings.NewReader(cmd)
	err = session.Run("sh")
	if err != nil {
		return out.String(), fmt.Errorf("command execution failed: %v", err)
	}
	return out.String(), nil
}
