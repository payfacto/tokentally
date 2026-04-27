package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"tokentally/internal/db"
	"tokentally/internal/pricing"
	"tokentally/internal/scanner"
	"tokentally/internal/tips"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	maxQueryLimit       = 1000
	defaultPromptLimit  = 50
	defaultSessionLimit = 20
)

// defaultExchangeRates seeds initial USD→currency rates (approximate, as of 2026-04).
// These are overwritten by RefreshExchangeRates once the user adds an API key.
var defaultExchangeRates = map[string]float64{
	"USD": 1.0,
	"CAD": 1.39,
	"EUR": 0.91,
	"GBP": 0.78,
	"AUD": 1.59,
	"NZD": 1.72,
	"CHF": 0.89,
	"JPY": 152.0,
	"MXN": 19.2,
	"BRL": 5.75,
}

// supportedCurrencies lists the currencies we track exchange rates for.
var supportedCurrencies = []string{"USD", "CAD", "EUR", "GBP", "AUD", "NZD", "CHF", "JPY", "MXN", "BRL"}

// App is the Wails application struct — all exported methods are bound to the JS frontend.
type App struct {
	ctx            context.Context
	conn           *sql.DB
	projectsDir    string
	pricing        *pricing.Pricing // live, rebuilt from DB on every change
	defaultPricing *pricing.Pricing // immutable seed from embedded pricing.json
}

// New creates a new App. conn must already be open.
func New(conn *sql.DB, projectsDir string, p *pricing.Pricing) *App {
	return &App{conn: conn, projectsDir: projectsDir, defaultPricing: p}
}

// Startup is called by Wails when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	seeded, _ := db.IsPricingSeeded(a.conn)
	if !seeded {
		a.seedFromDefaults()
	}
	a.reloadPricing()
	if needsInspectorBackfill(a.conn) {
		go a.runInspectorBackfill()
	} else {
		go a.scanLoop()
	}
}

// seedFromDefaults populates pricing_models, pricing_plans, and exchange_rates from defaults.
func (a *App) seedFromDefaults() {
	if a.defaultPricing == nil {
		return
	}
	for name, r := range a.defaultPricing.Models {
		db.UpsertPricingModel(a.conn, name, r.Tier, r.Input, r.Output, r.CacheRead, r.CacheCreate5m, r.CacheCreate1h) //nolint:errcheck
	}
	for key, pl := range a.defaultPricing.Plans {
		db.UpsertPricingPlan(a.conn, key, pl.Label, pl.Monthly) //nolint:errcheck
	}
	for currency, rate := range defaultExchangeRates {
		db.SeedExchangeRate(a.conn, currency, rate) //nolint:errcheck
	}
	db.MarkPricingSeeded(a.conn) //nolint:errcheck
}

// reloadPricing rebuilds a.pricing from the current DB rows.
func (a *App) reloadPricing() {
	models, err := db.GetPricingModels(a.conn)
	if err != nil {
		return
	}
	plans, err := db.GetPricingPlans(a.conn)
	if err != nil {
		return
	}
	p := &pricing.Pricing{
		Models: make(map[string]pricing.ModelRates, len(models)),
		Plans:  make(map[string]pricing.PlanDef, len(plans)),
	}
	for _, m := range models {
		name, _ := m["model_name"].(string)
		p.Models[name] = pricing.ModelRates{
			Tier:          stringVal(m["tier"]),
			Input:         asFloat64(m["input"]),
			Output:        asFloat64(m["output"]),
			CacheRead:     asFloat64(m["cache_read"]),
			CacheCreate5m: asFloat64(m["cache_create_5m"]),
			CacheCreate1h: asFloat64(m["cache_create_1h"]),
		}
	}
	for _, pl := range plans {
		key, _ := pl["plan_key"].(string)
		p.Plans[key] = pricing.PlanDef{
			Label:   stringVal(pl["label"]),
			Monthly: asFloat64(pl["monthly"]),
		}
	}
	a.pricing = p
}

func (a *App) scanLoop() {
	interval := 30 * time.Second
	for {
		result, err := scanner.ScanDir(a.conn, a.projectsDir)
		if err == nil && (result.Messages > 0 || result.Files > 0) {
			runtime.EventsEmit(a.ctx, "scan", result)
		}
		// Purge runs regardless of scan outcome — the two operations are independent.
		if days, _ := db.GetRetentionDays(a.conn); days > 0 {
			db.PurgeMessages(a.conn, days) //nolint:errcheck
		}
		time.Sleep(interval)
	}
}

// needsInspectorBackfill returns true if the one-time inspector backfill has not yet run.
func needsInspectorBackfill(conn *sql.DB) bool {
	var v string
	err := conn.QueryRow(`SELECT v FROM plan WHERE k='inspector_backfill_done'`).Scan(&v)
	return errors.Is(err, sql.ErrNoRows)
}

// runInspectorBackfill clears the file-scan cache to force a full rescan that
// populates the new inspector columns, then starts the normal scan loop.
func (a *App) runInspectorBackfill() {
	a.conn.Exec(`DELETE FROM files`) //nolint:errcheck
	if _, err := scanner.ScanDir(a.conn, a.projectsDir); err == nil {
		a.conn.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('inspector_backfill_done','1')`) //nolint:errcheck
	}
	a.scanLoop()
}

type overviewResult struct {
	Sessions            int64    `json:"sessions"`
	Turns               int64    `json:"turns"`
	InputTokens         int64    `json:"input_tokens"`
	OutputTokens        int64    `json:"output_tokens"`
	CacheReadTokens     int64    `json:"cache_read_tokens"`
	CacheCreate5mTokens int64    `json:"cache_create_5m_tokens"`
	CacheCreate1hTokens int64    `json:"cache_create_1h_tokens"`
	CostUSD             *float64 `json:"cost_usd"`
}

func (a *App) GetOverview(since, until string) (overviewResult, error) {
	totals, err := db.OverviewTotals(a.conn, since, until)
	if err != nil {
		return overviewResult{}, err
	}
	r := overviewResult{
		Sessions:            asInt64(totals["sessions"]),
		Turns:               asInt64(totals["turns"]),
		InputTokens:         asInt64(totals["input_tokens"]),
		OutputTokens:        asInt64(totals["output_tokens"]),
		CacheReadTokens:     asInt64(totals["cache_read_tokens"]),
		CacheCreate5mTokens: asInt64(totals["cache_create_5m_tokens"]),
		CacheCreate1hTokens: asInt64(totals["cache_create_1h_tokens"]),
	}
	models, err := db.ModelBreakdown(a.conn, since, until)
	if err != nil {
		return overviewResult{}, err
	}
	var totalCost float64
	for _, m := range models {
		model, _ := m["model"].(string)
		c := pricing.CostFor(model, usageFromRow(m), a.pricing, a.getPlan())
		if c != nil {
			totalCost += *c
		}
	}
	r.CostUSD = &totalCost
	return r, nil
}

func (a *App) GetPrompts(limit int, sort string) ([]map[string]any, error) {
	if limit <= 0 || limit > maxQueryLimit {
		limit = defaultPromptLimit
	}
	rows, err := db.ExpensivePrompts(a.conn, limit, sort)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		model, _ := r["model"].(string)
		r["estimated_cost_usd"] = pricing.CostFor(model, usageFromRow(r), a.pricing, a.getPlan())
	}
	return rows, nil
}

func (a *App) GetProjects(since, until string) ([]map[string]any, error) {
	return db.ProjectSummary(a.conn, since, until)
}

func (a *App) GetSessions(limit int, since, until string) ([]map[string]any, error) {
	if limit <= 0 || limit > maxQueryLimit {
		limit = defaultSessionLimit
	}
	return db.RecentSessions(a.conn, limit, since, until)
}

func (a *App) GetSessionTurns(sessionID string) ([]map[string]any, error) {
	return db.SessionTurns(a.conn, sessionID)
}

// GetSessionChunks returns a session as structured chunks for the Vue inspector.
func (a *App) GetSessionChunks(sessionID string) ([]db.SessionChunk, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID required")
	}
	return db.GetSessionChunks(a.conn, sessionID)
}

func (a *App) GetTools(since, until string) ([]map[string]any, error) {
	return db.ToolBreakdown(a.conn, since, until)
}

func (a *App) GetDaily(since, until string) ([]map[string]any, error) {
	return db.DailyBreakdown(a.conn, since, until)
}

func (a *App) GetByModel(since, until string) ([]map[string]any, error) {
	rows, err := db.ModelBreakdown(a.conn, since, until)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		model, _ := r["model"].(string)
		c := pricing.CostFor(model, usageFromRow(r), a.pricing, a.getPlan())
		r["cost_usd"] = c
		r["cost_estimated"] = (c == nil)
	}
	return rows, nil
}

func (a *App) GetSkills(since, until string) ([]map[string]any, error) {
	return db.SkillBreakdown(a.conn, since, until)
}

func (a *App) GetTips() ([]map[string]any, error) {
	result, err := tips.AllTips(a.conn)
	if result == nil {
		result = []map[string]any{}
	}
	return result, err
}

func (a *App) DismissTip(key string) error {
	return db.DismissTip(a.conn, key)
}

func (a *App) GetPlan() (map[string]any, error) {
	plan, err := db.GetPlan(a.conn)
	if err != nil {
		return nil, err
	}
	currency, _ := db.GetCurrency(a.conn)
	rates, _ := db.GetExchangeRates(a.conn)
	rate := rates[currency]
	if rate == 0 {
		rate = 1.0
	}
	return map[string]any{
		"plan":          plan,
		"pricing":       a.pricing,
		"currency":      currency,
		"exchange_rate": rate,
	}, nil
}

func (a *App) SetPlan(plan string) error {
	return db.SetPlan(a.conn, plan)
}

// --- Pricing model CRUD ---

func (a *App) GetPricingModels() ([]map[string]any, error) {
	return db.GetPricingModels(a.conn)
}

func (a *App) UpsertPricingModel(name, tier string, input, output, cacheRead, cache5m, cache1h float64) error {
	if err := db.UpsertPricingModel(a.conn, name, tier, input, output, cacheRead, cache5m, cache1h); err != nil {
		return err
	}
	a.reloadPricing()
	return nil
}

func (a *App) DeletePricingModel(name string) error {
	if err := db.DeletePricingModel(a.conn, name); err != nil {
		return err
	}
	a.reloadPricing()
	return nil
}

// --- Pricing plan CRUD ---

func (a *App) GetPricingPlans() ([]map[string]any, error) {
	return db.GetPricingPlans(a.conn)
}

func (a *App) UpsertPricingPlan(key, label string, monthly float64) error {
	if err := db.UpsertPricingPlan(a.conn, key, label, monthly); err != nil {
		return err
	}
	a.reloadPricing()
	return nil
}

func (a *App) DeletePricingPlan(key string) error {
	if err := db.DeletePricingPlan(a.conn, key); err != nil {
		return err
	}
	a.reloadPricing()
	return nil
}

// --- Currency and exchange rates ---

func (a *App) GetCurrency() (string, error) {
	return db.GetCurrency(a.conn)
}

func (a *App) SetCurrency(currency string) error {
	return db.SetCurrency(a.conn, currency)
}

func (a *App) GetExchangeRates() (map[string]float64, error) {
	return db.GetExchangeRates(a.conn)
}

func (a *App) SetExchangeRate(currency string, rate float64) error {
	return db.SetExchangeRate(a.conn, currency, rate)
}

func (a *App) GetExchangeApiKey() (string, error) {
	return db.GetExchangeApiKey(a.conn)
}

func (a *App) SetExchangeApiKey(key string) error {
	return db.SetExchangeApiKey(a.conn, key)
}

// RefreshExchangeRates fetches live rates from exchangerate-api.com and stores them.
func (a *App) RefreshExchangeRates() (map[string]float64, error) {
	key, err := db.GetExchangeApiKey(a.conn)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, fmt.Errorf("no API key — enter your exchangerate-api.com key first")
	}
	resp, err := http.Get("https://v6.exchangerate-api.com/v6/" + key + "/latest/USD") //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("fetching rates: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Result          string             `json:"result"`
		ErrorType       string             `json:"error-type"`
		ConversionRates map[string]float64 `json:"conversion_rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if payload.Result != "success" {
		msg := payload.ErrorType
		if msg == "" {
			msg = "unknown error"
		}
		return nil, fmt.Errorf("API error: %s", msg)
	}
	for _, cur := range supportedCurrencies {
		if rate, ok := payload.ConversionRates[cur]; ok {
			if err := db.SetExchangeRate(a.conn, cur, rate); err != nil {
				return nil, err
			}
		}
	}
	return db.GetExchangeRates(a.conn)
}

// --- Reset ---

// ResetPricingToDefaults clears all model and plan rows and re-seeds from the
// embedded pricing.json defaults. Currency is preserved.
func (a *App) ResetPricingToDefaults() error {
	if err := db.DeleteAllPricingModels(a.conn); err != nil {
		return err
	}
	if err := db.DeleteAllPricingPlans(a.conn); err != nil {
		return err
	}
	a.seedFromDefaults()
	a.reloadPricing()
	return nil
}

func (a *App) ScanNow() (scanner.ScanResult, error) {
	result, err := scanner.ScanDir(a.conn, a.projectsDir)
	if err == nil && a.ctx != nil {
		runtime.EventsEmit(a.ctx, "scan", result)
	}
	return result, err
}

// GetRetentionDays returns the configured retention period in days.
// Returns 0 if not set (auto-purge disabled).
func (a *App) GetRetentionDays() (int, error) {
	return db.GetRetentionDays(a.conn)
}

// SetRetentionDays persists the retention period. days=0 disables auto-purge.
func (a *App) SetRetentionDays(days int) error {
	return db.SetRetentionDays(a.conn, days)
}

// PurgeOlderThan deletes messages older than the given number of days.
// Returns the number of message rows deleted.
func (a *App) PurgeOlderThan(days int) (int64, error) {
	return db.PurgeMessages(a.conn, days)
}

// SaveHTMLExport opens a native Save-As dialog and writes the provided HTML
// to the chosen path. Returns the saved path, or empty string if the user cancelled.
func (a *App) SaveHTMLExport(html string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export session as HTML",
		DefaultFilename: "session.html",
		Filters: []runtime.FileFilter{
			{DisplayName: "HTML files (*.html)", Pattern: "*.html"},
		},
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(html), 0644); err != nil {
		return "", fmt.Errorf("SaveHTMLExport: %w", err)
	}
	return path, nil
}

func (a *App) getPlan() string {
	plan, _ := db.GetPlan(a.conn)
	return plan
}

func usageFromRow(r map[string]any) pricing.Usage {
	return pricing.Usage{
		InputTokens:         int(asInt64(r["input_tokens"])),
		OutputTokens:        int(asInt64(r["output_tokens"])),
		CacheReadTokens:     int(asInt64(r["cache_read_tokens"])),
		CacheCreate5mTokens: int(asInt64(r["cache_create_5m_tokens"])),
		CacheCreate1hTokens: int(asInt64(r["cache_create_1h_tokens"])),
	}
}

func asInt64(v any) int64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}

func asFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	}
	return 0
}

func stringVal(v any) string {
	s, _ := v.(string)
	return s
}
