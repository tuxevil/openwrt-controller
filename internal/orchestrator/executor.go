package orchestrator

import (
	"bytes"
	"database/sql"
	"fmt"
	"strings"
	"time"

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
		Timeout:         10 * time.Second,
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

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	script := cmd
	if !strings.HasSuffix(script, "\n") {
		script += "\n"
	}
	session.Stdin = strings.NewReader(script)
	err = session.Run("sh")
	combined := stdout.String() + stderr.String()
	if err != nil {
		// Detect specifically if nftables is missing (exit code 127 or "command not found" string)
		if e, ok := err.(*ssh.ExitError); ok {
			if e.ExitStatus() == 127 || combined == "bash: nft: command not found\n" || combined == "ash: nft: not found\n" {
				return fmt.Errorf("Incompatible Engine: nftables not supported on this device")
			}
		}
		return fmt.Errorf("command execution failed: %v, output: %s", err, combined)
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
		Timeout:         10 * time.Second,
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

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	script := cmd
	if !strings.HasSuffix(script, "\n") {
		script += "\n"
	}
	session.Stdin = strings.NewReader(script)
	err = session.Run("sh")
	combined := stdout.String()
	if stderr.Len() > 0 {
		if combined != "" && !strings.HasSuffix(combined, "\n") {
			combined += "\n"
		}
		combined += stderr.String()
	}
	if err != nil {
		return combined, fmt.Errorf("command execution failed: %v", err)
	}
	return combined, nil
}
