package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"

	"openwrt-controller/internal/api/middleware"
	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

// sanitiseFirmwareFilename returns a safe filename for storage. It strips
// any directory component (handling both POSIX and Windows separators),
// rejects control characters, and caps the length to 255 bytes (the
// practical limit for ext4/NTFS). Returns "" if the input is empty or
// reduces to nothing after sanitisation.
func sanitiseFirmwareFilename(name string) string {
	name = strings.TrimSpace(name)
	// Normalise Windows backslashes so filepath.Base can strip them.
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "" || name == "." || name == "/" {
		return ""
	}
	// Reject any non-printable runes / control characters.
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}

func CreateBackupTrigger(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error":"invalid device"}`, http.StatusBadRequest)
		return
	}

	// Primero verificamos que el dispositivo tenga last_ip registrada
	var lastIP string
	err := database.Tx(r.Context()).QueryRow(`SELECT COALESCE(last_ip, '') FROM devices WHERE id = $1`, deviceID).Scan(&lastIP)
	if err != nil || lastIP == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Device has no known IP address. Ensure the device has sent telemetry recently.",
		})
		return
	}

	// Correr asincrónicamente para no bloquear UI
	go func() {
		if err := services.CreateBackup(context.Background(), middleware.GetTenantSchema(r), deviceID); err != nil {
			log.Printf("[VAULT][ERROR] Backup failed for device %s: %v", deviceID, err)
		} else {
			log.Printf("[VAULT][OK] Backup completed for device %s", deviceID)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "BACKUP_STARTED"})
}

func GetDeviceBackupsHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	rows, err := database.Tx(r.Context()).Query(`
		SELECT id, checksum, created_at 
		FROM backups WHERE device_id = $1 ORDER BY created_at DESC
	`, deviceID)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var backups []map[string]interface{}
	for rows.Next() {
		var id string
		var chk string
		var d string
		if err := rows.Scan(&id, &chk, &d); err == nil {
			backups = append(backups, map[string]interface{}{
				"id": id, "checksum": chk, "created_at": d,
			})
		}
	}
	if backups == nil {
		backups = []map[string]interface{}{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": backups})
}

func DiffBackupHandler(w http.ResponseWriter, r *http.Request) {
	id1 := r.PathValue("backup_id")
	id2 := r.URL.Query().Get("compare_with")

	if id1 == "" || id2 == "" {
		http.Error(w, `{"error":"need both IDs"}`, http.StatusBadRequest)
		return
	}

	var buf1, buf2 []byte
	err := database.Tx(r.Context()).QueryRow(`SELECT content FROM backups WHERE id = $1`, id1).Scan(&buf1)
	if err != nil {
		http.Error(w, `{"error":"B1 missing"}`, 404)
		return
	}

	err = database.Tx(r.Context()).QueryRow(`SELECT content FROM backups WHERE id = $1`, id2).Scan(&buf2)
	if err != nil {
		http.Error(w, `{"error":"B2 missing"}`, 404)
		return
	}

	files1, err := backupFiles(buf1)
	if err != nil {
		http.Error(w, `{"error":"B1 invalid backup"}`, http.StatusUnprocessableEntity)
		return
	}
	files2, err := backupFiles(buf2)
	if err != nil {
		http.Error(w, `{"error":"B2 invalid backup"}`, http.StatusUnprocessableEntity)
		return
	}
	paths := make(map[string]bool)
	for path := range files1 {
		paths[path] = true
	}
	for path := range files2 {
		paths[path] = true
	}
	sortedPaths := make([]string, 0, len(paths))
	for path := range paths {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)
	type change struct {
		Path   string `json:"path"`
		Status string `json:"status"`
	}
	changes := make([]change, 0)
	for _, path := range sortedPaths {
		a, oka := files1[path]
		b, okb := files2[path]
		status := "unchanged"
		if !oka {
			status = "added"
		} else if !okb {
			status = "removed"
		} else if string(a) != string(b) {
			status = "modified"
		}
		if status != "unchanged" {
			changes = append(changes, change{Path: path, Status: status})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"changes": changes, "changed_files": len(changes)}})
}

func backupFiles(content []byte) (map[string][]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	files := map[string][]byte{}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if header.Name == "" || strings.Contains(header.Name, "..") {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tarReader, 10<<20))
		if err != nil {
			return nil, err
		}
		files[header.Name] = data
	}
	return files, nil
}

// UploadFirmwareHandler receives multipart and saves

// Limit concurrent firmware uploads to prevent OOM
var uploadSemaphore = make(chan struct{}, 5)

func UploadFirmwareHandler(w http.ResponseWriter, r *http.Request) {
	uploadSemaphore <- struct{}{}
	defer func() { <-uploadSemaphore }()

	r.ParseMultipartForm(50 << 20) // 50 MB
	file, handler, err := r.FormFile("firmware")
	if err != nil {
		http.Error(w, "missing firmware", http.StatusBadRequest)
		return
	}
	defer file.Close()

	buf, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}

	// Sanitise the user-supplied filename before persisting. The previous
	// version stored handler.Filename verbatim, which would propagate
	// directory-traversal sequences (../) and control characters to the
	// download endpoint and to sysupgrade invocations.
	safeName := sanitiseFirmwareFilename(handler.Filename)
	if safeName == "" {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	var id uuid.UUID
	err = database.Tx(r.Context()).QueryRow(`
		INSERT INTO firmwares (filename, version, data) VALUES ($1, $2, $3) RETURNING id
	`, safeName, r.FormValue("version"), buf).Scan(&id)

	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "FIRMWARE_STORED", "id": id.String()})
}

// TriggerSysupgradeHandler is intentionally disabled until firmware rollout
// has an explicit compatibility check, backup, health gate, and rollback plan.
func TriggerSysupgradeHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, `{"error":"firmware rollout is not available; use a validated staged workflow"}`, http.StatusNotImplemented)
}
