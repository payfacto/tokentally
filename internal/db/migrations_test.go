package db

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// freshConn opens a bare sqlite :memory: connection without applying schema.
// Used by migration tests that need to set up specific pre-states.
func freshConn(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestSchemaVersion_FreshDB(t *testing.T) {
	pool, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	got, err := SchemaVersion(pool)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if got != targetSchemaVersion {
		t.Errorf("fresh DB schema version = %d, want %d", got, targetSchemaVersion)
	}
}

func TestSchemaVersion_LegacyDBInferredFromGateFlags(t *testing.T) {
	conn := freshConn(t)
	if _, err := conn.Exec(`CREATE TABLE plan (k TEXT PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("plan table: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO plan (k,v) VALUES ('fix_user_string_content','1'), ('fts_backfill_done','1')`); err != nil {
		t.Fatalf("seed gates: %v", err)
	}
	if got := readSchemaVersion(conn); got != 2 {
		t.Errorf("readSchemaVersion with both legacy gates = %d, want 2", got)
	}
	// And only one gate set:
	conn2 := freshConn(t)
	if _, err := conn2.Exec(`CREATE TABLE plan (k TEXT PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("plan table: %v", err)
	}
	if _, err := conn2.Exec(`INSERT INTO plan (k,v) VALUES ('fix_user_string_content','1')`); err != nil {
		t.Fatalf("seed gate: %v", err)
	}
	if got := readSchemaVersion(conn2); got != 1 {
		t.Errorf("readSchemaVersion with only fix_user_string_content = %d, want 1", got)
	}
}

func TestMigrateDropToolCallsAutoincrement(t *testing.T) {
	pool, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	// Force the table back to the AUTOINCREMENT shape to simulate an
	// upgraded-from-old-schema database.
	stmts := []string{
		`DROP TABLE tool_calls`,
		`CREATE TABLE tool_calls (
		   id            INTEGER PRIMARY KEY AUTOINCREMENT,
		   message_uuid  TEXT NOT NULL,
		   session_id    TEXT NOT NULL,
		   project_slug  TEXT NOT NULL,
		   tool_name     TEXT NOT NULL,
		   target        TEXT,
		   result_tokens INTEGER,
		   is_error      INTEGER NOT NULL DEFAULT 0,
		   timestamp     TEXT NOT NULL,
		   tool_use_id   TEXT,
		   input_json    TEXT,
		   output_text   TEXT,
		   duration_ms   INTEGER
		 )`,
		`CREATE INDEX idx_tools_session      ON tool_calls(session_id)`,
		`CREATE INDEX idx_tools_name         ON tool_calls(tool_name)`,
		`CREATE INDEX idx_tools_target       ON tool_calls(target)`,
		`CREATE INDEX idx_tools_message_uuid ON tool_calls(message_uuid)`,
		`CREATE INDEX idx_tools_use_id       ON tool_calls(tool_use_id)`,
		`INSERT INTO tool_calls (message_uuid, session_id, project_slug, tool_name, timestamp)
		 VALUES ('m1','s1','p','Bash','2025-01-01T00:00:00Z')`,
		`INSERT INTO tool_calls (message_uuid, session_id, project_slug, tool_name, timestamp)
		 VALUES ('m2','s1','p','Read','2025-01-01T00:00:01Z')`,
	}
	for _, s := range stmts {
		if _, err := pool.Write.Exec(s); err != nil {
			t.Fatalf("setup (%q): %v", firstLine(s), err)
		}
	}

	// Sanity: confirm AUTOINCREMENT is currently present.
	var schemaSQL string
	if err := pool.Read.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='tool_calls'`).Scan(&schemaSQL); err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if !strings.Contains(strings.ToUpper(schemaSQL), "AUTOINCREMENT") {
		t.Fatalf("test setup invalid: AUTOINCREMENT missing from %s", schemaSQL)
	}

	if err := migrateDropToolCallsAutoincrement(pool.Write); err != nil {
		t.Fatalf("migrateDropToolCallsAutoincrement: %v", err)
	}

	// Schema no longer has AUTOINCREMENT.
	if err := pool.Read.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='tool_calls'`).Scan(&schemaSQL); err != nil {
		t.Fatalf("read schema after migration: %v", err)
	}
	if strings.Contains(strings.ToUpper(schemaSQL), "AUTOINCREMENT") {
		t.Errorf("AUTOINCREMENT still present after migration: %s", schemaSQL)
	}

	// Both rows preserved.
	var n int
	if err := pool.Read.QueryRow(`SELECT COUNT(*) FROM tool_calls`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("row count after migration = %d, want 2", n)
	}

	// All five indexes recreated.
	rows, err := pool.Read.Query(
		`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='tool_calls' ORDER BY name`,
	)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	defer rows.Close()
	indexes := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index name: %v", err)
		}
		indexes[name] = true
	}
	want := []string{
		"idx_tools_session", "idx_tools_name", "idx_tools_target",
		"idx_tools_message_uuid", "idx_tools_use_id",
	}
	for _, w := range want {
		if !indexes[w] {
			t.Errorf("missing index %q after migration; have %v", w, indexes)
		}
	}

	// Re-running is a no-op (idempotent).
	if err := migrateDropToolCallsAutoincrement(pool.Write); err != nil {
		t.Errorf("second run errored: %v", err)
	}
}

func TestMigrateDropToolCallsAutoincrement_NoOpOnFreshDB(t *testing.T) {
	// Fresh DBs ship without AUTOINCREMENT in the static schema, so the
	// migration must detect that and exit cleanly without recreating.
	pool, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()
	if err := migrateDropToolCallsAutoincrement(pool.Write); err != nil {
		t.Errorf("expected no-op, got %v", err)
	}
}
