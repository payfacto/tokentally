package scanner_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"tokentally/internal/db"
	"tokentally/internal/scanner"
)

// testdataDir returns an absolute path to the testdata directory, so tests
// work regardless of the working directory the test runner chooses.
func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "testdata")
}

func openMem(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open(:memory:) failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestScanDir_ParsesTwoMessages(t *testing.T) {
	conn := openMem(t)
	result, err := scanner.ScanDir(conn, testdataDir(t))
	if err != nil {
		t.Fatalf("ScanDir error: %v", err)
	}
	if result.Messages != 2 {
		t.Errorf("expected 2 messages, got %d", result.Messages)
	}
}

func TestScanDir_Incremental(t *testing.T) {
	conn := openMem(t)
	r1, err := scanner.ScanDir(conn, testdataDir(t))
	if err != nil {
		t.Fatalf("first ScanDir error: %v", err)
	}
	if r1.Messages != 2 {
		t.Errorf("first scan: expected 2 messages, got %d", r1.Messages)
	}

	r2, err := scanner.ScanDir(conn, testdataDir(t))
	if err != nil {
		t.Fatalf("second ScanDir error: %v", err)
	}
	if r2.Messages != 0 {
		t.Errorf("second scan: expected 0 new messages (incremental), got %d", r2.Messages)
	}
}

func TestScanDir_TokenCounts(t *testing.T) {
	conn := openMem(t)
	if _, err := scanner.ScanDir(conn, testdataDir(t)); err != nil {
		t.Fatalf("ScanDir error: %v", err)
	}

	totals, err := db.OverviewTotals(conn, "", "")
	if err != nil {
		t.Fatalf("OverviewTotals error: %v", err)
	}

	check := func(key string, want int64) {
		t.Helper()
		got, _ := totals[key].(int64)
		if got != want {
			t.Errorf("%s: expected %d, got %d", key, want, got)
		}
	}
	check("input_tokens", 100)
	check("output_tokens", 50)
	check("cache_read_tokens", 200)
}

func TestScanDir_ToolExtraction(t *testing.T) {
	conn := openMem(t)
	if _, err := scanner.ScanDir(conn, testdataDir(t)); err != nil {
		t.Fatalf("ScanDir error: %v", err)
	}

	var toolName, target string
	err := conn.QueryRow(
		`SELECT tool_name, target FROM tool_calls WHERE tool_name != '_tool_result' LIMIT 1`,
	).Scan(&toolName, &target)
	if err != nil {
		t.Fatalf("query tool_calls: %v", err)
	}
	if toolName != "Bash" {
		t.Errorf("tool_name: expected %q, got %q", "Bash", toolName)
	}
	if target != "ls -la" {
		t.Errorf("target: expected %q, got %q", "ls -la", target)
	}
}

func TestScanDir_PromptText(t *testing.T) {
	conn := openMem(t)
	if _, err := scanner.ScanDir(conn, testdataDir(t)); err != nil {
		t.Fatalf("ScanDir error: %v", err)
	}

	var promptText string
	err := conn.QueryRow(
		`SELECT prompt_text FROM messages WHERE type='user' LIMIT 1`,
	).Scan(&promptText)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if promptText != "hello world" {
		t.Errorf("prompt_text: expected %q, got %q", "hello world", promptText)
	}
}

func TestScanDir_StoresThinkingAndToolInput(t *testing.T) {
	conn := openMem(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "proj-b"), 0755); err != nil {
		t.Fatal(err)
	}
	content := `{"uuid":"t1","sessionId":"sess-think","type":"user","timestamp":"2025-01-02T10:00:00.000Z","message":{"content":[{"type":"text","text":"run echo"}]}}
{"uuid":"t2","parentUuid":"t1","sessionId":"sess-think","type":"assistant","timestamp":"2025-01-02T10:00:01.000Z","message":{"id":"mid2","model":"claude-sonnet-4-6","content":[{"type":"thinking","thinking":"I should use bash"},{"type":"tool_use","name":"Bash","id":"tu-abc","input":{"command":"echo hello"}}],"usage":{"input_tokens":100,"output_tokens":50}}}
`
	if err := os.WriteFile(filepath.Join(dir, "proj-b", "session-think.jsonl"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.ScanDir(conn, dir); err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	var thinkingText string
	if err := conn.QueryRow(`SELECT COALESCE(thinking_text,'') FROM messages WHERE uuid='t2'`).Scan(&thinkingText); err != nil {
		t.Fatalf("query thinking_text: %v", err)
	}
	if thinkingText != "I should use bash" {
		t.Errorf("thinking_text: want 'I should use bash', got %q", thinkingText)
	}

	var toolUseID, inputJSON string
	if err := conn.QueryRow(`SELECT COALESCE(tool_use_id,''), COALESCE(input_json,'') FROM tool_calls WHERE tool_name='Bash'`).Scan(&toolUseID, &inputJSON); err != nil {
		t.Fatalf("query tool_calls: %v", err)
	}
	if toolUseID != "tu-abc" {
		t.Errorf("tool_use_id: want 'tu-abc', got %q", toolUseID)
	}
	if inputJSON == "" {
		t.Error("input_json should not be empty")
	}
}

func TestScanDir_StreamingSnapshotDedup(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj-dedup")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Two records with the same message.id but different uuid — second is the updated snapshot.
	line1 := `{"uuid":"old-uuid","parentUuid":null,"sessionId":"session-dedup","type":"assistant","timestamp":"2025-01-01T10:00:00.000Z","message":{"id":"msg-shared","model":"claude-sonnet-4-6","stop_reason":"end_turn","content":[],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}}}}` + "\n"
	line2 := `{"uuid":"new-uuid","parentUuid":null,"sessionId":"session-dedup","type":"assistant","timestamp":"2025-01-01T10:00:01.000Z","message":{"id":"msg-shared","model":"claude-sonnet-4-6","stop_reason":"end_turn","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}}}}` + "\n"

	jsonlPath := filepath.Join(projDir, "session-dedup.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(line1+line2), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	conn := openMem(t)
	if _, err := scanner.ScanDir(conn, dir); err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM messages WHERE message_id='msg-shared'`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("dedup: expected 1 row with message_id='msg-shared', got %d", count)
	}

	var uuid string
	if err := conn.QueryRow(`SELECT uuid FROM messages WHERE message_id='msg-shared'`).Scan(&uuid); err != nil {
		t.Fatalf("uuid query: %v", err)
	}
	if uuid != "new-uuid" {
		t.Errorf("dedup: expected surviving uuid=%q, got %q", "new-uuid", uuid)
	}
}
