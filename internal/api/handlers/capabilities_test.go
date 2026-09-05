package handlers

import (
	"encoding/json"
	"testing"
)

func TestCapabilitiesPayloadRequiresObject(t *testing.T) {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(`{"openwrt_release":"23.05","architecture":"aarch64"}`), &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["architecture"].(string); !ok {
		t.Fatal("architecture should be a string capability")
	}
}
