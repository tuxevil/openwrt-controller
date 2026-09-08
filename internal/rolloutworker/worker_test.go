package rolloutworker

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func TestQueueNextPhaseResumesAfterPersistedCursor(t *testing.T) {
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

	plan := rolloutPlan{
		Namespace: "system",
		Devices: []rolloutDevice{
			{DeviceID: "device-1", Role: "AP", Commands: nil, ObservedState: map[string]string{"system": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
			{DeviceID: "device-2", Role: "AP", Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "next"}}, ObservedState: map[string]string{"system": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
		},
	}
	planRaw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	resultsRaw := []byte(`[{"device_id":"device-1","status":"SUCCESS"},{"device_id":"device-2","status":"WAITING"}]`)
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.devices AS device")).
		WillReturnRows(sqlmock.NewRows([]string{"desired_generation", "pending_change_set"}).
			AddRow(int64(2), []byte(`{"change_set_id":"next"}`)))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.rollout_runs")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	progress := database.RolloutProgress{Plan: planRaw, Results: resultsRaw}
	lease := database.RolloutLease{RolloutID: "rollout-1", SiteID: "site-1", Token: "worker-token", Cursor: 1}
	if err := (Worker{}).queueNextPhase(context.Background(), "tenant_demo", lease, progress); err != nil {
		t.Fatalf("queueNextPhase returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
