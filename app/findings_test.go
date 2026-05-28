package app

import (
	"testing"

	"tokentally/internal/db"
	"tokentally/internal/pricing"
)

// newTestApp opens an in-memory DB and returns a minimal *App with pricing seeded.
func newTestApp(t *testing.T) *App {
	t.Helper()
	pool, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })

	p := &pricing.Pricing{
		Models: map[string]pricing.ModelRates{
			"claude-sonnet-4-6": {Input: 3.0, Output: 15.0},
		},
		Plans: map[string]pricing.PlanDef{},
	}
	return &App{conn: pool, pricing: p, defaultPricing: p}
}

// seedOneAssistantMessage inserts a single assistant message row for the given model.
func seedOneAssistantMessage(t *testing.T, a *App, model string, inputTokens, outputTokens int64) {
	t.Helper()
	_, err := a.conn.Write.Exec(
		`INSERT INTO messages (uuid, session_id, project_slug, type, model, timestamp,
		   input_tokens, output_tokens, cache_read_tokens,
		   cache_create_5m_tokens, cache_create_1h_tokens)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		"test-uuid-1", "sess-test", "proj-test", "assistant", model,
		"2026-05-01T10:00:00Z", inputTokens, outputTokens, 0, 0, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBlendedRatePerToken(t *testing.T) {
	a := newTestApp(t)
	seedOneAssistantMessage(t, a, "claude-sonnet-4-6", 1_000_000, 0)
	rate := a.blendedRatePerToken("", "")
	if rate <= 0 {
		t.Errorf("blended rate should be positive, got %v", rate)
	}
}
