package services

import (
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	env, err := Seal("super-secret-telegram-token", key)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	got, err := Open(env, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got != "super-secret-telegram-token" {
		t.Errorf("Open() = %q, want %q", got, "super-secret-telegram-token")
	}
}

func TestOpen_WrongKeyFails(t *testing.T) {
	k1 := make([]byte, 32)
	k2 := make([]byte, 32)
	for i := range k2 {
		k2[i] = byte(i + 1)
	}
	env, err := Seal("payload", k1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(env, k2); err == nil {
		t.Error("expected Open with wrong key to fail authentication")
	}
}

func TestDeriveKeyFromPassphrase(t *testing.T) {
	salt := []byte("per-record-random-salt")
	k1 := DeriveKeyFromPassphrase("hunter2", salt)
	if len(k1) != 32 {
		t.Errorf("DeriveKeyFromPassphrase returned %d bytes, want 32", len(k1))
	}
	k2 := DeriveKeyFromPassphrase("hunter2", salt)
	if string(k1) != string(k2) {
		t.Error("derivation is not deterministic")
	}
	k3 := DeriveKeyFromPassphrase("hunter3", salt)
	if string(k1) == string(k3) {
		t.Error("different passphrases produced the same key")
	}
}

func TestSealOpenWithPassphraseUsesRandomSalt(t *testing.T) {
	env1, err := SealWithPassphrase("payload", "strong passphrase")
	if err != nil {
		t.Fatalf("SealWithPassphrase: %v", err)
	}
	env2, err := SealWithPassphrase("payload", "strong passphrase")
	if err != nil {
		t.Fatalf("second SealWithPassphrase: %v", err)
	}
	if string(env1.FullBlob) == string(env2.FullBlob) {
		t.Fatal("passphrase envelopes reused the same salt/nonce")
	}

	got, err := OpenWithPassphrase(env1, "strong passphrase")
	if err != nil {
		t.Fatalf("OpenWithPassphrase: %v", err)
	}
	if got != "payload" {
		t.Fatalf("OpenWithPassphrase = %q, want payload", got)
	}
	if _, err := OpenWithPassphrase(env1, "wrong passphrase"); err == nil {
		t.Fatal("wrong passphrase unexpectedly decrypted the envelope")
	}
}

func TestSealKeyLength(t *testing.T) {
	short := make([]byte, 16)
	if _, err := Seal("x", short); err == nil {
		t.Error("expected error for 16-byte key")
	}
	long := make([]byte, 64)
	if _, err := Seal("x", long); err == nil {
		t.Error("expected error for 64-byte key")
	}
}
