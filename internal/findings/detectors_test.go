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

func TestDetectToolCascade(t *testing.T) {
	tc := func(e bool) ToolCallRow { return ToolCallRow{ToolName: "Bash", IsError: e} }
	in := SessionInput{SessionID: "s1", ToolCalls: []ToolCallRow{
		tc(true), tc(true), tc(true), tc(true), tc(false),
	}}
	got := detectToolCascade(in)
	if len(got) != 1 || got[0].Kind != KindToolCascade {
		t.Fatalf("want 1 cascade finding, got %+v", got)
	}
	if got[0].Severity != SevMed { // streak 4 < high threshold 6
		t.Errorf("want med severity, got %s", got[0].Severity)
	}
	if got[0].EstTokens != 4*toolCascadeTokensPerErr {
		t.Errorf("EstTokens=%d", got[0].EstTokens)
	}
}

func TestDetectToolCascade_ShortStreakIgnored(t *testing.T) {
	tc := func(e bool) ToolCallRow { return ToolCallRow{IsError: e} }
	in := SessionInput{ToolCalls: []ToolCallRow{tc(true), tc(true), tc(true), tc(false)}}
	if got := detectToolCascade(in); len(got) != 0 {
		t.Errorf("streak of 3 should not fire, got %d", len(got))
	}
}

func TestDetectLooping(t *testing.T) {
	msg := func(txt string) MessageRow { return MessageRow{Type: "user", PromptText: txt} }
	in := SessionInput{SessionID: "s1", Messages: []MessageRow{
		msg("please fix the failing scanner test now"),
		msg("please fix the failing scanner test now please"),
		msg("please fix the failing scanner test right now"),
		msg("please fix the failing scanner test now again"),
	}}
	got := detectLooping(in)
	if len(got) != 1 || got[0].Kind != KindLooping {
		t.Fatalf("want 1 looping finding, got %+v", got)
	}
}

func TestDetectLooping_DistinctPromptsIgnored(t *testing.T) {
	msg := func(txt string) MessageRow { return MessageRow{Type: "user", PromptText: txt} }
	in := SessionInput{Messages: []MessageRow{
		msg("add a new database migration"),
		msg("now write the frontend view"),
		msg("explain the pricing tier fallback"),
		msg("commit everything and push"),
	}}
	if got := detectLooping(in); len(got) != 0 {
		t.Errorf("distinct prompts should not loop, got %d", len(got))
	}
}
