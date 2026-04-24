package pricing_test

import (
	"strings"
	"testing"

	"tokentally/internal/pricing"
)

const sampleJSON = `{
  "models": {
    "claude-sonnet-4-6": {
      "tier": "sonnet",
      "input": 3.0, "output": 15.0,
      "cache_read": 0.3, "cache_create_5m": 3.75, "cache_create_1h": 6.0
    }
  },
  "tier_fallback": {
    "opus":   { "input": 15.0, "output": 75.0, "cache_read": 1.5, "cache_create_5m": 18.75, "cache_create_1h": 30.0 },
    "sonnet": { "input":  3.0, "output": 15.0, "cache_read": 0.3, "cache_create_5m":  3.75, "cache_create_1h":  6.0 },
    "haiku":  { "input":  1.0, "output":  5.0, "cache_read": 0.1, "cache_create_5m":  1.25, "cache_create_1h":  2.0 }
  },
  "plans": {
    "api": { "monthly": 0, "label": "API (pay-per-token)" }
  }
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

func TestCostFor_TierFallback(t *testing.T) {
	p, _ := pricing.Load(strings.NewReader(sampleJSON))
	// "claude-opus-future" is not in models but contains "opus" → should use tier_fallback
	cost := pricing.CostFor("claude-opus-future", pricing.Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}, p, "api")
	// 1M input @ $15/Mtok + 1M output @ $75/Mtok = $90
	if cost == nil || *cost < 89.9 || *cost > 90.1 {
		t.Errorf("expected ~$90 via tier fallback, got %v", cost)
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

func TestPlanDefs(t *testing.T) {
	p, _ := pricing.Load(strings.NewReader(sampleJSON))
	if len(p.Plans) == 0 {
		t.Error("expected at least one plan")
	}
	plan, ok := p.Plans["api"]
	if !ok {
		t.Fatal("api plan not found")
	}
	if plan.Label == "" {
		t.Error("plan label should not be empty")
	}
}
