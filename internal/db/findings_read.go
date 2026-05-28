package db

import (
	"fmt"
	"strings"
)

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

// FindingsDistinctSessionCount returns the number of distinct sessions that have
// any finding in the date range. Used by the Findings tab banner to report an
// honest "across N sessions" total rather than the max-of-kinds approximation.
func FindingsDistinctSessionCount(p *Pool, since, until string) (int64, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `SELECT COUNT(DISTINCT session_id) FROM findings WHERE 1=1` + rng
	var n int64
	if err := p.Read.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("FindingsDistinctSessionCount: %w", err)
	}
	return n, nil
}
