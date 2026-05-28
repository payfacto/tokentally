package db

import "testing"

// seedRetrySession inserts one session whose tool calls trigger retry-churn.
func seedRetrySession(t *testing.T, p *Pool, sid string) {
	t.Helper()
	_, err := p.Write.Exec(
		`INSERT INTO messages (uuid, session_id, project_slug, type, timestamp,
		   input_tokens, output_tokens) VALUES (?,?,?,?,?,?,?)`,
		sid+"-m1", sid, "proj", "assistant", "2026-05-01T10:00:00Z", 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range []int{1, 1, 1, 0} {
		_, err := p.Write.Exec(
			`INSERT INTO tool_calls (message_uuid, session_id, project_slug,
			   tool_name, target, is_error, timestamp) VALUES (?,?,?,?,?,?,?)`,
			sid+"-m1", sid, "proj", "Read", "a.go", e, "2026-05-01T10:00:0"+itoa(i)+"Z")
		if err != nil {
			t.Fatal(err)
		}
	}
}

func itoa(i int) string { return string(rune('0' + i)) }

func TestRecomputeFindings(t *testing.T) {
	p, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	seedRetrySession(t, p, "sess1")

	if err := RecomputeFindings(p, "sess1"); err != nil {
		t.Fatal(err)
	}

	var kind string
	var est int64
	err = p.Read.QueryRow(
		`SELECT kind, est_tokens FROM findings WHERE session_id='sess1'`).Scan(&kind, &est)
	if err != nil {
		t.Fatalf("expected a finding row: %v", err)
	}
	if kind != "retry-churn" {
		t.Errorf("kind=%s", kind)
	}
	var score int
	if err := p.Read.QueryRow(
		`SELECT score FROM session_scores WHERE session_id='sess1'`).Scan(&score); err != nil {
		t.Fatalf("expected a score row: %v", err)
	}
	if score >= 100 {
		t.Errorf("score should be penalised, got %d", score)
	}
}

func TestRecomputeFindings_ReplacesPrior(t *testing.T) {
	p, _ := Open(":memory:")
	defer p.Close()
	seedRetrySession(t, p, "sess1")
	if err := RecomputeFindings(p, "sess1"); err != nil {
		t.Fatal(err)
	}
	// Re-run must not duplicate (PK is session_id+kind; loader clean-replaces).
	if err := RecomputeFindings(p, "sess1"); err != nil {
		t.Fatal(err)
	}
	var n int
	p.Read.QueryRow(`SELECT COUNT(*) FROM findings WHERE session_id='sess1'`).Scan(&n)
	if n != 1 {
		t.Errorf("want 1 finding after re-run, got %d", n)
	}
}
