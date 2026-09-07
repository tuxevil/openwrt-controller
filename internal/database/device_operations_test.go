package database

import (
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestParseDeviceOperationEnvelopeSeparatesAttemptAndContentIdentity(t *testing.T) {
	valid := json.RawMessage(`{"operation_id":"operation-1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)
	got, planHash, err := parseDeviceOperationEnvelope(valid)
	if err != nil || got != "operation-1" {
		t.Fatalf("valid envelope = %q, error %v", got, err)
	}
	if planHash != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("valid plan hash = %q", planHash)
	}
	for _, raw := range []json.RawMessage{
		json.RawMessage(`not-json`),
		json.RawMessage(`{"operation_id":"operation-1"}`),
		json.RawMessage(`{"operation_id":"operation-1","plan_hash":"different"}`),
		json.RawMessage(`{"operation_id":"operation;1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
	} {
		if _, _, err := parseDeviceOperationEnvelope(raw); err == nil {
			t.Fatalf("invalid envelope was accepted: %s", raw)
		}
	}
}

func TestValidDeviceOperationStateUsesKnownVocabulary(t *testing.T) {
	for _, state := range []string{"APPLYING", "PENDING_CONFIRM", "ROLLING_BACK", "COMMITTED", "RESTORED", "RECOVERY_REQUIRED"} {
		if !validDeviceOperationState(state) {
			t.Errorf("known operation state %q was rejected", state)
		}
	}
	for _, state := range []string{"", "succeeded", "DROP TABLE devices"} {
		if validDeviceOperationState(state) {
			t.Errorf("unknown operation state %q was accepted", state)
		}
	}
}

func TestQueueDeviceOperationReservesAndReturnsGeneration(t *testing.T) {
	mock := mockEnrollmentDB(t)
	plan := json.RawMessage(`{"operation_id":"operation-1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]}`)
	boundPlan := `{"operation_id":"operation-1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","generation":42,"config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]}`
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.devices AS device")+`.*`+regexp.QuoteMeta("FROM tenant_demo.rollout_runs AS active_rollout")).
		WithArgs(sqlmock.AnyArg(), "device-1", "operation-1", int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_operation"}).AddRow(int64(42), boundPlan))

	generation, err := QueueDeviceOperation(t.Context(), "tenant_demo", "device-1", plan)
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

func TestQueueDeviceOperationAcceptsCurrentBoundGeneration(t *testing.T) {
	mock := mockEnrollmentDB(t)
	plan := json.RawMessage(`{"operation_id":"operation-2","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","generation":42,"config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]}`)
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.devices")).
		WithArgs(sqlmock.AnyArg(), "device-1", "operation-2", int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_operation"}).AddRow(int64(42), plan))

	generation, err := QueueDeviceOperation(t.Context(), "tenant_demo", "device-1", plan)
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

func TestQueueDeviceOperationReturnsStoredGenerationForIdempotentRetry(t *testing.T) {
	mock := mockEnrollmentDB(t)
	plan := json.RawMessage(`{"operation_id":"operation-4","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]}`)
	boundPlan := `{"operation_id":"operation-4","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","generation":42,"config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]}`
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.devices")).
		WithArgs(sqlmock.AnyArg(), "device-1", "operation-4", int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_operation"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT desired_generation, pending_operation, last_operation")).
		WithArgs("device-1").
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_operation", "last_operation"}).AddRow(int64(42), boundPlan, nil))

	generation, err := QueueDeviceOperation(t.Context(), "tenant_demo", "device-1", plan)
	if err != nil {
		t.Fatal(err)
	}
	if generation != 42 {
		t.Fatalf("retry generation = %d, want stored generation 42", generation)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQueueDeviceOperationRejectsStaleBoundGeneration(t *testing.T) {
	mock := mockEnrollmentDB(t)
	plan := json.RawMessage(`{"operation_id":"operation-3","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","generation":41,"config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]}`)
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.devices")).
		WithArgs(sqlmock.AnyArg(), "device-1", "operation-3", int64(41)).
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_operation"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT desired_generation, pending_operation, last_operation")).
		WithArgs("device-1").
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_operation", "last_operation"}).AddRow(int64(42), nil, nil))

	_, err := QueueDeviceOperation(t.Context(), "tenant_demo", "device-1", plan)
	if !errors.Is(err, ErrDeviceOperationGenerationConflict) {
		t.Fatalf("queue error = %v, want generation conflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordDeviceOperationStatusReportsUnmatchedOperation(t *testing.T) {
	mock := mockEnrollmentDB(t)
	status := json.RawMessage(`{"id":"operation-1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","generation":42,"state":"COMMITTED"}`)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.devices")).
		WithArgs(sqlmock.AnyArg(), true, "operation-1", "COMMITTED", "device-1", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "42", int64(42)).
		WillReturnResult(sqlmock.NewResult(1, 0))

	err := RecordDeviceOperationStatus(t.Context(), "tenant_demo", "device-1", status)
	if !errors.Is(err, ErrDeviceOperationStatusNotApplied) {
		t.Fatalf("status error = %v, want %v", err, ErrDeviceOperationStatusNotApplied)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordDeviceOperationStatusAcceptsLegacyStatusWithoutGeneration(t *testing.T) {
	mock := mockEnrollmentDB(t)
	status := json.RawMessage(`{"id":"legacy-operation","state":"COMMITTED"}`)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.devices")).
		WithArgs(sqlmock.AnyArg(), true, "legacy-operation", "COMMITTED", "device-1", "", "", int64(0)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := RecordDeviceOperationStatus(t.Context(), "tenant_demo", "device-1", status); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordDeviceOperationStatusAcceptsLegacyAgentForBoundOperation(t *testing.T) {
	mock := mockEnrollmentDB(t)
	status := json.RawMessage(`{"id":"bound-operation","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","state":"COMMITTED"}`)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.devices")).
		WithArgs(sqlmock.AnyArg(), true, "bound-operation", "COMMITTED", "device-1", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "", int64(0)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := RecordDeviceOperationStatus(t.Context(), "tenant_demo", "device-1", status); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordDeviceOperationStatusPreservesActiveRolloutStatus(t *testing.T) {
	mock := mockEnrollmentDB(t)
	status := json.RawMessage(`{"id":"operation-1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","generation":42,"state":"APPLYING"}`)
	mock.ExpectExec(`(?s)UPDATE tenant_demo\.devices.*last_rollout_status\s*=\s*CASE\s+WHEN\s+COALESCE\(last_rollout_status, ''\)\s*=\s*'RUNNING'\s+THEN\s+last_rollout_status\s+ELSE\s+\$4::text\s+END`).
		WithArgs(sqlmock.AnyArg(), false, "operation-1", "APPLYING", "device-1", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "42", int64(42)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := RecordDeviceOperationStatus(t.Context(), "tenant_demo", "device-1", status); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
