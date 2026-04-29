package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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
// Tool calls are fetched in a single batch query to avoid N+1 round-trips.
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetSessionChunks rows: %w", err)
	}

	// Collect assistant message UUIDs for a single batch tool_calls fetch.
	var assistantUUIDs []string
	for _, m := range msgs {
		if m.msgType == "assistant" {
			assistantUUIDs = append(assistantUUIDs, m.uuid)
		}
	}
	toolCallMap, err := batchQueryToolCalls(conn, assistantUUIDs)
	if err != nil {
		return nil, fmt.Errorf("GetSessionChunks batch tools: %w", err)
	}

	chunks := make([]SessionChunk, 0, len(msgs))
	for _, m := range msgs {
		chunks = append(chunks, buildChunk(m, toolCallMap[m.uuid]))
	}
	return chunks, nil
}

// batchQueryToolCalls fetches all tool calls for the given message UUIDs in one
// query and returns them grouped by message_uuid.
func batchQueryToolCalls(conn *sql.DB, uuids []string) (map[string][]ToolCallChunk, error) {
	if len(uuids) == 0 {
		return map[string][]ToolCallChunk{}, nil
	}
	placeholders := strings.Repeat("?,", len(uuids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(uuids))
	for i, u := range uuids {
		args[i] = u
	}
	rows, err := conn.Query(`
		SELECT message_uuid, COALESCE(tool_use_id,''), tool_name,
		       COALESCE(input_json,'{}'), COALESCE(output_text,''),
		       COALESCE(target,''),
		       is_error, COALESCE(duration_ms,0)
		FROM tool_calls
		WHERE message_uuid IN (`+placeholders+`) AND tool_name != '_tool_result'
		ORDER BY rowid ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("batchQueryToolCalls: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]ToolCallChunk)
	for rows.Next() {
		var msgUUID, id, name, inputJSON, outputText, target string
		var isErrInt, durMs int
		if err := rows.Scan(&msgUUID, &id, &name, &inputJSON, &outputText, &target, &isErrInt, &durMs); err != nil {
			return nil, fmt.Errorf("batchQueryToolCalls scan: %w", err)
		}
		// Pre-migration rows have input_json = NULL → synthesize from target.
		if inputJSON == "{}" && target != "" {
			inputJSON = synthesizeInput(name, target)
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
		result[msgUUID] = append(result[msgUUID], tc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("batchQueryToolCalls rows: %w", err)
	}
	return result, nil
}

func buildChunk(m msgRow, tcs []ToolCallChunk) SessionChunk {
	switch m.msgType {
	case "user", "attachment":
		return SessionChunk{Type: "user", Timestamp: m.ts, Text: m.promptText}

	case "assistant":
		if tcs == nil {
			tcs = []ToolCallChunk{}
		}
		attrib := computeAttrib(m.thinkingText, m.inputTok, tcs)
		return SessionChunk{
			Type: "ai", Timestamp: m.ts,
			Thinking: m.thinkingText, ToolCalls: tcs,
			InputTokens: m.inputTok, OutputTokens: m.outputTok, CacheRead: m.cacheRead,
			ContextAttrib: &attrib,
		}

	case "summary":
		if m.tokensBefore != nil && m.tokensAfter != nil {
			return SessionChunk{Type: "compact", Timestamp: m.ts,
				TokensBefore: *m.tokensBefore, TokensAfter: *m.tokensAfter}
		}
		return SessionChunk{Type: "system", Timestamp: m.ts, Text: m.promptText}

	case "system":
		return SessionChunk{Type: "system", Timestamp: m.ts, Text: m.promptText}

	default:
		return SessionChunk{Type: "system", Timestamp: m.ts, Text: m.promptText}
	}
}

// ToolInputFields maps tool names to the field inside their input object that
// holds the human-readable "target" (file path, query, command, …). Used both
// during ingestion to populate the target column and during query to reconstruct
// input_json for pre-migration rows that have NULL input_json.
var ToolInputFields = map[string]string{
	"Read":      "file_path",
	"Edit":      "file_path",
	"Write":     "file_path",
	"Glob":      "pattern",
	"Grep":      "pattern",
	"Bash":      "command",
	"WebFetch":  "url",
	"WebSearch": "query",
	"Task":      "subagent_type",
	"Skill":     "skill",
}

// synthesizeInput builds a minimal JSON input object from the target string
// when input_json was not stored (pre-migration row).
func synthesizeInput(toolName, target string) string {
	field, ok := ToolInputFields[toolName]
	if !ok {
		field = "input"
	}
	b, err := json.Marshal(map[string]string{field: target})
	if err != nil {
		return "{}"
	}
	return string(b)
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
	userText := max(inputTok-toolOut-thinkTok, 0)
	return ContextAttrib{ToolOutput: toolOut, Thinking: thinkTok, UserText: userText}
}

func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
