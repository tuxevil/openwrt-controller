package handlers

import (
	"testing"
)

func TestRoleAtLeast(t *testing.T) {
	cases := []struct {
		role string
		min  string
		want bool
	}{
		{"VIEWER", "OPERATOR", false},
		{"VIEWER", "VIEWER", true},
		{"OPERATOR", "VIEWER", true},
		{"OPERATOR", "OPERATOR", true},
		{"OPERATOR", "ADMIN", false},
		{"ADMIN", "OPERATOR", true},
		{"ADMIN", "ADMIN", true},
		{"SUPERADMIN", "ADMIN", true},
		{"SUPERADMIN", "SUPERADMIN", true},
		{"SUPERADMIN", "OPERATOR", true},
		// case-insensitive
		{"superadmin", "admin", true},
		{"operator", "admin", false},
		// unknown role
		{"GUEST", "VIEWER", false},
		{"VIEWER", "GUEST", false},
	}
	for _, c := range cases {
		t.Run(c.role+"_gte_"+c.min, func(t *testing.T) {
			got := roleAtLeast(c.role, c.min)
			if got != c.want {
				t.Errorf("roleAtLeast(%q, %q) = %v, want %v", c.role, c.min, got, c.want)
			}
		})
	}
}

func TestSurveyAllow_RateLimit(t *testing.T) {
	// Use a fresh survey id so the test is isolated from other tests.
	sid := "test-rate-limit-survey"
	// First N calls (N = surveyRateLimitPerSecond) should be allowed.
	for i := 0; i < surveyRateLimitPerSecond; i++ {
		if !surveyAllow(sid) {
			t.Fatalf("expected surveyAllow to return true on call %d (within budget)", i+1)
		}
	}
	// The (N+1)th call within the same 1s window must be rejected.
	if surveyAllow(sid) {
		t.Errorf("expected surveyAllow to return false after exceeding %d/s budget", surveyRateLimitPerSecond)
	}
}

func TestSurveyAllow_DifferentSurveysAreIndependent(t *testing.T) {
	sid1 := "test-survey-A"
	sid2 := "test-survey-B"
	// Burn through sid1's budget
	for i := 0; i < surveyRateLimitPerSecond; i++ {
		surveyAllow(sid1)
	}
	if surveyAllow(sid1) {
		t.Errorf("sid1 should be rate-limited")
	}
	// sid2 has its own bucket — should still allow.
	if !surveyAllow(sid2) {
		t.Errorf("sid2 should not be rate-limited (separate bucket)")
	}
}
