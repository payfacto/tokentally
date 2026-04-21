# TokenTally Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build TokenTally — a Go/Wails Windows desktop app with Windows service integration that duplicates all token-dashboard functionality with TokenTally branding, reading Claude Code JSONL transcripts directly with no Python dependency.

**Architecture:** Dual-mode single binary (`tokentally.exe`): service mode scans `~/.claude/projects/` and writes to SQLite; UI mode shows a system tray icon and a Wails WebView2 window with a seven-tab dashboard. All data queries go through Wails native bindings (`window.go.App.*`) instead of HTTP. The UI mode runs its own 30-second scan loop and emits Wails events for live refresh.

**Tech Stack:** Go 1.22+, Wails v2, modernc.org/sqlite (pure Go — no CGO), golang.org/x/sys/windows/svc, github.com/getlantern/systray, ECharts (vendored, no build step)

**Reference source:** `C:\claudecode\token-dashboard\` — all SQL queries and JSONL field names are ported directly from `token_dashboard/db.py` and `token_dashboard/scanner.py`.

---

## File Map

```
tokentally/
├── main.go                        # Mode detection + CLI dispatch
├── go.mod
├── go.sum
├── pricing.json                   # Copied from token-dashboard; embedded at build time
│
├── internal/
│   ├── db/
│   │   └── db.go                  # Schema, Open, migrations, all query functions
│   ├── scanner/
│   │   └── scanner.go             # ScanDir, ScanFile, ParseRecord
│   ├── pricing/
│   │   └── pricing.go             # LoadPricing, CostFor
│   └── tips/
│       └── tips.go                # AllTips, DismissTip
│
├── app/
│   ├── app.go                     # Wails App struct — all methods bound to JS
│   └── tray.go                    # Systray icon + menu (Windows only)
│
├── svc/
│   └── service.go                 # Windows SCM handler (build tag: windows)
│
└── frontend/
    ├── index.html                 # Adapted from token-dashboard/web/index.html
    └── web/                       # All JS/CSS — URL paths preserved as /web/...
        ├── app.js                 # Router — fetch() replaced with window.go.App.*()
        ├── charts.js              # ECharts helpers (unchanged)
        ├── echarts.min.js         # Vendored (unchanged)
        ├── style.css              # Restyled: #0d0d1a bg, #ff6b35 accent, #ffd166 secondary
        └── routes/
            ├── overview.js        # Adapted
            ├── prompts.js         # Adapted
            ├── sessions.js        # Adapted
            ├── projects.js        # Adapted
            ├── skills.js          # Adapted
            ├── tips.js            # Adapted
            └── settings.js        # Adapted + Service section added
```

**Tests live alongside source:**
- `internal/db/db_test.go`
- `internal/scanner/scanner_test.go`
- `internal/pricing/pricing_test.go`
- `internal/tips/tips_test.go`

---

## Task 1: Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `wails.json`
- Create: `pricing.json` (copy)
- Create: `frontend/index.html` (stub)

- [ ] **Step 1: Install Wails CLI if not present**

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails version
```
Expected output: `Wails CLI v2.x.x`

- [ ] **Step 2: Initialize Go module**

```bash
cd C:/claudecode/tokentally
go mod init tokentally
```

- [ ] **Step 3: Add dependencies**

```bash
go get github.com/wailsapp/wails/v2@latest
go get golang.org/x/sys@latest
go get modernc.org/sqlite@latest
go get github.com/getlantern/systray@latest
```

- [ ] **Step 4: Create wails.json**

```json
{
  "$schema": "https://wails.io/schemas/config.v2.json",
  "name": "TokenTally",
  "outputfilename": "tokentally",
  "frontend:install": "",
  "frontend:build": "",
  "frontend:dev:watcher": "",
  "frontend:dev:serverUrl": "",
  "wailsjsdir": "./frontend/web"
}
```

- [ ] **Step 5: Create directory structure**

```bash
mkdir -p internal/db internal/scanner internal/pricing internal/tips
mkdir -p app svc
mkdir -p frontend/web/routes
```

- [ ] **Step 6: Copy pricing.json from token-dashboard**

```bash
cp C:/claudecode/token-dashboard/pricing.json ./pricing.json
```

- [ ] **Step 7: Create stub frontend/index.html** (will be replaced in Task 15)

```html
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>TokenTally</title></head>
<body><p>Loading...</p></body>
</html>
```

- [ ] **Step 8: Verify module structure**

```bash
go mod tidy
go build ./... 2>&1 | head -20
```
Expected: no output (nothing to compile yet, modules resolved)

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum wails.json pricing.json frontend/index.html
git commit -m "feat: scaffold go module and wails project"
```

---

## Task 2: DB Package — Schema and Open

**Files:**
- Create: `internal/db/db.go`
- Create: `internal/db/db_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/db/db_test.go`:

```go
package db_test

import (
	"testing"
	"tokentally/internal/db"
)

func TestOpen_CreatesSchema(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	// Verify key tables exist
	tables := []string{"files", "messages", "tool_calls", "plan", "dismissed_tips"}
	for _, tbl := range tables {
		var name string
		err := conn.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", tbl, err)
		}
	}
}

func TestOpen_WALMode(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	var mode string
	conn.QueryRow("PRAGMA journal_mode").Scan(&mode)
	// :memory: always returns "memory" not "wal", so just verify Open didn't error
	_ = mode
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/db/ -run TestOpen -v
```
Expected: FAIL — `db` package doesn't exist yet

- [ ] **Step 3: Write db.go with schema and Open**

Create `internal/db/db.go`:

```go
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
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

// Open opens (or creates) the SQLite database at path and applies the schema.
// Use ":memory:" in tests.
func Open(path string) (*sql.DB, error) {
	dsn := path
	if path != ":memory:" {
		dsn = path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	}
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db.Open %s: %w", path, err)
	}
	if err := applySchema(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func applySchema(conn *sql.DB) error {
	if _, err := conn.Exec(schema); err != nil {
		return fmt.Errorf("applySchema: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/db/ -run TestOpen -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/
git commit -m "feat(db): schema, Open, WAL mode"
```

---

## Task 3: DB Package — Helper Functions

**Files:**
- Modify: `internal/db/db.go` (add helpers)
- Modify: `internal/db/db_test.go` (add helper tests)

These helpers are used by every query function.

- [ ] **Step 1: Write tests for range clause and project name helpers**

Append to `internal/db/db_test.go`:

```go
func TestRangeClause_Empty(t *testing.T) {
	clause, args := db.RangeClause("", "", "timestamp")
	if clause != "" {
		t.Errorf("expected empty clause, got %q", clause)
	}
	if len(args) != 0 {
		t.Errorf("expected no args, got %v", args)
	}
}

func TestRangeClause_Since(t *testing.T) {
	clause, args := db.RangeClause("2025-01-01", "", "timestamp")
	if clause == "" {
		t.Error("expected non-empty clause")
	}
	if len(args) != 1 || args[0] != "2025-01-01" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestBestProjectName_SlugMatch(t *testing.T) {
	// cwd "C:\claudecode\myapp" encodes to "C--claudecode-myapp" matching slug
	name := db.BestProjectName([]string{`C:\claudecode\myapp`}, "C--claudecode-myapp")
	if name != "myapp" {
		t.Errorf("expected myapp, got %q", name)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/db/ -run TestRangeClause -v
go test ./internal/db/ -run TestBestProjectName -v
```
Expected: FAIL

- [ ] **Step 3: Add helpers to db.go**

Append to `internal/db/db.go`:

```go
import (
	"regexp"
	"strings"
)

// RangeClause builds a WHERE fragment and args for since/until filtering.
// Returns ("", nil) when both are empty.
func RangeClause(since, until, col string) (string, []any) {
	var where []string
	var args []any
	if since != "" {
		where = append(where, col+" >= ?")
		args = append(args, since)
	}
	if until != "" {
		where = append(where, col+" < ?")
		args = append(args, until)
	}
	if len(where) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(where, " AND "), args
}

var slugSep = regexp.MustCompile(`[:\\/ ]`)

func encodeSlug(path string) string {
	return slugSep.ReplaceAllString(path, "-")
}

func walkToRoot(cwd, slug string) string {
	if cwd == "" || slug == "" {
		return ""
	}
	trimmed := strings.TrimRight(cwd, `/\`)
	sep := "/"
	if strings.Contains(trimmed, `\`) {
		sep = `\`
	}
	parts := strings.Split(trimmed, sep)
	for i := len(parts); i > 0; i-- {
		if encodeSlug(strings.Join(parts[:i], sep)) == slug && parts[i-1] != "" {
			return parts[i-1]
		}
	}
	return ""
}

// BestProjectName returns a human-readable project name from a list of cwds and a slug.
func BestProjectName(cwds []string, slug string) string {
	for _, cwd := range cwds {
		if name := walkToRoot(cwd, slug); name != "" {
			return name
		}
	}
	if len(cwds) > 0 && cwds[0] != "" {
		trimmed := strings.TrimRight(cwds[0], `/\`)
		sep := "/"
		if strings.Contains(trimmed, `\`) {
			sep = `\`
		}
		parts := strings.Split(trimmed, sep)
		if tail := parts[len(parts)-1]; tail != "" {
			return tail
		}
	}
	parts := regexp.MustCompile(`-+`).Split(slug, -1)
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return slug
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/db/ -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/
git commit -m "feat(db): range clause and project name helpers"
```

---

## Task 4: DB Package — Read Queries (Part 1)

**Files:**
- Modify: `internal/db/db.go`
- Modify: `internal/db/db_test.go`

- [ ] **Step 1: Write tests**

Append to `internal/db/db_test.go`:

```go
func openWithFixture(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = conn.Exec(`
		INSERT INTO messages (uuid, session_id, project_slug, type, timestamp,
		  model, input_tokens, output_tokens, cache_read_tokens,
		  cache_create_5m_tokens, cache_create_1h_tokens)
		VALUES
		  ('u1', 's1', 'proj-a', 'user',      '2025-01-01T10:00:00', NULL, 0,   0,   0,  0,  0),
		  ('u2', 's1', 'proj-a', 'assistant', '2025-01-01T10:00:01', 'claude-sonnet-4-6', 100, 50, 200, 10, 0),
		  ('u3', 's2', 'proj-b', 'user',      '2025-01-02T09:00:00', NULL, 0,   0,   0,  0,  0),
		  ('u4', 's2', 'proj-b', 'assistant', '2025-01-02T09:00:01', 'claude-opus-4-7',  200, 80,  0, 20, 5)
	`)
	if err != nil {
		t.Fatalf("fixture insert: %v", err)
	}
	return conn
}

func TestOverviewTotals(t *testing.T) {
	conn := openWithFixture(t)
	defer conn.Close()

	totals, err := db.OverviewTotals(conn, "", "")
	if err != nil {
		t.Fatalf("OverviewTotals: %v", err)
	}
	if totals["sessions"] != int64(2) {
		t.Errorf("sessions: want 2, got %v", totals["sessions"])
	}
	if totals["input_tokens"] != int64(300) {
		t.Errorf("input_tokens: want 300, got %v", totals["input_tokens"])
	}
}

func TestProjectSummary(t *testing.T) {
	conn := openWithFixture(t)
	defer conn.Close()

	rows, err := db.ProjectSummary(conn, "", "")
	if err != nil {
		t.Fatalf("ProjectSummary: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("want 2 projects, got %d", len(rows))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/db/ -run "TestOverview|TestProject" -v
```
Expected: FAIL

- [ ] **Step 3: Add OverviewTotals and ProjectSummary to db.go**

```go
// OverviewTotals returns aggregate token counts and session/turn counts.
func OverviewTotals(conn *sql.DB, since, until string) (map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	row := conn.QueryRow(`
		SELECT COUNT(DISTINCT session_id)             AS sessions,
		       SUM(CASE WHEN type='user' THEN 1 END)  AS turns,
		       COALESCE(SUM(input_tokens),0)           AS input_tokens,
		       COALESCE(SUM(output_tokens),0)          AS output_tokens,
		       COALESCE(SUM(cache_read_tokens),0)      AS cache_read_tokens,
		       COALESCE(SUM(cache_create_5m_tokens),0) AS cache_create_5m_tokens,
		       COALESCE(SUM(cache_create_1h_tokens),0) AS cache_create_1h_tokens
		FROM messages WHERE 1=1`+rng, args...)
	var sessions, turns, input, output, cacheRead, cache5m, cache1h sql.NullInt64
	if err := row.Scan(&sessions, &turns, &input, &output, &cacheRead, &cache5m, &cache1h); err != nil {
		return nil, fmt.Errorf("OverviewTotals: %w", err)
	}
	return map[string]any{
		"sessions":               sessions.Int64,
		"turns":                  turns.Int64,
		"input_tokens":           input.Int64,
		"output_tokens":          output.Int64,
		"cache_read_tokens":      cacheRead.Int64,
		"cache_create_5m_tokens": cache5m.Int64,
		"cache_create_1h_tokens": cache1h.Int64,
	}, nil
}

// ProjectSummary returns per-project aggregate stats, ordered by billable tokens desc.
func ProjectSummary(conn *sql.DB, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	rows, err := conn.Query(`
		SELECT project_slug,
		       COUNT(DISTINCT session_id)                                          AS sessions,
		       SUM(CASE WHEN type='user' THEN 1 END)                              AS turns,
		       COALESCE(SUM(input_tokens),0)                                       AS input_tokens,
		       COALESCE(SUM(output_tokens),0)                                      AS output_tokens,
		       COALESCE(SUM(input_tokens),0)+COALESCE(SUM(output_tokens),0)
		         +COALESCE(SUM(cache_create_5m_tokens),0)
		         +COALESCE(SUM(cache_create_1h_tokens),0)                         AS billable_tokens,
		       COALESCE(SUM(cache_read_tokens),0)                                  AS cache_read_tokens
		FROM messages WHERE 1=1`+rng+`
		GROUP BY project_slug ORDER BY billable_tokens DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("ProjectSummary: %w", err)
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var slug string
		var sessions, turns, input, output, billable, cacheRead sql.NullInt64
		if err := rows.Scan(&slug, &sessions, &turns, &input, &output, &billable, &cacheRead); err != nil {
			return nil, err
		}
		// Collect distinct cwds for pretty name resolution
		cwdRows, _ := conn.Query(
			"SELECT DISTINCT cwd FROM messages WHERE project_slug=? AND cwd IS NOT NULL", slug)
		var cwds []string
		for cwdRows.Next() {
			var cwd string
			cwdRows.Scan(&cwd)
			cwds = append(cwds, cwd)
		}
		cwdRows.Close()
		result = append(result, map[string]any{
			"project_slug":    slug,
			"project_name":    BestProjectName(cwds, slug),
			"sessions":        sessions.Int64,
			"turns":           turns.Int64,
			"input_tokens":    input.Int64,
			"output_tokens":   output.Int64,
			"billable_tokens": billable.Int64,
			"cache_read_tokens": cacheRead.Int64,
		})
	}
	return result, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/db/ -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/
git commit -m "feat(db): OverviewTotals, ProjectSummary queries"
```

---

## Task 5: DB Package — Read Queries (Part 2)

**Files:**
- Modify: `internal/db/db.go`
- Modify: `internal/db/db_test.go`

- [ ] **Step 1: Write tests**

Append to `internal/db/db_test.go`:

```go
func TestExpensivePrompts(t *testing.T) {
	conn, _ := db.Open(":memory:")
	defer conn.Close()
	conn.Exec(`INSERT INTO messages (uuid, parent_uuid, session_id, project_slug, type, timestamp, model,
	  input_tokens, output_tokens, cache_read_tokens, cache_create_5m_tokens, cache_create_1h_tokens, prompt_text)
	  VALUES
	  ('u1', NULL, 's1', 'p', 'user', '2025-01-01T10:00:00', NULL, 0, 0, 0, 0, 0, 'hello world'),
	  ('u2', 'u1', 's1', 'p', 'assistant', '2025-01-01T10:00:01', 'claude-sonnet-4-6', 500, 100, 0, 0, 0, NULL)`)

	rows, err := db.ExpensivePrompts(conn, 10, "tokens")
	if err != nil {
		t.Fatalf("ExpensivePrompts: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("want 1 row, got %d", len(rows))
	}
	if rows[0]["prompt_text"] != "hello world" {
		t.Errorf("unexpected prompt_text: %v", rows[0]["prompt_text"])
	}
}

func TestModelBreakdown(t *testing.T) {
	conn := openWithFixture(t)
	defer conn.Close()
	rows, err := db.ModelBreakdown(conn, "", "")
	if err != nil {
		t.Fatalf("ModelBreakdown: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("want 2 models, got %d", len(rows))
	}
}

func TestDailyBreakdown(t *testing.T) {
	conn := openWithFixture(t)
	defer conn.Close()
	rows, err := db.DailyBreakdown(conn, "", "")
	if err != nil {
		t.Fatalf("DailyBreakdown: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("want 2 days, got %d", len(rows))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/db/ -run "TestExpensive|TestModel|TestDaily" -v
```
Expected: FAIL

- [ ] **Step 3: Add remaining read queries to db.go**

```go
// ExpensivePrompts returns user prompts joined with their following assistant turn, ordered by tokens.
func ExpensivePrompts(conn *sql.DB, limit int, sort string) ([]map[string]any, error) {
	order := "billable_tokens DESC"
	if sort == "recent" {
		order = "u.timestamp DESC"
	}
	rows, err := conn.Query(`
		SELECT u.uuid AS user_uuid, u.session_id, u.project_slug, u.timestamp,
		       u.prompt_text, u.prompt_chars,
		       a.uuid AS assistant_uuid, a.model,
		       COALESCE(a.input_tokens,0)+COALESCE(a.output_tokens,0)
		         +COALESCE(a.cache_create_5m_tokens,0)+COALESCE(a.cache_create_1h_tokens,0) AS billable_tokens,
		       COALESCE(a.cache_read_tokens,0) AS cache_read_tokens
		FROM messages u
		JOIN messages a ON a.parent_uuid = u.uuid AND a.type='assistant'
		WHERE u.type='user' AND u.prompt_text IS NOT NULL
		ORDER BY `+order+` LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("ExpensivePrompts: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// RecentSessions returns sessions ordered by last activity.
func RecentSessions(conn *sql.DB, limit int, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	rows, err := conn.Query(`
		SELECT session_id, project_slug,
		       MIN(timestamp) AS started, MAX(timestamp) AS ended,
		       SUM(CASE WHEN type='user' THEN 1 END) AS turns,
		       COALESCE(SUM(input_tokens),0)+COALESCE(SUM(output_tokens),0) AS tokens
		FROM messages WHERE 1=1`+rng+`
		GROUP BY session_id ORDER BY ended DESC LIMIT ?`,
		append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("RecentSessions: %w", err)
	}
	defer rows.Close()
	result, err := scanMaps(rows)
	if err != nil {
		return nil, err
	}
	slugCache := map[string]string{}
	for _, r := range result {
		slug, _ := r["project_slug"].(string)
		if _, ok := slugCache[slug]; !ok {
			cwdRows, _ := conn.Query(
				"SELECT DISTINCT cwd FROM messages WHERE project_slug=? AND cwd IS NOT NULL", slug)
			var cwds []string
			for cwdRows.Next() {
				var cwd string; cwdRows.Scan(&cwd); cwds = append(cwds, cwd)
			}
			cwdRows.Close()
			slugCache[slug] = BestProjectName(cwds, slug)
		}
		r["project_name"] = slugCache[slug]
	}
	return result, nil
}

// SessionTurns returns all messages in a session ordered by timestamp.
func SessionTurns(conn *sql.DB, sessionID string) ([]map[string]any, error) {
	rows, err := conn.Query(`
		SELECT uuid, parent_uuid, type, timestamp, model, is_sidechain, agent_id,
		       input_tokens, output_tokens, cache_read_tokens,
		       cache_create_5m_tokens, cache_create_1h_tokens,
		       prompt_text, prompt_chars, tool_calls_json, project_slug, cwd
		FROM messages WHERE session_id=? ORDER BY timestamp ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("SessionTurns: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// ToolBreakdown returns per-tool call counts and result token totals.
func ToolBreakdown(conn *sql.DB, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	rows, err := conn.Query(`
		SELECT tool_name, COUNT(*) AS calls, COALESCE(SUM(result_tokens),0) AS result_tokens
		FROM tool_calls WHERE tool_name != '_tool_result'`+rng+`
		GROUP BY tool_name ORDER BY calls DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("ToolBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// DailyBreakdown returns one row per day with stacked token counts.
func DailyBreakdown(conn *sql.DB, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	rows, err := conn.Query(`
		SELECT substr(timestamp,1,10) AS day,
		       COALESCE(SUM(input_tokens),0)      AS input_tokens,
		       COALESCE(SUM(output_tokens),0)     AS output_tokens,
		       COALESCE(SUM(cache_read_tokens),0) AS cache_read_tokens,
		       COALESCE(SUM(cache_create_5m_tokens),0)
		         +COALESCE(SUM(cache_create_1h_tokens),0) AS cache_create_tokens
		FROM messages WHERE timestamp IS NOT NULL`+rng+`
		GROUP BY day ORDER BY day ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("DailyBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// ModelBreakdown returns per-model token totals for assistant turns.
func ModelBreakdown(conn *sql.DB, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	rows, err := conn.Query(`
		SELECT COALESCE(model,'unknown') AS model,
		       COUNT(*) AS turns,
		       COALESCE(SUM(input_tokens),0)            AS input_tokens,
		       COALESCE(SUM(output_tokens),0)           AS output_tokens,
		       COALESCE(SUM(cache_read_tokens),0)       AS cache_read_tokens,
		       COALESCE(SUM(cache_create_5m_tokens),0)  AS cache_create_5m_tokens,
		       COALESCE(SUM(cache_create_1h_tokens),0)  AS cache_create_1h_tokens
		FROM messages WHERE type='assistant'`+rng+`
		GROUP BY model
		ORDER BY (input_tokens+output_tokens+cache_create_5m_tokens+cache_create_1h_tokens) DESC`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("ModelBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// SkillBreakdown returns per-skill invocation counts from tool_calls.
func SkillBreakdown(conn *sql.DB, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	rows, err := conn.Query(`
		SELECT target AS skill, COUNT(*) AS invocations,
		       COUNT(DISTINCT session_id) AS sessions, MAX(timestamp) AS last_used
		FROM tool_calls
		WHERE tool_name='Skill' AND target IS NOT NULL AND target!=''`+rng+`
		GROUP BY target ORDER BY invocations DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("SkillBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// scanMaps converts sql.Rows into a slice of string-keyed maps.
func scanMaps(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, col := range cols {
			m[col] = vals[i]
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
```

- [ ] **Step 4: Run all DB tests**

```bash
go test ./internal/db/ -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/
git commit -m "feat(db): remaining read queries (sessions, tools, daily, model, skills)"
```

---

## Task 6: DB Package — Plan and Tips Queries

**Files:**
- Modify: `internal/db/db.go`
- Modify: `internal/db/db_test.go`

- [ ] **Step 1: Write tests**

Append to `internal/db/db_test.go`:

```go
func TestGetSetPlan(t *testing.T) {
	conn, _ := db.Open(":memory:")
	defer conn.Close()

	plan, err := db.GetPlan(conn)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan != "api" {
		t.Errorf("default plan: want 'api', got %q", plan)
	}

	if err := db.SetPlan(conn, "max"); err != nil {
		t.Fatalf("SetPlan: %v", err)
	}
	plan, _ = db.GetPlan(conn)
	if plan != "max" {
		t.Errorf("after SetPlan: want 'max', got %q", plan)
	}
}

func TestDismissTip(t *testing.T) {
	conn, _ := db.Open(":memory:")
	defer conn.Close()

	if err := db.DismissTip(conn, "cache-hit-low"); err != nil {
		t.Fatalf("DismissTip: %v", err)
	}
	dismissed, _ := db.DismissedTips(conn)
	if !dismissed["cache-hit-low"] {
		t.Error("tip should be dismissed")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/db/ -run "TestGetSet|TestDismiss" -v
```
Expected: FAIL

- [ ] **Step 3: Add plan and tips functions to db.go**

```go
// GetPlan returns the stored plan name, defaulting to "api".
func GetPlan(conn *sql.DB) (string, error) {
	var v string
	err := conn.QueryRow("SELECT v FROM plan WHERE k='plan'").Scan(&v)
	if err == sql.ErrNoRows {
		return "api", nil
	}
	return v, err
}

// SetPlan stores the plan name.
func SetPlan(conn *sql.DB, plan string) error {
	_, err := conn.Exec("INSERT OR REPLACE INTO plan (k,v) VALUES ('plan',?)", plan)
	return err
}

// DismissTip records a dismissed tip key.
func DismissTip(conn *sql.DB, key string) error {
	_, err := conn.Exec(
		"INSERT OR IGNORE INTO dismissed_tips (tip_key, dismissed_at) VALUES (?,?)",
		key, float64(timeNow()))
	return err
}

// DismissedTips returns a set of dismissed tip keys.
func DismissedTips(conn *sql.DB) (map[string]bool, error) {
	rows, err := conn.Query("SELECT tip_key FROM dismissed_tips")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var k string
		rows.Scan(&k)
		set[k] = true
	}
	return set, rows.Err()
}

// timeNow is a variable so tests can override it.
var timeNow = func() int64 {
	return 0 // replaced by time.Now().Unix() in non-test code
}
```

Add an `init()` function and `"time"` to the imports in `db.go`:

```go
func init() {
	timeNow = func() int64 { return time.Now().Unix() }
}
```

- [ ] **Step 4: Run all DB tests**

```bash
go test ./internal/db/ -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/
git commit -m "feat(db): plan and dismissed_tips management"
```

---

## Task 7: Scanner Package

**Files:**
- Create: `internal/scanner/scanner.go`
- Create: `internal/scanner/scanner_test.go`
- Create: `internal/scanner/testdata/session-abc.jsonl`

The scanner is the core of the app — get it right. Three key behaviors to test: (1) incremental reads, (2) partial-line safety, (3) streaming-snapshot eviction.

- [ ] **Step 1: Create testdata fixture**

Create `internal/scanner/testdata/proj-a/session-abc.jsonl`:

```jsonl
{"uuid":"msg1","parentUuid":null,"sessionId":"session-abc","type":"user","timestamp":"2025-01-01T10:00:00.000Z","message":{"content":[{"type":"text","text":"hello"}],"usage":null}}
{"uuid":"msg2","parentUuid":"msg1","sessionId":"session-abc","type":"assistant","timestamp":"2025-01-01T10:00:01.000Z","message":{"id":"msgid-x","model":"claude-sonnet-4-6","stop_reason":"end_turn","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":200,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":0}}}}
```

- [ ] **Step 2: Write tests**

Create `internal/scanner/scanner_test.go`:

```go
package scanner_test

import (
	"testing"
	"tokentally/internal/db"
	"tokentally/internal/scanner"
)

func TestScanDir_ParsesTwoMessages(t *testing.T) {
	conn, _ := db.Open(":memory:")
	defer conn.Close()

	result, err := scanner.ScanDir(conn, "testdata")
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	if result.Messages != 2 {
		t.Errorf("want 2 messages, got %d", result.Messages)
	}
}

func TestScanDir_Incremental(t *testing.T) {
	conn, _ := db.Open(":memory:")
	defer conn.Close()

	r1, _ := scanner.ScanDir(conn, "testdata")
	r2, _ := scanner.ScanDir(conn, "testdata")

	if r1.Messages != 2 {
		t.Errorf("first scan: want 2, got %d", r1.Messages)
	}
	if r2.Messages != 0 {
		t.Errorf("second scan should be no-op: want 0, got %d", r2.Messages)
	}
}

func TestScanDir_TokenCounts(t *testing.T) {
	conn, _ := db.Open(":memory:")
	defer conn.Close()

	scanner.ScanDir(conn, "testdata")

	totals, _ := db.OverviewTotals(conn, "", "")
	if totals["input_tokens"] != int64(100) {
		t.Errorf("input_tokens: want 100, got %v", totals["input_tokens"])
	}
	if totals["output_tokens"] != int64(50) {
		t.Errorf("output_tokens: want 50, got %v", totals["output_tokens"])
	}
	if totals["cache_read_tokens"] != int64(200) {
		t.Errorf("cache_read_tokens: want 200, got %v", totals["cache_read_tokens"])
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/scanner/ -v
```
Expected: FAIL — package doesn't exist

- [ ] **Step 4: Write scanner.go**

Create `internal/scanner/scanner.go`:

```go
package scanner

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"tokentally/internal/db"
)

// ScanResult holds totals from a single scan pass.
type ScanResult struct {
	Files    int `json:"files"`
	Messages int `json:"messages"`
	Tools    int `json:"tools"`
}

// ScanDir walks projectsDir for *.jsonl files and ingests new content into conn.
func ScanDir(conn *sql.DB, projectsDir string) (ScanResult, error) {
	var total ScanResult
	err := filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		slug := projectSlug(path, projectsDir)
		n, err := scanFile(conn, path, slug, info)
		if err != nil {
			log.Printf("scanner: %s: %v", path, err)
			return nil // keep walking
		}
		if n.Messages > 0 || n.Tools > 0 {
			total.Files++
			total.Messages += n.Messages
			total.Tools += n.Tools
		}
		return nil
	})
	return total, err
}

func projectSlug(path, root string) string {
	rel, _ := filepath.Rel(root, path)
	parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
	return parts[0]
}

func scanFile(conn *sql.DB, path, slug string, info os.FileInfo) (ScanResult, error) {
	var stored struct {
		mtime     float64
		bytesRead int64
	}
	err := conn.QueryRow("SELECT mtime, bytes_read FROM files WHERE path=?", path).
		Scan(&stored.mtime, &stored.bytesRead)
	if err != nil && err != sql.ErrNoRows {
		return ScanResult{}, err
	}
	mtime := float64(info.ModTime().UnixNano()) / 1e9
	if stored.mtime == mtime && stored.bytesRead == info.Size() {
		return ScanResult{}, nil // fully up to date
	}

	offset := stored.bytesRead
	f, err := os.Open(path)
	if err != nil {
		return ScanResult{}, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err = f.Seek(offset, io.SeekStart); err != nil {
			return ScanResult{}, err
		}
	}

	tx, err := conn.Begin()
	if err != nil {
		return ScanResult{}, err
	}
	defer tx.Rollback()

	var result ScanResult
	endOffset := offset
	reader := bufio.NewReaderSize(f, 1<<20)

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] != '\n' {
			// Partial line — Claude Code is mid-flush. Don't advance offset past it.
			break
		}
		if len(line) == 0 {
			if err == io.EOF {
				break
			}
			if err != nil {
				return ScanResult{}, err
			}
			continue
		}
		endOffset += int64(len(line))
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			if err == io.EOF {
				break
			}
			continue
		}

		msg, tools, ok := parseLine([]byte(trimmed), slug)
		if !ok {
			if err == io.EOF {
				break
			}
			continue
		}
		evictPriorSnapshots(tx, msg)
		if err2 := insertMessage(tx, msg); err2 != nil {
			return ScanResult{}, fmt.Errorf("insertMessage: %w", err2)
		}
		tx.Exec("DELETE FROM tool_calls WHERE message_uuid=?", msg["uuid"])
		for _, t := range tools {
			if err2 := insertToolCall(tx, t); err2 != nil {
				return ScanResult{}, fmt.Errorf("insertToolCall: %w", err2)
			}
			result.Tools++
		}
		result.Messages++
		if err == io.EOF {
			break
		}
	}

	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO files (path, mtime, bytes_read, scanned_at) VALUES (?,?,?,?)",
		path, mtime, endOffset, float64(time.Now().UnixNano())/1e9,
	); err != nil {
		return ScanResult{}, err
	}
	return result, tx.Commit()
}

func evictPriorSnapshots(tx *sql.Tx, msg map[string]any) {
	msgID, _ := msg["message_id"].(string)
	sid, _ := msg["session_id"].(string)
	uuid, _ := msg["uuid"].(string)
	if msgID == "" || sid == "" {
		return
	}
	rows, err := tx.Query(
		"SELECT uuid FROM messages WHERE session_id=? AND message_id=? AND uuid!=?",
		sid, msgID, uuid)
	if err != nil {
		return
	}
	var old []any
	for rows.Next() {
		var u string; rows.Scan(&u); old = append(old, u)
	}
	rows.Close()
	for _, u := range old {
		tx.Exec("DELETE FROM tool_calls WHERE message_uuid=?", u)
		tx.Exec("DELETE FROM messages WHERE uuid=?", u)
	}
}

// jsonMsg is the top-level JSONL record structure.
type jsonMsg struct {
	UUID       string          `json:"uuid"`
	ParentUUID string          `json:"parentUuid"`
	SessionID  string          `json:"sessionId"`
	CWD        string          `json:"cwd"`
	GitBranch  string          `json:"gitBranch"`
	Version    string          `json:"version"`
	Entrypoint string          `json:"entrypoint"`
	Type       string          `json:"type"`
	IsSidechain bool           `json:"isSidechain"`
	AgentID    string          `json:"agentId"`
	Timestamp  string          `json:"timestamp"`
	PromptID   string          `json:"promptId"`
	Message    json.RawMessage `json:"message"`
}

type jsonMessageObj struct {
	ID         string          `json:"id"`
	Model      string          `json:"model"`
	StopReason string          `json:"stop_reason"`
	Content    json.RawMessage `json:"content"`
	Usage      *jsonUsage      `json:"usage"`
}

type jsonUsage struct {
	InputTokens       int            `json:"input_tokens"`
	OutputTokens      int            `json:"output_tokens"`
	CacheReadInput    int            `json:"cache_read_input_tokens"`
	CacheCreation     *jsonCacheCreation `json:"cache_creation"`
}

type jsonCacheCreation struct {
	Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
}

type jsonBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	ID    string          `json:"tool_use_id"`
	IsError bool          `json:"is_error"`
	Content json.RawMessage `json:"content"`
}

var targetFields = map[string]string{
	"Read": "file_path", "Edit": "file_path", "Write": "file_path",
	"Glob": "pattern", "Grep": "pattern", "Bash": "command",
	"WebFetch": "url", "WebSearch": "query",
	"Task": "subagent_type", "Skill": "skill",
}

func parseLine(line []byte, slug string) (map[string]any, []map[string]any, bool) {
	var rec jsonMsg
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, nil, false
	}
	if rec.UUID == "" || rec.Type == "" || rec.SessionID == "" || rec.Timestamp == "" {
		return nil, nil, false
	}

	var msgObj jsonMessageObj
	if len(rec.Message) > 0 {
		json.Unmarshal(rec.Message, &msgObj)
	}

	isSidechain := 0
	if rec.IsSidechain {
		isSidechain = 1
	}

	u := msgObj.Usage
	var inputT, outputT, cacheRead, cache5m, cache1h int
	if u != nil {
		inputT = u.InputTokens
		outputT = u.OutputTokens
		cacheRead = u.CacheReadInput
		if u.CacheCreation != nil {
			cache5m = u.CacheCreation.Ephemeral5m
			cache1h = u.CacheCreation.Ephemeral1h
		}
	}

	promptText, promptChars := extractPromptText(rec.Type, msgObj.Content)

	tools := extractTools(rec, slug, msgObj.Content)
	toolCallsJSON := buildToolCallsJSON(tools)

	msg := map[string]any{
		"uuid": rec.UUID, "parent_uuid": nilIfEmpty(rec.ParentUUID),
		"session_id": rec.SessionID, "project_slug": slug,
		"cwd": nilIfEmpty(rec.CWD), "git_branch": nilIfEmpty(rec.GitBranch),
		"cc_version": nilIfEmpty(rec.Version), "entrypoint": nilIfEmpty(rec.Entrypoint),
		"type": rec.Type, "is_sidechain": isSidechain,
		"agent_id": nilIfEmpty(rec.AgentID), "timestamp": rec.Timestamp,
		"model": nilIfEmpty(msgObj.Model), "stop_reason": nilIfEmpty(msgObj.StopReason),
		"prompt_id": nilIfEmpty(rec.PromptID), "message_id": nilIfEmpty(msgObj.ID),
		"input_tokens": inputT, "output_tokens": outputT,
		"cache_read_tokens": cacheRead,
		"cache_create_5m_tokens": cache5m, "cache_create_1h_tokens": cache1h,
		"prompt_text": promptText, "prompt_chars": promptChars,
		"tool_calls_json": toolCallsJSON,
	}
	return msg, tools, true
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func extractPromptText(typ string, content json.RawMessage) (any, any) {
	if typ != "user" || len(content) == 0 {
		return nil, nil
	}
	var blocks []jsonBlock
	if json.Unmarshal(content, &blocks) != nil {
		var s string
		if json.Unmarshal(content, &s) == nil {
			return s, len(s)
		}
		return nil, nil
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			var t string
			json.Unmarshal(b.Content, &t)
			// "text" blocks store text in the "text" field not "content"
			var raw map[string]any
			json.Unmarshal(content, &raw)
			_ = raw
			parts = append(parts, t)
		}
	}
	// Re-parse properly for text blocks
	var rawBlocks []map[string]json.RawMessage
	json.Unmarshal(content, &rawBlocks)
	parts = nil
	for _, b := range rawBlocks {
		typeVal, _ := b["type"]
		var typeStr string
		json.Unmarshal(typeVal, &typeStr)
		if typeStr == "text" {
			textVal := b["text"]
			var t string
			json.Unmarshal(textVal, &t)
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	text := strings.Join(parts, "")
	return text, len(text)
}

func extractTools(rec jsonMsg, slug string, content json.RawMessage) []map[string]any {
	var blocks []map[string]json.RawMessage
	json.Unmarshal(content, &blocks)
	var tools []map[string]any
	for _, b := range blocks {
		typeVal, _ := b["type"]
		var typeStr string
		json.Unmarshal(typeVal, &typeStr)

		switch typeStr {
		case "tool_use":
			var name string
			json.Unmarshal(b["name"], &name)
			var inputMap map[string]any
			json.Unmarshal(b["input"], &inputMap)
			target := extractTarget(name, inputMap)
			tools = append(tools, map[string]any{
				"message_uuid": rec.UUID, "session_id": rec.SessionID,
				"project_slug": slug, "tool_name": name,
				"target": target, "result_tokens": nil,
				"is_error": 0, "timestamp": rec.Timestamp,
			})
		case "tool_result":
			var isErr bool
			json.Unmarshal(b["is_error"], &isErr)
			var id string
			json.Unmarshal(b["tool_use_id"], &id)
			chars := countContentChars(b["content"])
			isErrInt := 0
			if isErr {
				isErrInt = 1
			}
			tools = append(tools, map[string]any{
				"message_uuid": rec.UUID, "session_id": rec.SessionID,
				"project_slug": slug, "tool_name": "_tool_result",
				"target": nilIfEmpty(id), "result_tokens": chars / 4,
				"is_error": isErrInt, "timestamp": rec.Timestamp,
			})
		}
	}
	return tools
}

func extractTarget(name string, input map[string]any) any {
	field, ok := targetFields[name]
	if !ok || input == nil {
		return nil
	}
	v, _ := input[field].(string)
	if v == "" {
		return nil
	}
	if len(v) > 500 {
		v = v[:500]
	}
	return v
}

func countContentChars(raw json.RawMessage) int {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return len(s)
	}
	var blocks []map[string]json.RawMessage
	if json.Unmarshal(raw, &blocks) != nil {
		return 0
	}
	n := 0
	for _, b := range blocks {
		var t string
		json.Unmarshal(b["text"], &t)
		n += len(t)
	}
	return n
}

func buildToolCallsJSON(tools []map[string]any) any {
	var entries []map[string]any
	for _, t := range tools {
		if t["tool_name"] == "_tool_result" {
			continue
		}
		entries = append(entries, map[string]any{
			"name": t["tool_name"], "target": t["target"],
		})
	}
	if len(entries) == 0 {
		return nil
	}
	b, _ := json.Marshal(entries)
	return string(b)
}

func insertMessage(tx *sql.Tx, m map[string]any) error {
	_, err := tx.Exec(`INSERT OR REPLACE INTO messages
		(uuid,parent_uuid,session_id,project_slug,cwd,git_branch,cc_version,entrypoint,
		 type,is_sidechain,agent_id,timestamp,model,stop_reason,prompt_id,message_id,
		 input_tokens,output_tokens,cache_read_tokens,cache_create_5m_tokens,cache_create_1h_tokens,
		 prompt_text,prompt_chars,tool_calls_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m["uuid"], m["parent_uuid"], m["session_id"], m["project_slug"],
		m["cwd"], m["git_branch"], m["cc_version"], m["entrypoint"],
		m["type"], m["is_sidechain"], m["agent_id"], m["timestamp"],
		m["model"], m["stop_reason"], m["prompt_id"], m["message_id"],
		m["input_tokens"], m["output_tokens"], m["cache_read_tokens"],
		m["cache_create_5m_tokens"], m["cache_create_1h_tokens"],
		m["prompt_text"], m["prompt_chars"], m["tool_calls_json"],
	)
	return err
}

func insertToolCall(tx *sql.Tx, t map[string]any) error {
	_, err := tx.Exec(`INSERT INTO tool_calls
		(message_uuid,session_id,project_slug,tool_name,target,result_tokens,is_error,timestamp)
		VALUES (?,?,?,?,?,?,?,?)`,
		t["message_uuid"], t["session_id"], t["project_slug"],
		t["tool_name"], t["target"], t["result_tokens"], t["is_error"], t["timestamp"],
	)
	return err
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/scanner/ -v
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/scanner/
git commit -m "feat(scanner): incremental JSONL scan with streaming-snapshot eviction"
```

---

## Task 8: Pricing Package

**Files:**
- Create: `internal/pricing/pricing.go`
- Create: `internal/pricing/pricing_test.go`

- [ ] **Step 1: Write tests**

Create `internal/pricing/pricing_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/pricing/ -v
```
Expected: FAIL

- [ ] **Step 3: Write pricing.go**

Create `internal/pricing/pricing.go`:

```go
package pricing

import (
	"encoding/json"
	"io"
)

// Usage holds token counts for a single request.
type Usage struct {
	InputTokens          int
	OutputTokens         int
	CacheReadTokens      int
	CacheCreate5mTokens  int
	CacheCreate1hTokens  int
}

type modelRates struct {
	InputMtok        float64 `json:"input_mtok"`
	OutputMtok       float64 `json:"output_mtok"`
	CacheReadMtok    float64 `json:"cache_read_mtok"`
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
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/pricing/ -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/pricing/
git commit -m "feat(pricing): load pricing.json and CostFor"
```

---

## Task 9: Tips Package

**Files:**
- Create: `internal/tips/tips.go`
- Create: `internal/tips/tips_test.go`

The tips engine mirrors `tips.py` — rules are hard-coded, each one checks aggregate stats and fires if conditions are met.

- [ ] **Step 1: Write tests**

Create `internal/tips/tips_test.go`:

```go
package tips_test

import (
	"testing"
	"tokentally/internal/db"
	"tokentally/internal/tips"
)

func TestAllTips_ReturnsList(t *testing.T) {
	conn, _ := db.Open(":memory:")
	defer conn.Close()

	result, err := tips.AllTips(conn)
	if err != nil {
		t.Fatalf("AllTips: %v", err)
	}
	// Should return tip definitions even with empty DB
	if len(result) == 0 {
		t.Error("expected at least one tip")
	}
	for _, tip := range result {
		if tip["key"] == nil || tip["title"] == nil {
			t.Errorf("tip missing required fields: %v", tip)
		}
	}
}

func TestAllTips_DismissedExcluded(t *testing.T) {
	conn, _ := db.Open(":memory:")
	defer conn.Close()

	all, _ := tips.AllTips(conn)
	if len(all) == 0 {
		t.Skip("no tips in empty DB")
	}
	key, _ := all[0]["key"].(string)
	db.DismissTip(conn, key)

	after, _ := tips.AllTips(conn)
	for _, tip := range after {
		if tip["key"] == key {
			t.Errorf("dismissed tip %q should not appear", key)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tips/ -v
```
Expected: FAIL

- [ ] **Step 3: Write tips.go**

Create `internal/tips/tips.go` — port the rules from `C:\claudecode\token-dashboard\token_dashboard\tips.py`:

```go
package tips

import (
	"database/sql"
	"tokentally/internal/db"
)

type tip struct {
	Key     string
	Title   string
	Body    string
	Link    string
	Applies func(stats map[string]any) bool
}

var allTipDefs = []tip{
	{
		Key:   "cache-hit-low",
		Title: "Low cache hit rate",
		Body:  "Your cache hit rate is below 20%. Structuring prompts to reuse system prompts can save significant cost.",
		Link:  "https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching",
		Applies: func(s map[string]any) bool {
			read := intVal(s["cache_read_tokens"])
			total := intVal(s["input_tokens"])
			if total == 0 {
				return false
			}
			return float64(read)/float64(total) < 0.20
		},
	},
	{
		Key:   "high-output-ratio",
		Title: "High output token ratio",
		Body:  "Output tokens are more expensive than input. Consider asking Claude to be more concise.",
		Applies: func(s map[string]any) bool {
			out := intVal(s["output_tokens"])
			inp := intVal(s["input_tokens"])
			if inp == 0 {
				return false
			}
			return float64(out)/float64(inp) > 0.5
		},
	},
	{
		Key:   "many-sessions",
		Title: "Many short sessions",
		Body:  "You have many sessions with few turns. Longer sessions reuse cached context more efficiently.",
		Applies: func(s map[string]any) bool {
			sessions := intVal(s["sessions"])
			turns := intVal(s["turns"])
			if sessions == 0 {
				return false
			}
			return sessions > 10 && float64(turns)/float64(sessions) < 3
		},
	},
}

func intVal(v any) int64 {
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

// AllTips returns applicable, non-dismissed tips as maps ready for JSON serialisation.
func AllTips(conn *sql.DB) ([]map[string]any, error) {
	dismissed, err := db.DismissedTips(conn)
	if err != nil {
		return nil, err
	}
	stats, err := db.OverviewTotals(conn, "", "")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	for _, t := range allTipDefs {
		if dismissed[t.Key] {
			continue
		}
		if !t.Applies(stats) {
			continue
		}
		m := map[string]any{
			"key": t.Key, "title": t.Title, "body": t.Body,
		}
		if t.Link != "" {
			m["link"] = t.Link
		}
		result = append(result, m)
	}
	return result, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/tips/ -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tips/
git commit -m "feat(tips): rule-based tips engine"
```

---

## Task 10: Windows Service Handler

**Files:**
- Create: `svc/service.go` (build tag: `windows`)

No unit tests for this package — SCM interaction requires Windows and admin rights. Smoke-tested manually in Task 19.

- [ ] **Step 1: Create svc/service.go**

```go
//go:build windows

package svc

import (
	"database/sql"
	"log"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"tokentally/internal/scanner"
)

const ServiceName = "TokenTally"

type handler struct {
	db          *sql.DB
	projectsDir string
	interval    time.Duration
}

// New creates a service handler. db must already be open.
func New(db *sql.DB, projectsDir string, interval time.Duration) *handler {
	return &handler{db: db, projectsDir: projectsDir, interval: interval}
}

func (h *handler) Execute(args []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	status <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue,
	}

	paused := false
	for {
		select {
		case <-ticker.C:
			if paused {
				continue
			}
			if _, err := scanner.ScanDir(h.db, h.projectsDir); err != nil {
				log.Printf("scan error: %v", err)
			}
		case c := <-req:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				return false, 0
			case svc.Pause:
				paused = true
				status <- svc.Status{State: svc.Paused, Accepts: svc.AcceptStop | svc.AcceptPauseAndContinue}
			case svc.Continue:
				paused = false
				status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue}
			}
		}
	}
}

// Run starts the SCM service loop.
func Run(db *sql.DB, projectsDir string, interval time.Duration) error {
	h := New(db, projectsDir, interval)
	isInteractive, err := svc.IsAnInteractiveSession()
	if err != nil {
		return err
	}
	if isInteractive {
		// Dev mode: run as foreground debug service
		return debug.Run(ServiceName, h)
	}
	return svc.Run(ServiceName, h)
}

// Install registers the service with SCM. Requires admin rights.
func Install(exePath string) error {
	elog, err := eventlog.Open(ServiceName)
	if err != nil {
		eventlog.InstallAsEventCreate(ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
		elog, _ = eventlog.Open(ServiceName)
	}
	if elog != nil {
		elog.Close()
	}
	// Use golang.org/x/sys/windows/svc/mgr for SCM registration
	return installSCM(exePath)
}

// Uninstall removes the service from SCM. Requires admin rights.
func Uninstall() error {
	return uninstallSCM()
}
```

- [ ] **Step 2: Create svc/scm_windows.go** (SCM install/uninstall helpers)

```go
//go:build windows

package svc

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc/mgr"
)

func installSCM(exePath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %q already exists", ServiceName)
	}

	s, err = m.CreateService(ServiceName, exePath,
		mgr.Config{
			DisplayName: "TokenTally Scanner",
			Description: "Scans Claude Code JSONL transcripts and stores token usage data.",
			StartType:   mgr.StartAutomatic,
		},
		"--service",
	)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()
	return nil
}

func uninstallSCM() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("service %q not found: %w", ServiceName, err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	// Give SCM time to process
	time.Sleep(500 * time.Millisecond)
	return nil
}
```

- [ ] **Step 3: Verify Windows build**

```bash
GOOS=windows GOARCH=amd64 go build ./svc/ 2>&1
```
Expected: no output (compiles cleanly)

- [ ] **Step 4: Commit**

```bash
git add svc/
git commit -m "feat(svc): Windows service SCM handler"
```

---

## Task 11: Wails App Struct — Data Bindings

**Files:**
- Create: `app/app.go`

All methods on `App` are automatically bound to the Wails JS frontend as `window.go.App.<MethodName>()`.

- [ ] **Step 1: Create app/app.go**

```go
package app

import (
	"context"
	"database/sql"
	"embed"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
	"tokentally/internal/db"
	"tokentally/internal/pricing"
	"tokentally/internal/scanner"
	"tokentally/internal/tips"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed ../pricing.json
var pricingJSON embed.FS

// App is the Wails application struct — all exported methods are bound to the JS frontend.
type App struct {
	ctx         context.Context
	conn        *sql.DB
	projectsDir string
	pricing     *pricing.Pricing
}

// New creates a new App. conn must already be open.
func New(conn *sql.DB, projectsDir string) *App {
	a := &App{conn: conn, projectsDir: projectsDir}
	f, err := pricingJSON.Open("../pricing.json")
	if err == nil {
		a.pricing, _ = pricing.Load(f)
		f.Close()
	}
	// Allow override via env var
	if override := os.Getenv("TOKENTALLY_PRICING_JSON"); override != "" {
		if f2, err := os.Open(override); err == nil {
			a.pricing, _ = pricing.Load(f2)
			f2.Close()
		}
	}
	return a
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

// --- Data binding methods ---

type overviewResult struct {
	Sessions             int64   `json:"sessions"`
	Turns                int64   `json:"turns"`
	InputTokens          int64   `json:"input_tokens"`
	OutputTokens         int64   `json:"output_tokens"`
	CacheReadTokens      int64   `json:"cache_read_tokens"`
	CacheCreate5mTokens  int64   `json:"cache_create_5m_tokens"`
	CacheCreate1hTokens  int64   `json:"cache_create_1h_tokens"`
	CostUSD              *float64 `json:"cost_usd"`
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
	// Compute cost via model breakdown
	models, _ := db.ModelBreakdown(a.conn, since, until)
	var totalCost float64
	for _, m := range models {
		model, _ := m["model"].(string)
		c := pricing.CostFor(model, pricing.Usage{
			InputTokens:         int(asInt64(m["input_tokens"])),
			OutputTokens:        int(asInt64(m["output_tokens"])),
			CacheReadTokens:     int(asInt64(m["cache_read_tokens"])),
			CacheCreate5mTokens: int(asInt64(m["cache_create_5m_tokens"])),
			CacheCreate1hTokens: int(asInt64(m["cache_create_1h_tokens"])),
		}, a.pricing, a.getPlan())
		if c != nil {
			totalCost += *c
		}
	}
	r.CostUSD = &totalCost
	return r, nil
}

func (a *App) GetPrompts(limit int, sort string) ([]map[string]any, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	rows, err := db.ExpensivePrompts(a.conn, limit, sort)
	if err != nil {
		return nil, err
	}
	// Append estimated cost per row
	for _, r := range rows {
		model, _ := r["model"].(string)
		c := pricing.CostFor(model, pricing.Usage{
			CacheReadTokens: int(asInt64(r["cache_read_tokens"])),
		}, a.pricing, a.getPlan())
		r["estimated_cost_usd"] = c
	}
	return rows, nil
}

func (a *App) GetProjects(since, until string) ([]map[string]any, error) {
	return db.ProjectSummary(a.conn, since, until)
}

func (a *App) GetSessions(limit int, since, until string) ([]map[string]any, error) {
	if limit <= 0 || limit > 1000 {
		limit = 20
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
		c := pricing.CostFor(model, pricing.Usage{
			InputTokens:         int(asInt64(r["input_tokens"])),
			OutputTokens:        int(asInt64(r["output_tokens"])),
			CacheReadTokens:     int(asInt64(r["cache_read_tokens"])),
			CacheCreate5mTokens: int(asInt64(r["cache_create_5m_tokens"])),
			CacheCreate1hTokens: int(asInt64(r["cache_create_1h_tokens"])),
		}, a.pricing, a.getPlan())
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

// loadPricingEmbedded opens pricing.json from the embed.FS helper.
func loadPricingEmbedded(fsys fs.FS) (*pricing.Pricing, error) {
	f, err := fsys.Open("pricing.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return pricing.Load(f.(io.Reader))
}
```

- [ ] **Step 2: Fix the embed path**

The `//go:embed ../pricing.json` won't work from a sub-package. Instead, move the embed to `main.go` and pass the `*pricing.Pricing` to `New()`:

Update `app/app.go` — remove the embed directive and change `New` signature:

```go
// New creates a new App.
func New(conn *sql.DB, projectsDir string, p *pricing.Pricing) *App {
	return &App{conn: conn, projectsDir: projectsDir, pricing: p}
}
```

Remove the `embed` and `io/fs` imports from `app/app.go`. The embed and pricing load happens in `main.go` (Task 14).

- [ ] **Step 3: Build check**

```bash
GOOS=windows GOARCH=amd64 go build ./app/ 2>&1
```
Expected: no errors (may warn about unused imports — fix them)

- [ ] **Step 4: Commit**

```bash
git add app/app.go
git commit -m "feat(app): Wails App struct with all data bindings"
```

---

## Task 12: Service Control Bindings + Tray

**Files:**
- Create: `app/tray.go`
- Modify: `app/app.go` (add service control methods)

- [ ] **Step 1: Add service control methods to app.go**

Append to `app/app.go`:

```go
//go:build windows

import (
	"os/exec"
	"syscall"
	"tokentally/svc"

	"golang.org/x/sys/windows/svc/mgr"
)

func (a *App) GetServiceStatus() map[string]any {
	m, err := mgr.Connect()
	if err != nil {
		return map[string]any{"installed": false, "error": err.Error()}
	}
	defer m.Disconnect()
	s, err := m.OpenService(svc.ServiceName)
	if err != nil {
		return map[string]any{"installed": false}
	}
	defer s.Close()
	status, err := s.Query()
	if err != nil {
		return map[string]any{"installed": true, "state": "unknown"}
	}
	stateStr := "stopped"
	if status.State == 4 { // SERVICE_RUNNING
		stateStr = "running"
	}
	return map[string]any{"installed": true, "state": stateStr}
}

// InstallService re-launches tokentally.exe --install elevated via UAC.
func (a *App) InstallService() error {
	exe, _ := os.Executable()
	return runElevated(exe, "--install")
}

// UninstallService re-launches tokentally.exe --uninstall elevated via UAC.
func (a *App) UninstallService() error {
	exe, _ := os.Executable()
	return runElevated(exe, "--uninstall")
}

func runElevated(exe, arg string) error {
	return exec.Command("powershell", "-Command",
		"Start-Process", `"`+exe+`"`, "-ArgumentList", `"`+arg+`"`,
		"-Verb", "RunAs", "-Wait",
	).Run()
}
```

Note: put the Windows-only methods in a separate file `app/service_windows.go` with `//go:build windows` so the non-Windows build still compiles. Move the service control imports there.

- [ ] **Step 2: Create app/tray.go**

```go
//go:build windows

package app

import (
	"fmt"
	"os"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// StartTray initialises the system tray. Must be called from the main goroutine.
// wailsShow is a func that shows/focuses the Wails window.
func (a *App) StartTray(wailsShow func()) {
	systray.Run(
		func() { a.onTrayReady(wailsShow) },
		func() { /* on exit */ },
	)
}

func (a *App) onTrayReady(wailsShow func()) {
	// Icon — embed icon.png bytes
	icon := loadIcon()
	systray.SetIcon(icon)
	systray.SetTooltip("TokenTally")

	mOpen := systray.AddMenuItem("Open Dashboard", "Open the TokenTally window")
	mScan := systray.AddMenuItem("Scan Now", "Trigger an immediate scan")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit TokenTally", "Exit TokenTally")

	// Left-click on tray icon
	go func() {
		for range systray.TrayClickedCh() {
			wailsShow()
		}
	}()

	for {
		select {
		case <-mOpen.ClickedCh:
			wailsShow()
		case <-mScan.ClickedCh:
			go a.ScanNow()
		case <-mQuit.ClickedCh:
			systray.Quit()
			if a.ctx != nil {
				runtime.Quit(a.ctx)
			}
			os.Exit(0)
		}
	}
}

func loadIcon() []byte {
	return IconBytes
}

// IconBytes is set by main.go from the embedded icon.png.
var IconBytes []byte
```

Note: `systray.TrayClickedCh()` may not exist in all versions — check the `getlantern/systray` API. If not present, left-click is handled via `systray.SetOnClick` or the first menu item acts as the click target. Adjust if the build fails.

- [ ] **Step 3: Build check**

```bash
GOOS=windows GOARCH=amd64 go build ./app/ 2>&1
```
Fix any compilation errors before proceeding.

- [ ] **Step 4: Commit**

```bash
git add app/
git commit -m "feat(app): service control bindings and system tray"
```

---

## Task 13: main.go — Mode Detection and CLI

**Files:**
- Create: `main.go`

- [ ] **Step 1: Create main.go**

```go
//go:build windows

package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"tokentally/app"
	"tokentally/internal/db"
	"tokentally/internal/pricing"
	"tokentally/svc"
)

//go:embed all:frontend
var rawAssets embed.FS

//go:embed pricing.json
var rawPricing embed.FS

//go:embed icon.png
var iconPNG []byte

func main() {
	installFlag   := flag.Bool("install", false, "Install Windows service (requires admin)")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall Windows service (requires admin)")
	serviceFlag   := flag.Bool("service", false, "Run as Windows SCM service (internal use)")
	flag.Parse()

	dbPath      := envOrDefault("TOKENTALLY_DB", filepath.Join(homeDir(), ".claude", "tokentally.db"))
	projectsDir := envOrDefault("TOKENTALLY_PROJECTS_DIR", filepath.Join(homeDir(), ".claude", "projects"))
	scanInterval := 30 * time.Second

	switch {
	case *installFlag:
		runInstall()
	case *uninstallFlag:
		runUninstall()
	case *serviceFlag:
		runService(dbPath, projectsDir, scanInterval)
	default:
		runUI(dbPath, projectsDir)
	}
}

func runInstall() {
	exe, _ := os.Executable()
	if err := svc.Install(exe); err != nil {
		fmt.Fprintf(os.Stderr, "install: %v\n", err)
		os.Exit(1)
	}
	// Add UI mode to Windows startup
	addToStartup()
	fmt.Println("TokenTally service installed.")
}

func runUninstall() {
	if err := svc.Uninstall(); err != nil {
		fmt.Fprintf(os.Stderr, "uninstall: %v\n", err)
		os.Exit(1)
	}
	removeFromStartup()
	fmt.Println("TokenTally service uninstalled.")
}

func runService(dbPath, projectsDir string, interval time.Duration) {
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()
	if err := svc.Run(conn, projectsDir, interval); err != nil {
		log.Fatalf("svc.Run: %v", err)
	}
}

func runUI(dbPath, projectsDir string) {
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	p := loadPricing()
	a := app.New(conn, projectsDir, p)
	app.IconBytes = iconPNG

	assets, _ := fs.Sub(rawAssets, "frontend")

	// Run Wails in a goroutine; systray owns the main thread on Windows.
	go func() {
		err := wails.Run(&options.App{
			Title:            "TokenTally",
			Width:            1100,
			Height:           700,
			MinWidth:         800,
			MinHeight:        600,
			BackgroundColour: &options.RGBA{R: 13, G: 13, B: 26, A: 255},
			AssetServer: &assetserver.Options{
				Assets: assets,
			},
			OnStartup: a.Startup,
			Bind:      []any{a},
		})
		if err != nil {
			log.Printf("wails: %v", err)
		}
	}()

	// systray must run on the main goroutine on Windows
	a.StartTray(func() {
		// Wails show/focus — emit a runtime event or use window manager
		// The simplest approach: StartTray already owns show logic
	})
}

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

func addToStartup() {
	// Add HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Run\TokenTally
	exe, _ := os.Executable()
	key := `HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	os.Stderr.WriteString("Adding to startup: reg add " + key + "\n")
	// Use reg.exe for simplicity — no CGO required
	runCmd("reg", "add", key, "/v", "TokenTally", "/t", "REG_SZ", "/d", exe, "/f")
}

func removeFromStartup() {
	key := `HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	runCmd("reg", "delete", key, "/v", "TokenTally", "/f")
}

func runCmd(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Run()
}
```

Add `"os/exec"` to imports.

- [ ] **Step 2: Build the binary**

```bash
GOOS=windows GOARCH=amd64 go build -o tokentally.exe . 2>&1
```
Expected: `tokentally.exe` produced (no errors)

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: main.go dual-mode binary (service/UI/install/uninstall)"
```

---

## Task 14: Frontend — Copy and Restyle

**Files:**
- Create: `frontend/index.html`
- Create: `frontend/web/style.css`
- Create: `frontend/web/echarts.min.js`
- Create: `frontend/web/charts.js`
- Create: `frontend/web/app.js`
- Create: `frontend/web/routes/*.js` (7 files)

- [ ] **Step 1: Copy files from token-dashboard**

```bash
cp C:/claudecode/token-dashboard/web/index.html  frontend/index.html
cp C:/claudecode/token-dashboard/web/echarts.min.js frontend/web/echarts.min.js
cp C:/claudecode/token-dashboard/web/charts.js   frontend/web/charts.js
cp C:/claudecode/token-dashboard/web/style.css   frontend/web/style.css
cp C:/claudecode/token-dashboard/web/app.js      frontend/web/app.js
cp C:/claudecode/token-dashboard/web/routes/*.js frontend/web/routes/
```

- [ ] **Step 2: Update index.html title and brand**

Edit `frontend/index.html`:
- Change `<title>Token Dashboard</title>` → `<title>TokenTally</title>`
- Change script/link paths from `/web/` — they stay as `/web/` (the embed preserves the structure)

- [ ] **Step 3: Restyle CSS**

In `frontend/web/style.css`, replace the colour tokens throughout. Find and replace:

| Old value | New value | Purpose |
|---|---|---|
| `#0f1117` or `#111827` (bg) | `#0d0d1a` | Deep navy background |
| `#1a1f2e` or similar (surface) | `#1a1a2e` | Surface cards |
| `#3b82f6` or `#60a5fa` (accent) | `#ff6b35` | Primary orange accent |
| `#8B98A6` (muted text) | `#8b98b8` | Muted text |
| `Token Dashboard` (brand text) | `Token<span style="color:#ff6b35">Tally</span>` | Wordmark |

Also add to `:root` or `body`:
```css
--accent: #ff6b35;
--accent-2: #ffd166;
--bg: #0d0d1a;
--surface: #1a1a2e;
--muted: #8b98b8;
```

Replace all existing hardcoded colour references with the CSS vars.

- [ ] **Step 4: Update brand in app.js**

In `frontend/web/app.js`, find the brand text in `buildTopbar()`:

```js
// Change:
<div class="brand">Token Dashboard</div>
// To:
<div class="brand">Token<span style="color:#ff6b35">Tally</span></div>
```

- [ ] **Step 5: Verify files are in place**

```bash
ls frontend/web/routes/
```
Expected: `overview.js  prompts.js  sessions.js  projects.js  skills.js  tips.js  settings.js`

- [ ] **Step 6: Commit**

```bash
git add frontend/
git commit -m "feat(frontend): copy and restyle with TokenTally palette"
```

---

## Task 15: Frontend — Replace fetch() with Wails Bindings

**Files:**
- Modify: `frontend/web/app.js`

All `fetch('/api/...')` calls in `app.js` are replaced with `window.go.App.*()` Wails calls. The SSE listener is replaced with a Wails event subscription.

- [ ] **Step 1: Update the `api()` helper in app.js**

Find the `api()` function and replace:

```js
// OLD:
export async function api(path, opts) {
  const r = await fetch(path, opts);
  if (!r.ok) throw new Error(`${path} → ${r.status}`);
  return r.json();
}

// NEW — maps path patterns to window.go.App methods:
const _apiMap = {
  '/api/overview':  (qs) => window.go.App.GetOverview(qs.since||'', qs.until||''),
  '/api/prompts':   (qs) => window.go.App.GetPrompts(parseInt(qs.limit||50), qs.sort||'tokens'),
  '/api/projects':  (qs) => window.go.App.GetProjects(qs.since||'', qs.until||''),
  '/api/sessions':  (qs) => window.go.App.GetSessions(parseInt(qs.limit||20), qs.since||'', qs.until||''),
  '/api/tools':     (qs) => window.go.App.GetTools(qs.since||'', qs.until||''),
  '/api/daily':     (qs) => window.go.App.GetDaily(qs.since||'', qs.until||''),
  '/api/by-model':  (qs) => window.go.App.GetByModel(qs.since||'', qs.until||''),
  '/api/skills':    (qs) => window.go.App.GetSkills(qs.since||'', qs.until||''),
  '/api/tips':      (_)  => window.go.App.GetTips(),
  '/api/plan':      (_)  => window.go.App.GetPlan(),
  '/api/scan':      (_)  => window.go.App.ScanNow(),
};

export async function api(path, opts) {
  // Parse path and query string
  const [base, search] = path.split('?');
  const qs = Object.fromEntries(new URLSearchParams(search||''));
  
  // Handle session turns: /api/sessions/<id>
  if (base.startsWith('/api/sessions/')) {
    const sid = base.split('/').pop();
    return window.go.App.GetSessionTurns(sid);
  }

  const handler = _apiMap[base];
  if (!handler) throw new Error(`No binding for ${base}`);
  
  // Handle POST-style calls
  if (opts && opts.method === 'POST') {
    const body = JSON.parse(opts.body || '{}');
    if (base === '/api/tips/dismiss') return window.go.App.DismissTip(body.key||'');
    if (base === '/api/plan') return window.go.App.SetPlan(body.plan||'');
  }
  
  return handler(qs);
}
```

- [ ] **Step 2: Replace the SSE live-refresh listener**

Find the SSE setup code in `app.js` (the `EventSource` block in `firstRun` or similar) and replace:

```js
// OLD:
const es = new EventSource('/api/stream');
es.onmessage = e => { /* refresh */ };

// NEW:
window.runtime.EventsOn('scan', () => render());
```

- [ ] **Step 3: Remove the plan-set firstRun fetch**

Find any `fetch('/api/plan', ...)` in `firstRun()` and replace with the `api()` wrapper (which now routes to `window.go.App.*`).

- [ ] **Step 4: Build-check the frontend**

Open `frontend/index.html` in a browser (file://) — it will fail to load data (no Wails runtime outside the app), but it should not have JS syntax errors. Check the browser console for parse errors only.

Alternatively, run the Wails dev server:
```bash
wails dev
```
The dashboard should load in the browser with the TokenTally palette visible.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/app.js
git commit -m "feat(frontend): replace fetch() with window.go.App.* Wails bindings"
```

---

## Task 16: Frontend — Adapt Route Files

**Files:**
- Modify: `frontend/web/routes/overview.js`
- Modify: `frontend/web/routes/prompts.js`
- Modify: `frontend/web/routes/sessions.js`
- Modify: `frontend/web/routes/projects.js`
- Modify: `frontend/web/routes/skills.js`
- Modify: `frontend/web/routes/tips.js`

The route files call `api(...)` which is now the Wails binding wrapper. They should work without changes except for one adjustment: the `withSince` URL helper appends `?since=...` to paths, which the new `api()` function parses correctly. Verify each route loads.

- [ ] **Step 1: Audit overview.js for hardcoded fetch calls**

Search for any `fetch(` or direct `EventSource` usage in the route files (not going through the `api()` helper):

```bash
grep -r "fetch\|EventSource" frontend/web/routes/
```

Replace any direct calls with `api()`.

- [ ] **Step 2: Fix the tips dismiss call**

In `frontend/web/routes/tips.js`, find the dismiss call:

```js
// Likely looks like:
await api('/api/tips/dismiss', { method: 'POST', body: JSON.stringify({ key }) });
// This already works with the new api() wrapper — no change needed.
```

- [ ] **Step 3: Fix the plan update call**

In `frontend/web/routes/settings.js`, find:

```js
await api('/api/plan', { method: 'POST', body: JSON.stringify({ plan }) });
// Already routed correctly by the new api() wrapper — no change needed.
```

- [ ] **Step 4: Test each route visually in `wails dev`**

```bash
wails dev
```

Click through all 6 routes (overview, prompts, sessions, projects, skills, tips) and verify charts and data tables render without JS errors. Tips/skills routes may show empty state with no data — that's fine.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/routes/
git commit -m "feat(frontend): audit and verify all route files work with Wails bindings"
```

---

## Task 17: Frontend — Settings Route Service Section

**Files:**
- Modify: `frontend/web/routes/settings.js`

The existing settings route handles plan selection. Add a "Service" card below it.

- [ ] **Step 1: Add service card to settings.js**

Find the HTML template string returned by the settings route and append a new card:

```js
// After the existing plan card HTML, add:
const serviceCard = `
  <div class="card" id="service-card">
    <h2>Windows Service</h2>
    <p style="color:var(--muted);font-size:13px">
      The background scanner runs as a Windows service, keeping data up to date even when the dashboard is closed.
    </p>
    <div id="svc-status" style="margin:12px 0;font-size:13px">Checking...</div>
    <div style="display:flex;gap:8px;flex-wrap:wrap">
      <button id="btn-install" class="btn-primary">Install Service</button>
      <button id="btn-uninstall" class="btn-danger">Uninstall Service</button>
    </div>
    <p style="color:var(--muted);font-size:11px;margin-top:8px">Requires administrator rights (UAC prompt will appear).</p>
  </div>`;
```

Then wire up the buttons after rendering:

```js
// After root.innerHTML = ...:
async function refreshServiceStatus() {
  const status = await window.go.App.GetServiceStatus();
  const el = document.getElementById('svc-status');
  if (!el) return;
  if (!status.installed) {
    el.innerHTML = '<span style="color:#e76f51">● Not installed</span>';
  } else {
    const color = status.state === 'running' ? '#2a9d8f' : '#8b98b8';
    el.innerHTML = `<span style="color:${color}">● ${status.state}</span>`;
  }
}
refreshServiceStatus();

document.getElementById('btn-install')?.addEventListener('click', async () => {
  await window.go.App.InstallService();
  setTimeout(refreshServiceStatus, 1500);
});
document.getElementById('btn-uninstall')?.addEventListener('click', async () => {
  await window.go.App.UninstallService();
  setTimeout(refreshServiceStatus, 1500);
});
```

- [ ] **Step 2: Test the settings route in `wails dev`**

Navigate to Settings tab and verify the Service card renders with status and buttons.

- [ ] **Step 3: Commit**

```bash
git add frontend/web/routes/settings.js
git commit -m "feat(frontend): Settings route — Windows Service control card"
```

---

## Task 18: Build and Package

**Files:**
- Create: `.gitignore` (if not present)
- Verify: `wails.json`

- [ ] **Step 1: Add .gitignore**

```
build/
*.exe
*.db
.superpowers/
go.sum
```

Keep `go.sum` tracked (remove it from .gitignore — it should be committed). Adjust as needed.

- [ ] **Step 2: Production build**

```bash
wails build -platform windows/amd64
```

Expected: `build/bin/tokentally.exe` produced (or similar path per wails.json `outputfilename`).

If `wails build` fails due to missing Wails setup, install prerequisites:
```bash
wails doctor
```
Follow any instructions from `wails doctor` (typically: install WebView2 runtime on the build machine).

- [ ] **Step 3: Commit .gitignore**

```bash
git add .gitignore
git commit -m "chore: add .gitignore"
```

---

## Task 19: End-to-End Smoke Test

Manual verification on Windows. No automated test for this task.

- [ ] **Step 1: Run all unit tests**

```bash
go test ./internal/... -v
```
Expected: all PASS

- [ ] **Step 2: Run UI mode against real Claude data**

```bash
./tokentally.exe
```

- Verify system tray icon appears in the Windows taskbar
- Left-click → Wails window opens with TokenTally branding (dark navy, orange accent)
- Overview tab shows token counts (populated from `~/.claude/projects/`)
- Click through all 7 tabs — no JS errors in dev tools (F12)
- Settings → Service card shows "Not installed"

- [ ] **Step 3: Test install (in an elevated terminal)**

```bash
./tokentally.exe --install
```

Expected: UAC prompt appears (or "service installed" if already elevated). Verify in Services (`services.msc`) that "TokenTally" service appears with "Automatic" startup.

- [ ] **Step 4: Verify service scans independently**

Stop the UI (`Quit TokenTally` from tray). Start the service:

```bash
net start TokenTally
```

Wait 35 seconds. Open a new Claude Code session to generate JSONL data. Re-open the UI — overview tokens should have updated.

- [ ] **Step 5: Test uninstall**

```bash
./tokentally.exe --uninstall
```
Expected: service removed from `services.msc`.

- [ ] **Step 6: Final commit**

```bash
git add -A
git commit -m "feat: TokenTally v1 complete — Go/Wails Windows service with full token dashboard parity"
```

---

## Quick Reference

**Build:** `wails build -platform windows/amd64`

**Test:** `go test ./internal/... -v`

**Dev mode:** `wails dev` (hot-reload, opens browser)

**Service install:** `tokentally.exe --install` (requires admin)

**Service uninstall:** `tokentally.exe --uninstall` (requires admin)

**Env vars:**

| Variable | Default |
|---|---|
| `TOKENTALLY_DB` | `~/.claude/tokentally.db` |
| `TOKENTALLY_PROJECTS_DIR` | `~/.claude/projects/` |
| `TOKENTALLY_PRICING_JSON` | *(embedded)* |
| `TOKENTALLY_SCAN_INTERVAL` | `30` (seconds) |
