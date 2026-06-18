package handlers

import (
	"crypto/tls"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestSurveyBaseURLOverride ensures the SURVEY_PUBLIC_URL env var
// forces the QR base URL to a known value, ignoring whatever the
// inbound Host header says. This is the escape hatch used when the
// controller is reached through a reverse proxy (e.g. Coolify) that
// injects scripts or serves a hostname the phone can't reach.
func TestSurveyBaseURLOverride(t *testing.T) {
	const override = "https://10.128.128.6:3000"
	t.Setenv("SURVEY_PUBLIC_URL", override)
	defer os.Unsetenv("SURVEY_PUBLIC_URL")

	tests := []struct {
		name   string
		host   string
		tls    bool
		xfp    string
		expect string
	}{
		{"override beats plain host", "controller.local:3000", false, "", override},
		{"override beats xfp", "controller.local", true, "https", override},
		{"override beats loopback host", "localhost:3000", false, "", override},
		{"override beats another port", "1.2.3.4:8443", true, "https", override},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "https://"+c.host+"/x", nil)
			if c.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if c.xfp != "" {
				r.Header.Set("X-Forwarded-Proto", c.xfp)
			}
			got := surveyBaseURL(r)
			if got != c.expect {
				t.Errorf("surveyBaseURL() = %q, want %q", got, c.expect)
			}
		})
	}
}

// TestSurveyBaseURLFallback verifies the default path: when the env
// var is unset, the function falls back to r.Host + scheme, honouring
// X-Forwarded-Proto for TLS-terminating proxies.
func TestSurveyBaseURLFallback(t *testing.T) {
	os.Unsetenv("SURVEY_PUBLIC_URL")
	tests := []struct {
		name   string
		host   string
		tls    bool
		xfp    string
		expect string
	}{
		{"plain http", "10.0.0.1:3000", false, "", "http://10.0.0.1:3000"},
		{"https direct", "10.0.0.1:3000", true, "", "https://10.0.0.1:3000"},
		{"http + xfp https", "controller.example", false, "https", "https://controller.example"},
		{"https + xfp http (xfp wins)", "controller.example", true, "http", "http://controller.example"},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			scheme := "https"
			if !c.tls && c.xfp == "" {
				scheme = "http"
			}
			r := httptest.NewRequest("GET", scheme+"://"+c.host+"/x", nil)
			if c.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if c.xfp != "" {
				r.Header.Set("X-Forwarded-Proto", c.xfp)
			}
			got := surveyBaseURL(r)
			if !strings.HasPrefix(got, c.expect) {
				t.Errorf("surveyBaseURL() = %q, want prefix %q", got, c.expect)
			}
		})
	}
}
