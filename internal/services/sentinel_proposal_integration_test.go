package services

import (
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"

	"openwrt-controller/internal/database"
)

func TestSentinelProposalApprovalBoundaryPostgres(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL_TEST")
	if os.Getenv("OPENWRT_INTEGRATION_DB") != "1" || dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previous := database.DB
	database.DB = db
	defer func() { database.DB = previous }()
	schema := "tenant_proposal_" + uuid.NewString()[:8]
	quoted, err := database.SafeSQLSchemaIdent(schema)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA IF EXISTS " + quoted + " CASCADE")
	if err := database.RunTenantMigrations(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO " + quoted + ".devices (id) VALUES ('sandbox-node')"); err != nil {
		t.Fatal(err)
	}
	draft := SentinelProposalDraft{
		DeviceID: "sandbox-node", Config: "system", Summary: "Lab hostname",
		Commands:      []UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "sentinel-lab"}},
		BlockedReason: "device outside Case scope",
	}
	blocked, err := CreateSentinelProposal(schema, "", "", "synthetic", &draft)
	if err != nil || blocked.Status != "BLOCKED" {
		t.Fatalf("blocked proposal status: %s (%v)", blocked.Status, err)
	}

	draft.BlockedReason = ""
	pending, err := CreateSentinelProposal(schema, "", "", "synthetic", &draft)
	if err != nil || pending.Status != "PENDING" {
		t.Fatalf("valid proposal status: %s (%v)", pending.Status, err)
	}
	if _, err := db.Exec("UPDATE "+quoted+".sentinel_proposals SET status='PENDING', plan=$2 WHERE id=$1", blocked.ID, []byte(pending.Plan)); err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveSentinelProposal(t.Context(), schema, blocked.ID, "operator"); err == nil {
		t.Fatal("historical pending proposal with a block reason was approved")
	}

	var queued bool
	if err := db.QueryRow("SELECT pending_operation IS NOT NULL FROM " + quoted + ".devices WHERE id='sandbox-node'").Scan(&queued); err != nil || queued {
		t.Fatalf("operation queued before approval: %t (%v)", queued, err)
	}
	operation, err := ApproveSentinelProposal(t.Context(), schema, pending.ID, "operator")
	if err != nil || operation.Generation == 0 || operation.OperationID == "" || operation.PlanHash == "" {
		t.Fatalf("could not approve valid operation: %v", err)
	}
	var operationID string
	if err := db.QueryRow("SELECT pending_operation->>'operation_id' FROM " + quoted + ".devices WHERE id='sandbox-node'").Scan(&operationID); err != nil ||
		operationID != operation.OperationID {
		t.Fatalf("queued operation identity mismatch: %s (%v)", operationID, err)
	}
}
