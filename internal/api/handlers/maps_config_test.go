package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetMapsConfigHandler(t *testing.T) {
	t.Run("returns configured CARTO API key", func(t *testing.T) {
		t.Setenv("CARTO_API_KEY", "  cb1_test_key_123  ")

		req := httptest.NewRequest(http.MethodGet, "/api/maps/config", nil)
		rr := httptest.NewRecorder()
		GetMapsConfigHandler(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		var body map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if body["carto_api_key"] != "cb1_test_key_123" {
			t.Fatalf("expected trimmed key, got %q", body["carto_api_key"])
		}
	})

	t.Run("returns empty string when unset", func(t *testing.T) {
		t.Setenv("CARTO_API_KEY", "")

		req := httptest.NewRequest(http.MethodGet, "/api/maps/config", nil)
		rr := httptest.NewRecorder()
		GetMapsConfigHandler(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		var body map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if body["carto_api_key"] != "" {
			t.Fatalf("expected empty key, got %q", body["carto_api_key"])
		}
	})
}
