package services

import "testing"

func TestNewDeviceChangeSetSeparatesAttemptAndContentIdentity(t *testing.T) {
	commands := []UciCommand{{
		Action:  "set",
		Config:  "system",
		Section: "@system[0]",
		Option:  "hostname",
		Value:   "lab-router",
	}}

	first, err := NewDeviceChangeSet("ROUTER-01", "system", commands, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil, ConfirmationLocalAuto)
	if err != nil {
		t.Fatalf("first changeset: %v", err)
	}
	second, err := NewDeviceChangeSet("ROUTER-01", "system", commands, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil, ConfirmationLocalAuto)
	if err != nil {
		t.Fatalf("second changeset: %v", err)
	}

	if first.ChangeSetID == "" || first.ChangeSetID == second.ChangeSetID {
		t.Fatalf("change-set identities = %q and %q, want distinct non-empty attempts", first.ChangeSetID, second.ChangeSetID)
	}
	if first.PlanHash == "" || first.PlanHash != second.PlanHash {
		t.Fatalf("plan hashes = %q and %q, want stable content identity", first.PlanHash, second.PlanHash)
	}
	if first.DeviceID != "ROUTER-01" || first.Generation != 0 || len(first.Operations) != 1 {
		t.Fatalf("changeset envelope = %#v, want device, unassigned generation, and one operation", first)
	}
	if first.Operations[0].Config != "system" || first.Operations[0].ObservedStateHash != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("changeset operation = %#v, want system operation with observed-state hash", first.Operations[0])
	}
	if first.ConfirmationPolicy != ConfirmationLocalAuto {
		t.Fatalf("confirmation policy = %q, want %q", first.ConfirmationPolicy, ConfirmationLocalAuto)
	}
}

func TestNewDeviceChangeSetForRolloutUsesStableIdentities(t *testing.T) {
	commands := []UciCommand{{
		Action:  "set",
		Config:  "system",
		Section: "@system[0]",
		Option:  "hostname",
		Value:   "lab-router",
	}}
	first, err := NewDeviceChangeSetForRollout("rollout-1", "ROUTER-01", "system", commands, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil, ConfirmationLocalAuto)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewDeviceChangeSetForRollout("rollout-1", "ROUTER-01", "system", commands, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil, ConfirmationLocalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if first.ChangeSetID == "" || first.ChangeSetID != second.ChangeSetID || first.Operations[0].OperationID != second.Operations[0].OperationID {
		t.Fatalf("rollout identities = %#v and %#v, want stable IDs", first, second)
	}
}

func TestNewDeviceChangeSetUsesCanonicalJSONForHTMLSensitiveValues(t *testing.T) {
	changeSet, err := NewDeviceChangeSet(
		"ROUTER-01",
		"system",
		[]UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "a<b>&c"}},
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		nil,
		ConfirmationLocalAuto,
	)
	if err != nil {
		t.Fatal(err)
	}
	if changeSet.PlanHash != "01366ffd944f1a7e48462a8829e86eaa2d631fb175fe9029ce1e6316c96897f3" {
		t.Fatalf("plan hash = %s, want canonical non-HTML-escaped hash", changeSet.PlanHash)
	}
}

func TestValidateDeviceChangeSetRejectsUnsafeOrIncompleteEnvelope(t *testing.T) {
	validHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cases := []struct {
		name string
		set  DeviceChangeSet
	}{
		{
			name: "missing device identity",
			set: DeviceChangeSet{
				ChangeSetID:        "changeset-1",
				PlanHash:           validHash,
				Generation:         1,
				ConfirmationPolicy: ConfirmationLocalAuto,
				Operations: []DeviceChangeOperation{{
					OperationID:       "operation-1",
					Config:            "system",
					ObservedStateHash: validHash,
					Commands:          []UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
				}},
			},
		},
		{
			name: "unsafe namespace",
			set: DeviceChangeSet{
				ChangeSetID:        "changeset-1",
				DeviceID:           "ROUTER-01",
				PlanHash:           validHash,
				Generation:         1,
				ConfirmationPolicy: ConfirmationLocalAuto,
				Operations: []DeviceChangeOperation{{
					OperationID:       "operation-1",
					Config:            "network",
					ObservedStateHash: validHash,
					Commands:          []UciCommand{{Action: "set", Config: "network", Section: "lan", Option: "ipaddr", Value: "192.0.2.1"}},
				}},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateDeviceChangeSet(tt.set); err == nil {
				t.Fatal("incomplete or unsafe changeset was accepted")
			}
		})
	}
}
