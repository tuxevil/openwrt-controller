package handlers

import (
	"strings"
	"testing"

	"openwrt-controller/internal/services"
)

func TestRenderPortalAuthHTMLEscapesUntrustedValues(t *testing.T) {
	html, err := renderPortalAuthHTML(&services.PortalSettings{
		WelcomeText: `<img src=x onerror=alert(1)>`,
		TermsText:   `<script>alert(2)</script>`,
		BgColor:     `#123456`,
	}, "site-1", `<script>alert(4)</script>`)
	if err != nil {
		t.Fatalf("renderPortalAuthHTML: %v", err)
	}

	if strings.Contains(html, "<script>") || strings.Contains(html, "<img") {
		t.Fatalf("rendered portal contains executable markup:\n%s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") || !strings.Contains(html, "&lt;img") {
		t.Fatalf("rendered portal did not HTML-escape untrusted values:\n%s", html)
	}
}
