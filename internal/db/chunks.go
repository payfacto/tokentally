package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// SessionChunk represents one logical turn in a conversation, structured for
// the Vue inspector frontend. The shape mirrors CCC's TypeScript Chunk interface
// so Vue components require no data-shape adaptation.
type SessionChunk struct {
	Type          string          `json:"type"` // "user"|"ai"|"compact"|"system"
	Timestamp     string          `json:"timestamp"`
	Text          string          `json:"text,omitempty"`
	Thinking      string          `json:"thinking,omitempty"`
	ToolCalls     []ToolCallChunk `json:"toolCalls,omitempty"`
	InputTokens   int             `json:"inputTokens,omitempty"`
	OutputTokens  int             `json:"outputTokens,omitempty"`
	CacheRead     int             `json:"cacheRead,omitempty"`
	ContextAttrib *ContextAttrib  `json:"contextAttrib,omitempty"`
	TokensBefore  int             `json:"tokensBefore,omitempty"`
	TokensAfter   int             `json:"tokensAfter,omitempty"`
}

// ToolCallChunk represents one tool invocation within an AI turn.
type ToolCallChunk struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Input        json.RawMessage `json:"input"`
	Output       string          `json:"output,omitempty"`
	IsError      bool            `json:"isError"`
	DurationMs   int             `json:"durationMs,omitempty"`
	SubagentID   string          `json:"subagentId,omitempty"`
	SubagentName string          `json:"subagentName,omitempty"`
}

// ContextAttrib is a heuristic breakdown of where context tokens come from.
type ContextAttrib struct {
	ToolOutput int `json:"toolOutput"`
	Thinking   int `json:"thinking"`
	UserText   int `json:"userText"`
}

// msgRow holds the raw columns from a single messages row.
type msgRow struct {
	uuid, msgType, ts, promptText, thinkingText string
	inputTok, outputTok, cacheRead              int
	tokensBefore, tokensAfter                   *int
}

// GetSessionChunks reconstructs a session as []SessionChunk from the messages
// and tool_calls tables. It returns chunks ordered by message timestamp ASC.
func GetSessionChunks(conn *sql.DB, sessionID string) ([]SessionChunk, error) {
	rows, err := conn.Query(`
		SELECT uuid, type, timestamp,
		       COALESCE(prompt_text,''), COALESCE(thinking_text,''),
		       COALESCE(input_tokens,0), COALESCE(output_tokens,0), COALESCE(cache_read_tokens,0),
		       tokens_before, tokens_after
		FROM messages WHERE session_id = ? ORDER BY timestamp ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("GetSessionChunks: %w", err)
	}

	// Collect all message rows before closing the cursor so that the nested
	// tool_calls queries below don't compete for the same SQLite connection
	// (critical for :memory: databases used in tests).
	var msgs []msgRow
	for rows.Next() {
		var m msgRow
		if err := rows.Scan(&m.uuid, &m.msgType, &m.ts, &m.promptText, &m.thinkingText,
			&m.inputTok, &m.outputTok, &m.cacheRead, &m.tokensBefore, &m.tokensAfter); err != nil {
			rows.Close()
			return nil, fmt.Errorf("GetSessionChunks scan: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("GetSessionChunks rows close: %w", err)
	}

	chunks := make([]SessionChunk, 0, len(msgs))
	for _, m := range msgs {
		chunk := buildChunk(conn, m.uuid, m.msgType, m.ts, m.promptText, m.thinkingText,
			m.inputTok, m.outputTok, m.cacheRead, m.tokensBefore, m.tokensAfter)
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

func buildChunk(conn *sql.DB, uuid, msgType, ts, promptText, thinkingText string,
	inputTok, outputTok, cacheRead int, tokensBefore, tokensAfter *int) SessionChunk {

	switch msgType {
	case "user", "attachment":
		return SessionChunk{Type: "user", Timestamp: ts, Text: promptText}

	case "assistant":
		tcs := queryToolCalls(conn, uuid)
		attrib := computeAttrib(thinkingText, inputTok, tcs)
		return SessionChunk{
			Type: "ai", Timestamp: ts,
			Thinking: thinkingText, ToolCalls: tcs,
			InputTokens: inputTok, OutputTokens: outputTok, CacheRead: cacheRead,
			ContextAttrib: &attrib,
		}

	case "summary":
		if tokensBefore != nil && tokensAfter != nil {
			return SessionChunk{Type: "compact", Timestamp: ts,
				TokensBefore: *tokensBefore, TokensAfter: *tokensAfter}
		}
		return SessionChunk{Type: "system", Timestamp: ts, Text: promptText}

	case "system":
		return SessionChunk{Type: "system", Timestamp: ts, Text: promptText}

	default:
		return SessionChunk{Type: "system", Timestamp: ts, Text: promptText}
	}
}

func queryToolCalls(conn *sql.DB, messageUUID string) []ToolCallChunk {
	rows, err := conn.Query(`
		SELECT COALESCE(tool_use_id,''), tool_name,
		       COALESCE(input_json,'{}'), COALESCE(output_text,''),
		       is_error, COALESCE(duration_ms,0)
		FROM tool_calls
		WHERE message_uuid = ? AND tool_name != '_tool_result'
		ORDER BY rowid ASC`, messageUUID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []ToolCallChunk
	for rows.Next() {
		var id, name, inputJSON, outputText string
		var isErrInt, durMs int
		if err := rows.Scan(&id, &name, &inputJSON, &outputText, &isErrInt, &durMs); err != nil {
			continue
		}
		tc := ToolCallChunk{
			ID:         id,
			Name:       name,
			Input:      json.RawMessage(inputJSON),
			Output:     outputText,
			IsError:    isErrInt != 0,
			DurationMs: durMs,
		}
		if name == "Task" || name == "Agent" {
			enrichSubagent(&tc, outputText, inputJSON)
		}
		out = append(out, tc)
	}
	return out
}

func enrichSubagent(tc *ToolCallChunk, outputText, inputJSON string) {
	var result struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal([]byte(outputText), &result) == nil {
		tc.SubagentID = result.SessionID
	}
	var input struct {
		Description  string `json:"description"`
		SubagentType string `json:"subagent_type"`
	}
	if json.Unmarshal([]byte(inputJSON), &input) == nil {
		if input.Description != "" {
			tc.SubagentName = truncateRunes(input.Description, 60)
		} else {
			tc.SubagentName = input.SubagentType
		}
	}
}

func computeAttrib(thinking string, inputTok int, tcs []ToolCallChunk) ContextAttrib {
	toolOut := 0
	for _, tc := range tcs {
		toolOut += len(tc.Output) / 4
	}
	thinkTok := len(thinking) / 4
	userText := inputTok - toolOut - thinkTok
	if userText < 0 {
		userText = 0
	}
	return ContextAttrib{ToolOutput: toolOut, Thinking: thinkTok, UserText: userText}
}

func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
