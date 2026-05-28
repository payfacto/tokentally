package findings

import "testing"

func TestDetectRetryChurn(t *testing.T) {
	in := SessionInput{SessionID: "s1", ToolCalls: []ToolCallRow{
		{ToolName: "Read", Target: "a.go", IsError: true},
		{ToolName: "Read", Target: "a.go", IsError: true},
		{ToolName: "Read", Target: "a.go", IsError: true},
		{ToolName: "Read", Target: "a.go", IsError: false},
	}}
	got := detectRetryChurn(in)
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d", len(got))
	}
	if got[0].Kind != KindRetryChurn || got[0].Severity != SevHigh {
		t.Errorf("wrong kind/severity: %+v", got[0])
	}
	if got[0].EstTokens != 3*retryChurnTokensPerRetry {
		t.Errorf("EstTokens=%d", got[0].EstTokens)
	}
}

func TestDetectRetryChurn_NoErrorNoFinding(t *testing.T) {
	in := SessionInput{ToolCalls: []ToolCallRow{
		{ToolName: "Read", Target: "a.go"}, {ToolName: "Read", Target: "a.go"},
		{ToolName: "Read", Target: "a.go"}, {ToolName: "Read", Target: "a.go"},
	}}
	if got := detectRetryChurn(in); len(got) != 0 {
		t.Errorf("want 0 findings (no errors), got %d", len(got))
	}
}
