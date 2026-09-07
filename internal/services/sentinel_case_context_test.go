package services

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"openwrt-controller/internal/database"
)

func TestSentinelContextToolPlanIsDomainBounded(t *testing.T) {
	tests := []struct {
		name  string
		query string
		item  SentinelCase
		want  []string
	}{
		{
			name:  "hardware",
			query: "What hardware and firmware does this router have?",
			item:  SentinelCase{},
			want:  []string{"get_device_status"},
		},
		{
			name:  "clients",
			query: "How many clients are connected?",
			item:  SentinelCase{},
			want:  []string{"get_site_clients"},
		},
		{
			name:  "connectivity",
			query: "Why is WAN connectivity offline?",
			item:  SentinelCase{},
			want:  []string{"get_device_status", "get_incidents", "get_recent_changes"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sentinelContextToolPlan(tt.query, tt.item)
			if len(got) != len(tt.want) {
				t.Fatalf("tool plan = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("tool plan = %v, want %v", got, tt.want)
				}
			}
		}
	}
}

func TestSentinelCaseScopeInjectionOverridesModelScope(t *testing.T) {
	call := SentinelToolCall{
		Name:      "get_device_status",
		Arguments: json.RawMessage(`{"site_id":"site-other","device_id":"device-other","limit":5}`),
	}
	compiled := SentinelCompiledContext{SiteID: "site-case", DeviceID: "device-case"}
	args := sentinelToolArgumentsForCaseCall(call, "tenant_demo", compiled)
	var decoded map[string]interface{}
	if err := json.Unmarshal(args, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["schema"] != "tenant_demo" || decoded["site_id"] != "site-case" || decoded["device_id"] != "device-case" {
		t.Fatalf("Case scope was not enforced: %#v", decoded)
	}
}

func TestSentinelCaseProposalCannotEscapeDeviceScope(t *testing.T) {
	compiled := SentinelCompiledContext{DeviceID: "gw-1"}
	proposal := &SentinelProposalDraft{DeviceID: "ap-2"}
	if err := sentinelCaseAllowsProposal("tenant_demo", compiled, proposal); err == nil {
		t.Fatal("proposal outside Case device scope should be rejected")
	}
}

func TestDecodeSentinelCaseEvidenceLedgerPreservesLegacyEvidence(t *testing.T) {
	legacy := json.RawMessage(`{"source_ip":"192.0.2.4","message":"bad password"}`)
	ledger := decodeSentinelCaseEvidenceLedger(legacy)
	if ledger.Version != sentinelCaseEvidenceLedgerVersion || len(ledger.Runs) != 0 {
		t.Fatalf("unexpected ledger: %#v", ledger)
	}
	if string(ledger.Origin) != string(legacy) {
		t.Fatalf("legacy origin = %s, want %s", ledger.Origin, legacy)
	}
}

func TestCompileSentinelCaseContextLabelsDynamicData(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previous
		_ = db.Close()
	})

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id::text, fingerprint, source, COALESCE(site_id::text,''),
        COALESCE(device_id,''), severity, status, title, summary, evidence, created_at, updated_at, resolved_at
        FROM tenant_demo.sentinel_cases WHERE id = $1`)).
		WithArgs("case-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "fingerprint", "source", "site_id", "device_id", "severity", "status", "title", "summary", "evidence", "created_at", "updated_at", "resolved_at",
		}).AddRow("case-1", "operator_chat:conv-1", "operator_chat", "site-1", "", "INFO", "OPEN", "General review", "previous model summary", []byte(`{"version":1,"runs":[]}`), now, now, nil))

	compiled, prefetched, err := CompileSentinelCaseContext(t.Context(), "tenant_demo", "case-1", []SentinelStoredMessage{
		{Role: "user", Content: "the WAN seems slow"},
		{Role: "assistant", Content: "maybe DNS is failing"},
	}, "summarize this case", NewSentinelToolBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(prefetched) != 0 {
		t.Fatalf("unexpected prefetch for generic summary: %#v", prefetched)
	}

	seenScope, seenOperator, seenAssistant, seenDescription := false, false, false, false
	for _, item := range compiled.Items {
		switch item.Kind {
		case "case_scope":
			seenScope = item.TrustClass == SentinelContextAuthoritativeState
		case "case_description":
			seenDescription = item.TrustClass == SentinelContextUntrustedObservation
		case "conversation_message":
			var payload map[string]string
			_ = json.Unmarshal(item.Data, &payload)
			if payload["role"] == "user" {
				seenOperator = item.TrustClass == SentinelContextOperatorAssertion
			}
			if payload["role"] == "assistant" {
				seenAssistant = item.TrustClass == SentinelContextUntrustedObservation
			}
		}
	}
	if !seenScope || !seenDescription || !seenOperator || !seenAssistant {
		t.Fatalf("trust labels incomplete: scope=%v description=%v operator=%v assistant=%v", seenScope, seenDescription, seenOperator, seenAssistant)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPersistSentinelCaseEvidenceAppendsStableReferences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previous
		_ = db.Close()
	})

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT evidence FROM tenant_demo.sentinel_cases WHERE id = $1 FOR UPDATE")).
		WithArgs("case-1").
		WillReturnRows(sqlmock.NewRows([]string{"evidence"}).AddRow([]byte(`{"version":1,"runs":[]}`)))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.sentinel_cases SET evidence = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2")).
		WithArgs(sqlmock.AnyArg(), "case-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	refs, err := PersistSentinelCaseEvidence("tenant_demo", "case-1", "run-1", "operator_chat", []SentinelEvidence{
		{Tool: "get_device_status", Args: json.RawMessage(`{"device_id":"gw-1"}`), Result: json.RawMessage(`[{"id":"gw-1"}]`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ID == "" || refs[0].TrustClass != SentinelContextMeasurement {
		t.Fatalf("unexpected evidence refs: %#v", refs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSentinelRunViewSurfacesCaseAttachment(t *testing.T) {
	run := SentinelRun{ID: "run-1", Evidence: json.RawMessage(`{"case_id":"case-1","evidence_refs":[]}`)}
	view := SentinelRunViewFor(run)
	if view.ID != "run-1" || view.CaseID != "case-1" {
		t.Fatalf("run view = %#v", view)
	}
}
