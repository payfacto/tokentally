package db

import (
	"encoding/json"
	"fmt"

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
