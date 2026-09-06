package services

import "testing"

func TestNewDeviceOperationPlanIsStableAndTyped(t *testing.T) {
	commands := []UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}}
	first, err := NewDeviceOperationPlan("system", commands, nil, true)
	if err != nil {
		t.Fatalf("NewDeviceOperationPlan returned error: %v", err)
	}
	second, err := NewDeviceOperationPlan("system", commands, nil, true)
	if err != nil {
		t.Fatalf("NewDeviceOperationPlan returned error: %v", err)
	}
	if first.OperationID == "" || first.OperationID != second.OperationID || first.PlanHash != first.OperationID {
		t.Fatalf("plan id is not stable: %#v %#v", first, second)
	}
}

func TestValidateDeviceOperationRequiresHealthForNetwork(t *testing.T) {
	commands := []UciCommand{{Action: "set", Config: "network", Section: "lan", Option: "metric", Value: "10"}}
	if err := ValidateDeviceOperation("network", commands, nil); err == nil {
		t.Fatal("network operation without a health target must be rejected")
	}
	if err := ValidateDeviceOperation("network", commands, []string{"10.128.128.1"}); err != nil {
		t.Fatalf("valid network operation was rejected: %v", err)
	}
}

func TestValidateDeviceOperationAcceptsTypedDHCPLease(t *testing.T) {
	commands := []UciCommand{{Action: "ensure_host", Config: "dhcp", Section: "lab", Option: "AA:BB:CC:DD:EE:FF", Value: "192.0.2.10"}}
	if err := ValidateDeviceOperation("dhcp", commands, nil); err != nil {
		t.Fatalf("typed DHCP lease was rejected: %v", err)
	}
}

func TestValidateDeviceOperationRejectsInvalidDHCPLease(t *testing.T) {
	invalid := []UciCommand{
		{Action: "ensure_host", Config: "dhcp", Section: "lab", Option: "AA:BB:CC:DD:EE:FF;touch", Value: "192.0.2.10"},
		{Action: "ensure_host", Config: "dhcp", Section: "lab", Option: "AA:BB:CC:DD:EE:FF", Value: "not-an-ip"},
		{Action: "ensure_host", Config: "dhcp", Section: "lab", Option: "AA:BB:CC:DD:EE:FF", Value: "2001:db8::10"},
	}
	for _, command := range invalid {
		if err := ValidateDeviceOperation("dhcp", []UciCommand{command}, nil); err == nil {
			t.Fatalf("invalid DHCP lease was accepted: %#v", command)
		}
	}
}

func TestValidateDeviceOperationRejectsRenameOutsideAgentGrammar(t *testing.T) {
	commands := []UciCommand{{Action: "rename", Config: "system", Section: "@system[0]", Value: "new hostname"}}
	if err := ValidateDeviceOperation("system", commands, nil); err == nil {
		t.Fatal("rename with a non-UCI identifier was accepted")
	}
}

func TestValidateDeviceOperationRejectsUnsupportedExecutorNamespace(t *testing.T) {
	commands := []UciCommand{{Action: "set", Config: "openvpn", Section: "client", Option: "enabled", Value: "1"}}
	if err := ValidateDeviceOperation("openvpn", commands, nil); err == nil {
		t.Fatal("namespace without a local restart/recovery contract must be rejected")
	}
}
