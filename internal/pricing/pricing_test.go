package pricing_test

import (
	"strings"
	"testing"

	"tokentally/internal/pricing"
)

const sampleJSON = `{
  "plans": {"api": {"models": {"claude-sonnet-4-6": {
    "input_mtok": 3.0, "output_mtok": 15.0,
    "cache_read_mtok": 0.3, "cache_create_5m_mtok": 3.75, "cache_create_1h_mtok": 3.75
  }}}},
  "default_plan": "api"
}`

func TestLoadPricing(t *testing.T) {
	p, err := pricing.Load(strings.NewReader(sampleJSON))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p == nil {
		t.Fatal("pricing is nil")
	}
}

func TestCostFor_Sonnet(t *testing.T) {
	p, _ := pricing.Load(strings.NewReader(sampleJSON))
	cost := pricing.CostFor("claude-sonnet-4-6", pricing.Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}, p, "api")
	// 1M input @ $3/Mtok = $3, 1M output @ $15/Mtok = $15 → $18
	if cost == nil || *cost < 17.9 || *cost > 18.1 {
		t.Errorf("expected ~$18, got %v", cost)
	}
}

func TestCostFor_UnknownModel(t *testing.T) {
	p, _ := pricing.Load(strings.NewReader(sampleJSON))
	cost := pricing.CostFor("unknown-model", pricing.Usage{InputTokens: 1000}, p, "api")
	if cost != nil {
		t.Errorf("expected nil cost for unknown model, got %v", cost)
	}
}

func TestCostFor_NilPricing(t *testing.T) {
	cost := pricing.CostFor("claude-sonnet-4-6", pricing.Usage{InputTokens: 1000}, nil, "api")
	if cost != nil {
		t.Errorf("expected nil cost for nil pricing, got %v", cost)
	}
}

func TestCostFor_CacheTokens(t *testing.T) {
	p, _ := pricing.Load(strings.NewReader(sampleJSON))
	cost := pricing.CostFor("claude-sonnet-4-6", pricing.Usage{
		CacheReadTokens:     1_000_000,
		CacheCreate5mTokens: 1_000_000,
	}, p, "api")
	// 1M cache_read @ $0.3/Mtok + 1M cache_create_5m @ $3.75/Mtok = $4.05
	if cost == nil || *cost < 4.04 || *cost > 4.06 {
		t.Errorf("expected ~$4.05, got %v", cost)
	}
}
