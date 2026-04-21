package scanner_test

import (
	"database/sql"
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
