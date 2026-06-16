package orchestrator

import "testing"

func TestEndpointRegexp(t *testing.T) {
	good := []string{
		"1.2.3.4",
		"10.0.0.1:51820",
		"203.0.113.42:51820",
		"255.255.255.255:65535",
		"127.0.0.1",
	}
	bad := []string{
		"",
		"example.com",
		"1.2.3",
		"1.2.3.4.5",
		"1.2.3.4; reboot",
		"$(rm -rf /)",
		"1.2.3.4:99999", // port out of range
		"1.2.3.4:0",     // port 0 invalid
		"abc.def.ghi.jkl",
		"256.1.1.1", // octet out of range
		"1.2.3.4:",  // empty port
		"1.2.3.4:abc",
		"-1.2.3.4",
	}
	for _, in := range good {
		if !endpointRegexp.MatchString(in) {
			t.Errorf("expected %q to match endpoint regex", in)
		}
	}
	for _, in := range bad {
		if endpointRegexp.MatchString(in) {
			t.Errorf("expected %q to NOT match endpoint regex", in)
		}
	}
}

func TestFingerprint_Nil(t *testing.T) {
	// Calling Fingerprint with a nil key must not panic; it should
	// return an empty string so the caller can skip the audit log entry
	// rather than crash.
	if got := Fingerprint(nil); got != "" {
		t.Errorf("Fingerprint(nil) = %q, want empty", got)
	}
}
