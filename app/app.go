package app

import (
	"context"
	"database/sql"
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

// App is the Wails application struct — all exported methods are bound to the JS frontend.
type App struct {
	ctx         context.Context
	conn        *sql.DB
	projectsDir string
	pricing     *pricing.Pricing
}

// New creates a new App. conn must already be open.
func New(conn *sql.DB, projectsDir string, p *pricing.Pricing) *App {
	return &App{conn: conn, projectsDir: projectsDir, pricing: p}
}

// Startup is called by Wails when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	go a.scanLoop()
}

func (a *App) scanLoop() {
	interval := 30 * time.Second
	for {
		result, err := scanner.ScanDir(a.conn, a.projectsDir)
		if err == nil && (result.Messages > 0 || result.Files > 0) {
			runtime.EventsEmit(a.ctx, "scan", result)
		}
		time.Sleep(interval)
	}
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
	return tips.AllTips(a.conn)
}

func (a *App) DismissTip(key string) error {
	return db.DismissTip(a.conn, key)
}

func (a *App) GetPlan() (map[string]any, error) {
	plan, err := db.GetPlan(a.conn)
	if err != nil {
		return nil, err
	}
	return map[string]any{"plan": plan, "pricing": a.pricing}, nil
}

func (a *App) SetPlan(plan string) error {
	return db.SetPlan(a.conn, plan)
}

func (a *App) ScanNow() (scanner.ScanResult, error) {
	result, err := scanner.ScanDir(a.conn, a.projectsDir)
	if err == nil && a.ctx != nil {
		runtime.EventsEmit(a.ctx, "scan", result)
	}
	return result, err
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
