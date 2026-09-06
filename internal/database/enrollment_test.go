package database

import "testing"

func TestHashDeviceEnrollmentToken(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const expected = "a8ae6e6ee929abea3afcfc5258c8ccd6f85273e0d4626d26c7279f3250f77c8e"

	if got := HashDeviceEnrollmentToken(token); got != expected {
		t.Fatalf("HashDeviceEnrollmentToken() = %q, want %q", got, expected)
	}
	if HashDeviceEnrollmentToken(token) == HashDeviceEnrollmentToken(token+"x") {
		t.Fatal("different enrollment tokens produced the same hash")
	}
}
