// Package db provides SQLite schema management and shared query helpers.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS pricing_models (
  model_name       TEXT PRIMARY KEY,
  tier             TEXT NOT NULL DEFAULT '',
  input            REAL NOT NULL DEFAULT 0,
  output           REAL NOT NULL DEFAULT 0,
  cache_read       REAL NOT NULL DEFAULT 0,
  cache_create_5m  REAL NOT NULL DEFAULT 0,
  cache_create_1h  REAL NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS pricing_plans (
  plan_key  TEXT PRIMARY KEY,
  label     TEXT NOT NULL DEFAULT '',
  monthly   REAL NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS exchange_rates (
  currency TEXT PRIMARY KEY,
  rate     REAL NOT NULL DEFAULT 1.0
);
CREATE TABLE IF NOT EXISTS files (
  path        TEXT PRIMARY KEY,
  mtime       REAL    NOT NULL,
  bytes_read  INTEGER NOT NULL,
  scanned_at  REAL    NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
  uuid                    TEXT PRIMARY KEY,
  parent_uuid             TEXT,
  session_id              TEXT NOT NULL,
  project_slug            TEXT NOT NULL,
  cwd                     TEXT,
  git_branch              TEXT,
  cc_version              TEXT,
  entrypoint              TEXT,
  type                    TEXT NOT NULL,
  is_sidechain            INTEGER NOT NULL DEFAULT 0,
  agent_id                TEXT,
  timestamp               TEXT NOT NULL,
  model                   TEXT,
  stop_reason             TEXT,
  prompt_id               TEXT,
  message_id              TEXT,
  input_tokens            INTEGER NOT NULL DEFAULT 0,
  output_tokens           INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens       INTEGER NOT NULL DEFAULT 0,
  cache_create_5m_tokens  INTEGER NOT NULL DEFAULT 0,
  cache_create_1h_tokens  INTEGER NOT NULL DEFAULT 0,
  prompt_text             TEXT,
  prompt_chars            INTEGER,
  tool_calls_json         TEXT
);
CREATE INDEX IF NOT EXISTS idx_messages_session   ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_messages_project   ON messages(project_slug);
CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp);
CREATE INDEX IF NOT EXISTS idx_messages_model     ON messages(model);
CREATE INDEX IF NOT EXISTS idx_messages_msgid     ON messages(session_id, message_id);
CREATE TABLE IF NOT EXISTS tool_calls (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  message_uuid  TEXT    NOT NULL,
  session_id    TEXT    NOT NULL,
  project_slug  TEXT    NOT NULL,
  tool_name     TEXT    NOT NULL,
  target        TEXT,
  result_tokens INTEGER,
  is_error      INTEGER NOT NULL DEFAULT 0,
  timestamp     TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tools_session ON tool_calls(session_id);
CREATE INDEX IF NOT EXISTS idx_tools_name    ON tool_calls(tool_name);
CREATE INDEX IF NOT EXISTS idx_tools_target  ON tool_calls(target);
CREATE TABLE IF NOT EXISTS plan (
  k TEXT PRIMARY KEY,
  v TEXT
);
CREATE TABLE IF NOT EXISTS dismissed_tips (
  tip_key       TEXT PRIMARY KEY,
  dismissed_at  REAL NOT NULL
);
`

// nowFunc is a package-level variable so unit tests can inject a deterministic
// clock without requiring real wall-clock time.
var nowFunc = func() float64 { return float64(time.Now().UnixNano()) / 1e9 }

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*sql.DB, error) {
	dsn := path
	if path != ":memory:" {
		dsn = path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	}
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db.Open %s: %w", path, err)
	}
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("db.Open schema: %w", err)
	}
	for _, m := range []struct{ table, column, def string }{
		{"messages", "thinking_text", "TEXT"},
		{"messages", "tokens_before", "INTEGER"},
		{"messages", "tokens_after", "INTEGER"},
		{"tool_calls", "tool_use_id", "TEXT"},
		{"tool_calls", "input_json", "TEXT"},
		{"tool_calls", "output_text", "TEXT"},
		{"tool_calls", "duration_ms", "INTEGER"},
	} {
		if err := addColumnIfMissing(conn, m.table, m.column, m.def); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

// addColumnIfMissing runs ALTER TABLE ADD COLUMN and ignores duplicate-column errors,
// making it safe to call on every startup regardless of whether the column exists.
func addColumnIfMissing(conn *sql.DB, table, column, def string) error {
	stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, def)
	_, err := conn.Exec(stmt)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return fmt.Errorf("addColumnIfMissing %s.%s: %w", table, column, err)
	}
	return nil
}

// RangeClause builds a WHERE fragment for since/until date filters.
// Returns ("", nil) when both are empty.
// The returned clause begins with " AND " when non-empty.
func RangeClause(since, until, col string) (string, []any) {
	var parts []string
	var args []any
	if since != "" {
		parts = append(parts, col+" >= ?")
		args = append(args, since)
	}
	if until != "" {
		parts = append(parts, col+" < ?")
		args = append(args, until)
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(parts, " AND "), args
}

// slugSep matches characters that Claude Code encodes as "-" in project slugs.
var slugSep = regexp.MustCompile(`[:\\/ ]`)

// slugDashRe splits a slug on runs of dashes for last-segment fallback.
var slugDashRe = regexp.MustCompile(`-+`)

// encodeSlug replicates Claude Code's project-slug encoding.
func encodeSlug(path string) string {
	return slugSep.ReplaceAllString(path, "-")
}

// pathParts trims trailing separators from p and splits it into components,
// also returning the separator character used ("/" or `\`).
func pathParts(p string) (parts []string, sep string) {
	trimmed := strings.TrimRight(p, `/\`)
	sep = "/"
	if strings.Contains(trimmed, `\`) {
		sep = `\`
	}
	return strings.Split(trimmed, sep), sep
}

// walkToRoot walks up the path components of cwd looking for an ancestor
// whose slug encoding equals slug. Returns the basename of that ancestor, or "".
func walkToRoot(cwd, slug string) string {
	if cwd == "" || slug == "" {
		return ""
	}
	parts, sep := pathParts(cwd)
	for i := len(parts); i > 0; i-- {
		candidate := strings.Join(parts[:i], sep)
		if encodeSlug(candidate) == slug && parts[i-1] != "" {
			return parts[i-1]
		}
	}
	return ""
}

// projectNameFor returns a pretty project name from a single cwd + slug.
func projectNameFor(cwd, slug string) string {
	if name := walkToRoot(cwd, slug); name != "" {
		return name
	}
	if cwd != "" {
		parts, _ := pathParts(cwd)
		if tail := parts[len(parts)-1]; tail != "" {
			return tail
		}
	}
	if slug != "" {
		segments := slugDashRe.Split(slug, -1)
		filtered := make([]string, 0, len(segments))
		for _, s := range segments {
			if s != "" {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) > 0 {
			return filtered[len(filtered)-1]
		}
	}
	return slug
}

// BestProjectName returns a human-readable project name from a list of cwds
// and a project slug. It prefers a cwd whose walk-up matches the slug, then
// falls back to projectNameFor on the first cwd.
func BestProjectName(cwds []string, slug string) string {
	filtered := make([]string, 0, len(cwds))
	for _, c := range cwds {
		if c != "" {
			filtered = append(filtered, c)
		}
	}
	for _, cwd := range filtered {
		if name := walkToRoot(cwd, slug); name != "" {
			return name
		}
	}
	var first string
	if len(filtered) > 0 {
		first = filtered[0]
	}
	return projectNameFor(first, slug)
}

// OverviewTotals returns aggregate token counts, session count, and turn count.
func OverviewTotals(conn *sql.DB, since, until string) (map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT COUNT(DISTINCT session_id) AS sessions,
       SUM(CASE WHEN type='user' THEN 1 END) AS turns,
       COALESCE(SUM(input_tokens),0) AS input_tokens,
       COALESCE(SUM(output_tokens),0) AS output_tokens,
       COALESCE(SUM(cache_read_tokens),0) AS cache_read_tokens,
       COALESCE(SUM(cache_create_5m_tokens),0) AS cache_create_5m_tokens,
       COALESCE(SUM(cache_create_1h_tokens),0) AS cache_create_1h_tokens
FROM messages WHERE 1=1` + rng

	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("OverviewTotals: %w", err)
	}
	defer rows.Close()

	results, err := scanMaps(rows)
	if err != nil {
		return nil, fmt.Errorf("OverviewTotals scan: %w", err)
	}
	if len(results) == 0 {
		return map[string]any{}, nil
	}
	return results[0], nil
}

// ExpensivePrompts returns user prompts joined with the following assistant
// turn, ordered by billable_tokens DESC (sort="tokens") or timestamp DESC (sort="recent").
func ExpensivePrompts(conn *sql.DB, limit int, sort string) ([]map[string]any, error) {
	order := "billable_tokens DESC"
	if sort == "recent" {
		order = "u.timestamp DESC"
	}
	q := `
SELECT u.uuid AS user_uuid, u.session_id, u.project_slug, u.timestamp,
       u.prompt_text, u.prompt_chars,
       a.uuid AS assistant_uuid, a.model,
       COALESCE(a.input_tokens,0) AS input_tokens,
       COALESCE(a.output_tokens,0) AS output_tokens,
       COALESCE(a.cache_read_tokens,0) AS cache_read_tokens,
       COALESCE(a.cache_create_5m_tokens,0) AS cache_create_5m_tokens,
       COALESCE(a.cache_create_1h_tokens,0) AS cache_create_1h_tokens,
       COALESCE(a.input_tokens,0)+COALESCE(a.output_tokens,0)
         +COALESCE(a.cache_create_5m_tokens,0)+COALESCE(a.cache_create_1h_tokens,0) AS billable_tokens
FROM messages u
JOIN messages a ON a.parent_uuid = u.uuid AND a.type='assistant'
WHERE u.type='user' AND u.prompt_text IS NOT NULL
ORDER BY ` + order + `
LIMIT ?`

	rows, err := conn.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("ExpensivePrompts: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// ProjectSummary returns per-project aggregates ordered by billable_tokens DESC.
// Each row includes a "project_name" field derived from BestProjectName.
func ProjectSummary(conn *sql.DB, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT project_slug,
       COUNT(DISTINCT session_id) AS sessions,
       SUM(CASE WHEN type='user' THEN 1 END) AS turns,
       COALESCE(SUM(input_tokens),0) AS input_tokens,
       COALESCE(SUM(output_tokens),0) AS output_tokens,
       COALESCE(SUM(input_tokens),0)+COALESCE(SUM(output_tokens),0)
         +COALESCE(SUM(cache_create_5m_tokens),0)
         +COALESCE(SUM(cache_create_1h_tokens),0) AS billable_tokens,
       COALESCE(SUM(cache_read_tokens),0) AS cache_read_tokens
FROM messages WHERE 1=1` + rng + `
GROUP BY project_slug ORDER BY billable_tokens DESC`

	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("ProjectSummary: %w", err)
	}
	defer rows.Close()

	results, err := scanMaps(rows)
	if err != nil {
		return nil, fmt.Errorf("ProjectSummary scan: %w", err)
	}

	for _, r := range results {
		slug, _ := r["project_slug"].(string)
		cwds, err := distinctCWDs(conn, slug)
		if err != nil {
			return nil, err
		}
		r["project_name"] = BestProjectName(cwds, slug)
	}
	return results, nil
}

// RecentSessions returns sessions ordered by last activity, newest first.
func RecentSessions(conn *sql.DB, limit int, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT session_id, project_slug,
       MIN(timestamp) AS started, MAX(timestamp) AS ended,
       SUM(CASE WHEN type='user' THEN 1 END) AS turns,
       COALESCE(SUM(input_tokens),0)+COALESCE(SUM(output_tokens),0) AS tokens
FROM messages WHERE 1=1` + rng + `
GROUP BY session_id ORDER BY ended DESC LIMIT ?`

	rows, err := conn.Query(q, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("RecentSessions: %w", err)
	}
	defer rows.Close()

	results, err := scanMaps(rows)
	if err != nil {
		return nil, fmt.Errorf("RecentSessions scan: %w", err)
	}

	slugCache := map[string]string{}
	for _, r := range results {
		slug, _ := r["project_slug"].(string)
		if _, ok := slugCache[slug]; !ok {
			cwds, err := distinctCWDs(conn, slug)
			if err != nil {
				return nil, err
			}
			slugCache[slug] = BestProjectName(cwds, slug)
		}
		r["project_name"] = slugCache[slug]
	}
	return results, nil
}

// SessionTurns returns all messages in a session ordered by timestamp ASC.
func SessionTurns(conn *sql.DB, sessionID string) ([]map[string]any, error) {
	rows, err := conn.Query(
		`SELECT * FROM messages WHERE session_id=? ORDER BY timestamp ASC`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("SessionTurns: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// ToolBreakdown returns per-tool call counts, excluding _tool_result rows.
func ToolBreakdown(conn *sql.DB, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT tool_name, COUNT(*) AS calls, COALESCE(SUM(result_tokens),0) AS result_tokens
FROM tool_calls WHERE tool_name != '_tool_result'` + rng + `
GROUP BY tool_name ORDER BY calls DESC`

	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("ToolBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// DailyBreakdown returns one row per calendar day with stacked token counts.
func DailyBreakdown(conn *sql.DB, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT substr(timestamp,1,10) AS day,
       COALESCE(SUM(input_tokens),0) AS input_tokens,
       COALESCE(SUM(output_tokens),0) AS output_tokens,
       COALESCE(SUM(cache_read_tokens),0) AS cache_read_tokens,
       COALESCE(SUM(cache_create_5m_tokens),0)+COALESCE(SUM(cache_create_1h_tokens),0) AS cache_create_tokens
FROM messages WHERE timestamp IS NOT NULL` + rng + `
GROUP BY day ORDER BY day ASC`

	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("DailyBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// ModelBreakdown returns per-model token totals for assistant turns.
func ModelBreakdown(conn *sql.DB, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT COALESCE(model,'unknown') AS model, COUNT(*) AS turns,
       COALESCE(SUM(input_tokens),0) AS input_tokens,
       COALESCE(SUM(output_tokens),0) AS output_tokens,
       COALESCE(SUM(cache_read_tokens),0) AS cache_read_tokens,
       COALESCE(SUM(cache_create_5m_tokens),0) AS cache_create_5m_tokens,
       COALESCE(SUM(cache_create_1h_tokens),0) AS cache_create_1h_tokens
FROM messages WHERE type='assistant'` + rng + `
GROUP BY model
ORDER BY (input_tokens+output_tokens+cache_create_5m_tokens+cache_create_1h_tokens) DESC`

	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("ModelBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// SkillBreakdown returns per-skill invocation counts from tool_calls where
// tool_name='Skill'.
func SkillBreakdown(conn *sql.DB, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT target AS skill, COUNT(*) AS invocations,
       COUNT(DISTINCT session_id) AS sessions, MAX(timestamp) AS last_used
FROM tool_calls
WHERE tool_name='Skill' AND target IS NOT NULL AND target!=''` + rng + `
GROUP BY target ORDER BY invocations DESC`

	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("SkillBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// GetPlan returns the stored plan name, defaulting to "api".
func GetPlan(conn *sql.DB) (string, error) {
	var v string
	err := conn.QueryRow(`SELECT v FROM plan WHERE k='plan'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "api", nil
	}
	if err != nil {
		return "", fmt.Errorf("GetPlan: %w", err)
	}
	return v, nil
}

// SetPlan stores the plan name.
func SetPlan(conn *sql.DB, plan string) error {
	_, err := conn.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('plan',?)`, plan)
	if err != nil {
		return fmt.Errorf("SetPlan: %w", err)
	}
	return nil
}

// DismissTip records a dismissed tip key with the current Unix timestamp.
func DismissTip(conn *sql.DB, key string) error {
	_, err := conn.Exec(
		`INSERT OR IGNORE INTO dismissed_tips (tip_key, dismissed_at) VALUES (?,?)`,
		key, nowFunc(),
	)
	if err != nil {
		return fmt.Errorf("DismissTip: %w", err)
	}
	return nil
}

// DismissedTips returns the set of dismissed tip keys.
func DismissedTips(conn *sql.DB) (map[string]bool, error) {
	rows, err := conn.Query(`SELECT tip_key FROM dismissed_tips`)
	if err != nil {
		return nil, fmt.Errorf("DismissedTips: %w", err)
	}
	defer rows.Close()

	result := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("DismissedTips scan: %w", err)
		}
		result[key] = true
	}
	return result, rows.Err()
}

// scanMaps converts sql.Rows into a slice of map[string]any.
func scanMaps(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var results []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = vals[i]
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// GetPricingModels returns all model rate rows ordered by model name.
func GetPricingModels(conn *sql.DB) ([]map[string]any, error) {
	rows, err := conn.Query(
		`SELECT model_name, tier, input, output, cache_read, cache_create_5m, cache_create_1h
		 FROM pricing_models ORDER BY model_name`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetPricingModels: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// UpsertPricingModel inserts or replaces a model rate row.
func UpsertPricingModel(conn *sql.DB, name, tier string, input, output, cacheRead, cache5m, cache1h float64) error {
	_, err := conn.Exec(
		`INSERT OR REPLACE INTO pricing_models
		 (model_name, tier, input, output, cache_read, cache_create_5m, cache_create_1h)
		 VALUES (?,?,?,?,?,?,?)`,
		name, tier, input, output, cacheRead, cache5m, cache1h,
	)
	if err != nil {
		return fmt.Errorf("UpsertPricingModel: %w", err)
	}
	return nil
}

// DeletePricingModel removes a model rate row by name.
func DeletePricingModel(conn *sql.DB, name string) error {
	_, err := conn.Exec(`DELETE FROM pricing_models WHERE model_name=?`, name)
	if err != nil {
		return fmt.Errorf("DeletePricingModel: %w", err)
	}
	return nil
}

// DeleteAllPricingModels removes every model rate row (used for reset-to-defaults).
func DeleteAllPricingModels(conn *sql.DB) error {
	_, err := conn.Exec(`DELETE FROM pricing_models`)
	if err != nil {
		return fmt.Errorf("DeleteAllPricingModels: %w", err)
	}
	return nil
}

// GetPricingPlans returns all plan rows ordered by monthly cost ascending.
func GetPricingPlans(conn *sql.DB) ([]map[string]any, error) {
	rows, err := conn.Query(`SELECT plan_key, label, monthly FROM pricing_plans ORDER BY monthly ASC`)
	if err != nil {
		return nil, fmt.Errorf("GetPricingPlans: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// UpsertPricingPlan inserts or replaces a plan row.
func UpsertPricingPlan(conn *sql.DB, key, label string, monthly float64) error {
	_, err := conn.Exec(
		`INSERT OR REPLACE INTO pricing_plans (plan_key, label, monthly) VALUES (?,?,?)`,
		key, label, monthly,
	)
	if err != nil {
		return fmt.Errorf("UpsertPricingPlan: %w", err)
	}
	return nil
}

// DeletePricingPlan removes a plan row by key.
func DeletePricingPlan(conn *sql.DB, key string) error {
	_, err := conn.Exec(`DELETE FROM pricing_plans WHERE plan_key=?`, key)
	if err != nil {
		return fmt.Errorf("DeletePricingPlan: %w", err)
	}
	return nil
}

// DeleteAllPricingPlans removes every plan row (used for reset-to-defaults).
func DeleteAllPricingPlans(conn *sql.DB) error {
	_, err := conn.Exec(`DELETE FROM pricing_plans`)
	if err != nil {
		return fmt.Errorf("DeleteAllPricingPlans: %w", err)
	}
	return nil
}

// GetCurrency returns the stored currency code, defaulting to "CAD".
func GetCurrency(conn *sql.DB) (string, error) {
	var v string
	err := conn.QueryRow(`SELECT v FROM plan WHERE k='currency'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "CAD", nil
	}
	if err != nil {
		return "", fmt.Errorf("GetCurrency: %w", err)
	}
	return v, nil
}

// SetCurrency stores the currency code.
func SetCurrency(conn *sql.DB, currency string) error {
	_, err := conn.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('currency',?)`, currency)
	if err != nil {
		return fmt.Errorf("SetCurrency: %w", err)
	}
	return nil
}

// IsPricingSeeded returns true if the pricing tables have been populated from defaults.
func IsPricingSeeded(conn *sql.DB) (bool, error) {
	var v string
	err := conn.QueryRow(`SELECT v FROM plan WHERE k='pricing_seeded'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("IsPricingSeeded: %w", err)
	}
	return v == "1", nil
}

// MarkPricingSeeded records that the pricing tables have been seeded.
func MarkPricingSeeded(conn *sql.DB) error {
	_, err := conn.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('pricing_seeded','1')`)
	if err != nil {
		return fmt.Errorf("MarkPricingSeeded: %w", err)
	}
	return nil
}

// GetExchangeRates returns all stored currency→rate pairs (base: USD).
func GetExchangeRates(conn *sql.DB) (map[string]float64, error) {
	rows, err := conn.Query(`SELECT currency, rate FROM exchange_rates`)
	if err != nil {
		return nil, fmt.Errorf("GetExchangeRates: %w", err)
	}
	defer rows.Close()

	rates := map[string]float64{}
	for rows.Next() {
		var currency string
		var rate float64
		if err := rows.Scan(&currency, &rate); err != nil {
			return nil, fmt.Errorf("GetExchangeRates scan: %w", err)
		}
		rates[currency] = rate
	}
	return rates, rows.Err()
}

// SeedExchangeRate inserts a rate only if none exists for that currency (preserves user overrides).
func SeedExchangeRate(conn *sql.DB, currency string, rate float64) error {
	_, err := conn.Exec(`INSERT OR IGNORE INTO exchange_rates (currency, rate) VALUES (?,?)`, currency, rate)
	if err != nil {
		return fmt.Errorf("SeedExchangeRate: %w", err)
	}
	return nil
}

// SetExchangeRate inserts or replaces a rate for a currency (used by user edits and API refresh).
func SetExchangeRate(conn *sql.DB, currency string, rate float64) error {
	_, err := conn.Exec(`INSERT OR REPLACE INTO exchange_rates (currency, rate) VALUES (?,?)`, currency, rate)
	if err != nil {
		return fmt.Errorf("SetExchangeRate: %w", err)
	}
	return nil
}

// GetExchangeApiKey returns the stored exchangerate-api.com API key.
func GetExchangeApiKey(conn *sql.DB) (string, error) {
	var v string
	err := conn.QueryRow(`SELECT v FROM plan WHERE k='exchange_api_key'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("GetExchangeApiKey: %w", err)
	}
	return v, nil
}

// SetExchangeApiKey stores the exchangerate-api.com API key.
func SetExchangeApiKey(conn *sql.DB, key string) error {
	_, err := conn.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('exchange_api_key',?)`, key)
	if err != nil {
		return fmt.Errorf("SetExchangeApiKey: %w", err)
	}
	return nil
}

// distinctCWDs returns all distinct non-null cwd values for a project slug.
func distinctCWDs(conn *sql.DB, slug string) ([]string, error) {
	rows, err := conn.Query(
		`SELECT DISTINCT cwd FROM messages WHERE project_slug=? AND cwd IS NOT NULL`, slug,
	)
	if err != nil {
		return nil, fmt.Errorf("distinctCWDs: %w", err)
	}
	defer rows.Close()

	var cwds []string
	for rows.Next() {
		var cwd string
		if err := rows.Scan(&cwd); err != nil {
			return nil, err
		}
		cwds = append(cwds, cwd)
	}
	return cwds, rows.Err()
}
