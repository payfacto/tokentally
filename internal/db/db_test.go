package db_test

import (
	"database/sql"
	"strings"
	"testing"

	"tokentally/internal/db"
)

func openMem(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func tableExists(t *testing.T, conn *sql.DB, name string) bool {
	t.Helper()
	var n int
	err := conn.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name,
	).Scan(&n)
	if err != nil {
		t.Fatalf("tableExists query failed: %v", err)
	}
	return n > 0
}

func TestOpen_CreatesSchema(t *testing.T) {
	conn := openMem(t)
	for _, tbl := range []string{"files", "messages", "tool_calls", "plan", "dismissed_tips"} {
		if !tableExists(t, conn, tbl) {
			t.Errorf("expected table %q to exist after Open", tbl)
		}
	}
}

func TestRangeClause_Empty(t *testing.T) {
	clause, args := db.RangeClause("", "", "timestamp")
	if clause != "" {
		t.Errorf("expected empty clause, got %q", clause)
	}
	if len(args) != 0 {
		t.Errorf("expected nil/empty args, got %v", args)
	}
}

func TestRangeClause_Since(t *testing.T) {
	clause, args := db.RangeClause("2025-01-01", "", "timestamp")
	if clause == "" {
		t.Fatal("expected non-empty clause for since")
	}
	found := false
	for _, a := range args {
		if s, ok := a.(string); ok && s == "2025-01-01" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected args to contain '2025-01-01', got %v", args)
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
	if !strings.Contains(clause, ">=") {
		t.Errorf("clause %q should contain '>='", clause)
	}
}

func TestBestProjectName(t *testing.T) {
	got := db.BestProjectName([]string{`C:\claudecode\myapp`}, "C--claudecode-myapp")
	if got != "myapp" {
		t.Errorf("BestProjectName = %q, want 'myapp'", got)
	}
}

func insertMessage(t *testing.T, conn *sql.DB, fields map[string]any) {
	t.Helper()
	_, err := conn.Exec(`
		INSERT INTO messages (
			uuid, session_id, project_slug, type, timestamp,
			input_tokens, output_tokens, cache_read_tokens,
			cache_create_5m_tokens, cache_create_1h_tokens,
			parent_uuid, model, prompt_text, prompt_chars, is_sidechain
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		fields["uuid"], fields["session_id"], fields["project_slug"],
		fields["type"], fields["timestamp"],
		toInt64(fields["input_tokens"]), toInt64(fields["output_tokens"]),
		toInt64(fields["cache_read_tokens"]), toInt64(fields["cache_create_5m_tokens"]),
		toInt64(fields["cache_create_1h_tokens"]),
		fields["parent_uuid"], fields["model"],
		fields["prompt_text"], fields["prompt_chars"],
		toInt64(fields["is_sidechain"]),
	)
	if err != nil {
		t.Fatalf("insertMessage failed: %v", err)
	}
}

func toInt64(v any) int64 {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	}
	return 0
}

func TestOverviewTotals(t *testing.T) {
	conn := openMem(t)
	// Insert 2 assistant rows with known token counts
	insertMessage(t, conn, map[string]any{
		"uuid": "a1", "session_id": "s1", "project_slug": "proj1",
		"type": "assistant", "timestamp": "2025-06-01T10:00:00Z",
		"input_tokens": 100, "output_tokens": 50,
		"cache_read_tokens": 20, "cache_create_5m_tokens": 10, "cache_create_1h_tokens": 5,
	})
	insertMessage(t, conn, map[string]any{
		"uuid": "a2", "session_id": "s2", "project_slug": "proj2",
		"type": "assistant", "timestamp": "2025-06-02T10:00:00Z",
		"input_tokens": 200, "output_tokens": 80,
		"cache_read_tokens": 30, "cache_create_5m_tokens": 15, "cache_create_1h_tokens": 7,
	})
	// Also a user row to test turn counting
	insertMessage(t, conn, map[string]any{
		"uuid": "u1", "session_id": "s1", "project_slug": "proj1",
		"type": "user", "timestamp": "2025-06-01T09:59:00Z",
	})

	totals, err := db.OverviewTotals(conn, "", "")
	if err != nil {
		t.Fatalf("OverviewTotals failed: %v", err)
	}

	assertInt64(t, totals, "sessions", 2)
	assertInt64(t, totals, "turns", 1)
	assertInt64(t, totals, "input_tokens", 300)
	assertInt64(t, totals, "output_tokens", 130)
	assertInt64(t, totals, "cache_read_tokens", 50)
	assertInt64(t, totals, "cache_create_5m_tokens", 25)
	assertInt64(t, totals, "cache_create_1h_tokens", 12)
}

func assertInt64(t *testing.T, m map[string]any, key string, want int64) {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Errorf("key %q missing from result", key)
		return
	}
	var got int64
	switch x := v.(type) {
	case int64:
		got = x
	case int:
		got = int64(x)
	case float64:
		got = int64(x)
	default:
		t.Errorf("key %q has unexpected type %T", key, v)
		return
	}
	if got != want {
		t.Errorf("key %q = %d, want %d", key, got, want)
	}
}

func TestProjectSummary(t *testing.T) {
	conn := openMem(t)
	insertMessage(t, conn, map[string]any{
		"uuid": "m1", "session_id": "s1", "project_slug": "alpha",
		"type": "assistant", "timestamp": "2025-06-01T10:00:00Z",
		"input_tokens": 100, "output_tokens": 50,
	})
	insertMessage(t, conn, map[string]any{
		"uuid": "m2", "session_id": "s2", "project_slug": "beta",
		"type": "assistant", "timestamp": "2025-06-02T10:00:00Z",
		"input_tokens": 200, "output_tokens": 80,
	})

	rows, err := db.ProjectSummary(conn, "", "")
	if err != nil {
		t.Fatalf("ProjectSummary failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// beta has higher billable tokens (280 vs 150), should be first
	if rows[0]["project_slug"] != "beta" {
		t.Errorf("expected beta first (more tokens), got %q", rows[0]["project_slug"])
	}
}

func TestExpensivePrompts(t *testing.T) {
	conn := openMem(t)
	// user message
	insertMessage(t, conn, map[string]any{
		"uuid": "user1", "session_id": "s1", "project_slug": "proj1",
		"type": "user", "timestamp": "2025-06-01T10:00:00Z",
		"prompt_text": "What is 2+2?", "prompt_chars": 12,
	})
	// assistant response (parent_uuid = user1)
	_, err := conn.Exec(`
		INSERT INTO messages (uuid, session_id, project_slug, type, timestamp,
			input_tokens, output_tokens, cache_create_5m_tokens, cache_create_1h_tokens,
			cache_read_tokens, parent_uuid, model, is_sidechain)
		VALUES ('asst1','s1','proj1','assistant','2025-06-01T10:00:01Z',
			500,200,10,5,50,'user1','claude-3-5-sonnet',0)`,
	)
	if err != nil {
		t.Fatalf("insert assistant failed: %v", err)
	}

	prompts, err := db.ExpensivePrompts(conn, 10, "tokens")
	if err != nil {
		t.Fatalf("ExpensivePrompts failed: %v", err)
	}
	if len(prompts) != 1 {
		t.Fatalf("expected 1 prompt, got %d", len(prompts))
	}
	p := prompts[0]
	if p["user_uuid"] != "user1" {
		t.Errorf("user_uuid = %v, want 'user1'", p["user_uuid"])
	}
	if p["prompt_text"] != "What is 2+2?" {
		t.Errorf("prompt_text = %v, want 'What is 2+2?'", p["prompt_text"])
	}
	// billable = 500+200+10+5 = 715
	assertInt64(t, p, "billable_tokens", 715)
}

func TestGetSetPlan(t *testing.T) {
	conn := openMem(t)

	// default should be "api"
	plan, err := db.GetPlan(conn)
	if err != nil {
		t.Fatalf("GetPlan failed: %v", err)
	}
	if plan != "api" {
		t.Errorf("default plan = %q, want 'api'", plan)
	}

	// set to "max"
	if err := db.SetPlan(conn, "max"); err != nil {
		t.Fatalf("SetPlan failed: %v", err)
	}

	plan, err = db.GetPlan(conn)
	if err != nil {
		t.Fatalf("GetPlan after set failed: %v", err)
	}
	if plan != "max" {
		t.Errorf("plan after set = %q, want 'max'", plan)
	}
}

func TestDismissTip(t *testing.T) {
	conn := openMem(t)

	if err := db.DismissTip(conn, "tip-cache-ratio"); err != nil {
		t.Fatalf("DismissTip failed: %v", err)
	}

	dismissed, err := db.DismissedTips(conn)
	if err != nil {
		t.Fatalf("DismissedTips failed: %v", err)
	}
	if !dismissed["tip-cache-ratio"] {
		t.Errorf("expected 'tip-cache-ratio' in dismissed set, got %v", dismissed)
	}

	// Dismissing again should be idempotent (INSERT OR IGNORE)
	if err := db.DismissTip(conn, "tip-cache-ratio"); err != nil {
		t.Fatalf("DismissTip (duplicate) failed: %v", err)
	}
}
