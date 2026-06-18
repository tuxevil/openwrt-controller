package spa

import (
	"strings"
	"testing"
)

func TestStripForeignScriptsKeepsViteBundles(t *testing.T) {
	html := `<html><head>
		<script type="module" crossorigin src="/assets/index-AbCd1234.js"></script>
		<link rel="modulepreload" href="/assets/vue-router-XyZ.js">
	</head><body></body></html>`

	got := stripForeignScripts(html)
	if !strings.Contains(got, `/assets/index-AbCd1234.js`) {
		t.Errorf("expected the Vite bundle to be preserved; got: %s", got)
	}
	if !strings.Contains(got, `<link`) {
		t.Errorf("link tags should be untouched; got: %s", got)
	}
}

func TestStripForeignScriptsRemovesInjectedScripts(t *testing.T) {
	html := `<html><head>
		<script type="module" src="/assets/index-OK.js"></script>
	</head><body>
		<script src="/wrs_env.js"></script>
		<script src="/web-client-content-script.js"></script>
		<script src="https://evil.example.com/payload.js"></script>
		<script>console.log('inline injection');</script>
		<script>/* another inline */</script>
	</body></html>`

	got := stripForeignScripts(html)
	for _, bad := range []string{"/wrs_env.js", "web-client-content-script.js", "evil.example.com", "console.log", "inline injection", "another inline"} {
		if strings.Contains(got, bad) {
			t.Errorf("expected %q to be stripped; still present in:\n%s", bad, got)
		}
	}
	if !strings.Contains(got, "/assets/index-OK.js") {
		t.Errorf("legitimate bundle should be preserved; got: %s", got)
	}
}

func TestStripForeignScriptsIdempotent(t *testing.T) {
	html := `<html><head><script type="module" src="/assets/index-X.js"></script></head>
		<body><script src="/wrs_env.js"></script><script>alert(1)</script></body></html>`

	first := stripForeignScripts(html)
	second := stripForeignScripts(first)
	if first != second {
		t.Errorf("stripForeignScripts should be idempotent\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestStripForeignScriptsAllowsExternalAssetJS(t *testing.T) {
	// We accept any path under /assets/ that ends in .js — Vite names
	// them with content hashes, so this covers all chunks.
	cases := []struct {
		src      string
		keep     bool
	}{
		{`src="/assets/index-AbCd.js"`, true},
		{`src="/assets/vue-router-1234.js"`, true},
		{`src="/assets/leaflet-99.js"`, true},
		{`src="/assets/SurveyorPwa-AbCd.js"`, true},
		{`src="/wrs_env.js"`, false},
		{`src="/wrs_env"`, false},
		{`src="https://coolify.example/wrs_env.js"`, false},
		{`src=""`, false},
	}
	for _, c := range cases {
		html := `<html><body><script ` + c.src + `></script></body></html>`
		got := stripForeignScripts(html)
		hasScript := strings.Contains(got, "<script")
		if c.keep && !hasScript {
			t.Errorf("src=%q should be kept; got: %s", c.src, got)
		}
		if !c.keep && hasScript {
			t.Errorf("src=%q should be stripped; got: %s", c.src, got)
		}
	}
}
