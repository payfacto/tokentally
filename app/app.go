package app

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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

	overageScannerBufSize = 4 * 1024 * 1024 // 4 MiB — large enough for verbose stream-json lines

	scanCooldown    = 5 * time.Second
	refreshCooldown = 60 * time.Second
)

func clampLimit(limit, defaultVal int) int {
	if limit <= 0 || limit > maxQueryLimit {
		return defaultVal
	}
	return limit
}

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

var supportedCurrencies = []string{"USD", "CAD", "EUR", "GBP", "AUD", "NZD", "CHF", "JPY", "MXN", "BRL"}

// App is the Wails application struct — all exported methods are bound to the JS frontend.
type App struct {
	ctx            context.Context
	conn           *db.Pool
	projectsDir    string
	pricingMu      sync.RWMutex
	pricing        *pricing.Pricing // guarded by pricingMu; rebuilt from DB on every change
	defaultPricing *pricing.Pricing // immutable seed from embedded pricing.json
	rateMu         sync.Mutex
	lastScan       time.Time
	lastRefresh    time.Time
}

// New creates a new App. pool must already be open.
func New(pool *db.Pool, projectsDir string, p *pricing.Pricing) *App {
	return &App{conn: pool, projectsDir: projectsDir, defaultPricing: p}
}

// Startup is called by Wails when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	seeded, err := db.IsPricingSeeded(a.conn)
	if err != nil {
		log.Printf("IsPricingSeeded: %v", err)
	}
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
		log.Printf("reloadPricing: GetPricingModels: %v", err)
		return
	}
	plans, err := db.GetPricingPlans(a.conn)
	if err != nil {
		log.Printf("reloadPricing: GetPricingPlans: %v", err)
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
	a.pricingMu.Lock()
	a.pricing = p
	a.pricingMu.Unlock()
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
		// Truncate the -wal sidecar so it doesn't grow unbounded over a long-
		// running session. No-op when nothing's pending.
		a.conn.CheckpointWAL() //nolint:errcheck
		time.Sleep(interval)
	}
}

// needsInspectorBackfill returns true if the one-time inspector backfill has not yet run.
func needsInspectorBackfill(p *db.Pool) bool {
	var v string
	err := p.Read.QueryRow(`SELECT v FROM plan WHERE k='inspector_backfill_done'`).Scan(&v)
	return errors.Is(err, sql.ErrNoRows)
}

// runInspectorBackfill clears the file-scan cache to force a full rescan that
// populates the new inspector columns, then starts the normal scan loop.
func (a *App) runInspectorBackfill() {
	a.conn.Write.Exec(`DELETE FROM files`) //nolint:errcheck
	if _, err := scanner.ScanDir(a.conn, a.projectsDir); err == nil {
		a.conn.Write.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('inspector_backfill_done','1')`) //nolint:errcheck
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
		c := pricing.CostFor(model, usageFromRow(m), a.getPricing(), a.getPlan())
		if c != nil {
			totalCost += *c
		}
	}
	r.CostUSD = &totalCost
	return r, nil
}

func (a *App) GetPrompts(limit int, sort string) ([]map[string]any, error) {
	rows, err := db.ExpensivePrompts(a.conn, clampLimit(limit, defaultPromptLimit), sort)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		model, _ := r["model"].(string)
		r["estimated_cost_usd"] = pricing.CostFor(model, usageFromRow(r), a.getPricing(), a.getPlan())
	}
	return rows, nil
}

func (a *App) SearchPrompts(query, types, from, to string) ([]map[string]any, error) {
	rows, err := db.SearchPrompts(a.conn, query, types, from, to)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		model, _ := r["model"].(string)
		r["estimated_cost_usd"] = pricing.CostFor(model, usageFromRow(r), a.getPricing(), a.getPlan())
	}
	return rows, nil
}

func (a *App) GetProjects(since, until string) ([]map[string]any, error) {
	return db.ProjectSummary(a.conn, since, until)
}

func (a *App) GetSessions(limit int, since, until string) ([]map[string]any, error) {
	return db.RecentSessions(a.conn, clampLimit(limit, defaultSessionLimit), since, until, "")
}

func (a *App) GetSessionsByProject(limit int, projectSlug, since, until string) ([]map[string]any, error) {
	return db.RecentSessions(a.conn, clampLimit(limit, defaultSessionLimit), since, until, projectSlug)
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
		c := pricing.CostFor(model, usageFromRow(r), a.getPricing(), a.getPlan())
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
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result, nil
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
		"pricing":       a.getPricing(),
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
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return fmt.Errorf("invalid rate: must be a positive finite number")
	}
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
	a.rateMu.Lock()
	if time.Since(a.lastRefresh) < refreshCooldown {
		a.rateMu.Unlock()
		return nil, fmt.Errorf("rate refresh on cooldown: please wait before refreshing again")
	}
	a.lastRefresh = time.Now()
	a.rateMu.Unlock()

	key, err := db.GetExchangeApiKey(a.conn)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, fmt.Errorf("no API key — enter your exchangerate-api.com key first")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://v6.exchangerate-api.com/v6/" + key + "/latest/USD")
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
	a.rateMu.Lock()
	if time.Since(a.lastScan) < scanCooldown {
		a.rateMu.Unlock()
		return scanner.ScanResult{}, fmt.Errorf("scan on cooldown: please wait a moment")
	}
	a.lastScan = time.Now()
	a.rateMu.Unlock()

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
func (a *App) SaveHTMLExport(html string, filename string) (string, error) {
	if filename == "" {
		filename = "session.html"
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export session as HTML",
		DefaultFilename: filename,
		Filters: []runtime.FileFilter{
			{DisplayName: "HTML files (*.html)", Pattern: "*.html"},
		},
	})
	if err != nil || path == "" {
		return "", err
	}
	path = filepath.Clean(path)
	if err := os.WriteFile(path, []byte(html), 0644); err != nil {
		return "", fmt.Errorf("SaveHTMLExport: %w", err)
	}
	return path, nil
}

// OverageInfo holds the rate-limit and authentication details returned by the
// Claude CLI when called with --verbose --output-format stream-json.
type OverageInfo struct {
	Model                 string   `json:"model"`
	ServiceTier           string   `json:"service_tier"`
	RateLimitType         string   `json:"rate_limit_type"`
	OverageStatus         string   `json:"overage_status"`
	OverageDisabledReason string   `json:"overage_disabled_reason"`
	IsUsingOverage        bool     `json:"is_using_overage"`
	Error                 string   `json:"error,omitempty"`
	RawOutput             []string `json:"raw_output,omitempty"`
}

type claudeStreamLine struct {
	Type          string          `json:"type"`
	Message       json.RawMessage `json:"message,omitempty"`
	RateLimitInfo json.RawMessage `json:"rate_limit_info,omitempty"`
	Error         string          `json:"error,omitempty"`
}

type claudeAssistantMsg struct {
	Model string `json:"model"`
	Usage struct {
		ServiceTier string `json:"service_tier"`
	} `json:"usage"`
}

type claudeRateLimitInfo struct {
	RateLimitType         string `json:"rateLimitType"`
	OverageStatus         string `json:"overageStatus"`
	OverageDisabledReason string `json:"overageDisabledReason"`
	IsUsingOverage        bool   `json:"isUsingOverage"`
}

// GetOverageInfo launches the Claude CLI with --verbose --output-format stream-json
// to collect authentication and rate-limit metadata for the current session.
func (a *App) GetOverageInfo() (OverageInfo, error) {
	cmd := exec.Command("claude",
		"-p", "Reply with exactly these three words: oauth cli works",
		"--verbose",
		"--output-format", "stream-json",
		"--model", "sonnet",
	)
	hideConsole(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return OverageInfo{Error: err.Error()}, nil
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return OverageInfo{Error: err.Error()}, nil
	}

	lineScanner := bufio.NewScanner(stdout)
	lineScanner.Buffer(make([]byte, 0, overageScannerBufSize), overageScannerBufSize)

	var result OverageInfo
	var streamError string

	for lineScanner.Scan() {
		line := lineScanner.Text()
		if line == "" {
			continue
		}
		var sl claudeStreamLine
		if err := json.Unmarshal([]byte(line), &sl); err != nil {
			continue
		}
		if sl.Type != "system" {
			result.RawOutput = append(result.RawOutput, line)
		}
		if sl.Error != "" {
			streamError = sl.Error
		}
		switch sl.Type {
		case "assistant":
			if sl.Message != nil {
				var msg claudeAssistantMsg
				if err := json.Unmarshal(sl.Message, &msg); err == nil {
					if msg.Model != "" {
						result.Model = msg.Model
					}
					if msg.Usage.ServiceTier != "" {
						result.ServiceTier = msg.Usage.ServiceTier
					}
				}
			}
		case "rate_limit_event":
			if sl.RateLimitInfo != nil {
				var rli claudeRateLimitInfo
				if err := json.Unmarshal(sl.RateLimitInfo, &rli); err == nil {
					result.RateLimitType = rli.RateLimitType
					result.OverageStatus = rli.OverageStatus
					result.OverageDisabledReason = rli.OverageDisabledReason
					result.IsUsingOverage = rli.IsUsingOverage
				}
			}
		}
	}

	if err := lineScanner.Err(); err != nil {
		streamError = err.Error()
	}
	cmd.Wait() //nolint:errcheck
	result.Error = streamError
	if result.Error == "" {
		if s := strings.TrimSpace(stderrBuf.String()); s != "" && !strings.Contains(strings.ToLower(s), "hook") {
			result.Error = s
		}
	}
	return result, nil
}

// RTKCommandRow is one row from `rtk gain`'s "By Command" table.
type RTKCommandRow struct {
	Rank    int     `json:"rank"`
	Command string  `json:"command"`
	Count   int     `json:"count"`
	Saved   string  `json:"saved"`
	AvgPct  float64 `json:"avg_pct"`
	Time    string  `json:"time"`
	Impact  float64 `json:"impact"` // 0.0–1.0 fraction of filled █ blocks
}

// RTKGainResult holds parsed output from `rtk gain`.
type RTKGainResult struct {
	Efficiency    float64          `json:"efficiency"`
	TotalCommands int              `json:"total_commands"`
	InputTokens   string           `json:"input_tokens"`
	OutputTokens  string           `json:"output_tokens"`
	TokensSaved   string           `json:"tokens_saved"`
	TotalExecTime string           `json:"total_exec_time"`
	Commands      []RTKCommandRow  `json:"commands,omitempty"`
	RawOutput     []string         `json:"raw_output,omitempty"`
	NotFound      bool             `json:"not_found,omitempty"`
	Error         string           `json:"error,omitempty"`
}

var ansiEscRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

var (
	rtkTotalCmdsRe = regexp.MustCompile(`Total commands:\s+(\d+)`)
	rtkInputRe     = regexp.MustCompile(`Input tokens:\s+([\d.]+\s*[KMBkmb]?)`)
	rtkOutputRe    = regexp.MustCompile(`Output tokens:\s+([\d.]+\s*[KMBkmb]?)`)
	rtkSavedRe     = regexp.MustCompile(`Tokens saved:\s+([\d.]+\s*[KMBkmb]?)\s+\((\d+(?:\.\d+)?)%\)`)
	rtkExecTimeRe  = regexp.MustCompile(`Total exec time:\s+(.+)`)
	// Table row: "  1.  command name     count  saved   avg%   time  ██░░"
	rtkTableRowRe = regexp.MustCompile(`^\s*(\d+)\.\s+(.+?)\s{2,}(\d+)\s+([\d.]+[KMBkmb]?)\s+([\d.]+)%\s+(\S+)\s+([█░]+)`)
)

// GetRTKGain runs `rtk gain` and returns fully-parsed token savings data.
func (a *App) GetRTKGain() (RTKGainResult, error) {
	cmd := exec.Command("rtk", "gain")
	hideConsole(cmd)

	out, err := cmd.CombinedOutput()
	clean := ansiEscRe.ReplaceAllString(string(out), "")
	lines := rtkSplitLines(clean)

	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return RTKGainResult{NotFound: true}, nil
		}
		return RTKGainResult{Error: err.Error(), RawOutput: lines}, nil
	}

	result := RTKGainResult{RawOutput: lines}

	for _, line := range lines {
		switch {
		case rtkTotalCmdsRe.MatchString(line):
			m := rtkTotalCmdsRe.FindStringSubmatch(line)
			fmt.Sscanf(m[1], "%d", &result.TotalCommands)
		case rtkInputRe.MatchString(line):
			m := rtkInputRe.FindStringSubmatch(line)
			result.InputTokens = strings.TrimSpace(m[1])
		case rtkOutputRe.MatchString(line):
			m := rtkOutputRe.FindStringSubmatch(line)
			result.OutputTokens = strings.TrimSpace(m[1])
		case rtkSavedRe.MatchString(line):
			m := rtkSavedRe.FindStringSubmatch(line)
			result.TokensSaved = strings.TrimSpace(m[1])
			fmt.Sscanf(m[2], "%f", &result.Efficiency)
		case rtkExecTimeRe.MatchString(line):
			m := rtkExecTimeRe.FindStringSubmatch(line)
			result.TotalExecTime = strings.TrimSpace(m[1])
		default:
			if m := rtkTableRowRe.FindStringSubmatch(line); m != nil {
				row := RTKCommandRow{
					Command: strings.TrimSpace(m[2]),
					Saved:   strings.TrimSpace(m[4]),
					Time:    strings.TrimSpace(m[6]),
				}
				fmt.Sscanf(m[1], "%d", &row.Rank)
				fmt.Sscanf(m[3], "%d", &row.Count)
				fmt.Sscanf(m[5], "%f", &row.AvgPct)
				impactStr := m[7]
				filled := strings.Count(impactStr, "█")
				total := len([]rune(impactStr))
				if total > 0 {
					row.Impact = float64(filled) / float64(total)
				}
				result.Commands = append(result.Commands, row)
			}
		}
	}

	return result, nil
}

func rtkSplitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimRight(l, "\r"); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func (a *App) getPlan() string {
	plan, err := db.GetPlan(a.conn)
	if err != nil {
		log.Printf("getPlan: %v", err)
	}
	return plan
}

func (a *App) getPricing() *pricing.Pricing {
	a.pricingMu.RLock()
	p := a.pricing
	a.pricingMu.RUnlock()
	return p
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
