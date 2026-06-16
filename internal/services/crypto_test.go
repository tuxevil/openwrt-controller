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
	k1 := DeriveKeyFromPassphrase("hunter2")
	if len(k1) != 32 {
		t.Errorf("DeriveKeyFromPassphrase returned %d bytes, want 32", len(k1))
	}
	k2 := DeriveKeyFromPassphrase("hunter2")
	if string(k1) != string(k2) {
		t.Error("derivation is not deterministic")
	}
	k3 := DeriveKeyFromPassphrase("hunter3")
	if string(k1) == string(k3) {
		t.Error("different passphrases produced the same key")
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
