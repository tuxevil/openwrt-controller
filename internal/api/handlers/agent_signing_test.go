package handlers

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentSignatureVerification(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	content := "#!/bin/sh\necho agent"
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(content)))
	if !verifyAgentSignature(content, signature, publicKey) {
		t.Fatal("valid signature was rejected")
	}
	if verifyAgentSignature(content+" modified", signature, publicKey) {
		t.Fatal("modified content was accepted")
	}
	if verifyAgentSignature(content, "not-base64", publicKey) {
		t.Fatal("invalid signature was accepted")
	}
}

func TestAgentSigningFailsClosedWithoutKey(t *testing.T) {
	t.Setenv("AGENT_UPDATE_SIGNING_KEY", "")

	if _, _, err := signAgentContent("agent"); err == nil {
		t.Fatal("unsigned agent content was accepted without a signing key")
	}
}

func TestAgentUpdatesRejectSiteKeyOnlyClients(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/agent/latest", nil)
	req.Header.Set("X-Site-Key", "legacy-site-key")
	res := httptest.NewRecorder()

	GetLatestAgentHandler(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d", res.Code, http.StatusForbidden)
	}
}
