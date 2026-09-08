package devices_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"openwrt-controller/internal/services"
)

func TestAgentChangesetConfigPullAndTelemetryRoundTrip(t *testing.T) {
	const (
		deviceID = "self-test-device"
		token    = "device-token"
	)
	const observedState = "system.@system[0].hostname=baseline\n"
	observedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(observedState)))
	changeSet, err := services.NewDeviceChangeSet(
		deviceID,
		"system",
		[]services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
		observedHash,
		nil,
		services.ConfirmationLocalAuto,
	)
	if err != nil {
		t.Fatal(err)
	}
	changeSet.Generation = 42

	var mu sync.Mutex
	configPulls := 0
	var serverErr error
	setServerErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if serverErr == nil {
			serverErr = err
		}
	}
	committed := make(chan struct{})
	var committedOnce sync.Once

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Device-Token"); got != token {
			setServerErr(fmt.Errorf("%s %s used token %q", r.Method, r.URL.Path, got))
		}
		switch r.URL.Path {
		case "/api/agent/latest":
			_, _ = io.WriteString(w, "{}")
		case "/api/devices/" + deviceID + "/config":
			mu.Lock()
			pull := configPulls
			configPulls++
			mu.Unlock()
			response := map[string]interface{}{"action": "apply", "config": map[string]interface{}{}}
			if pull == 0 {
				response["config"].(map[string]interface{})["apply_change_set"] = changeSet
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		case "/api/telemetry":
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				setServerErr(fmt.Errorf("decode telemetry: %w", err))
			} else if status, ok := payload["change_set_transaction"].(map[string]interface{}); ok && status["state"] == "COMMITTED" {
				if status["change_set_id"] != changeSet.ChangeSetID || status["device_id"] != deviceID || status["plan_hash"] != changeSet.PlanHash {
					setServerErr(fmt.Errorf("telemetry identity = %#v", status))
				}
				if generation, ok := status["generation"].(float64); !ok || int64(generation) != changeSet.Generation {
					setServerErr(fmt.Errorf("telemetry generation = %#v", status["generation"]))
				}
				committedOnce.Do(func() { close(committed) })
			}
			w.WriteHeader(http.StatusAccepted)
		default:
			setServerErr(fmt.Errorf("unexpected agent request %s %s", r.Method, r.URL.Path))
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	configRoot := filepath.Join(root, "etc", "config")
	transactionRoot := filepath.Join(root, "etc", "nerve", "transactions")
	if err := os.MkdirAll(configRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	deviceIDFile := filepath.Join(root, "device-id")
	deviceTokenFile := filepath.Join(root, "device-token")
	uciState := filepath.Join(root, "uci-state")
	uciLog := filepath.Join(root, "uci.log")
	for path, value := range map[string]string{
		deviceIDFile:    deviceID + "\n",
		deviceTokenFile: token + "\n",
		uciState:        observedState,
	} {
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate agent test")
	}
	devicesDir := filepath.Dir(filename)
	fixtureDir := filepath.Join(devicesDir, "test-fixtures", "transaction")
	pathEnv := fixtureDir + ":" + os.Getenv("PATH")
	cmd := exec.Command("sh", filepath.Join(devicesDir, "agent.sh"))
	cmd.Env = append(os.Environ(),
		"CONTROLLER_URL="+server.URL+"/api",
		"REQUIRE_TLS=false",
		"DEVICE_ID_FILE="+deviceIDFile,
		"DEVICE_TOKEN_FILE="+deviceTokenFile,
		"NERVE_TRANSACTION_ROOT="+transactionRoot,
		"NERVE_CONFIG_ROOT="+configRoot,
		"NERVE_OPERATION_STATUS_FILE="+filepath.Join(transactionRoot, "operation_status"),
		"NERVE_CHANGE_SET_STATUS_FILE="+filepath.Join(transactionRoot, "change_set_status"),
		"UCI_FIXTURE_STATE="+uciState,
		"UCI_FIXTURE_LOG="+uciLog,
		"PATH="+pathEnv,
	)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	waited := false
	defer func() {
		if !waited && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
		}
		if !waited {
			select {
			case <-waitDone:
			case <-time.After(2 * time.Second):
				t.Errorf("agent did not stop after cleanup")
			}
		}
	}()

	var agentErr error
	select {
	case <-committed:
		_ = cmd.Process.Kill()
		agentErr = <-waitDone
		waited = true
	case err := <-waitDone:
		waited = true
		t.Fatalf("agent exited before committed telemetry: %v\nstderr: %s", err, stderr.String())
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		agentErr = <-waitDone
		waited = true
		t.Fatalf("timed out waiting for committed telemetry\nstderr: %s", stderr.String())
	}
	if agentErr != nil {
		if !strings.Contains(agentErr.Error(), "signal: killed") {
			t.Fatalf("agent termination: %v\nstderr: %s", agentErr, stderr.String())
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if serverErr != nil {
		t.Fatal(serverErr)
	}
	if configPulls == 0 {
		t.Fatal("agent did not pull configuration")
	}
	logBytes, err := os.ReadFile(uciLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(logBytes), "set system."); got != 1 {
		t.Fatalf("system mutation count = %d, want 1; log: %s", got, logBytes)
	}
}

func TestAgentReportsRecoveryRequiredBeforeDestructiveWork(t *testing.T) {
	const (
		deviceID = "recovery-test-device"
		token    = "device-token"
		planHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	recoveryReported := make(chan struct{})
	var reportedOnce sync.Once
	var mu sync.Mutex
	var serverErr error
	setServerErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if serverErr == nil {
			serverErr = err
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Device-Token"); got != token {
			setServerErr(fmt.Errorf("%s %s used token %q", r.Method, r.URL.Path, got))
		}
		switch r.URL.Path {
		case "/api/agent/latest":
			_, _ = io.WriteString(w, "{}")
		case "/api/telemetry":
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				setServerErr(fmt.Errorf("decode telemetry: %w", err))
			} else if status, ok := payload["change_set_transaction"].(map[string]interface{}); ok && status["state"] == "RECOVERY_REQUIRED" {
				reportedOnce.Do(func() { close(recoveryReported) })
			}
			w.WriteHeader(http.StatusAccepted)
		case "/api/devices/" + deviceID + "/config":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"action":"none","config":{}}`)
		default:
			setServerErr(fmt.Errorf("unexpected agent request %s %s", r.Method, r.URL.Path))
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	configRoot := filepath.Join(root, "etc", "config")
	transactionRoot := filepath.Join(root, "etc", "nerve", "transactions")
	if err := os.MkdirAll(configRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	deviceIDFile := filepath.Join(root, "device-id")
	deviceTokenFile := filepath.Join(root, "device-token")
	uciState := filepath.Join(root, "uci-state")
	uciLog := filepath.Join(root, "uci.log")
	for path, value := range map[string]string{
		deviceIDFile:    deviceID + "\n",
		deviceTokenFile: token + "\n",
		uciState:        "system.@system[0].hostname=baseline\n",
	} {
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	journalPath := filepath.Join(transactionRoot, "recovery-change-set")
	if err := os.MkdirAll(journalPath, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]string{
		"change_set_id":                  "recovery-change-set",
		"change_set_device_id":           deviceID,
		"change_set_plan_hash":           planHash,
		"change_set_generation":          "42",
		"change_set_operation_id":        "recovery-entry",
		"change_set_commands":            `[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"must-not-apply"}]`,
		"change_set_observed_state_hash": planHash,
		"change_set_health_checks":       "[]",
		"change_set_confirmation_policy": "local_auto",
		"config":                         "system",
		"plan_hash":                      planHash,
		"generation":                     "42",
		"state":                          "RECOVERY_REQUIRED",
	}
	for name, value := range manifest {
		if err := os.WriteFile(filepath.Join(journalPath, name), []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate agent test")
	}
	devicesDir := filepath.Dir(filename)
	fixtureDir := filepath.Join(devicesDir, "test-fixtures", "transaction")
	cmd := exec.Command("sh", filepath.Join(devicesDir, "agent.sh"))
	cmd.Env = append(os.Environ(),
		"CONTROLLER_URL="+server.URL+"/api",
		"REQUIRE_TLS=false",
		"DEVICE_ID_FILE="+deviceIDFile,
		"DEVICE_TOKEN_FILE="+deviceTokenFile,
		"NERVE_TRANSACTION_ROOT="+transactionRoot,
		"NERVE_CONFIG_ROOT="+configRoot,
		"NERVE_OPERATION_STATUS_FILE="+filepath.Join(transactionRoot, "operation_status"),
		"NERVE_CHANGE_SET_STATUS_FILE="+filepath.Join(transactionRoot, "change_set_status"),
		"UCI_FIXTURE_STATE="+uciState,
		"UCI_FIXTURE_LOG="+uciLog,
		"PATH="+fixtureDir+":"+os.Getenv("PATH"),
	)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	select {
	case <-recoveryReported:
		_ = cmd.Process.Kill()
		if err := <-waitDone; err != nil && !strings.Contains(err.Error(), "signal: killed") {
			t.Fatalf("agent termination: %v\nstderr: %s", err, stderr.String())
		}
	case err := <-waitDone:
		t.Fatalf("agent exited before recovery telemetry: %v\nstderr: %s", err, stderr.String())
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		_ = <-waitDone
		t.Fatalf("timed out waiting for recovery telemetry\nstderr: %s", stderr.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if serverErr != nil {
		t.Fatal(serverErr)
	}
	logBytes, err := os.ReadFile(uciLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(logBytes), "set "); got != 0 {
		t.Fatalf("recovery-required agent mutated UCI %d times; log: %s", got, logBytes)
	}
}
