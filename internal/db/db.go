// Package db provides SQLite schema management and shared query helpers.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
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
CREATE INDEX IF NOT EXISTS idx_messages_parent    ON messages(parent_uuid);
CREATE TABLE IF NOT EXISTS tool_calls (
  id            INTEGER PRIMARY KEY,
  message_uuid  TEXT    NOT NULL,
  session_id    TEXT    NOT NULL,
  project_slug  TEXT    NOT NULL,
  tool_name     TEXT    NOT NULL,
  target        TEXT,
  result_tokens INTEGER,
  is_error      INTEGER NOT NULL DEFAULT 0,
  timestamp     TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tools_session      ON tool_calls(session_id);
CREATE INDEX IF NOT EXISTS idx_tools_name         ON tool_calls(tool_name);
CREATE INDEX IF NOT EXISTS idx_tools_target       ON tool_calls(target);
CREATE INDEX IF NOT EXISTS idx_tools_message_uuid ON tool_calls(message_uuid);
CREATE TABLE IF NOT EXISTS plan (
  k TEXT PRIMARY KEY,
  v TEXT
);
CREATE TABLE IF NOT EXISTS dismissed_tips (
  tip_key       TEXT PRIMARY KEY,
  dismissed_at  REAL NOT NULL
);
CREATE TABLE IF NOT EXISTS skill_sizes (
  skill_name TEXT PRIMARY KEY,
  file_bytes INTEGER NOT NULL,
  updated_at TEXT    NOT NULL
);

-- Full-text search index over messages.prompt_text. Uses the trigram tokenizer
-- so MATCH 'dep' finds 'deploy' / 'deploying' / 'redeploy' just like
-- LIKE '%dep%' did, but with index support.
-- External-content mode: FTS5 stores the index, the data stays in messages.
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
  prompt_text,
  content='messages',
  content_rowid='rowid',
  tokenize='trigram'
);

-- INSERT OR REPLACE on messages fires DELETE-then-INSERT, so AI + AD cover
-- both the streaming-snapshot replay path and the eviction path. messages
-- is never UPDATE'd directly anywhere in the codebase, so no AU trigger.
-- WHEN clauses skip rows without prompt_text so the FTS index only carries
-- searchable text (skips assistant turns, system records, etc.).
CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages
  WHEN new.prompt_text IS NOT NULL AND new.prompt_text != '' BEGIN
  INSERT INTO messages_fts(rowid, prompt_text) VALUES (new.rowid, new.prompt_text);
END;
CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages
  WHEN old.prompt_text IS NOT NULL AND old.prompt_text != '' BEGIN
  INSERT INTO messages_fts(messages_fts, rowid, prompt_text) VALUES('delete', old.rowid, old.prompt_text);
END;
`

const nanosPerSec = 1e9 // UnixNano → seconds for mtime/scanned_at storage

// nowFunc is a package-level variable so unit tests can inject a deterministic
// clock without requiring real wall-clock time.
var nowFunc = func() float64 { return float64(time.Now().UnixNano()) / nanosPerSec }

// Pool holds two database/sql handles to the same SQLite database under WAL mode.
//
// Read accepts SELECT-only queries and allows multiple concurrent connections so
// reads never block on writes.
//
// Write accepts all mutations and is limited to a single connection so concurrent
// Go writers queue at the pool layer instead of hitting "database is locked"
// after busy_timeout expires inside SQLite.
//
// For :memory: paths both fields share one handle since each :memory: connection
// is its own database.
type Pool struct {
	Read  *sql.DB
	Write *sql.DB
}

// Close closes the underlying handles. When Read and Write share a handle
// (the :memory: case) it is closed once.
func (p *Pool) Close() error {
	if p.Read == p.Write {
		return p.Write.Close()
	}
	werr := p.Write.Close()
	rerr := p.Read.Close()
	if werr != nil {
		return werr
	}
	return rerr
}

// CheckpointWAL runs a TRUNCATE checkpoint, which copies any pending pages
// from the -wal file into the main database and then truncates the wal back
// to zero length. Safe to call regularly — a no-op when nothing is pending,
// and best-effort when readers are active (returns busy=1 in the result row,
// which we ignore since the next call will retry).
func (p *Pool) CheckpointWAL() error {
	_, err := p.Write.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	if err != nil {
		return fmt.Errorf("CheckpointWAL: %w", err)
	}
	return nil
}

// Open opens (or creates) the SQLite database at path and applies the schema.
// File-backed databases get separate read and write pools; :memory: shares one
// handle.
func Open(path string) (*Pool, error) {
	if path == ":memory:" {
		conn, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, fmt.Errorf("db.Open %s: %w", path, err)
		}
		if err := initSchema(conn); err != nil {
			conn.Close()
			return nil, err
		}
		return &Pool{Read: conn, Write: conn}, nil
	}

	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"

	write, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db.Open write %s: %w", path, err)
	}
	write.SetMaxOpenConns(1)
	if err := initSchema(write); err != nil {
		write.Close()
		return nil, err
	}

	read, err := sql.Open("sqlite", dsn)
	if err != nil {
		write.Close()
		return nil, fmt.Errorf("db.Open read %s: %w", path, err)
	}
	read.SetMaxOpenConns(4)

	return &Pool{Read: read, Write: write}, nil
}

// initSchema applies the static schema, column migrations, post-migration
// indexes, and one-time fixes against a single connection.
func initSchema(conn *sql.DB) error {
	if _, err := conn.Exec(schema); err != nil {
		return fmt.Errorf("db.Open schema: %w", err)
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
			return err
		}
	}
	// Index on tool_use_id must be created after addColumnIfMissing — the column
	// is added via migration, not the static schema.
	if _, err := conn.Exec(`CREATE INDEX IF NOT EXISTS idx_tools_use_id ON tool_calls(tool_use_id)`); err != nil {
		return fmt.Errorf("db.Open idx_tools_use_id: %w", err)
	}
	return applyMigrations(conn)
}

// targetSchemaVersion is the schema generation this binary expects. Bump it
// whenever a new migration is appended to the migrations slice.
const targetSchemaVersion = 3

// migrations are applied in order; index N produces schema version N+1.
// To add a new one: append the function and bump targetSchemaVersion.
var migrations = []func(*sql.DB) error{
	migrateFixUserStringContent,
	migrateFTSBackfill,
	migrateDropToolCallsAutoincrement,
}

// applyMigrations runs every migration whose version is greater than the
// version recorded in the plan table. Each migration is responsible for
// being safe to re-run if a previous attempt was interrupted.
func applyMigrations(conn *sql.DB) error {
	current := readSchemaVersion(conn)
	if current >= targetSchemaVersion {
		return nil
	}
	for v := current; v < targetSchemaVersion; v++ {
		if err := migrations[v](conn); err != nil {
			return fmt.Errorf("migration %d→%d: %w", v, v+1, err)
		}
	}
	_, err := conn.Exec(
		`INSERT OR REPLACE INTO plan (k,v) VALUES ('schema_version',?)`,
		strconv.Itoa(targetSchemaVersion),
	)
	if err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}
	return nil
}

// readSchemaVersion returns the recorded schema version, falling back to
// inferring it from legacy per-migration gate flags so existing DBs that
// pre-date the schema_version row don't repeat already-applied work.
func readSchemaVersion(conn *sql.DB) int {
	var v string
	if err := conn.QueryRow(`SELECT v FROM plan WHERE k='schema_version'`).Scan(&v); err == nil {
		if n, _ := strconv.Atoi(v); n > 0 {
			return n
		}
	}
	if hasLegacyGate(conn, "fts_backfill_done") {
		return 2 // FTS backfill (and by implication the fix-user-content reset) was done.
	}
	if hasLegacyGate(conn, "fix_user_string_content") {
		return 1
	}
	return 0
}

func hasLegacyGate(conn *sql.DB, key string) bool {
	var v string
	err := conn.QueryRow(`SELECT v FROM plan WHERE k=?`, key).Scan(&v)
	return err == nil && v == "1"
}

// SchemaVersion returns the recorded schema generation. Useful for diagnostics.
func SchemaVersion(p *Pool) (int, error) {
	var v string
	err := p.Read.QueryRow(`SELECT v FROM plan WHERE k='schema_version'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("SchemaVersion: %w", err)
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("SchemaVersion parse %q: %w", v, err)
	}
	return n, nil
}

// migrateFixUserStringContent (v0→v1) resets all file-scan states so the
// scanner re-processes files that stored NULL prompt_text for user messages
// whose content was a plain string (not a content-block array).
func migrateFixUserStringContent(conn *sql.DB) error {
	if _, err := conn.Exec(`DELETE FROM files`); err != nil {
		return fmt.Errorf("reset files: %w", err)
	}
	_, err := conn.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('fix_user_string_content','1')`)
	return err
}

// migrateFTSBackfill (v1→v2) populates messages_fts from existing messages on
// the first run after the FTS5 index was added. Subsequent INSERTs and
// DELETEs are kept in sync by triggers.
func migrateFTSBackfill(conn *sql.DB) error {
	if _, err := conn.Exec(
		`INSERT INTO messages_fts(rowid, prompt_text)
		 SELECT rowid, prompt_text FROM messages
		 WHERE prompt_text IS NOT NULL AND prompt_text != ''`,
	); err != nil {
		return fmt.Errorf("fts backfill: %w", err)
	}
	_, err := conn.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('fts_backfill_done','1')`)
	return err
}

// migrateDropToolCallsAutoincrement (v2→v3) recreates the tool_calls table
// without AUTOINCREMENT on the id column. Plain INTEGER PRIMARY KEY is
// sufficient (rowid alias, monotonic) and avoids the per-INSERT
// sqlite_sequence write the AUTOINCREMENT keyword forces.
//
// Detects "AUTOINCREMENT" in the stored CREATE TABLE so fresh databases
// (which never had it) and re-runs are no-ops.
func migrateDropToolCallsAutoincrement(conn *sql.DB) error {
	var sqlText string
	err := conn.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='tool_calls'`,
	).Scan(&sqlText)
	if err != nil {
		return fmt.Errorf("read tool_calls schema: %w", err)
	}
	if !strings.Contains(strings.ToUpper(sqlText), "AUTOINCREMENT") {
		return nil // already migrated or fresh DB
	}

	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmts := []string{
		`CREATE TABLE tool_calls_new (
		   id            INTEGER PRIMARY KEY,
		   message_uuid  TEXT    NOT NULL,
		   session_id    TEXT    NOT NULL,
		   project_slug  TEXT    NOT NULL,
		   tool_name     TEXT    NOT NULL,
		   target        TEXT,
		   result_tokens INTEGER,
		   is_error      INTEGER NOT NULL DEFAULT 0,
		   timestamp     TEXT    NOT NULL,
		   tool_use_id   TEXT,
		   input_json    TEXT,
		   output_text   TEXT,
		   duration_ms   INTEGER
		 )`,
		`INSERT INTO tool_calls_new
		   (id, message_uuid, session_id, project_slug, tool_name, target,
		    result_tokens, is_error, timestamp, tool_use_id, input_json,
		    output_text, duration_ms)
		 SELECT id, message_uuid, session_id, project_slug, tool_name, target,
		        result_tokens, is_error, timestamp, tool_use_id, input_json,
		        output_text, duration_ms
		   FROM tool_calls`,
		`DROP TABLE tool_calls`,
		`ALTER TABLE tool_calls_new RENAME TO tool_calls`,
		// Recreate every index that lived on the old table.
		`CREATE INDEX idx_tools_session      ON tool_calls(session_id)`,
		`CREATE INDEX idx_tools_name         ON tool_calls(tool_name)`,
		`CREATE INDEX idx_tools_target       ON tool_calls(target)`,
		`CREATE INDEX idx_tools_message_uuid ON tool_calls(message_uuid)`,
		`CREATE INDEX idx_tools_use_id       ON tool_calls(tool_use_id)`,
		// Drop the now-orphan sqlite_sequence row so it doesn't grow on
		// future restarts.
		`DELETE FROM sqlite_sequence WHERE name='tool_calls'`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("recreate tool_calls (%q): %w", firstLine(s), err)
		}
	}
	return tx.Commit()
}

// firstLine returns the first non-blank line of s, used to make migration
// errors point at the failing statement without dumping whole CREATE bodies.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return s
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
func OverviewTotals(p *Pool, since, until string) (map[string]any, error) {
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

	rows, err := p.Read.Query(q, args...)
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

// sanitizeFTSQuery escapes user input for FTS5 MATCH. Each whitespace-
// separated token becomes a quoted phrase, embedded double quotes are
// doubled (FTS5's escape inside quoted strings), and tokens are joined by
// space which is implicit AND. This means "auth bug" finds messages
// containing both substrings anywhere, in any order — closer to the UX
// users expect than the previous strict-substring LIKE semantics.
//
// The trigram tokenizer requires ≥3 characters per token to match anything,
// so the frontend should not send queries shorter than that.
func sanitizeFTSQuery(q string) string {
	fields := strings.Fields(q)
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(parts, " ")
}

// SearchPrompts returns user/hook prompts matching an optional text query,
// filtered by type and date range. types is a comma-separated list of
// "user", "subagent", "hook". from/to are YYYY-MM-DD date strings.
func SearchPrompts(p *Pool, query, types, from, to string) ([]map[string]any, error) {
	var whereParts []string
	var args []any

	if ftsQuery := sanitizeFTSQuery(query); ftsQuery != "" {
		whereParts = append(whereParts, "u.rowid IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)")
		args = append(args, ftsQuery)
	}

	typeSet := map[string]bool{}
	for _, t := range strings.Split(types, ",") {
		typeSet[strings.TrimSpace(t)] = true
	}
	if !(typeSet["user"] && typeSet["subagent"] && typeSet["hook"]) {
		var typeConds []string
		if typeSet["user"] {
			typeConds = append(typeConds, "(u.type='user' AND u.is_sidechain=0)")
		}
		if typeSet["subagent"] {
			typeConds = append(typeConds, "(u.type='user' AND u.is_sidechain=1)")
		}
		if typeSet["hook"] {
			typeConds = append(typeConds, "u.type='attachment'")
		}
		if len(typeConds) == 0 {
			return []map[string]any{}, nil
		}
		whereParts = append(whereParts, "("+strings.Join(typeConds, " OR ")+")")
	}

	if from != "" {
		whereParts = append(whereParts, "u.timestamp >= ?")
		args = append(args, from)
	}
	if to != "" {
		// Advance one day so the upper bound is exclusive and includes all
		// timestamps on the 'to' date (ISO strings sort lexicographically,
		// so "2024-01-15T23:59:59Z" > "2024-01-15" but < "2024-01-16").
		if t, err := time.Parse("2006-01-02", to); err == nil {
			whereParts = append(whereParts, "u.timestamp < ?")
			args = append(args, t.AddDate(0, 0, 1).Format("2006-01-02"))
		}
	}

	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " AND " + strings.Join(whereParts, " AND ")
	}

	q := `
SELECT u.uuid AS user_uuid, u.session_id, u.project_slug, u.timestamp,
       u.prompt_text, u.prompt_chars,
       u.is_sidechain, u.type AS msg_type,
       a.uuid AS assistant_uuid, COALESCE(a.model,'') AS model,
       COALESCE(a.input_tokens,0) AS input_tokens,
       COALESCE(a.output_tokens,0) AS output_tokens,
       COALESCE(a.cache_read_tokens,0) AS cache_read_tokens,
       COALESCE(a.cache_create_5m_tokens,0) AS cache_create_5m_tokens,
       COALESCE(a.cache_create_1h_tokens,0) AS cache_create_1h_tokens,
       COALESCE(a.input_tokens,0)+COALESCE(a.output_tokens,0)
         +COALESCE(a.cache_create_5m_tokens,0)+COALESCE(a.cache_create_1h_tokens,0) AS billable_tokens
FROM messages u
LEFT JOIN messages a ON a.rowid = (
    SELECT MIN(rowid) FROM messages
    WHERE parent_uuid = u.uuid AND type='assistant'
)
WHERE u.type IN ('user','attachment') AND u.prompt_text IS NOT NULL AND u.prompt_text != ''` +
		whereClause + `
ORDER BY u.timestamp DESC
LIMIT 200`

	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("SearchPrompts: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// ExpensivePrompts returns user and hook prompts joined with the following
// assistant turn, ordered by billable_tokens DESC (sort="tokens") or
// timestamp DESC (sort="recent").
func ExpensivePrompts(p *Pool, limit int, sort string) ([]map[string]any, error) {
	order := "billable_tokens DESC"
	if sort == "recent" {
		order = "u.timestamp DESC"
	}
	q := `
SELECT u.uuid AS user_uuid, u.session_id, u.project_slug, u.timestamp,
       u.prompt_text, u.prompt_chars,
       u.is_sidechain, u.type AS msg_type,
       a.uuid AS assistant_uuid, a.model,
       COALESCE(a.input_tokens,0) AS input_tokens,
       COALESCE(a.output_tokens,0) AS output_tokens,
       COALESCE(a.cache_read_tokens,0) AS cache_read_tokens,
       COALESCE(a.cache_create_5m_tokens,0) AS cache_create_5m_tokens,
       COALESCE(a.cache_create_1h_tokens,0) AS cache_create_1h_tokens,
       COALESCE(a.input_tokens,0)+COALESCE(a.output_tokens,0)
         +COALESCE(a.cache_create_5m_tokens,0)+COALESCE(a.cache_create_1h_tokens,0) AS billable_tokens
FROM messages u
JOIN messages a ON a.rowid = (
    SELECT MIN(rowid) FROM messages
    WHERE parent_uuid = u.uuid AND type='assistant'
)
WHERE u.type IN ('user','attachment') AND u.prompt_text IS NOT NULL AND u.prompt_text != ''
ORDER BY ` + order + `
LIMIT ?`

	rows, err := p.Read.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("ExpensivePrompts: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// ProjectSummary returns per-project aggregates ordered by billable_tokens DESC.
// Each row includes a "project_name" field derived from BestProjectName.
func ProjectSummary(p *Pool, since, until string) ([]map[string]any, error) {
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
       COALESCE(SUM(cache_read_tokens),0) AS cache_read_tokens,
       MAX(timestamp) AS last_active
FROM messages WHERE 1=1` + rng + `
GROUP BY project_slug ORDER BY billable_tokens DESC`

	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("ProjectSummary: %w", err)
	}
	defer rows.Close()

	results, err := scanMaps(rows)
	if err != nil {
		return nil, fmt.Errorf("ProjectSummary scan: %w", err)
	}

	slugs := make([]string, 0, len(results))
	for _, r := range results {
		if slug, _ := r["project_slug"].(string); slug != "" {
			slugs = append(slugs, slug)
		}
	}
	cwdMap, err := cwdsForSlugs(p, slugs)
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		slug, _ := r["project_slug"].(string)
		r["project_name"] = BestProjectName(cwdMap[slug], slug)
	}
	return results, nil
}

// RecentSessions returns sessions ordered by last activity, newest first.
// Pass a non-empty projectSlug to restrict results to a single project.
func RecentSessions(p *Pool, limit int, since, until, projectSlug string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	slugClause := ""
	if projectSlug != "" {
		slugClause = " AND project_slug = ?"
		args = append(args, projectSlug)
	}
	q := `
SELECT session_id, project_slug,
       MIN(timestamp) AS started, MAX(timestamp) AS ended,
       SUM(CASE WHEN type='user' THEN 1 END) AS turns,
       COALESCE(SUM(input_tokens),0)+COALESCE(SUM(output_tokens),0) AS tokens
FROM messages WHERE 1=1` + rng + slugClause + `
GROUP BY session_id ORDER BY ended DESC LIMIT ?`

	rows, err := p.Read.Query(q, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("RecentSessions: %w", err)
	}
	defer rows.Close()

	results, err := scanMaps(rows)
	if err != nil {
		return nil, fmt.Errorf("RecentSessions scan: %w", err)
	}

	slugSet := map[string]struct{}{}
	for _, r := range results {
		if slug, _ := r["project_slug"].(string); slug != "" {
			slugSet[slug] = struct{}{}
		}
	}
	uniqueSlugs := make([]string, 0, len(slugSet))
	for slug := range slugSet {
		uniqueSlugs = append(uniqueSlugs, slug)
	}
	cwdMap, err := cwdsForSlugs(p, uniqueSlugs)
	if err != nil {
		return nil, err
	}
	nameCache := make(map[string]string, len(uniqueSlugs))
	for _, slug := range uniqueSlugs {
		nameCache[slug] = BestProjectName(cwdMap[slug], slug)
	}
	for _, r := range results {
		slug, _ := r["project_slug"].(string)
		r["project_name"] = nameCache[slug]
	}
	return results, nil
}

// ToolBreakdown returns per-tool call counts, excluding _tool_result rows.
func ToolBreakdown(p *Pool, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT tool_name, COUNT(*) AS calls, COALESCE(SUM(result_tokens),0) AS result_tokens
FROM tool_calls WHERE tool_name != '_tool_result'` + rng + `
GROUP BY tool_name ORDER BY calls DESC`

	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("ToolBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// DailyBreakdown returns one row per calendar day with stacked token counts.
func DailyBreakdown(p *Pool, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT substr(timestamp,1,10) AS day,
       COALESCE(SUM(input_tokens),0) AS input_tokens,
       COALESCE(SUM(output_tokens),0) AS output_tokens,
       COALESCE(SUM(cache_read_tokens),0) AS cache_read_tokens,
       COALESCE(SUM(cache_create_5m_tokens),0)+COALESCE(SUM(cache_create_1h_tokens),0) AS cache_create_tokens
FROM messages WHERE timestamp IS NOT NULL` + rng + `
GROUP BY day ORDER BY day ASC`

	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("DailyBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// ModelBreakdown returns per-model token totals for assistant turns.
func ModelBreakdown(p *Pool, since, until string) ([]map[string]any, error) {
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

	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("ModelBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// SkillBreakdown returns per-skill invocation counts from tool_calls where
// tool_name='Skill'. tokens_per_call is null when the skill file size is unknown.
func SkillBreakdown(p *Pool, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "t.timestamp")
	q := `
SELECT t.target AS skill, COUNT(*) AS invocations,
       COUNT(DISTINCT t.session_id) AS sessions, MAX(t.timestamp) AS last_used,
       CAST(ROUND(ss.file_bytes / 4.0) AS INTEGER) AS tokens_per_call
FROM tool_calls t
LEFT JOIN skill_sizes ss ON ss.skill_name = t.target
WHERE t.tool_name='Skill' AND t.target IS NOT NULL AND t.target!=''` + rng + `
GROUP BY t.target ORDER BY invocations DESC`

	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("SkillBreakdown: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// UpsertSkillSize records the byte size of a skill's SKILL.md file.
// Subsequent calls with the same name update the stored size.
// Accepts dbExec so the scanner can pass an active write transaction; otherwise
// it would deadlock on the single Write-pool connection that the tx holds.
func UpsertSkillSize(exec dbExec, skillName string, fileBytes int64) error {
	_, err := exec.Exec(
		`INSERT INTO skill_sizes (skill_name, file_bytes, updated_at) VALUES (?,?,?)
		 ON CONFLICT(skill_name) DO UPDATE SET file_bytes=excluded.file_bytes, updated_at=excluded.updated_at`,
		skillName, fileBytes, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// dbExec lets helpers run against either *sql.DB or *sql.Tx — used so the
// scanner can pass an open transaction without UpsertSkillSize trying to
// acquire a second connection from the single-conn Write pool.
type dbExec interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// GetPlan returns the stored plan name, defaulting to "api".
func GetPlan(p *Pool) (string, error) {
	var v string
	err := p.Read.QueryRow(`SELECT v FROM plan WHERE k='plan'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "api", nil
	}
	if err != nil {
		return "", fmt.Errorf("GetPlan: %w", err)
	}
	return v, nil
}

// SetPlan stores the plan name.
func SetPlan(p *Pool, plan string) error {
	_, err := p.Write.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('plan',?)`, plan)
	if err != nil {
		return fmt.Errorf("SetPlan: %w", err)
	}
	return nil
}

// GetRetentionDays reads the retention policy from the plan table.
// Returns 0 if not set (= keep forever / auto-purge disabled).
func GetRetentionDays(p *Pool) (int, error) {
	var v string
	err := p.Read.QueryRow(`SELECT v FROM plan WHERE k='retention_days'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("GetRetentionDays: %w", err)
	}
	days, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("GetRetentionDays: corrupt value %q: %w", v, err)
	}
	return days, nil
}

// SetRetentionDays persists the retention policy.
// days=0 effectively disables auto-purge.
func SetRetentionDays(p *Pool, days int) error {
	_, err := p.Write.Exec(
		`INSERT OR REPLACE INTO plan (k,v) VALUES ('retention_days',?)`,
		strconv.Itoa(days),
	)
	if err != nil {
		return fmt.Errorf("SetRetentionDays: %w", err)
	}
	return nil
}

// PurgeMessages deletes tool_calls and messages whose timestamp is older than
// the given number of days. Returns the number of message rows deleted.
// The files table is left intact so the scanner skips already-processed paths
// and does not re-import the pruned data.
// days=0 is a no-op.
func PurgeMessages(p *Pool, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := fmt.Sprintf("-%d days", days)
	tx, err := p.Write.Begin()
	if err != nil {
		return 0, fmt.Errorf("PurgeMessages: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(
		`DELETE FROM tool_calls WHERE timestamp < datetime('now', ?)`, cutoff,
	); err != nil {
		return 0, fmt.Errorf("PurgeMessages tool_calls: %w", err)
	}
	result, err := tx.Exec(
		`DELETE FROM messages WHERE timestamp < datetime('now', ?)`, cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("PurgeMessages messages: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("PurgeMessages rows affected: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("PurgeMessages commit: %w", err)
	}
	return n, nil
}

// DismissTip records a dismissed tip key with the current Unix timestamp.
func DismissTip(p *Pool, key string) error {
	_, err := p.Write.Exec(
		`INSERT OR IGNORE INTO dismissed_tips (tip_key, dismissed_at) VALUES (?,?)`,
		key, nowFunc(),
	)
	if err != nil {
		return fmt.Errorf("DismissTip: %w", err)
	}
	return nil
}

// DismissedTips returns the set of dismissed tip keys.
func DismissedTips(p *Pool) (map[string]bool, error) {
	rows, err := p.Read.Query(`SELECT tip_key FROM dismissed_tips`)
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
// Returns an empty (non-nil) slice when there are no rows so callers (and JSON
// serialisers like Wails) get [] rather than null.
func scanMaps(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0)
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
func GetPricingModels(p *Pool) ([]map[string]any, error) {
	rows, err := p.Read.Query(
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
func UpsertPricingModel(p *Pool, name, tier string, input, output, cacheRead, cache5m, cache1h float64) error {
	_, err := p.Write.Exec(
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
func DeletePricingModel(p *Pool, name string) error {
	_, err := p.Write.Exec(`DELETE FROM pricing_models WHERE model_name=?`, name)
	if err != nil {
		return fmt.Errorf("DeletePricingModel: %w", err)
	}
	return nil
}

// DeleteAllPricingModels removes every model rate row (used for reset-to-defaults).
func DeleteAllPricingModels(p *Pool) error {
	_, err := p.Write.Exec(`DELETE FROM pricing_models`)
	if err != nil {
		return fmt.Errorf("DeleteAllPricingModels: %w", err)
	}
	return nil
}

// GetPricingPlans returns all plan rows ordered by monthly cost ascending.
func GetPricingPlans(p *Pool) ([]map[string]any, error) {
	rows, err := p.Read.Query(`SELECT plan_key, label, monthly FROM pricing_plans ORDER BY monthly ASC`)
	if err != nil {
		return nil, fmt.Errorf("GetPricingPlans: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// UpsertPricingPlan inserts or replaces a plan row.
func UpsertPricingPlan(p *Pool, key, label string, monthly float64) error {
	_, err := p.Write.Exec(
		`INSERT OR REPLACE INTO pricing_plans (plan_key, label, monthly) VALUES (?,?,?)`,
		key, label, monthly,
	)
	if err != nil {
		return fmt.Errorf("UpsertPricingPlan: %w", err)
	}
	return nil
}

// DeletePricingPlan removes a plan row by key.
func DeletePricingPlan(p *Pool, key string) error {
	_, err := p.Write.Exec(`DELETE FROM pricing_plans WHERE plan_key=?`, key)
	if err != nil {
		return fmt.Errorf("DeletePricingPlan: %w", err)
	}
	return nil
}

// DeleteAllPricingPlans removes every plan row (used for reset-to-defaults).
func DeleteAllPricingPlans(p *Pool) error {
	_, err := p.Write.Exec(`DELETE FROM pricing_plans`)
	if err != nil {
		return fmt.Errorf("DeleteAllPricingPlans: %w", err)
	}
	return nil
}

// GetCurrency returns the stored currency code, defaulting to "CAD".
func GetCurrency(p *Pool) (string, error) {
	var v string
	err := p.Read.QueryRow(`SELECT v FROM plan WHERE k='currency'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "CAD", nil
	}
	if err != nil {
		return "", fmt.Errorf("GetCurrency: %w", err)
	}
	return v, nil
}

// SetCurrency stores the currency code.
func SetCurrency(p *Pool, currency string) error {
	_, err := p.Write.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('currency',?)`, currency)
	if err != nil {
		return fmt.Errorf("SetCurrency: %w", err)
	}
	return nil
}

// IsPricingSeeded returns true if the pricing tables have been populated from defaults.
func IsPricingSeeded(p *Pool) (bool, error) {
	var v string
	err := p.Read.QueryRow(`SELECT v FROM plan WHERE k='pricing_seeded'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("IsPricingSeeded: %w", err)
	}
	return v == "1", nil
}

// MarkPricingSeeded records that the pricing tables have been seeded.
func MarkPricingSeeded(p *Pool) error {
	_, err := p.Write.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('pricing_seeded','1')`)
	if err != nil {
		return fmt.Errorf("MarkPricingSeeded: %w", err)
	}
	return nil
}

// GetExchangeRates returns all stored currency→rate pairs (base: USD).
func GetExchangeRates(p *Pool) (map[string]float64, error) {
	rows, err := p.Read.Query(`SELECT currency, rate FROM exchange_rates`)
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
func SeedExchangeRate(p *Pool, currency string, rate float64) error {
	_, err := p.Write.Exec(`INSERT OR IGNORE INTO exchange_rates (currency, rate) VALUES (?,?)`, currency, rate)
	if err != nil {
		return fmt.Errorf("SeedExchangeRate: %w", err)
	}
	return nil
}

// SetExchangeRate inserts or replaces a rate for a currency (used by user edits and API refresh).
func SetExchangeRate(p *Pool, currency string, rate float64) error {
	_, err := p.Write.Exec(`INSERT OR REPLACE INTO exchange_rates (currency, rate) VALUES (?,?)`, currency, rate)
	if err != nil {
		return fmt.Errorf("SetExchangeRate: %w", err)
	}
	return nil
}

// GetExchangeApiKey returns the stored exchangerate-api.com API key, decrypted.
func GetExchangeApiKey(p *Pool) (string, error) {
	var v string
	err := p.Read.QueryRow(`SELECT v FROM plan WHERE k='exchange_api_key'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("GetExchangeApiKey: %w", err)
	}
	return decryptAPIKey(v)
}

// SetExchangeApiKey encrypts and stores the exchangerate-api.com API key.
func SetExchangeApiKey(p *Pool, key string) error {
	encrypted, err := encryptAPIKey(key)
	if err != nil {
		return fmt.Errorf("SetExchangeApiKey: %w", err)
	}
	_, err = p.Write.Exec(`INSERT OR REPLACE INTO plan (k,v) VALUES ('exchange_api_key',?)`, encrypted)
	if err != nil {
		return fmt.Errorf("SetExchangeApiKey: %w", err)
	}
	return nil
}

// cwdsForSlugs fetches distinct (project_slug, cwd) pairs for all given slugs
// in one query, returning a slug → cwds map. Used by ProjectSummary and
// RecentSessions to resolve project names without an N+1 round-trip per row.
func cwdsForSlugs(p *Pool, slugs []string) (map[string][]string, error) {
	result := map[string][]string{}
	if len(slugs) == 0 {
		return result, nil
	}
	placeholders := strings.Repeat("?,", len(slugs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(slugs))
	for i, s := range slugs {
		args[i] = s
	}
	rows, err := p.Read.Query(
		`SELECT DISTINCT project_slug, cwd FROM messages
		 WHERE project_slug IN (`+placeholders+`) AND cwd IS NOT NULL`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("cwdsForSlugs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var slug, cwd string
		if err := rows.Scan(&slug, &cwd); err != nil {
			return nil, fmt.Errorf("cwdsForSlugs scan: %w", err)
		}
		result[slug] = append(result[slug], cwd)
	}
	return result, rows.Err()
}

