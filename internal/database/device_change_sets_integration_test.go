package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func TestDeviceChangeSetIntegrationQueuesAndPersistsTerminalStatus(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL_TEST")
	if os.Getenv("OPENWRT_INTEGRATION_DB") != "1" || dsn == "" {
		t.Skip("set OPENWRT_INTEGRATION_DB=1 and DATABASE_URL_TEST to run PostgreSQL device changeset integration tests")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()

	schema := "tenant_changeset_" + strings.ToLower(strings.ReplaceAll(fmt.Sprint(time.Now().UnixNano()), "-", ""))
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	quotedDevices := pgx.Identifier{schema, "devices"}.Sanitize()
	quotedRollouts := pgx.Identifier{schema, "rollout_runs"}.Sanitize()
	if _, err := database.DB.Exec("CREATE SCHEMA " + quotedSchema); err != nil {
		t.Fatalf("create temporary schema: %v", err)
	}
	defer database.DB.Exec("DROP SCHEMA " + quotedSchema + " CASCADE")

	if _, err := database.DB.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id VARCHAR(50) PRIMARY KEY,
			site_id UUID,
			device_role VARCHAR(32),
			capabilities JSONB,
			desired_generation BIGINT NOT NULL DEFAULT 0,
			observed_generation BIGINT NOT NULL DEFAULT 0,
			last_successful_generation BIGINT NOT NULL DEFAULT 0,
			pending_operation JSONB,
			last_operation JSONB,
			pending_change_set JSONB,
			last_change_set JSONB,
			last_rollout_status VARCHAR(32),
			last_rollout_at TIMESTAMP WITH TIME ZONE,
			last_health_check_at TIMESTAMP WITH TIME ZONE,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE %s (
			id UUID PRIMARY KEY,
			site_id UUID,
			status VARCHAR(32) NOT NULL,
			results JSONB NOT NULL DEFAULT '[]',
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`, quotedDevices, quotedRollouts)); err != nil {
		t.Fatalf("create temporary tables: %v", err)
	}

	deviceID := "device-1"
	siteID := "00000000-0000-0000-0000-000000000001"
	rolloutID := "00000000-0000-0000-0000-000000000002"
	if _, err := database.DB.Exec(fmt.Sprintf("INSERT INTO %s (id, site_id, device_role, capabilities) VALUES ($1, $2, 'AP', '{\"device_change_set\":{\"version\":2,\"namespaces\":[\"system\",\"dhcp\",\"firewall\",\"dropbear\",\"sqm\"],\"max_operations\":8,\"confirmation_policies\":[\"local_auto\"]}}')", quotedDevices), deviceID, siteID); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	if _, err := database.DB.Exec(fmt.Sprintf("INSERT INTO %s (id, site_id, status) VALUES ($1, $2, 'RUNNING')", quotedRollouts), rolloutID, siteID); err != nil {
		t.Fatalf("seed owning rollout: %v", err)
	}

	changeSet, err := services.NewDeviceChangeSet(
		deviceID,
		"system",
		[]services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
		strings.Repeat("a", 64),
		nil,
		services.ConfirmationLocalAuto,
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(changeSet)
	if err != nil {
		t.Fatal(err)
	}

	uppercaseRolloutID := strings.ToUpper(rolloutID)
	generation, err := database.QueueDeviceChangeSetForRollout(context.Background(), schema, siteID, "AP", deviceID, raw, uppercaseRolloutID)
	if err != nil {
		t.Fatalf("queue changeset: %v", err)
	}
	if generation != 1 {
		t.Fatalf("queued generation = %d, want 1", generation)
	}
	if retryGeneration, err := database.QueueDeviceChangeSetForRollout(context.Background(), schema, siteID, "AP", deviceID, raw, rolloutID); err != nil || retryGeneration != generation {
		t.Fatalf("idempotent queue = %d, %v; want %d, nil", retryGeneration, err, generation)
	}
	queuedResults := fmt.Sprintf(`[{"device_id":%q,"change_set_id":%q,"status":"QUEUED"}]`, deviceID, changeSet.ChangeSetID)
	if _, err := database.DB.Exec(fmt.Sprintf("UPDATE %s SET status = 'QUEUED', results = $1 WHERE id = $2", quotedRollouts), queuedResults, rolloutID); err != nil {
		t.Fatalf("seed queued rollout result: %v", err)
	}

	status, err := json.Marshal(map[string]interface{}{
		"change_set_id": changeSet.ChangeSetID,
		"device_id":     deviceID,
		"plan_hash":     changeSet.PlanHash,
		"generation":    generation,
		"state":         "COMMITTED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.RecordDeviceChangeSetStatus(context.Background(), schema, deviceID, status); err != nil {
		t.Fatalf("record terminal changeset status: %v", err)
	}

	var pending, last []byte
	var observed, successful int64
	if err := database.DB.QueryRow(fmt.Sprintf("SELECT pending_change_set, last_change_set, observed_generation, last_successful_generation FROM %s", quotedDevices)).Scan(&pending, &last, &observed, &successful); err != nil {
		t.Fatalf("read changeset state: %v", err)
	}
	if pending != nil {
		t.Fatalf("pending changeset = %s, want NULL", pending)
	}
	if string(last) == "" || observed != generation || successful != generation {
		t.Fatalf("terminal state = %s, observed=%d, successful=%d", last, observed, successful)
	}
	var rolloutStatus string
	if err := database.DB.QueryRow(fmt.Sprintf("SELECT status FROM %s WHERE id = $1", quotedRollouts), rolloutID).Scan(&rolloutStatus); err != nil {
		t.Fatalf("read rollout status: %v", err)
	}
	if rolloutStatus != "completed" {
		t.Fatalf("rollout status = %q, want completed", rolloutStatus)
	}
	var rolloutResults []byte
	if err := database.DB.QueryRow(fmt.Sprintf("SELECT results FROM %s WHERE id = $1", quotedRollouts), rolloutID).Scan(&rolloutResults); err != nil {
		t.Fatalf("read rollout results: %v", err)
	}
	var resultRows []map[string]interface{}
	if err := json.Unmarshal(rolloutResults, &resultRows); err != nil || len(resultRows) != 1 {
		t.Fatalf("rollout results = %s, want one terminal device result: %v", rolloutResults, err)
	}
	if resultRows[0]["status"] != "SUCCESS" || resultRows[0]["change_set_state"] != "COMMITTED" || resultRows[0]["device_generation"] != float64(1) {
		t.Fatalf("rollout result = %#v, want committed generation 1", resultRows[0])
	}

	lateStatus := []byte(fmt.Sprintf(`{"change_set_id":%q,"device_id":%q,"plan_hash":%q,"generation":%d,"state":"APPLYING"}`, changeSet.ChangeSetID, deviceID, changeSet.PlanHash, generation))
	if err := database.RecordDeviceChangeSetStatus(context.Background(), schema, deviceID, lateStatus); !errors.Is(err, database.ErrDeviceChangeSetStatusNotApplied) {
		t.Fatalf("late status error = %v, want %v", err, database.ErrDeviceChangeSetStatusNotApplied)
	}

	unsupportedCapabilities := []struct {
		name string
		raw  string
	}{
		{name: "legacy boolean", raw: `{"device_change_set":true}`},
		{name: "unsupported version", raw: `{"device_change_set":{"version":1,"namespaces":["system"],"max_operations":1,"confirmation_policies":["local_auto"]}}`},
		{name: "unsupported namespace", raw: `{"device_change_set":{"version":2,"namespaces":["system","network"],"max_operations":8,"confirmation_policies":["local_auto"]}}`},
		{name: "unsupported policy", raw: `{"device_change_set":{"version":2,"namespaces":["system","dhcp","firewall","dropbear","sqm"],"max_operations":8,"confirmation_policies":["controller_confirm"]}}`},
	}
	for index, unsupported := range unsupportedCapabilities {
		unsupportedDeviceID := fmt.Sprintf("unsupported-%d", index)
		if _, err := database.DB.Exec(fmt.Sprintf("INSERT INTO %s (id, site_id, device_role, capabilities) VALUES ($1, $2, 'AP', $3::jsonb)", quotedDevices), unsupportedDeviceID, siteID, unsupported.raw); err != nil {
			t.Fatalf("seed %s device: %v", unsupported.name, err)
		}
		unsupportedChangeSet, err := services.NewDeviceChangeSet(
			unsupportedDeviceID,
			"system",
			[]services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
			strings.Repeat("a", 64),
			nil,
			services.ConfirmationLocalAuto,
		)
		if err != nil {
			t.Fatalf("build %s changeset: %v", unsupported.name, err)
		}
		unsupportedRaw, err := json.Marshal(unsupportedChangeSet)
		if err != nil {
			t.Fatalf("marshal %s changeset: %v", unsupported.name, err)
		}
		if _, err := database.QueueDeviceChangeSet(context.Background(), schema, unsupportedDeviceID, unsupportedRaw); !errors.Is(err, database.ErrDeviceChangeSetCapability) {
			t.Errorf("%s capability error = %v, want %v", unsupported.name, err, database.ErrDeviceChangeSetCapability)
		}
	}
}
