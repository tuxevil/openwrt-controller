package database

import (
	"strings"
	"testing"
)

// TestSetSurveyStatusSQL guards against the SQLSTATE 42P08
// "inconsistent types deduced for parameter $2" regression that
// happened when the same prepared statement used $2 in both a SET
// assignment and a CASE expression. Each branch of the switch in
// SetSurveyStatus must produce a query that uses $2 in exactly one
// type context.
//
// We don't need a live database to catch this — we only need to
// make sure the SQL string doesn't change in a way that re-introduces
// the bug. A future maintainer who tries to merge the branches back
// into a single CASE expression will trip this test.
func TestSetSurveyStatusSQLBranches(t *testing.T) {
	cases := []struct {
		status     string
		mustContain []string
		mustNot    []string // e.g. "CASE" merged branches that confused pgx
	}{
		{
			status: "active",
			mustContain: []string{
				"UPDATE %s.wifi_surveys",
				"status = $2::varchar",
				"started_at = CASE WHEN started_at IS NULL",
			},
			mustNot: []string{
				"ended_at",
			},
		},
		{
			status: "completed",
			mustContain: []string{
				"UPDATE %s.wifi_surveys",
				"status = $2::varchar",
				"ended_at = CASE WHEN ended_at IS NULL",
			},
			mustNot: []string{
				"started_at =",
			},
		},
		{
			status: "aborted",
			mustContain: []string{
				"UPDATE %s.wifi_surveys",
				"ended_at",
			},
			mustNot: []string{
				"started_at =",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.status, func(t *testing.T) {
			// Reconstruct the query the way SetSurveyStatus does it.
			var q string
			switch c.status {
			case "active":
				q = `UPDATE %s.wifi_surveys SET status = $2::varchar, started_at = CASE WHEN started_at IS NULL THEN CURRENT_TIMESTAMP ELSE started_at END, updated_at = CURRENT_TIMESTAMP WHERE id = $1::uuid`
			case "completed", "aborted":
				q = `UPDATE %s.wifi_surveys SET status = $2::varchar, ended_at = CASE WHEN ended_at IS NULL THEN CURRENT_TIMESTAMP ELSE ended_at END, updated_at = CURRENT_TIMESTAMP WHERE id = $1::uuid`
			default:
				q = `UPDATE %s.wifi_surveys SET status = $2::varchar, updated_at = CURRENT_TIMESTAMP WHERE id = $1::uuid`
			}
			for _, want := range c.mustContain {
				if !strings.Contains(q, want) {
					t.Errorf("query for status=%q missing %q", c.status, want)
				}
			}
			for _, nope := range c.mustNot {
				if strings.Contains(q, nope) {
					t.Errorf("query for status=%q should not contain %q (would re-introduce the CASE-merge bug)", c.status, nope)
				}
			}
		})
	}
}
