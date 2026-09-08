package database_test

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func TestQueueDeviceChangeSetReservesDeviceGeneration(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previousDB
		_ = db.Close()
	})

	changeSet, err := services.NewDeviceChangeSet(
		"device-1",
		"system",
		[]services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		nil,
		services.ConfirmationLocalAuto,
	)
	if err != nil {
		t.Fatal(err)
	}
	stored := changeSet
	stored.Generation = 42
	storedRaw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}

	queueQuery := `(?s)` +
		regexp.QuoteMeta("UPDATE tenant_demo.devices AS device") +
		`.*WHERE device\.id = \$2\s+` +
		regexp.QuoteMeta("AND LOWER(device.site_id::text) = LOWER($5)") +
		`.*` + regexp.QuoteMeta("AND COALESCE(device.device_role, 'AP') = $7") +
		`.*` + regexp.QuoteMeta("AND COALESCE(device.capabilities->'device_change_set' = '{\"version\":1,\"namespaces\":[\"system\"],\"max_operations\":1,\"confirmation_policies\":[\"local_auto\"]}'::jsonb, false)") +
		`.*AND NOT EXISTS\s+\(\s+SELECT 1\s+FROM ` + regexp.QuoteMeta("tenant_demo.rollout_runs AS active_rollout") +
		`.*` + regexp.QuoteMeta("LOWER(active_rollout.id::text) <> LOWER($6::text)")
	mock.ExpectQuery(queueQuery).
		WithArgs(sqlmock.AnyArg(), "device-1", changeSet.ChangeSetID, int64(0), "site-1", "rollout-1", "AP").
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_change_set"}).AddRow(int64(42), storedRaw))

	generation, err := database.QueueDeviceChangeSetForRollout(t.Context(), "tenant_demo", "site-1", "AP", "device-1", mustMarshalChangeSet(t, changeSet), "rollout-1")
	if err != nil {
		t.Fatal(err)
	}
	if generation != 42 {
		t.Fatalf("queued generation = %d, want 42", generation)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordDeviceChangeSetStatusPersistsTerminalIdentity(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previousDB
		_ = db.Close()
	})

	status, err := json.Marshal(map[string]any{
		"change_set_id": "cs-operation-1",
		"device_id":     "device-1",
		"plan_hash":     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"generation":    42,
		"state":         "COMMITTED",
	})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`(?s)UPDATE tenant_demo\.devices.*last_change_set\s*=\s*\$1.*pending_change_set\s*=\s*CASE`).
		WithArgs(sqlmock.AnyArg(), true, "cs-operation-1", "COMMITTED", "device-1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "42", int64(42)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.rollout_runs")).
		WithArgs("completed", "device-1", "cs-operation-1", "SUCCESS", "COMMITTED", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", int64(42), "").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := database.RecordDeviceChangeSetStatus(t.Context(), "tenant_demo", "device-1", status); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordDeviceChangeSetStatusRejectsOutOfOrderNonterminalState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previousDB
		_ = db.Close()
	})

	planHash := strings.Repeat("a", 64)
	status := func(state string) []byte {
		raw, err := json.Marshal(map[string]interface{}{
			"change_set_id": "cs-ordering",
			"device_id":     "device-1",
			"plan_hash":     planHash,
			"generation":    7,
			"state":         state,
		})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	update := `(?s)` + regexp.QuoteMeta("UPDATE tenant_demo.devices") + `.*COALESCE\(last_change_set->>'change_set_id', ''\).*CASE \$4::text`
	for _, state := range []string{"PREPARED", "APPLYING"} {
		mock.ExpectExec(update).
			WithArgs(sqlmock.AnyArg(), false, "cs-ordering", state, "device-1", planHash, "7", int64(7)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		if err := database.RecordDeviceChangeSetStatus(t.Context(), "tenant_demo", "device-1", status(state)); err != nil {
			t.Fatalf("record %s: %v", state, err)
		}
	}
	mock.ExpectExec(update).
		WithArgs(sqlmock.AnyArg(), false, "cs-ordering", "PREPARED", "device-1", planHash, "7", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := database.RecordDeviceChangeSetStatus(t.Context(), "tenant_demo", "device-1", status("PREPARED")); !errors.Is(err, database.ErrDeviceChangeSetStatusNotApplied) {
		t.Fatalf("late status error = %v, want %v", err, database.ErrDeviceChangeSetStatusNotApplied)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQueueDeviceChangeSetReturnsStoredGenerationForTerminalRetry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previousDB
		_ = db.Close()
	})

	changeSet, err := services.NewDeviceChangeSet(
		"device-1",
		"system",
		[]services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		nil,
		services.ConfirmationLocalAuto,
	)
	if err != nil {
		t.Fatal(err)
	}
	changeSet.Generation = 42
	statusRaw, err := json.Marshal(map[string]any{
		"change_set_id": changeSet.ChangeSetID,
		"device_id":     changeSet.DeviceID,
		"plan_hash":     changeSet.PlanHash,
		"generation":    changeSet.Generation,
		"state":         "COMMITTED",
	})
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.devices AS device")).
		WithArgs(sqlmock.AnyArg(), "device-1", changeSet.ChangeSetID, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_change_set"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT site_id::text, COALESCE(device_role, 'AP'), COALESCE(capabilities->'device_change_set' = '{\"version\":1,\"namespaces\":[\"system\"],\"max_operations\":1,\"confirmation_policies\":[\"local_auto\"]}'::jsonb, false), desired_generation, pending_change_set, last_change_set")).
		WithArgs("device-1").
		WillReturnRows(sqlmock.NewRows([]string{"site_id", "device_role", "device_change_set_capability", "desired_generation", "pending_change_set", "last_change_set"}).AddRow("site-1", "AP", true, int64(42), nil, statusRaw))

	generation, err := database.QueueDeviceChangeSet(t.Context(), "tenant_demo", "device-1", mustMarshalChangeSet(t, changeSet))
	if err != nil {
		t.Fatal(err)
	}
	if generation != 42 {
		t.Fatalf("retry generation = %d, want 42", generation)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQueueDeviceChangeSetRejectsRolloutSiteMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previousDB
		_ = db.Close()
	})

	changeSet, err := services.NewDeviceChangeSet(
		"device-1",
		"system",
		[]services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		nil,
		services.ConfirmationLocalAuto,
	)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.devices AS device")).
		WithArgs(sqlmock.AnyArg(), "device-1", changeSet.ChangeSetID, int64(0), "site-2", "rollout-1", "AP").
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_change_set"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT site_id::text, COALESCE(device_role, 'AP'), COALESCE(capabilities->'device_change_set' = '{\"version\":1,\"namespaces\":[\"system\"],\"max_operations\":1,\"confirmation_policies\":[\"local_auto\"]}'::jsonb, false), desired_generation, pending_change_set, last_change_set")).
		WithArgs("device-1").
		WillReturnRows(sqlmock.NewRows([]string{"site_id", "device_role", "device_change_set_capability", "desired_generation", "pending_change_set", "last_change_set"}).AddRow("site-1", "AP", true, int64(0), nil, nil))

	if _, err := database.QueueDeviceChangeSetForRollout(t.Context(), "tenant_demo", "site-2", "AP", "device-1", mustMarshalChangeSet(t, changeSet), "rollout-1"); err == nil {
		t.Fatal("cross-site rollout enqueue was accepted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQueueDeviceChangeSetRejectsRecoveryRequiredDevice(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previousDB
		_ = db.Close()
	})

	changeSet, err := services.NewDeviceChangeSet(
		"device-1",
		"system",
		[]services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		nil,
		services.ConfirmationLocalAuto,
	)
	if err != nil {
		t.Fatal(err)
	}
	lastStatus, err := json.Marshal(map[string]any{
		"change_set_id": "previous-change-set",
		"device_id":     "device-1",
		"plan_hash":     strings.Repeat("b", 64),
		"generation":    42,
		"state":         "RECOVERY_REQUIRED",
	})
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.devices AS device")).
		WithArgs(sqlmock.AnyArg(), "device-1", changeSet.ChangeSetID, int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_change_set"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT site_id::text, COALESCE(device_role, 'AP'), COALESCE(capabilities->'device_change_set' = '{\"version\":1,\"namespaces\":[\"system\"],\"max_operations\":1,\"confirmation_policies\":[\"local_auto\"]}'::jsonb, false), desired_generation, pending_change_set, last_change_set")).
		WithArgs("device-1").
		WillReturnRows(sqlmock.NewRows([]string{"site_id", "device_role", "device_change_set_capability", "desired_generation", "pending_change_set", "last_change_set"}).AddRow("site-1", "AP", false, int64(42), nil, lastStatus))

	if _, err := database.QueueDeviceChangeSet(t.Context(), "tenant_demo", "device-1", mustMarshalChangeSet(t, changeSet)); err == nil {
		t.Fatal("changeset was accepted for a recovery-required device")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQueueDeviceChangeSetRejectsMissingCapability(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previousDB
		_ = db.Close()
	})

	changeSet, err := services.NewDeviceChangeSet(
		"device-1",
		"system",
		[]services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
		strings.Repeat("a", 64),
		nil,
		services.ConfirmationLocalAuto,
	)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.devices AS device")).
		WithArgs(sqlmock.AnyArg(), "device-1", changeSet.ChangeSetID, int64(0), "site-1", "rollout-1", "AP").
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_change_set"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT site_id::text, COALESCE(device_role, 'AP'), COALESCE(capabilities->'device_change_set' = '{\"version\":1,\"namespaces\":[\"system\"],\"max_operations\":1,\"confirmation_policies\":[\"local_auto\"]}'::jsonb, false), desired_generation, pending_change_set, last_change_set")).
		WithArgs("device-1").
		WillReturnRows(sqlmock.NewRows([]string{"site_id", "device_role", "device_change_set_capability", "desired_generation", "pending_change_set", "last_change_set"}).AddRow("site-1", "AP", false, int64(0), nil, nil))

	if _, err := database.QueueDeviceChangeSetForRollout(t.Context(), "tenant_demo", "site-1", "AP", "device-1", mustMarshalChangeSet(t, changeSet), "rollout-1"); !errors.Is(err, database.ErrDeviceChangeSetCapability) {
		t.Fatalf("capability error = %v, want %v", err, database.ErrDeviceChangeSetCapability)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQueueDeviceChangeSetRequiresCapabilityWithoutRollout(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previousDB
		_ = db.Close()
	})

	changeSet, err := services.NewDeviceChangeSet(
		"device-1",
		"system",
		[]services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
		strings.Repeat("a", 64),
		nil,
		services.ConfirmationLocalAuto,
	)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("UPDATE tenant_demo.devices AS device")+`.*`+regexp.QuoteMeta("COALESCE(device.capabilities->'device_change_set' @> '{\"version\":1,\"namespaces\":[\"system\"],\"max_operations\":1,\"confirmation_policies\":[\"local_auto\"]}'::jsonb, false)")).
		WithArgs(sqlmock.AnyArg(), "device-1", changeSet.ChangeSetID, int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_change_set"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT site_id::text, COALESCE(device_role, 'AP'), COALESCE(capabilities->'device_change_set' = '{\"version\":1,\"namespaces\":[\"system\"],\"max_operations\":1,\"confirmation_policies\":[\"local_auto\"]}'::jsonb, false), desired_generation, pending_change_set, last_change_set")).
		WithArgs("device-1").
		WillReturnRows(sqlmock.NewRows([]string{"site_id", "device_role", "device_change_set_capability", "desired_generation", "pending_change_set", "last_change_set"}).AddRow("site-1", "AP", false, int64(0), nil, nil))

	if _, err := database.QueueDeviceChangeSet(t.Context(), "tenant_demo", "device-1", mustMarshalChangeSet(t, changeSet)); !errors.Is(err, database.ErrDeviceChangeSetCapability) {
		t.Fatalf("capability error = %v, want %v", err, database.ErrDeviceChangeSetCapability)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQueueDeviceChangeSetRejectsTamperedContentHashBeforeDatabaseWrite(t *testing.T) {
	changeSet, err := services.NewDeviceChangeSet(
		"device-1",
		"system",
		[]services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		nil,
		services.ConfirmationLocalAuto,
	)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(strings.Replace(string(mustMarshalChangeSet(t, changeSet)), "lab-router", "tampered-router", 1))
	if _, err := database.QueueDeviceChangeSet(t.Context(), "tenant_demo", "device-1", raw); err == nil {
		t.Fatal("tampered changeset was accepted")
	}
}

func mustMarshalChangeSet(t *testing.T, changeSet services.DeviceChangeSet) []byte {
	t.Helper()
	raw, err := json.Marshal(changeSet)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
