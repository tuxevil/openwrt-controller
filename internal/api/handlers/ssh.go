package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	"openwrt-controller/internal/authtickets"
	"openwrt-controller/internal/database"
	"openwrt-controller/internal/orchestrator"
)

// allowedWSOrigins is the explicit allowlist of Origin header values that
// may upgrade a WebSocket connection. Configured via the WS_ALLOWED_ORIGINS
// environment variable (comma-separated). An empty / missing value means
// "no origins allowed" (fail-closed) unless WS_ALLOW_ALL_ORIGINS=true is
// explicitly set (intended only for local dev).
var allowedWSOrigins map[string]struct{}

func init() {
	raw := os.Getenv("WS_ALLOWED_ORIGINS")
	if raw != "" {
		allowedWSOrigins = make(map[string]struct{})
		for _, o := range strings.Split(raw, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowedWSOrigins[o] = struct{}{}
			}
		}
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: checkWSOrigin,
}

func checkWSOrigin(r *http.Request) bool {
	if os.Getenv("WS_ALLOW_ALL_ORIGINS") == "true" {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Non-browser clients (curl, native apps) do not send Origin; allow
		// them only when no allowlist is configured.
		return allowedWSOrigins == nil
	}
	if allowedWSOrigins == nil {
		return false
	}
	_, ok := allowedWSOrigins[origin]
	return ok
}

// PrivateKey and PublicKey are kept for backwards compatibility with the
// many call sites that still reference them. The canonical store is
// orchestrator.GetKeyStore(); the package-level aliases are populated by
// RefreshSSHKeys() which is called from main on startup.
var (
	PrivateKey         ssh.Signer
	PublicKey          string
	refreshSSHKeysOnce sync.Once
)

// RefreshSSHKeys is called from main to mirror the process-wide KeyStore
// into the package-level aliases (PublicKey, PrivateKey) that older code
// still reads. New code should call orchestrator.GetKeyStore().Get()
// directly.
func RefreshSSHKeys() {
	refreshSSHKeysOnce.Do(func() {
		ks := orchestrator.GetKeyStore()
		if ks == nil {
			log.Println("[ssh] no KeyStore available; SSH endpoints will be disabled")
			return
		}
		signer, err := ks.Get()
		if err == nil {
			PrivateKey = signer
		}
		pub, perr := orchestrator.LoadPublicKey()
		if perr == nil {
			PublicKey = strings.TrimSpace(pub)
		}
	})
}

func DeviceSSHHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id required", http.StatusBadRequest)
		return
	}

	// ── Authentication: ticket first, then legacy JWT query string ──
	// The ticket path is the new flow:
	//   1. Dashboard POSTs /api/ws-ticket with the JWT in the
	//      Authorization header.
	//   2. Dashboard opens this WS with ?ticket=<id>.
	//   3. We redeem the ticket here (single-use, 30s TTL) and
	//      never see the JWT in this request.
	// The legacy JWT query string is kept for backwards compatibility
	// behind the WS_ALLOW_QUERY_TOKEN env flag.
	username := "system"
	ticketID := r.URL.Query().Get("ticket")
	if ticketID != "" {
		store := authtickets.GetStore()
		if store == nil {
			http.Error(w, "ws ticket store not initialised", http.StatusServiceUnavailable)
			return
		}
		t, err := store.Consume(ticketID)
		if err != nil {
			log.Printf("[ssh] ticket reject: %v (from %s)", err, r.RemoteAddr)
			http.Error(w, "invalid or expired ticket", http.StatusUnauthorized)
			return
		}
		username = t.Username
	} else if os.Getenv("WS_ALLOW_QUERY_TOKEN") == "true" {
		// Legacy path: ?token=<jwt>. Off by default.
		raw := r.URL.Query().Get("token")
		if raw == "" {
			http.Error(w, "ticket required (legacy JWT in query string disabled)", http.StatusUnauthorized)
			return
		}
		username = GetUsernameFromReq(r)
	} else {
		http.Error(w, "ticket required", http.StatusUnauthorized)
		return
	}

	var targetIP sql.NullString
	err := database.Tx(r.Context()).QueryRow("SELECT last_ip FROM devices WHERE id = $1", deviceID).Scan(&targetIP)
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

	signer, err := orchestrator.GetKeyStore().Get()
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n[!] ERROR: Controller SSH private key not configured\r\n"))
		return
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: orchestrator.TofuHostKeyCallback,
	}

	// d) Abre una conexión SSH hacia la IP del router
	sshConn, err := ssh.Dial("tcp", targetAddr, config)
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n[!] SSH Connection Failed\r\n"))
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
	// The previous version appended every WS read into a single []byte
	// shared across three goroutines without synchronisation; that raced
	// under -race. The fixed version accumulates into a *sync.Mutex-guarded
	// buffer.
	clientIP := r.RemoteAddr

	const maxBufferedInput = 1 << 20 // 1 MiB; anything larger is truncated.
	var (
		inputMu      sync.Mutex
		inputBuffer  []byte
		writeInputMu sync.Mutex
	)

	go func() {
		for {
			ws.SetReadDeadline(time.Now().Add(60 * time.Second))
			_, msg, err := ws.ReadMessage()
			if err != nil {
				sshConn.Close()
				break
			}
			inputMu.Lock()
			if len(inputBuffer) < maxBufferedInput {
				inputBuffer = append(inputBuffer, msg...)
			}
			inputMu.Unlock()
			writeInputMu.Lock()
			_, _ = sshIn.Write(msg)
			writeInputMu.Unlock()
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

	if username != "system" {
		inputMu.Lock()
		buf := append([]byte(nil), inputBuffer...)
		inputMu.Unlock()
		if len(buf) > 0 {
			database.InsertAuditLog(username, "MATRIX_SHELL_SESSION", "DEVICE", deviceID, string(buf), clientIP)
		}
	}
}
