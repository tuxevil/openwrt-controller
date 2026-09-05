package handlers

import (
	"strings"
	"testing"
)

func TestBuildStarlinkQueryScopesSite(t *testing.T) {
	query, err := buildStarlinkQuery("d982665c-8fd8-4c32-815f-257cf5bb450a")
	if err != nil {
		t.Fatalf("buildStarlinkQuery returned error: %v", err)
	}
	if !strings.Contains(query, `r["site_id"] == "d982665c-8fd8-4c32-815f-257cf5bb450a"`) {
		t.Fatalf("query does not scope records to the requested site: %s", query)
	}
	if !strings.Contains(query, `r["_measurement"] == "starlink_health"`) {
		t.Fatalf("query lost the Starlink measurement filter: %s", query)
	}
}

func TestBuildStarlinkQueryRejectsInvalidSiteID(t *testing.T) {
	if _, err := buildStarlinkQuery(`site" |> drop() |> from(bucket: "other`); err == nil {
		t.Fatal("expected invalid site ID to be rejected")
	}
}
