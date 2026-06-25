package formations

import "testing"

func TestParseChroteOutputsAcceptsBarePortKeyedJSON(t *testing.T) {
	clean, outputs, err := parseChroteOutputs("audit summary\n{\"port_report\":{\"text\":\"routed payload\"}}")
	if err != nil {
		t.Fatalf("parse outputs: %v", err)
	}
	if clean != "audit summary" {
		t.Fatalf("clean text = %q, want audit summary", clean)
	}
	if outputs["port_report"].Text != "routed payload" {
		t.Fatalf("outputs = %#v, want routed payload", outputs)
	}
}

func TestParseChroteOutputsDoesNotTreatFreeformJSONAsRoutedOutput(t *testing.T) {
	_, outputs, err := parseChroteOutputs("audit summary\n{\"text\":\"not a port-keyed payload\"}")
	if err != nil {
		t.Fatalf("parse outputs: %v", err)
	}
	if outputs != nil {
		t.Fatalf("outputs = %#v, want nil for non-port JSON", outputs)
	}
}

func TestParseChroteOutputsRepairsTerminalWrappedJSONString(t *testing.T) {
	_, outputs, err := parseChroteOutputs("{\n  \"port_report\": {\n    \"text\": \"done\",\n    \"ref\": \"/home/perttu/.formations/artifacts/run_123/\n  agents-tab-audit.md\"\n  }\n}")
	if err != nil {
		t.Fatalf("parse wrapped outputs: %v", err)
	}
	if got, want := outputs["port_report"].Ref, "/home/perttu/.formations/artifacts/run_123/agents-tab-audit.md"; got != want {
		t.Fatalf("ref = %q, want %q", got, want)
	}
}
