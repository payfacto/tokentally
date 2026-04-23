package main

import (
	"embed"
	"os"

	"tokentally/internal/pricing"
)

//go:embed pricing.json
var rawPricing embed.FS

func loadPricing() *pricing.Pricing {
	if override := os.Getenv("TOKENTALLY_PRICING_JSON"); override != "" {
		f, err := os.Open(override)
		if err == nil {
			p, _ := pricing.Load(f)
			f.Close()
			return p
		}
	}
	f, err := rawPricing.Open("pricing.json")
	if err != nil {
		return nil
	}
	defer f.Close()
	p, _ := pricing.Load(f)
	return p
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
