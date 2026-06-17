package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatusClass(t *testing.T) {
	cases := map[int]string{
		100: "error",  // outside the defined ranges → "error"
		200: "2xx",
		204: "2xx",
		301: "3xx",
		404: "4xx",
		500: "5xx",
		599: "5xx",
		600: "error",
	}
	for code, want := range cases {
		if got := statusClass(code); got != want {
			t.Errorf("statusClass(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestMiddleware_Records(t *testing.T) {
	reg := New()
	defer reg.Handler() // smoke: the handler must construct without panicking

	mw := reg.Middleware(func(r *http.Request) string {
		return "/test"
	})

	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "ok")
	}))

	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Fatal("downstream handler was not invoked")
	}
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rr.Code)
	}
	if got := rr.Body.String(); !strings.Contains(got, "ok") {
		t.Fatalf("expected body to contain %q, got %q", "ok", got)
	}
}

func TestNew_RegistersRuntimeCollectors(t *testing.T) {
	reg := New()
	// Gather must not panic and must report at least one metric
	// (the go_* collectors are always present).
	mfs, err := reg.Reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}
	if len(mfs) == 0 {
		t.Fatal("expected at least one metric family, got 0")
	}
}

func TestSetVersion(t *testing.T) {
	reg := New()
	reg.SetVersion("1.2.3-test")

	// The BuildInfo gauge should now have a series with version=1.2.3-test.
	mfs, err := reg.Reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "openwrt_controller_build_info" {
			for _, m := range mf.GetMetric() {
				for _, lp := range m.GetLabel() {
					if lp.GetName() == "version" && lp.GetValue() == "1.2.3-test" {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("expected BuildInfo gauge to have a series with version=1.2.3-test")
	}
}
