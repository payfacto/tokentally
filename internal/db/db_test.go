package db_test

import (
	"database/sql"
	"fmt"
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

func TestOpen_InspectorColumns(t *testing.T) {
	conn := openMem(t)
	columnExists := func(table, col string) bool {
		rows, err := conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
		if err != nil {
			t.Fatalf("pragma table_info(%s): %v", table, err)
		}
		defer rows.Close()
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt any
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				t.Fatalf("scan: %v", err)
			}
			if name == col {
				return true
			}
		}
		return false
	}
	for _, want := range []struct{ table, col string }{
		{"messages", "thinking_text"},
		{"messages", "tokens_before"},
		{"messages", "tokens_after"},
		{"tool_calls", "tool_use_id"},
		{"tool_calls", "input_json"},
		{"tool_calls", "output_text"},
		{"tool_calls", "duration_ms"},
	} {
		if !columnExists(want.table, want.col) {
			t.Errorf("column %s.%s not found after Open", want.table, want.col)
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
	// individual token columns must be present for per-prompt cost calculation
	assertInt64(t, p, "input_tokens", 500)
	assertInt64(t, p, "output_tokens", 200)
	assertInt64(t, p, "cache_read_tokens", 50)
	assertInt64(t, p, "cache_create_5m_tokens", 10)
	assertInt64(t, p, "cache_create_1h_tokens", 5)
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

func insertToolCall(t *testing.T, conn *sql.DB, fields map[string]any) {
	t.Helper()
	_, err := conn.Exec(`
		INSERT INTO tool_calls (message_uuid, session_id, project_slug, tool_name, target, result_tokens, is_error, timestamp)
		VALUES (?,?,?,?,?,?,?,?)`,
		fields["message_uuid"], fields["session_id"], fields["project_slug"],
		fields["tool_name"], fields["target"],
		toInt64(fields["result_tokens"]), toInt64(fields["is_error"]),
		fields["timestamp"],
	)
	if err != nil {
		t.Fatalf("insertToolCall failed: %v", err)
	}
}

func TestOpen_WALMode(t *testing.T) {
	// For :memory: WAL mode is not applied (that's by design), so just verify Open succeeds.
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	// Ping to verify connection is live.
	if err := conn.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestRangeClause_Until(t *testing.T) {
	clause, args := db.RangeClause("", "2025-12-31", "timestamp")
	if clause == "" {
		t.Error("expected non-empty clause for until")
	}
	if len(args) != 1 || args[0] != "2025-12-31" {
		t.Errorf("unexpected args: %v", args)
	}
	if !strings.Contains(clause, "<") {
		t.Errorf("until clause should use '<', got: %s", clause)
	}
}

func TestModelBreakdown(t *testing.T) {
	conn := openMem(t)
	insertMessage(t, conn, map[string]any{
		"uuid": "mb1", "session_id": "s1", "project_slug": "proj1",
		"type": "assistant", "timestamp": "2025-06-01T10:00:00Z",
		"model": "claude-3-5-sonnet", "input_tokens": 100, "output_tokens": 50,
	})
	insertMessage(t, conn, map[string]any{
		"uuid": "mb2", "session_id": "s2", "project_slug": "proj1",
		"type": "assistant", "timestamp": "2025-06-02T10:00:00Z",
		"model": "claude-3-opus", "input_tokens": 200, "output_tokens": 80,
	})
	// A user row — should be excluded from ModelBreakdown (type='assistant' only).
	insertMessage(t, conn, map[string]any{
		"uuid": "mb3", "session_id": "s1", "project_slug": "proj1",
		"type": "user", "timestamp": "2025-06-01T09:59:00Z",
		"model": "claude-3-5-sonnet",
	})

	rows, err := db.ModelBreakdown(conn, "", "")
	if err != nil {
		t.Fatalf("ModelBreakdown failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 model rows, got %d", len(rows))
	}
	// claude-3-opus has more billable tokens (280) — should be first.
	if rows[0]["model"] != "claude-3-opus" {
		t.Errorf("expected claude-3-opus first (more tokens), got %q", rows[0]["model"])
	}
}

func TestDailyBreakdown(t *testing.T) {
	conn := openMem(t)
	insertMessage(t, conn, map[string]any{
		"uuid": "db1", "session_id": "s1", "project_slug": "proj1",
		"type": "assistant", "timestamp": "2025-06-01T10:00:00Z",
		"input_tokens": 100, "output_tokens": 40,
	})
	insertMessage(t, conn, map[string]any{
		"uuid": "db2", "session_id": "s1", "project_slug": "proj1",
		"type": "assistant", "timestamp": "2025-06-02T10:00:00Z",
		"input_tokens": 200, "output_tokens": 60,
	})

	rows, err := db.DailyBreakdown(conn, "", "")
	if err != nil {
		t.Fatalf("DailyBreakdown failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 day rows, got %d", len(rows))
	}
	// ORDER BY day ASC — 2025-06-01 should come first.
	if rows[0]["day"] != "2025-06-01" {
		t.Errorf("expected first row to be '2025-06-01', got %q", rows[0]["day"])
	}
	if rows[1]["day"] != "2025-06-02" {
		t.Errorf("expected second row to be '2025-06-02', got %q", rows[1]["day"])
	}
}

func TestRecentSessions(t *testing.T) {
	conn := openMem(t)
	// Session s1 ends earlier.
	insertMessage(t, conn, map[string]any{
		"uuid": "rs1", "session_id": "s1", "project_slug": "proj1",
		"type": "assistant", "timestamp": "2025-06-01T10:00:00Z",
		"input_tokens": 50, "output_tokens": 20,
	})
	// Session s2 ends later — should appear first (ended DESC).
	insertMessage(t, conn, map[string]any{
		"uuid": "rs2", "session_id": "s2", "project_slug": "proj1",
		"type": "assistant", "timestamp": "2025-06-02T10:00:00Z",
		"input_tokens": 80, "output_tokens": 30,
	})

	rows, err := db.RecentSessions(conn, 10, "", "")
	if err != nil {
		t.Fatalf("RecentSessions failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 session rows, got %d", len(rows))
	}
	// Newest session first.
	if rows[0]["session_id"] != "s2" {
		t.Errorf("expected s2 first (more recent ended), got %q", rows[0]["session_id"])
	}
}

func TestSessionTurns(t *testing.T) {
	conn := openMem(t)
	insertMessage(t, conn, map[string]any{
		"uuid": "st1", "session_id": "sess-abc", "project_slug": "proj1",
		"type": "user", "timestamp": "2025-06-01T09:00:00Z",
	})
	insertMessage(t, conn, map[string]any{
		"uuid": "st2", "session_id": "sess-abc", "project_slug": "proj1",
		"type": "assistant", "timestamp": "2025-06-01T09:01:00Z",
		"input_tokens": 100, "output_tokens": 50,
	})
	// A message in a different session — should be excluded.
	insertMessage(t, conn, map[string]any{
		"uuid": "st3", "session_id": "other-sess", "project_slug": "proj1",
		"type": "user", "timestamp": "2025-06-01T08:00:00Z",
	})

	rows, err := db.SessionTurns(conn, "sess-abc")
	if err != nil {
		t.Fatalf("SessionTurns failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 turns for sess-abc, got %d", len(rows))
	}
	// ORDER BY timestamp ASC — user message should come first.
	if rows[0]["uuid"] != "st1" {
		t.Errorf("expected st1 first (earlier timestamp), got %q", rows[0]["uuid"])
	}
	if rows[1]["uuid"] != "st2" {
		t.Errorf("expected st2 second, got %q", rows[1]["uuid"])
	}
}

func TestToolBreakdown(t *testing.T) {
	conn := openMem(t)
	insertToolCall(t, conn, map[string]any{
		"message_uuid": "m1", "session_id": "s1", "project_slug": "proj1",
		"tool_name": "Bash", "timestamp": "2025-06-01T10:00:00Z", "result_tokens": 10,
	})
	insertToolCall(t, conn, map[string]any{
		"message_uuid": "m2", "session_id": "s1", "project_slug": "proj1",
		"tool_name": "Bash", "timestamp": "2025-06-01T10:01:00Z", "result_tokens": 20,
	})
	insertToolCall(t, conn, map[string]any{
		"message_uuid": "m3", "session_id": "s1", "project_slug": "proj1",
		"tool_name": "Read", "timestamp": "2025-06-01T10:02:00Z", "result_tokens": 5,
	})
	// _tool_result rows must be excluded.
	insertToolCall(t, conn, map[string]any{
		"message_uuid": "m4", "session_id": "s1", "project_slug": "proj1",
		"tool_name": "_tool_result", "timestamp": "2025-06-01T10:03:00Z",
	})

	rows, err := db.ToolBreakdown(conn, "", "")
	if err != nil {
		t.Fatalf("ToolBreakdown failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 tool rows (Bash, Read), got %d", len(rows))
	}
	// Bash has 2 calls — should be first (ORDER BY calls DESC).
	if rows[0]["tool_name"] != "Bash" {
		t.Errorf("expected Bash first (2 calls), got %q", rows[0]["tool_name"])
	}
	assertInt64(t, rows[0], "calls", 2)
	assertInt64(t, rows[0], "result_tokens", 30)
}

func TestSkillBreakdown(t *testing.T) {
	conn := openMem(t)
	insertToolCall(t, conn, map[string]any{
		"message_uuid": "m1", "session_id": "s1", "project_slug": "proj1",
		"tool_name": "Skill", "target": "go-testing",
		"timestamp": "2025-06-01T10:00:00Z",
	})
	insertToolCall(t, conn, map[string]any{
		"message_uuid": "m2", "session_id": "s1", "project_slug": "proj1",
		"tool_name": "Skill", "target": "go-testing",
		"timestamp": "2025-06-01T10:05:00Z",
	})
	insertToolCall(t, conn, map[string]any{
		"message_uuid": "m3", "session_id": "s2", "project_slug": "proj1",
		"tool_name": "Skill", "target": "go-error-handling",
		"timestamp": "2025-06-01T11:00:00Z",
	})
	// Non-Skill tool — should not appear in SkillBreakdown.
	insertToolCall(t, conn, map[string]any{
		"message_uuid": "m4", "session_id": "s1", "project_slug": "proj1",
		"tool_name": "Bash", "target": "go-testing",
		"timestamp": "2025-06-01T10:10:00Z",
	})

	rows, err := db.SkillBreakdown(conn, "", "")
	if err != nil {
		t.Fatalf("SkillBreakdown failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 skill rows, got %d", len(rows))
	}
	// go-testing has 2 invocations — should be first (ORDER BY invocations DESC).
	if rows[0]["skill"] != "go-testing" {
		t.Errorf("expected go-testing first (2 invocations), got %q", rows[0]["skill"])
	}
	assertInt64(t, rows[0], "invocations", 2)
	// Both invocations of go-testing are in s1 — distinct sessions = 1.
	assertInt64(t, rows[0], "sessions", 1)
	assertInt64(t, rows[1], "invocations", 1)
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
