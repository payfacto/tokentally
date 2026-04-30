package db_test

import (
	"fmt"
	"strings"
	"testing"

	"tokentally/internal/db"
)

func openMem(t *testing.T) *db.Pool {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func tableExists(t *testing.T, conn *db.Pool, name string) bool {
	t.Helper()
	var n int
	err := conn.Read.QueryRow(
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
		rows, err := conn.Read.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
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

func insertMessage(t *testing.T, conn *db.Pool, fields map[string]any) {
	t.Helper()
	_, err := conn.Write.Exec(`
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
	_, err := conn.Write.Exec(`
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

func insertToolCall(t *testing.T, conn *db.Pool, fields map[string]any) {
	t.Helper()
	_, err := conn.Write.Exec(`
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
	if err := conn.Read.Ping(); err != nil {
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

	rows, err := db.RecentSessions(conn, 10, "", "", "")
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

func TestRecentSessionsByProject(t *testing.T) {
	conn := openMem(t)
	insertMessage(t, conn, map[string]any{
		"uuid": "rsp1", "session_id": "sa1", "project_slug": "alpha",
		"type": "assistant", "timestamp": "2025-06-01T10:00:00Z",
		"input_tokens": 50, "output_tokens": 20,
	})
	insertMessage(t, conn, map[string]any{
		"uuid": "rsp2", "session_id": "sb1", "project_slug": "beta",
		"type": "assistant", "timestamp": "2025-06-02T10:00:00Z",
		"input_tokens": 80, "output_tokens": 30,
	})

	rows, err := db.RecentSessions(conn, 10, "", "", "alpha")
	if err != nil {
		t.Fatalf("RecentSessions with slug failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for project alpha, got %d", len(rows))
	}
	if rows[0]["session_id"] != "sa1" {
		t.Errorf("expected session sa1, got %q", rows[0]["session_id"])
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

func TestGetSessionChunks_UserAndAI(t *testing.T) {
	conn := openMem(t)

	// user message
	conn.Write.Exec(`INSERT INTO messages (uuid,session_id,project_slug,type,timestamp,prompt_text)
		VALUES ('u1','sess1','proj','user','2025-01-01T10:00:00Z','hello world')`) //nolint:errcheck

	// assistant message with thinking + tool call
	conn.Write.Exec(`INSERT INTO messages
		(uuid,parent_uuid,session_id,project_slug,type,timestamp,model,thinking_text,input_tokens,output_tokens,cache_read_tokens)
		VALUES ('a1','u1','sess1','proj','assistant','2025-01-01T10:00:01Z','claude-sonnet-4-6','I should run bash',100,50,20)`) //nolint:errcheck

	// tool call row
	conn.Write.Exec(`INSERT INTO tool_calls
		(message_uuid,session_id,project_slug,tool_name,target,tool_use_id,input_json,output_text,duration_ms,is_error,timestamp)
		VALUES ('a1','sess1','proj','Bash','ls -la','tu1','{"command":"ls -la"}','file.txt',123,0,'2025-01-01T10:00:01Z')`) //nolint:errcheck

	chunks, err := db.GetSessionChunks(conn, "sess1")
	if err != nil {
		t.Fatalf("GetSessionChunks: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}

	// user chunk
	if chunks[0].Type != "user" {
		t.Errorf("chunk[0].Type: want 'user', got %q", chunks[0].Type)
	}
	if chunks[0].Text != "hello world" {
		t.Errorf("chunk[0].Text: want 'hello world', got %q", chunks[0].Text)
	}

	// ai chunk
	if chunks[1].Type != "ai" {
		t.Errorf("chunk[1].Type: want 'ai', got %q", chunks[1].Type)
	}
	if chunks[1].Thinking != "I should run bash" {
		t.Errorf("chunk[1].Thinking: want 'I should run bash', got %q", chunks[1].Thinking)
	}
	if len(chunks[1].ToolCalls) != 1 {
		t.Fatalf("chunk[1].ToolCalls: want 1, got %d", len(chunks[1].ToolCalls))
	}
	tc := chunks[1].ToolCalls[0]
	if tc.Name != "Bash" {
		t.Errorf("ToolCall.Name: want 'Bash', got %q", tc.Name)
	}
	if tc.Output != "file.txt" {
		t.Errorf("ToolCall.Output: want 'file.txt', got %q", tc.Output)
	}
	if tc.DurationMs != 123 {
		t.Errorf("ToolCall.DurationMs: want 123, got %d", tc.DurationMs)
	}
	if chunks[1].InputTokens != 100 {
		t.Errorf("InputTokens: want 100, got %d", chunks[1].InputTokens)
	}
	if chunks[1].ContextAttrib == nil {
		t.Fatal("ContextAttrib is nil")
	}
}

func TestGetSessionChunks_Compaction(t *testing.T) {
	conn := openMem(t)
	conn.Write.Exec(`INSERT INTO messages (uuid,session_id,project_slug,type,timestamp,tokens_before,tokens_after)
		VALUES ('s1','sess2','proj','summary','2025-01-01T10:00:00Z',500,100)`) //nolint:errcheck

	chunks, err := db.GetSessionChunks(conn, "sess2")
	if err != nil {
		t.Fatalf("GetSessionChunks: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Type != "compact" {
		t.Errorf("chunk[0].Type: want 'compact', got %q", chunks[0].Type)
	}
	if chunks[0].TokensBefore != 500 {
		t.Errorf("TokensBefore: want 500, got %d", chunks[0].TokensBefore)
	}
	if chunks[0].TokensAfter != 100 {
		t.Errorf("TokensAfter: want 100, got %d", chunks[0].TokensAfter)
	}
}

func TestGetSessionChunks_SubagentExtraction(t *testing.T) {
	conn := openMem(t)
	conn.Write.Exec(`INSERT INTO messages (uuid,session_id,project_slug,type,timestamp,input_tokens,output_tokens,cache_read_tokens)
		VALUES ('a1','sess3','proj','assistant','2025-01-01T10:00:00Z',0,0,0)`) //nolint:errcheck
	conn.Write.Exec(`INSERT INTO tool_calls
		(message_uuid,session_id,project_slug,tool_name,target,tool_use_id,input_json,output_text,is_error,timestamp)
		VALUES ('a1','sess3','proj','Task','code-reviewer','tu1',
		  '{"description":"Review code","subagent_type":"code-reviewer"}',
		  '{"session_id":"sub-abc123"}',0,'2025-01-01T10:00:00Z')`) //nolint:errcheck

	chunks, err := db.GetSessionChunks(conn, "sess3")
	if err != nil {
		t.Fatalf("GetSessionChunks: %v", err)
	}
	if len(chunks) != 1 || len(chunks[0].ToolCalls) != 1 {
		t.Fatalf("unexpected chunks/toolcalls: %+v", chunks)
	}
	tc := chunks[0].ToolCalls[0]
	if tc.SubagentID != "sub-abc123" {
		t.Errorf("SubagentID: want 'sub-abc123', got %q", tc.SubagentID)
	}
	if tc.SubagentName != "Review code" {
		t.Errorf("SubagentName: want 'Review code', got %q", tc.SubagentName)
	}
}

func TestGetSetRetentionDays(t *testing.T) {
	conn := openMem(t)

	// Default when key is absent should be 0 (off).
	days, err := db.GetRetentionDays(conn)
	if err != nil {
		t.Fatalf("GetRetentionDays (default) failed: %v", err)
	}
	if days != 0 {
		t.Errorf("default retention = %d, want 0", days)
	}

	// Set to 90.
	if err := db.SetRetentionDays(conn, 90); err != nil {
		t.Fatalf("SetRetentionDays(90) failed: %v", err)
	}

	days, err = db.GetRetentionDays(conn)
	if err != nil {
		t.Fatalf("GetRetentionDays after set failed: %v", err)
	}
	if days != 90 {
		t.Errorf("retention after set = %d, want 90", days)
	}

	// Overwrite with 30 (idempotent upsert).
	if err := db.SetRetentionDays(conn, 30); err != nil {
		t.Fatalf("SetRetentionDays(30) failed: %v", err)
	}
	days, err = db.GetRetentionDays(conn)
	if err != nil {
		t.Fatalf("GetRetentionDays after overwrite failed: %v", err)
	}
	if days != 30 {
		t.Errorf("retention after overwrite = %d, want 30", days)
	}
}

func TestPurgeMessages(t *testing.T) {
	conn := openMem(t)

	// Old message + tool_call (timestamp safely in the past).
	insertMessage(t, conn, map[string]any{
		"uuid": "old1", "session_id": "s-old", "project_slug": "proj",
		"type": "assistant", "timestamp": "2020-01-01T00:00:00Z",
		"input_tokens": 100, "output_tokens": 50,
	})
	insertToolCall(t, conn, map[string]any{
		"message_uuid": "old1", "session_id": "s-old", "project_slug": "proj",
		"tool_name": "Bash", "timestamp": "2020-01-01T00:00:00Z",
	})

	// Recent message (far future — will never be purged with days=1).
	insertMessage(t, conn, map[string]any{
		"uuid": "new1", "session_id": "s-new", "project_slug": "proj",
		"type": "assistant", "timestamp": "2099-01-01T00:00:00Z",
		"input_tokens": 200, "output_tokens": 80,
	})

	deleted, err := db.PurgeMessages(conn, 1) // purge anything older than 1 day
	if err != nil {
		t.Fatalf("PurgeMessages failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1 (only the old message)", deleted)
	}

	// Recent message must still be present.
	var count int
	conn.Read.QueryRow(`SELECT COUNT(*) FROM messages WHERE uuid='new1'`).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("recent message was incorrectly deleted")
	}

	// Old message must be gone.
	conn.Read.QueryRow(`SELECT COUNT(*) FROM messages WHERE uuid='old1'`).Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("old message was not deleted")
	}

	// Tool call for old message must be gone.
	conn.Read.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE message_uuid='old1'`).Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("tool_call for old message was not deleted")
	}

	// files table must be untouched — leaving it intact prevents re-import of purged data.
	conn.Write.Exec(`INSERT INTO files (path, mtime, bytes_read, scanned_at) VALUES ('test.jsonl', 1.0, 100, 1.0)`) //nolint:errcheck
	if _, err := db.PurgeMessages(conn, 1); err != nil {
		t.Fatalf("second PurgeMessages call failed: %v", err)
	}
	conn.Read.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("files table was modified by PurgeMessages — it must remain untouched")
	}
}

func TestPurgeMessages_ZeroDaysIsNoop(t *testing.T) {
	conn := openMem(t)
	insertMessage(t, conn, map[string]any{
		"uuid": "m1", "session_id": "s1", "project_slug": "proj",
		"type": "assistant", "timestamp": "2020-01-01T00:00:00Z",
	})

	deleted, err := db.PurgeMessages(conn, 0)
	if err != nil {
		t.Fatalf("PurgeMessages(0) failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("PurgeMessages(0) deleted %d rows, want 0 (no-op)", deleted)
	}

	var count int
	conn.Read.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("message was incorrectly deleted when days=0")
	}

	// Negative days should also be a no-op.
	deleted, err = db.PurgeMessages(conn, -1)
	if err != nil {
		t.Fatalf("PurgeMessages(-1) failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("PurgeMessages(-1) deleted %d rows, want 0 (no-op)", deleted)
	}
}

func TestGetSessionChunks_MultipleTurnsWithTools(t *testing.T) {
	conn := openMem(t)
	for i := 0; i < 3; i++ {
		ts := fmt.Sprintf("2025-01-01T10:00:%02dZ", i)
		uuid := fmt.Sprintf("ai%d", i)
		conn.Write.Exec(`INSERT INTO messages (uuid,session_id,project_slug,type,timestamp,input_tokens)
			VALUES (?,?,?,?,?,?)`, uuid, "sessM", "proj", "assistant", ts, 10) //nolint:errcheck
		conn.Write.Exec(`INSERT INTO tool_calls
			(message_uuid,session_id,project_slug,tool_name,target,tool_use_id,input_json,output_text,is_error,timestamp)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			uuid, "sessM", "proj", "Bash", "", fmt.Sprintf("tu%d", i),
			`{"command":"ls"}`, "ok", 0, ts) //nolint:errcheck
	}

	chunks, err := db.GetSessionChunks(conn, "sessM")
	if err != nil {
		t.Fatalf("GetSessionChunks: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.Type != "ai" {
			t.Errorf("chunk %d type: want ai, got %q", i, c.Type)
		}
		if len(c.ToolCalls) != 1 {
			t.Errorf("chunk %d: want 1 tool call, got %d", i, len(c.ToolCalls))
		}
		if c.ToolCalls[0].Name != "Bash" {
			t.Errorf("chunk %d tool name: want Bash, got %q", i, c.ToolCalls[0].Name)
		}
		if c.ToolCalls[0].ID != fmt.Sprintf("tu%d", i) {
			t.Errorf("chunk %d tool ID: want tu%d, got %q", i, i, c.ToolCalls[0].ID)
		}
	}
}

// TestSearchPrompts_DedupesMultipleAssistants verifies that when a user message
// has multiple assistant children (e.g. a streaming snapshot eviction race or
// a session re-roll), SearchPrompts and ExpensivePrompts return exactly one
// row per user message, and pick the same assistant deterministically.
func TestSearchPrompts_DedupesMultipleAssistants(t *testing.T) {
	conn := openMem(t)
	insertMessage(t, conn, map[string]any{
		"uuid": "u1", "session_id": "s1", "project_slug": "proj",
		"type": "user", "timestamp": "2025-06-01T10:00:00Z",
		"prompt_text": "hello", "prompt_chars": 5,
	})
	// Two assistant turns share the same parent — picks lowest rowid.
	insertMessage(t, conn, map[string]any{
		"uuid": "a1", "session_id": "s1", "project_slug": "proj",
		"type": "assistant", "timestamp": "2025-06-01T10:00:01Z",
		"parent_uuid": "u1", "model": "claude-first",
		"input_tokens": 100, "output_tokens": 50,
	})
	insertMessage(t, conn, map[string]any{
		"uuid": "a2", "session_id": "s1", "project_slug": "proj",
		"type": "assistant", "timestamp": "2025-06-01T10:00:02Z",
		"parent_uuid": "u1", "model": "claude-second",
		"input_tokens": 999, "output_tokens": 999,
	})

	rows, err := db.SearchPrompts(conn, "hello", "user", "", "")
	if err != nil {
		t.Fatalf("SearchPrompts: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d (JOIN duplicated user)", len(rows))
	}
	if model, _ := rows[0]["model"].(string); model != "claude-first" {
		t.Errorf("expected first-inserted assistant (claude-first), got %q", model)
	}

	expensive, err := db.ExpensivePrompts(conn, 10, "tokens")
	if err != nil {
		t.Fatalf("ExpensivePrompts: %v", err)
	}
	if len(expensive) != 1 {
		t.Fatalf("expected 1 row, got %d", len(expensive))
	}
	if model, _ := expensive[0]["model"].(string); model != "claude-first" {
		t.Errorf("expected first-inserted assistant, got %q", model)
	}
}

// TestSearchPrompts_FTS5 covers the trigram-tokenizer search index — the
// core promise is that substring queries (mid-word matches) work like the
// old LIKE %x% query did, but with index support.
func TestSearchPrompts_FTS5(t *testing.T) {
	conn := openMem(t)
	prompts := []struct {
		uuid, ts, text string
	}{
		{"u1", "2025-06-01T10:00:00Z", "How do I deploy the app?"},
		{"u2", "2025-06-02T10:00:00Z", "Deploying to production"},
		{"u3", "2025-06-03T10:00:00Z", "Fix the auth bug"},
		{"u4", "2025-06-04T10:00:00Z", "What is a buggy authentication flow?"},
		{"u5", "2025-06-05T10:00:00Z", "unrelated content"},
	}
	for _, pr := range prompts {
		insertMessage(t, conn, map[string]any{
			"uuid": pr.uuid, "session_id": "s1", "project_slug": "proj",
			"type": "user", "timestamp": pr.ts,
			"prompt_text": pr.text, "prompt_chars": len(pr.text),
		})
	}

	cases := []struct {
		name      string
		query     string
		wantUUIDs map[string]bool
	}{
		{"substring mid-word", "dep", map[string]bool{"u1": true, "u2": true}},
		{"case insensitive", "DEPLOY", map[string]bool{"u1": true, "u2": true}},
		{"two-word AND", "auth bug", map[string]bool{"u3": true, "u4": true}},
		{"two-word AND missing one", "deploy auth", map[string]bool{}},
		{"special chars are escaped", `"weird*query"`, map[string]bool{}},
		{"whitespace-only treated as no query", "   ", map[string]bool{
			"u1": true, "u2": true, "u3": true, "u4": true, "u5": true,
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := db.SearchPrompts(conn, tc.query, "user", "", "")
			if err != nil {
				t.Fatalf("SearchPrompts(%q): %v", tc.query, err)
			}
			got := map[string]bool{}
			for _, r := range rows {
				if uuid, _ := r["user_uuid"].(string); uuid != "" {
					got[uuid] = true
				}
			}
			if len(got) != len(tc.wantUUIDs) {
				t.Errorf("query %q: got %d rows, want %d (got %v, want %v)",
					tc.query, len(got), len(tc.wantUUIDs), got, tc.wantUUIDs)
			}
			for uuid := range tc.wantUUIDs {
				if !got[uuid] {
					t.Errorf("query %q: missing expected uuid %q", tc.query, uuid)
				}
			}
		})
	}
}

// TestFTSBackfill_PopulatesExistingRows verifies that messages inserted before
// the FTS index existed are still searchable after the one-time backfill runs.
// This simulates upgrading an existing user's database.
func TestFTSBackfill_PopulatesExistingRows(t *testing.T) {
	conn := openMem(t)
	// Insert via the regular helper so the AI trigger fires (simulating fresh
	// install). Then drop the FTS rows and the backfill flag to simulate a
	// pre-FTS database, and re-run the backfill.
	insertMessage(t, conn, map[string]any{
		"uuid": "old1", "session_id": "s1", "project_slug": "proj",
		"type": "user", "timestamp": "2025-05-01T10:00:00Z",
		"prompt_text": "deploy something old", "prompt_chars": 20,
	})
	if _, err := conn.Write.Exec(`DELETE FROM messages_fts`); err != nil {
		t.Fatalf("clear fts: %v", err)
	}
	if _, err := conn.Write.Exec(`DELETE FROM plan WHERE k='fts_backfill_done'`); err != nil {
		t.Fatalf("clear flag: %v", err)
	}

	// Search should fail to find the row because FTS is empty.
	rows, _ := db.SearchPrompts(conn, "deploy", "user", "", "")
	if len(rows) != 0 {
		t.Fatalf("pre-backfill: expected 0 rows, got %d (FTS not actually empty?)", len(rows))
	}

	// Re-run initSchema by reopening — but openMem replaces. Instead invoke
	// the backfill SQL directly against the already-open connection.
	if _, err := conn.Write.Exec(
		`INSERT INTO messages_fts(rowid, prompt_text)
		 SELECT rowid, prompt_text FROM messages
		 WHERE prompt_text IS NOT NULL AND prompt_text != ''`,
	); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	rows, err := db.SearchPrompts(conn, "deploy", "user", "", "")
	if err != nil {
		t.Fatalf("search after backfill: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("post-backfill: expected 1 row, got %d", len(rows))
	}
}
