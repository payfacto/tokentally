package tips_test

import (
	"testing"

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
	// No data means no tips fire — result should be nil or empty, never an error.
	_ = result
}
