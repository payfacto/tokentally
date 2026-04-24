package pricing

import (
	"encoding/json"
	"io"
	"strings"
)

// Usage holds token counts for a single request.
type Usage struct {
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreate5mTokens int
	CacheCreate1hTokens int
}

// ModelRates holds per-million-token rates for one model.
type ModelRates struct {
	Tier          string  `json:"tier"`
	Input         float64 `json:"input"`
	Output        float64 `json:"output"`
	CacheRead     float64 `json:"cache_read"`
	CacheCreate5m float64 `json:"cache_create_5m"`
	CacheCreate1h float64 `json:"cache_create_1h"`
}

// PlanDef describes a subscription plan.
type PlanDef struct {
	Monthly float64 `json:"monthly"`
	Label   string  `json:"label"`
}

// Pricing holds the loaded pricing data (mirrors pricing.json).
type Pricing struct {
	Models       map[string]ModelRates `json:"models"`
	TierFallback map[string]ModelRates `json:"tier_fallback"`
	Plans        map[string]PlanDef    `json:"plans"`
}

// Load reads pricing data from r (JSON).
func Load(r io.Reader) (*Pricing, error) {
	var p Pricing
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// tierFromModel infers opus/sonnet/haiku from the model name string.
func tierFromModel(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return "opus"
	case strings.Contains(m, "sonnet"):
		return "sonnet"
	case strings.Contains(m, "haiku"):
		return "haiku"
	}
	return ""
}

// CostFor returns the USD cost for a usage record.
// It looks up the model directly; if not found, falls back to tier_fallback.
// Returns nil if neither model nor tier rates are found.
func CostFor(model string, u Usage, p *Pricing, plan string) *float64 {
	if p == nil {
		return nil
	}
	rates, ok := p.Models[model]
	if !ok {
		tier := tierFromModel(model)
		if tier == "" {
			return nil
		}
		rates, ok = p.TierFallback[tier]
		if !ok {
			return nil
		}
	}
	cost := float64(u.InputTokens)/1e6*rates.Input +
		float64(u.OutputTokens)/1e6*rates.Output +
		float64(u.CacheReadTokens)/1e6*rates.CacheRead +
		float64(u.CacheCreate5mTokens)/1e6*rates.CacheCreate5m +
		float64(u.CacheCreate1hTokens)/1e6*rates.CacheCreate1h
	return &cost
}
