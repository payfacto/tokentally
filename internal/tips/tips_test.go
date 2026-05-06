package tips_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"tokentally/internal/db"
	"tokentally/internal/tips"
)

// openWithUsage opens an in-memory DB and inserts one assistant message with
// enough data to trigger the cache-hit-low tip (cache_read < 20% of input).
func openWithUsage(t *testing.T) *db.Pool {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	_, err = conn.Write.Exec(`
		INSERT INTO messages
		  (uuid, session_id, project_slug, type, timestamp,
		   model, input_tokens, output_tokens, cache_read_tokens,
		   cache_create_5m_tokens, cache_create_1h_tokens)
		VALUES
		  ('u1','s1','proj','assistant','2025-01-01T10:00:00',
		   'claude-sonnet-4-6', 1000, 600, 100, 0, 0)
	`)
	if err != nil {
		t.Fatalf("fixture insert: %v", err)
	}
	return conn
}

func TestAllTips_ReturnsList(t *testing.T) {
	conn := openWithUsage(t)

	result, err := tips.AllTips(conn)
	if err != nil {
		t.Fatalf("AllTips: %v", err)
	}
	// cache-hit-low fires (100/1000 = 10% < 20%) and high-output-ratio fires (600/1000 = 60% > 50%)
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
	conn := openWithUsage(t)

	all, err := tips.AllTips(conn)
	if err != nil {
		t.Fatalf("AllTips: %v", err)
	}
	if len(all) == 0 {
		t.Skip("no tips fired — cannot test dismiss")
	}
	key, _ := all[0]["key"].(string)
	if err := db.DismissTip(conn, key); err != nil {
		t.Fatalf("DismissTip: %v", err)
	}

	after, err := tips.AllTips(conn)
	if err != nil {
		t.Fatalf("AllTips after dismiss: %v", err)
	}
	for _, tip := range after {
		if tip["key"] == key {
			t.Errorf("dismissed tip %q should not appear", key)
		}
	}
}

func TestAllTips_EmptyDB(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	result, err := tips.AllTips(conn)
	if err != nil {
		t.Fatalf("AllTips on empty DB: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected no tips on empty DB, got %d", len(result))
	}
}

func insertTC(t *testing.T, conn *db.Pool, uuid, sessionID, toolName, target, ts string) {
	t.Helper()
	_, err := conn.Write.Exec(
		`INSERT INTO tool_calls (message_uuid, session_id, project_slug, tool_name, target, timestamp, is_error)
		 VALUES (?, ?, 'p1', ?, ?, ?, 0)`,
		uuid, sessionID, toolName, target, ts,
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLowReadEditRatioTip(t *testing.T) {
	p, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	for i := 0; i < 20; i++ {
		insertTC(t, p, fmt.Sprintf("m%d", i), "s1", "Edit", "foo.go", fmt.Sprintf("2025-06-01T10:%02d:00Z", i))
	}
	insertTC(t, p, "mr1", "s1", "Read", "foo.go", "2025-06-01T10:21:00Z")

	result, err := tips.AllTips(p)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tip := range result {
		if tip["key"] == "low-read-edit-ratio" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected low-read-edit-ratio tip to appear")
	}
}

func TestUnusedMCPTip_DoesNotFireWhenUsed(t *testing.T) {
	p, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	insertTC(t, p, "m1", "s1", "mcp__zoho__getTicket", "", "2025-06-01T10:00:00Z")

	orig := tips.ConfiguredMCPLoader
	tips.ConfiguredMCPLoader = func() int { return 1 }
	tips.ResetMCPCache()
	t.Cleanup(func() { tips.ConfiguredMCPLoader = orig; tips.ResetMCPCache() })

	result, err := tips.AllTips(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, tip := range result {
		if tip["key"] == "unused-mcp-servers" {
			t.Fatal("unused-mcp-servers should not fire when MCP servers are used")
		}
	}
}

func TestLongSessionTip_FiresAboveThreshold(t *testing.T) {
	p, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	recent := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	for i := 0; i < 30; i++ {
		_, err = p.Write.Exec(`
			INSERT INTO messages (uuid, session_id, project_slug, type, timestamp)
			VALUES (?, 'long-sess', 'proj', 'user', ?)`,
			fmt.Sprintf("u%d", i), recent,
		)
		if err != nil {
			t.Fatalf("seed user turn: %v", err)
		}
	}

	result, err := tips.AllTips(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, tip := range result {
		if tip["key"] == "long-session" {
			body, _ := tip["body"].(string)
			// Body truncates session id to 8 chars.
			if !strings.Contains(body, "long-ses") || !strings.Contains(body, "30 turns") {
				t.Errorf("expected body to cite truncated session id and turn count, got %q", body)
			}
			return
		}
	}
	t.Fatal("expected long-session tip to fire")
}

func TestLongSessionTip_IgnoresStaleSessions(t *testing.T) {
	p, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	stale := time.Now().UTC().Add(-72 * time.Hour).Format("2006-01-02T15:04:05Z")
	for i := 0; i < 50; i++ {
		_, err = p.Write.Exec(`
			INSERT INTO messages (uuid, session_id, project_slug, type, timestamp)
			VALUES (?, 'old-sess', 'proj', 'user', ?)`,
			fmt.Sprintf("u%d", i), stale,
		)
		if err != nil {
			t.Fatalf("seed user turn: %v", err)
		}
	}

	result, err := tips.AllTips(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, tip := range result {
		if tip["key"] == "long-session" {
			t.Fatal("long-session tip should not fire for sessions outside the lookback window")
		}
	}
}

func TestUnusedMCPTip_FiresWhenConfiguredButNeverCalled(t *testing.T) {
	p, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	// Seed enough sessions to cross the threshold (unusedMCPMinSessions = 5).
	for i := range 5 {
		_, err = p.Write.Exec(`
			INSERT INTO messages (uuid, session_id, project_slug, type, timestamp)
			VALUES (?, ?, 'proj', 'assistant', '2025-01-01T10:00:00Z')`,
			fmt.Sprintf("u%d", i), fmt.Sprintf("s%d", i),
		)
		if err != nil {
			t.Fatalf("seed message: %v", err)
		}
	}

	orig := tips.ConfiguredMCPLoader
	tips.ConfiguredMCPLoader = func() int { return 2 }
	tips.ResetMCPCache()
	t.Cleanup(func() { tips.ConfiguredMCPLoader = orig; tips.ResetMCPCache() })

	result, err := tips.AllTips(p)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tip := range result {
		if tip["key"] == "unused-mcp-servers" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected unused-mcp-servers tip to fire when MCP is configured but never called")
	}
}
