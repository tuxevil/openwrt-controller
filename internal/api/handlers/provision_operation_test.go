package handlers

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func TestResolveDeviceIdentityMatchesStoredCase(t *testing.T) {
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

	for _, requestedID := range []string{"04:A1:51:96:A6:4D", "04:a1:51:96:a6:4d"} {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, device_token FROM tenant_demo.devices WHERE LOWER(id) = LOWER($1)")).
			WithArgs(requestedID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "device_token"}).AddRow("04:a1:51:96:a6:4d", "device-token"))

		storedID, storedToken, err := resolveDeviceIdentity(context.Background(), "tenant_demo", requestedID)
		if err != nil {
			t.Fatal(err)
		}
		if storedID != "04:a1:51:96:a6:4d" || !storedToken.Valid || storedToken.String != "device-token" {
			t.Fatalf("requested %q resolved identity = %q, %v; want stored lowercase ID and token", requestedID, storedID, storedToken)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResolveDeviceIdentityByTokenMatchesStoredIdentity(t *testing.T) {
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

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, device_token FROM tenant_demo.devices WHERE device_token = $1")).
		WithArgs("device-token").
		WillReturnRows(sqlmock.NewRows([]string{"id", "device_token"}).AddRow("e8:9f:80:14:69:c5", "device-token"))

	storedID, storedToken, err := resolveDeviceIdentityByToken(context.Background(), "tenant_demo", "device-token")
	if err != nil {
		t.Fatal(err)
	}
	if storedID != "e8:9f:80:14:69:c5" || !storedToken.Valid || storedToken.String != "device-token" {
		t.Fatalf("resolved identity = %q, %v; want token-owned identity", storedID, storedToken)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceConfigResponseKeepsLegacyTokenLocation(t *testing.T) {
	config := map[string]interface{}{"wireless": map[string]interface{}{"wlans": []string{"lab"}}}
	legacyResponse := deviceConfigResponse(config, "device-token", true)
	if legacyResponse["device_token"] != "device-token" {
		t.Fatalf("legacy response token = %#v, want root-level token", legacyResponse["device_token"])
	}
	legacyConfig, ok := legacyResponse["config"].(map[string]interface{})
	if !ok || legacyConfig["wireless"] == nil {
		t.Fatal("legacy response changed the config payload")
	}

	strictResponse := deviceConfigResponse(config, "device-token", false)
	if _, ok := strictResponse["device_token"]; ok {
		t.Fatal("strict response unexpectedly exposed the legacy root-level token")
	}
}

func TestResolveDeviceIdentityByIPMatchesLegacyBridgeIdentity(t *testing.T) {
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

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, device_token FROM tenant_demo.devices WHERE site_id IS NOT NULL AND last_ip = $1 ORDER BY last_seen_at DESC NULLS LAST LIMIT 1")).
		WithArgs("10.128.128.1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "device_token"}).AddRow("e8:9f:80:14:69:c5", "device-token"))

	storedID, storedToken, err := resolveDeviceIdentityByIP(context.Background(), "tenant_demo", "10.128.128.1")
	if err != nil {
		t.Fatal(err)
	}
	if storedID != "e8:9f:80:14:69:c5" || !storedToken.Valid || storedToken.String != "device-token" {
		t.Fatalf("resolved identity = %q, %v; want legacy bridge identity", storedID, storedToken)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDecodePendingDeviceOperationAcceptsTypedPlan(t *testing.T) {
	raw := `{"operation_id":"operation-1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]}`
	plan, err := decodePendingDeviceOperation([]byte(raw))
	if err != nil {
		t.Fatalf("valid operation was rejected: %v", err)
	}
	if plan.OperationID != "operation-1" || plan.Config != "system" {
		t.Fatalf("unexpected decoded plan: %#v", plan)
	}
}

func TestDecodePendingDeviceOperationAllowsRetryIdentityToDifferFromPlanHash(t *testing.T) {
	raw := `{"operation_id":"rollout-147-router3-attempt1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]}`
	plan, err := decodePendingDeviceOperation([]byte(raw))
	if err != nil {
		t.Fatalf("distinct operation identity was rejected: %v", err)
	}
	if plan.OperationID == plan.PlanHash {
		t.Fatal("operation identity was collapsed into plan hash")
	}
}

func TestDecodePendingDeviceOperationRejectsInvalidPlans(t *testing.T) {
	cases := []string{
		"not-json",
		`{"operation_id":"operation-1","plan_hash":"different","config":"system","commands":[]}`,
		`{"operation_id":"operation-1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","config":"openvpn","commands":[{"action":"set","config":"openvpn","section":"client","option":"enabled","value":"1"}]}`,
		`{"operation_id":"operation;1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]}`,
	}
	for _, raw := range cases {
		if _, err := decodePendingDeviceOperation([]byte(raw)); err == nil {
			t.Fatalf("invalid operation was accepted: %s", raw)
		}
	}
}

func TestDecodePendingDeviceOperationPreservesHealthChecks(t *testing.T) {
	raw := `{"operation_id":"network-1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","config":"network","health_checks":["10.128.128.1"],"commands":[{"action":"set","config":"network","section":"lan","option":"metric","value":"10"}]}`
	plan, err := decodePendingDeviceOperation([]byte(raw))
	if err != nil {
		t.Fatalf("network operation was rejected: %v", err)
	}
	if got := strings.Join(plan.HealthChecks, ","); got != "10.128.128.1" {
		t.Fatalf("health checks changed during decode: %q", got)
	}
	if err := services.ValidateDeviceOperation(plan.Config, plan.Commands, plan.HealthChecks); err != nil {
		t.Fatalf("decoded plan no longer validates: %v", err)
	}
}

func TestDecodePendingDeviceChangeSetAcceptsSafePlan(t *testing.T) {
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
	changeSet.Generation = 42
	raw, err := json.Marshal(changeSet)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodePendingDeviceChangeSet(raw)
	if err != nil {
		t.Fatalf("safe changeset was rejected: %v", err)
	}
	if decoded.ChangeSetID != changeSet.ChangeSetID || decoded.Operations[0].Config != "system" {
		t.Fatalf("decoded changeset = %#v", decoded)
	}
	if err := validatePendingDeviceChangeSetForDevice(decoded, "device-1"); err != nil {
		t.Fatalf("valid changeset identity was rejected: %v", err)
	}
}

func TestValidatePendingDeviceChangeSetForDeviceRejectsUnreservedOrMismatchedPlans(t *testing.T) {
	changeSet := services.DeviceChangeSet{DeviceID: "device-1", Generation: 0}
	if err := validatePendingDeviceChangeSetForDevice(changeSet, "device-1"); err == nil {
		t.Fatal("unreserved changeset was accepted for delivery")
	}
	changeSet.Generation = 42
	if err := validatePendingDeviceChangeSetForDevice(changeSet, "device-2"); err == nil {
		t.Fatal("cross-device changeset was accepted for delivery")
	}
}

func TestDeviceConfigResponseIncludesPendingChangeSet(t *testing.T) {
	changeSet := map[string]interface{}{
		"change_set_id": "cs-1",
		"device_id":     "device-1",
		"plan_hash":     strings.Repeat("a", 64),
		"generation":    42,
	}
	response := deviceConfigResponse(map[string]interface{}{"apply_change_set": changeSet}, "device-token", false)
	config, ok := response["config"].(map[string]interface{})
	if !ok {
		t.Fatal("response config was not an object")
	}
	if got, ok := config["apply_change_set"].(map[string]interface{}); !ok || got["change_set_id"] != "cs-1" {
		t.Fatalf("pending changeset missing from config response: %#v", config)
	}
}
