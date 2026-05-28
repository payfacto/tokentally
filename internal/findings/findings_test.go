package findings

import "testing"

func TestContextWindowFor(t *testing.T) {
	cases := map[string]int64{
		"claude-opus-4-7":      200000,
		"claude-sonnet-4-6":    200000,
		"claude-sonnet-4-6-1m": 1000000,
		"claude-opus-4-7[1m]":  1000000,
		"":                     200000,
	}
	for model, want := range cases {
		if got := ContextWindowFor(model); got != want {
			t.Errorf("ContextWindowFor(%q)=%d, want %d", model, got, want)
		}
	}
}

func TestTierOf(t *testing.T) {
	if tierOf("claude-opus-4-7") != "opus" {
		t.Error("opus not detected")
	}
	if tierOf("gpt-4") != "" {
		t.Error("non-claude should be empty tier")
	}
}
