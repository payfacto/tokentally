package pricing

import (
	"encoding/json"
	"io"
)

// Usage holds token counts for a single request.
type Usage struct {
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreate5mTokens int
	CacheCreate1hTokens int
}

type modelRates struct {
	InputMtok         float64 `json:"input_mtok"`
	OutputMtok        float64 `json:"output_mtok"`
	CacheReadMtok     float64 `json:"cache_read_mtok"`
	CacheCreate5mMtok float64 `json:"cache_create_5m_mtok"`
	CacheCreate1hMtok float64 `json:"cache_create_1h_mtok"`
}

type planDef struct {
	Models map[string]modelRates `json:"models"`
}

// Pricing holds the loaded pricing data.
type Pricing struct {
	Plans       map[string]planDef `json:"plans"`
	DefaultPlan string             `json:"default_plan"`
}

// Load reads pricing data from r (JSON).
func Load(r io.Reader) (*Pricing, error) {
	var p Pricing
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// CostFor returns the USD cost for a usage record, or nil if the model is unknown.
func CostFor(model string, u Usage, p *Pricing, plan string) *float64 {
	if p == nil {
		return nil
	}
	pd, ok := p.Plans[plan]
	if !ok {
		if pd2, ok2 := p.Plans[p.DefaultPlan]; ok2 {
			pd = pd2
		} else {
			return nil
		}
	}
	rates, ok := pd.Models[model]
	if !ok {
		return nil
	}
	cost := float64(u.InputTokens)/1e6*rates.InputMtok +
		float64(u.OutputTokens)/1e6*rates.OutputMtok +
		float64(u.CacheReadTokens)/1e6*rates.CacheReadMtok +
		float64(u.CacheCreate5mTokens)/1e6*rates.CacheCreate5mMtok +
		float64(u.CacheCreate1hTokens)/1e6*rates.CacheCreate1hMtok
	return &cost
}
