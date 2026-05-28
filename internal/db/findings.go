package db

import (
	"encoding/json"
	"fmt"
	"strings"

	"tokentally/internal/findings"
)

// RecomputeFindings loads a session's messages + tool calls, runs the
// detectors and scorer, then clean-replaces the session's rows in the findings
// and session_scores tables. Findings are derived data, so deleting and
// reinserting is safe (unlike the files table).
func RecomputeFindings(p *Pool, sessionID string) error {
	in, err := loadSessionInput(p, sessionID)
	if err != nil {
		return err
	}
	if len(in.Messages) == 0 && len(in.ToolCalls) == 0 {
		return nil
	}
	fs := findings.Detect(in)
	score, grade := findings.Score(in, fs)

	tx, err := p.Write.Begin()
	if err != nil {
		return fmt.Errorf("RecomputeFindings begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM findings WHERE session_id=?`, sessionID); err != nil {
		return fmt.Errorf("clear findings: %w", err)
	}
	for _, f := range fs {
		metaJSON, _ := json.Marshal(f.Meta)
		if _, err := tx.Exec(
			`INSERT INTO findings (session_id, project_slug, kind, severity,
			   est_tokens, detail, meta_json, timestamp)
			 VALUES (?,?,?,?,?,?,?,?)`,
			sessionID, in.ProjectSlug, f.Kind, f.Severity, f.EstTokens,
			f.Detail, string(metaJSON), in.LastActivity,
		); err != nil {
			return fmt.Errorf("insert finding: %w", err)
		}
	}
	if _, err := tx.Exec(
		`INSERT OR REPLACE INTO session_scores
		   (session_id, project_slug, score, grade, timestamp) VALUES (?,?,?,?,?)`,
		sessionID, in.ProjectSlug, score, grade, in.LastActivity,
	); err != nil {
		return fmt.Errorf("upsert score: %w", err)
	}
	return tx.Commit()
}

// loadSessionInput assembles a findings.SessionInput from stored rows.
func loadSessionInput(p *Pool, sessionID string) (findings.SessionInput, error) {
	in := findings.SessionInput{SessionID: sessionID}

	mrows, err := p.Read.Query(
		`SELECT uuid, COALESCE(project_slug,''), type, is_sidechain,
		        COALESCE(model,''), timestamp, input_tokens, output_tokens,
		        cache_create_5m_tokens + cache_create_1h_tokens,
		        COALESCE(prompt_text,''), COALESCE(thinking_text,''),
		        COALESCE(tokens_before,0)
		   FROM messages WHERE session_id=? ORDER BY timestamp ASC`, sessionID)
	if err != nil {
		return in, fmt.Errorf("loadSessionInput messages: %w", err)
	}
	defer mrows.Close()

	byUUID := map[string]int{} // uuid → index into in.Messages
	var dominantModel string
	modelTokens := map[string]int64{}
	for mrows.Next() {
		var m findings.MessageRow
		var sidechain int
		if err := mrows.Scan(&m.UUID, &in.ProjectSlug, &m.Type, &sidechain,
			&m.Model, &m.Timestamp, &m.InputTokens, &m.OutputTokens, &m.CacheCreate,
			&m.PromptText, &m.ThinkingText, &m.TokensBefore); err != nil {
			return in, fmt.Errorf("scan message: %w", err)
		}
		m.IsSidechain = sidechain != 0
		in.LastActivity = m.Timestamp // rows ascending → last wins
		byUUID[m.UUID] = len(in.Messages)
		in.Messages = append(in.Messages, m)
		if m.Model != "" {
			modelTokens[m.Model] += m.InputTokens + m.OutputTokens + m.CacheCreate
			if modelTokens[m.Model] > modelTokens[dominantModel] {
				dominantModel = m.Model
			}
		}
	}
	if err := mrows.Err(); err != nil {
		return in, err
	}
	in.ContextWindow = findings.ContextWindowFor(dominantModel)

	trows, err := p.Read.Query(
		`SELECT message_uuid, tool_name, COALESCE(target,''), is_error, timestamp
		   FROM tool_calls WHERE session_id=? ORDER BY timestamp ASC`, sessionID)
	if err != nil {
		return in, fmt.Errorf("loadSessionInput tools: %w", err)
	}
	defer trows.Close()
	for trows.Next() {
		var muid string
		var tc findings.ToolCallRow
		var isErr int
		if err := trows.Scan(&muid, &tc.ToolName, &tc.Target, &isErr, &tc.Timestamp); err != nil {
			return in, fmt.Errorf("scan tool: %w", err)
		}
		tc.IsError = isErr != 0
		in.ToolCalls = append(in.ToolCalls, tc)
		if idx, ok := byUUID[muid]; ok {
			in.Messages[idx].ToolNames = append(in.Messages[idx].ToolNames, tc.ToolName)
		}
	}
	return in, trows.Err()
}

// FindingsSummary rolls findings up by kind within the date range, ranked by
// total estimated waste. sev_rank: 3=high, 2=med, 1=low (max within the kind).
func FindingsSummary(p *Pool, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT kind,
       SUM(est_tokens) AS est_tokens,
       COUNT(*) AS occurrences,
       COUNT(DISTINCT session_id) AS sessions,
       MAX(CASE severity WHEN 'high' THEN 3 WHEN 'med' THEN 2 ELSE 1 END) AS sev_rank
FROM findings WHERE 1=1` + rng + `
GROUP BY kind ORDER BY est_tokens DESC`
	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("FindingsSummary: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// LowestScoringSessions returns sessions in the range ordered worst-first.
func LowestScoringSessions(p *Pool, since, until string, limit int) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	args = append(args, limit)
	q := `
SELECT session_id, project_slug, score, grade
FROM session_scores WHERE 1=1` + rng + `
ORDER BY score ASC, timestamp DESC LIMIT ?`
	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("LowestScoringSessions: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// SessionFindings returns one session's findings plus its score/grade.
func SessionFindings(p *Pool, sessionID string) (map[string]any, error) {
	rows, err := p.Read.Query(
		`SELECT kind, severity, est_tokens, detail
		   FROM findings WHERE session_id=? ORDER BY est_tokens DESC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("SessionFindings: %w", err)
	}
	defer rows.Close()
	fs, err := scanMaps(rows)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"findings": fs, "score": nil, "grade": nil}
	var score int
	var g string
	err = p.Read.QueryRow(
		`SELECT score, grade FROM session_scores WHERE session_id=?`, sessionID).
		Scan(&score, &g)
	if err == nil {
		out["score"] = int64(score)
		out["grade"] = g
	}
	return out, nil
}

// FindingsBadges returns, per requested session id, its finding count, the max
// severity rank, and grade. Sessions with no findings/score are omitted.
func FindingsBadges(p *Pool, sessionIDs []string) (map[string]any, error) {
	out := map[string]any{}
	if len(sessionIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(sessionIDs))
	args := make([]any, len(sessionIDs))
	for i, id := range sessionIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	in := "(" + strings.Join(placeholders, ",") + ")"

	q := `
SELECT s.session_id, s.grade, s.score,
       COALESCE(f.cnt,0) AS findings,
       COALESCE(f.sev_rank,0) AS sev_rank
FROM session_scores s
LEFT JOIN (
  SELECT session_id, COUNT(*) AS cnt,
         MAX(CASE severity WHEN 'high' THEN 3 WHEN 'med' THEN 2 ELSE 1 END) AS sev_rank
  FROM findings GROUP BY session_id
) f ON f.session_id = s.session_id
WHERE s.session_id IN ` + in
	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("FindingsBadges: %w", err)
	}
	defer rows.Close()
	maps, err := scanMaps(rows)
	if err != nil {
		return nil, err
	}
	for _, m := range maps {
		sid, _ := m["session_id"].(string)
		out[sid] = m
	}
	return out, nil
}
