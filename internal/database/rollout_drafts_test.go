package database

import (
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateRolloutDraftReservesIndependentSiteRolloutSequence(t *testing.T) {
	mock := mockEnrollmentDB(t)
	targets := json.RawMessage(`["device-1"]`)
	plan := json.RawMessage(`{"site_id":"site-1","devices":[]}`)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(hashtextextended($1, 0))")).
		WithArgs("site-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(generation\), 0\) \+ 1\s+FROM tenant_demo\.rollout_runs\s+WHERE site_id = \$1`).
		WithArgs("site-1").
		WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(int64(7)))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.rollout_runs SET status = 'STALE'")).
		WithArgs("site-1", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO tenant_demo.rollout_runs")).
		WithArgs("site-1", int64(7), "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "operator", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("rollout-1"))
	mock.ExpectCommit()

	record, err := CreateRolloutDraft(t.Context(), "tenant_demo", "site-1", "operator", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", targets, plan)
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != "rollout-1" || record.Generation != 7 || record.Status != "DRAFT" {
		t.Fatalf("draft record = %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetRolloutDraftLoadsImmutablePlan(t *testing.T) {
	mock := mockEnrollmentDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id::text, site_id::text, generation, status, claim_token::text, plan_hash, requested_by, target_device_ids, plan, results FROM tenant_demo.rollout_runs WHERE id = $1 AND site_id = $2")).
		WithArgs("rollout-1", "site-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "site_id", "generation", "status", "claim_token", "plan_hash", "requested_by", "target_device_ids", "plan", "results"}).
			AddRow("rollout-1", "site-1", int64(7), "DRAFT", nil, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "operator", []byte(`["device-1"]`), []byte(`{"site_id":"site-1"}`), []byte(`[]`)))

	record, err := GetRolloutDraft(t.Context(), "tenant_demo", "site-1", "rollout-1")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "DRAFT" || string(record.Plan) != `{"site_id":"site-1"}` {
		t.Fatalf("draft record = %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimRolloutDraftSerializesSiteClaims(t *testing.T) {
	mock := mockEnrollmentDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(hashtextextended($1, 0))")).
		WithArgs("site-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("WITH expired AS")).
		WithArgs("rollout-1", "site-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "site_id", "generation", "status", "claim_token", "plan_hash", "requested_by", "target_device_ids", "plan", "results"}).
			AddRow("rollout-1", "site-1", int64(7), "RUNNING", "claim-token", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "operator", []byte(`["device-1"]`), []byte(`{"site_id":"site-1"}`), []byte(`[]`)))
	mock.ExpectCommit()

	record, err := ClaimRolloutDraft(t.Context(), "tenant_demo", "site-1", "rollout-1")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "RUNNING" || record.Generation != 7 || record.ClaimToken != "claim-token" {
		t.Fatalf("claimed draft record = %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimRolloutDraftUsesAnExecutionLease(t *testing.T) {
	mock := mockEnrollmentDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(hashtextextended($1, 0))")).
		WithArgs("site-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("updated_at < CURRENT_TIMESTAMP - INTERVAL '20 minutes'")).WithArgs("rollout-1", "site-1").WillReturnRows(sqlmock.NewRows([]string{"id", "site_id", "generation", "status", "claim_token", "plan_hash", "requested_by", "target_device_ids", "plan", "results"}).AddRow("rollout-1", "site-1", int64(7), "RUNNING", "claim-token", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "operator", []byte(`["device-1"]`), []byte(`{"site_id":"site-1"}`), []byte(`[]`)))
	mock.ExpectCommit()

	if _, err := ClaimRolloutDraft(t.Context(), "tenant_demo", "site-1", "rollout-1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimAndQueueDeviceChangeSetCommitsClaimAndQueueTogether(t *testing.T) {
	mock := mockEnrollmentDB(t)
	changeSetID := "cs-test-1"
	changeSetRaw := json.RawMessage(`{"change_set_id":"cs-test-1","device_id":"device-1","plan_hash":"aa034f6d8e438065932060bef117be1500717ca0798047a449344dc90025b21f","operations":[{"operation_id":"operation-test-1","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}],"observed_state_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}],"confirmation_policy":"local_auto"}`)
	queuedResult := json.RawMessage(`[{"device_id":"device-1","status":"QUEUED"}]`)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(hashtextextended($1, 0))")).
		WithArgs("site-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("WITH expired AS")).
		WithArgs("rollout-1", "site-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "site_id", "generation", "status", "claim_token", "plan_hash", "requested_by", "target_device_ids", "plan", "results"}).
			AddRow("rollout-1", "site-1", int64(7), "RUNNING", "claim-token", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "operator", []byte(`["device-1"]`), []byte(`{"site_id":"site-1"}`), []byte(`[]`)))
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.devices AS device")).
		WithArgs(changeSetRaw, "device-1", changeSetID, int64(0), "site-1", "rollout-1", "AP").
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_change_set"}).AddRow(int64(42), changeSetRaw))
	mock.ExpectExec(`UPDATE tenant_demo\.rollout_runs\s+SET status = 'QUEUED'`).
		WithArgs(queuedResult, "rollout-1", "site-1", "claim-token", int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_logs")).
		WithArgs("operator", "SITE_ORCHESTRATOR_ROLLOUT_START", "SITE", "site-1", sqlmock.AnyArg(), "127.0.0.1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_logs")).
		WithArgs("operator", "SITE_ORCHESTRATOR_CHANGESET_QUEUED", "SITE", "site-1", sqlmock.AnyArg(), "127.0.0.1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	generation, err := ClaimAndQueueDeviceChangeSet(t.Context(), "tenant_demo", "site-1", "rollout-1", "AP", changeSetRaw, queuedResult, "operator", "127.0.0.1")
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

func TestRejectRolloutDraftPersistsUnsupportedReason(t *testing.T) {
	mock := mockEnrollmentDB(t)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.rollout_runs")).
		WithArgs("rollout-1", "site-1", "device does not support changesets").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := RejectRolloutDraft(t.Context(), "tenant_demo", "site-1", "rollout-1", "device does not support changesets"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimAndQueueDeviceChangeSetRollsBackWhenQueueFails(t *testing.T) {
	mock := mockEnrollmentDB(t)
	changeSetRaw := json.RawMessage(`{"change_set_id":"cs-test-1","device_id":"device-1","plan_hash":"aa034f6d8e438065932060bef117be1500717ca0798047a449344dc90025b21f","operations":[{"operation_id":"operation-test-1","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}],"observed_state_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}],"confirmation_policy":"local_auto"}`)
	queuedResult := json.RawMessage(`[{"device_id":"device-1","status":"QUEUED"}]`)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(hashtextextended($1, 0))")).
		WithArgs("site-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("WITH expired AS")).
		WithArgs("rollout-1", "site-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "site_id", "generation", "status", "claim_token", "plan_hash", "requested_by", "target_device_ids", "plan", "results"}).
			AddRow("rollout-1", "site-1", int64(7), "RUNNING", "claim-token", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "operator", []byte(`[]`), []byte(`{}`), []byte(`[]`)))
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.devices AS device")).
		WithArgs(changeSetRaw, "device-1", "cs-test-1", int64(0), "site-1", "rollout-1", "AP").
		WillReturnError(errors.New("queue failed"))
	mock.ExpectRollback()

	if _, err := ClaimAndQueueDeviceChangeSet(t.Context(), "tenant_demo", "site-1", "rollout-1", "AP", changeSetRaw, queuedResult, "operator", "127.0.0.1"); err == nil {
		t.Fatal("queue failure was accepted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMarkRolloutDraftStaleIsConditional(t *testing.T) {
	mock := mockEnrollmentDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM tenant_demo.rollout_runs")).
		WithArgs("rollout-1", "site-1", "claim-token").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("RUNNING"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.rollout_runs SET status = 'STALE'")).
		WithArgs("rollout-1", "site-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.devices")).
		WithArgs("site-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := MarkRolloutDraftStale(t.Context(), "tenant_demo", "site-1", "rollout-1", "claim-token"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseRolloutDraftReturnsTransportFailuresToDraft(t *testing.T) {
	mock := mockEnrollmentDB(t)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.rollout_runs SET status = 'DRAFT'")).
		WithArgs("rollout-1", "site-1", "claim-token").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := ReleaseRolloutDraft(t.Context(), "tenant_demo", "site-1", "rollout-1", "claim-token"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
