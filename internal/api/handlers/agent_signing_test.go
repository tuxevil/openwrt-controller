package handlers

import (
	"crypto/ed25519"
	"encoding/base64"
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
