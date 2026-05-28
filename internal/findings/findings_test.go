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

func TestHumanTokens(t *testing.T) {
	cases := map[int64]string{
		0:        "0",
		500:      "500",
		1000:     "1K",
		1500:     "1.5K",
		12000:    "12K",
		1000000:  "1.0M",
		1500000:  "1.5M",
	}
	for n, want := range cases {
		if got := humanTokens(n); got != want {
			t.Errorf("humanTokens(%d)=%q, want %q", n, got, want)
		}
	}
}

func TestShortTarget(t *testing.T) {
	cases := map[string]string{
		"":                   "(no target)",
		"a.go":               "a.go",
		"x/y/z.go":           ".../y/z.go",
		"internal/db/db.go":  ".../db/db.go",
	}
	for target, want := range cases {
		if got := shortTarget(target); got != want {
			t.Errorf("shortTarget(%q)=%q, want %q", target, got, want)
		}
	}
}
