package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	"openwrt-controller/internal/database"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for the prototype
	},
}

var (
	PrivateKey ssh.Signer
	PublicKey  string
)

func init() {
	keyBytes, err := os.ReadFile("certs/id_controller")
	if err != nil {
		log.Println("[CRITICAL] Private key ./certs/id_controller not found. SSH Matrix will be disabled.")
		return
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		log.Printf("[CRITICAL] Failed to parse private key: %v", err)
		return
	}
	PrivateKey = signer

	pubBytes, err := os.ReadFile("certs/id_controller.pub")
	if err != nil {
		log.Println("[WARNING] Public key ./certs/id_controller.pub not found.")
		return
	}
	PublicKey = string(pubBytes)
}

func DeviceSSHHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id required", http.StatusBadRequest)
		return
	}

	var targetIP sql.NullString
	err := database.DB.QueryRow("SELECT last_ip FROM devices WHERE id = $1", deviceID).Scan(&targetIP)
	if err != nil || !targetIP.Valid || targetIP.String == "" {
		http.Error(w, "Device IP not found", http.StatusNotFound)
		return
	}
	targetAddr := targetIP.String + ":22"

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade websocket: %v", err)
		return
	}
	defer ws.Close()

	if PrivateKey == nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n[!] ERROR: Controller SSH private key not configured\r\n"))
		return
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(PrivateKey),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// d) Abre una conexión SSH hacia la IP del router
	sshConn, err := ssh.Dial("tcp", targetAddr, config)
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n[!] SSH Connection Failed: "+err.Error()+"\r\n"))
		return
	}
	defer sshConn.Close()

	session, err := sshConn.NewSession()
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n[!] Failed to create SSH session\r\n"))
		return
	}
	defer session.Close()

	// e) Crea una PTY (Pseudo-Terminal)
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}

	if err := session.RequestPty("vt100", 24, 80, modes); err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n[!] Request for PTY failed\r\n"))
		return
	}

	sshIn, err := session.StdinPipe()
	if err != nil {
		return
	}

	sshOut, err := session.StdoutPipe()
	if err != nil {
		return
	}

	sshErr, err := session.StderrPipe()
	if err != nil {
		return
	}

	if err := session.Shell(); err != nil {
		return
	}

	// f) Inicia un pipe bidireccional
	// ws -> ssh
	var inputBuffer []byte
	username := GetUsernameFromReq(r)
	clientIP := r.RemoteAddr

	go func() {
		for {
			_, msg, err := ws.ReadMessage()
			if err != nil {
				sshConn.Close()
				break
			}
			inputBuffer = append(inputBuffer, msg...)
			sshIn.Write(msg)
		}
	}()

	// ssh -> ws (stdout)
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := sshOut.Read(buf)
			if err != nil {
				ws.Close()
				break
			}
			ws.WriteMessage(websocket.TextMessage, buf[:n])
		}
	}()

	// ssh -> ws (stderr)
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := sshErr.Read(buf)
			if err != nil {
				break
			}
			ws.WriteMessage(websocket.TextMessage, buf[:n])
		}
	}()

	// Wait for the session to finish
	session.Wait()

	if username != "system" && len(inputBuffer) > 0 {
		database.InsertAuditLog(username, "MATRIX_SHELL_SESSION", "DEVICE", deviceID, string(inputBuffer), clientIP)
	}
}
