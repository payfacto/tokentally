// Package scanner walks Claude Code JSONL transcript files and ingests new
// content into the token-tally SQLite database.
package scanner

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ScanResult summarises one call to ScanDir.
type ScanResult struct {
	Files    int `json:"files"`
	Messages int `json:"messages"`
	Tools    int `json:"tools"`
}

// targetFields maps a tool name to the field inside its input object that holds
// the human-readable "target" (file path, query, command, …).
var targetFields = map[string]string{
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

const (
	maxTargetLen  = 500
	charsPerToken = 4 // rough approximation for tool result token estimation
)

// jsonlRecord mirrors the top-level structure of a Claude Code JSONL line.
type jsonlRecord struct {
	UUID        string          `json:"uuid"`
	ParentUUID  *string         `json:"parentUuid"`
	SessionID   string          `json:"sessionId"`
	Type        string          `json:"type"`
	Timestamp   string          `json:"timestamp"`
	CWD         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	Version     string          `json:"version"`
	Entrypoint  string          `json:"entrypoint"`
	IsSidechain bool            `json:"isSidechain"`
	AgentID     string          `json:"agentId"`
	PromptID    string          `json:"promptId"`
	Message     json.RawMessage `json:"message"`
}

// messageObject mirrors the nested "message" field.
type messageObject struct {
	ID         string          `json:"id"`
	Model      string          `json:"model"`
	StopReason string          `json:"stop_reason"`
	Usage      usageObject     `json:"usage"`
	Content    json.RawMessage `json:"content"`
}

// usageObject mirrors the "usage" sub-field.
type usageObject struct {
	InputTokens          int                  `json:"input_tokens"`
	OutputTokens         int                  `json:"output_tokens"`
	CacheReadInputTokens int                  `json:"cache_read_input_tokens"`
	CacheCreation        cacheCreationObject  `json:"cache_creation"`
}

// cacheCreationObject mirrors the "cache_creation" sub-field.
type cacheCreationObject struct {
	Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
}

// contentBlock mirrors a single element of a "content" array.
type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Input     json.RawMessage `json:"input"`
	Content   json.RawMessage `json:"content"` // tool_result body
}

// toolCall holds the data for one row in the tool_calls table.
type toolCall struct {
	toolName     string
	target       string
	resultTokens *int
	isError      int
	timestamp    string
}

// fileState holds the persisted scan position for one JSONL file.
type fileState struct {
	mtime     float64
	bytesRead int64
}

// ScanDir walks projectsDir recursively for *.jsonl files and ingests new
// content into conn. Safe to call concurrently with DB readers (WAL mode).
func ScanDir(conn *sql.DB, projectsDir string) (ScanResult, error) {
	var result ScanResult

	err := filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		state, err := loadFileState(conn, path)
		if err != nil {
			return fmt.Errorf("loadFileState %s: %w", path, err)
		}

		currentMtime := float64(info.ModTime().UnixNano()) / 1e9
		if state != nil && state.mtime == currentMtime && state.bytesRead == info.Size() {
			return nil // nothing new
		}

		startByte := int64(0)
		if state != nil && info.Size() > state.bytesRead {
			startByte = state.bytesRead
		}

		slug := projectSlug(path, projectsDir)
		sub, err := scanFile(conn, path, slug, startByte)
		if err != nil {
			return nil // permission or I/O error — skip this file, walk continues
		}

		if err := saveFileState(conn, path, currentMtime, sub.endOffset); err != nil {
			return fmt.Errorf("saveFileState %s: %w", path, err)
		}

		result.Files++
		result.Messages += sub.messages
		result.Tools += sub.tools
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("ScanDir walk: %w", err)
	}
	return result, nil
}

// scanFileResult holds internal scan metrics for one file.
type scanFileResult struct {
	messages  int
	tools     int
	endOffset int64
}

// scanFile reads new JSONL lines from path starting at startByte.
// It stops at a partial (newline-less) line to preserve partial-flush safety.
func scanFile(conn *sql.DB, path, projectSlug string, startByte int64) (scanFileResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return scanFileResult{endOffset: startByte}, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	if startByte > 0 {
		if _, err := f.Seek(startByte, 0); err != nil {
			return scanFileResult{endOffset: startByte}, fmt.Errorf("seek: %w", err)
		}
	}

	var result scanFileResult
	result.endOffset = startByte

	reader := bufio.NewReader(f)
	lineStart := startByte

	for {
		raw, err := reader.ReadBytes('\n')
		if len(raw) == 0 {
			break
		}

		lineEnd := lineStart + int64(len(raw))

		// Partial line — Claude Code is mid-flush; stop and retry next scan.
		if !bytes.HasSuffix(raw, []byte("\n")) {
			break
		}

		msgs, tools, parseErr := processLine(conn, raw, projectSlug)
		if parseErr == nil {
			result.messages += msgs
			result.tools += tools
		}
		// Advance high-water mark past this complete line even on parse errors.
		result.endOffset = lineEnd
		lineStart = lineEnd

		if err != nil {
			break // EOF after a complete line
		}
	}

	return result, nil
}

// processLine parses one JSONL line and inserts a message + tool rows.
// Returns (messagesInserted, toolsInserted, error).
func processLine(conn *sql.DB, raw []byte, slug string) (int, int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return 0, 0, nil
	}

	var rec jsonlRecord
	if err := json.Unmarshal(trimmed, &rec); err != nil {
		return 0, 0, nil // skip malformed JSON
	}
	if rec.UUID == "" || rec.Type == "" || rec.SessionID == "" || rec.Timestamp == "" {
		return 0, 0, nil // skip incomplete records
	}

	msg, tlist, err := parseLine(rec, slug)
	if err != nil {
		return 0, 0, err
	}

	if err := insertMessage(conn, msg); err != nil {
		return 0, 0, fmt.Errorf("insertMessage: %w", err)
	}

	// Clear stale tool rows so full rescans stay idempotent.
	if _, err := conn.Exec(`DELETE FROM tool_calls WHERE message_uuid=?`, rec.UUID); err != nil {
		return 0, 0, fmt.Errorf("delete old tool_calls: %w", err)
	}

	for _, tc := range tlist {
		if err := insertToolCall(conn, rec.UUID, rec.SessionID, slug, rec.Timestamp, tc); err != nil {
			return 0, 0, fmt.Errorf("insertToolCall: %w", err)
		}
	}

	return 1, len(tlist), nil
}

// messageRow holds all fields for one INSERT into the messages table.
type messageRow struct {
	uuid                 string
	parentUUID           *string
	sessionID            string
	projectSlug          string
	cwd                  string
	gitBranch            string
	ccVersion            string
	entrypoint           string
	msgType              string
	isSidechain          int
	agentID              string
	timestamp            string
	model                string
	stopReason           string
	promptID             string
	messageID            string
	inputTokens          int
	outputTokens         int
	cacheReadTokens      int
	cacheCreate5mTokens  int
	cacheCreate1hTokens  int
	promptText           *string
	promptChars          *int
	toolCallsJSON        *string
}

// parseLine converts a decoded JSONL record into a messageRow and tool-call list.
func parseLine(rec jsonlRecord, slug string) (messageRow, []toolCall, error) {
	var msgObj messageObject
	if len(rec.Message) > 0 {
		_ = json.Unmarshal(rec.Message, &msgObj) // best-effort
	}

	var content []contentBlock
	if len(msgObj.Content) > 0 {
		_ = json.Unmarshal(msgObj.Content, &content)
	}

	promptText, promptChars := extractPromptText(rec.Type, content)
	toolUses := extractTools(rec.Timestamp, content)
	toolResults := extractResults(rec.Timestamp, content)
	allTools := make([]toolCall, 0, len(toolUses)+len(toolResults))
	allTools = append(allTools, toolUses...)
	allTools = append(allTools, toolResults...)

	isSidechain := 0
	if rec.IsSidechain {
		isSidechain = 1
	}

	row := messageRow{
		uuid:                rec.UUID,
		parentUUID:          rec.ParentUUID,
		sessionID:           rec.SessionID,
		projectSlug:         slug,
		cwd:                 rec.CWD,
		gitBranch:           rec.GitBranch,
		ccVersion:           rec.Version,
		entrypoint:          rec.Entrypoint,
		msgType:             rec.Type,
		isSidechain:         isSidechain,
		agentID:             rec.AgentID,
		timestamp:           rec.Timestamp,
		model:               msgObj.Model,
		stopReason:          msgObj.StopReason,
		promptID:            rec.PromptID,
		messageID:           msgObj.ID,
		inputTokens:         msgObj.Usage.InputTokens,
		outputTokens:        msgObj.Usage.OutputTokens,
		cacheReadTokens:     msgObj.Usage.CacheReadInputTokens,
		cacheCreate5mTokens: msgObj.Usage.CacheCreation.Ephemeral5m,
		cacheCreate1hTokens: msgObj.Usage.CacheCreation.Ephemeral1h,
		promptText:          promptText,
		promptChars:         promptChars,
		toolCallsJSON:       buildToolCallsJSON(toolUses),
	}

	return row, allTools, nil
}

// extractPromptText returns the joined text content for user-type messages.
func extractPromptText(msgType string, content []contentBlock) (*string, *int) {
	if msgType != "user" {
		return nil, nil
	}
	var parts []string
	for _, b := range content {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	text := strings.Join(parts, "")
	chars := len(text)
	return &text, &chars
}

// extractTools returns tool_use content blocks as toolCall rows.
func extractTools(timestamp string, content []contentBlock) []toolCall {
	var out []toolCall
	for _, b := range content {
		if b.Type != "tool_use" {
			continue
		}
		name := b.Name
		if name == "" {
			name = "unknown"
		}
		out = append(out, toolCall{
			toolName:  name,
			target:    extractTarget(name, b.Input),
			timestamp: timestamp,
		})
	}
	return out
}

// extractResults returns tool_result content blocks as toolCall rows.
func extractResults(timestamp string, content []contentBlock) []toolCall {
	var out []toolCall
	for _, b := range content {
		if b.Type != "tool_result" {
			continue
		}
		chars := resultChars(b.Content)
		tokens := chars / charsPerToken
		isError := 0
		if b.IsError {
			isError = 1
		}
		out = append(out, toolCall{
			toolName:     "_tool_result",
			target:       b.ToolUseID,
			resultTokens: &tokens,
			isError:      isError,
			timestamp:    timestamp,
		})
	}
	return out
}

// resultChars counts the character length of a tool_result body.
func resultChars(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	// Try string body first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return len(s)
	}
	// Try array of text blocks.
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		total := 0
		for _, b := range blocks {
			total += len(b.Text)
		}
		return total
	}
	return 0
}

// extractTarget resolves the "target" field from a tool's input object.
func extractTarget(toolName string, inputRaw json.RawMessage) string {
	field, ok := targetFields[toolName]
	if !ok || len(inputRaw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(inputRaw, &m); err != nil {
		return ""
	}
	val, ok := m[field]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(val, &s); err != nil {
		return ""
	}
	runes := []rune(s)
	if len(runes) > maxTargetLen {
		return string(runes[:maxTargetLen])
	}
	return s
}

// buildToolCallsJSON serialises non-result tool uses as a JSON array.
func buildToolCallsJSON(tools []toolCall) *string {
	if len(tools) == 0 {
		return nil
	}
	type entry struct {
		Name   string `json:"name"`
		Target string `json:"target"`
	}
	entries := make([]entry, 0, len(tools))
	for _, t := range tools {
		entries = append(entries, entry{Name: t.toolName, Target: t.target})
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

// evictPriorSnapshots removes older streaming snapshots for the same
// (session_id, message_id) so token counts are not double-counted.
func evictPriorSnapshots(conn *sql.DB, sessionID, messageID, keepUUID string) error {
	rows, err := conn.Query(
		`SELECT uuid FROM messages WHERE session_id=? AND message_id=? AND uuid!=?`,
		sessionID, messageID, keepUUID,
	)
	if err != nil {
		return fmt.Errorf("evict query: %w", err)
	}

	var uuids []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			rows.Close()
			return fmt.Errorf("evict scan: %w", err)
		}
		uuids = append(uuids, u)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("evict rows: %w", err)
	}
	rows.Close() // close before issuing writes on the same connection

	for _, u := range uuids {
		if _, err := conn.Exec(`DELETE FROM tool_calls WHERE message_uuid=?`, u); err != nil {
			return fmt.Errorf("evict tool_calls %s: %w", u, err)
		}
		if _, err := conn.Exec(`DELETE FROM messages WHERE uuid=?`, u); err != nil {
			return fmt.Errorf("evict messages %s: %w", u, err)
		}
	}
	return nil
}

// insertMessage upserts one row into the messages table.
func insertMessage(conn *sql.DB, row messageRow) error {
	if row.messageID != "" {
		if err := evictPriorSnapshots(conn, row.sessionID, row.messageID, row.uuid); err != nil {
			return err
		}
	}

	const q = `
INSERT OR REPLACE INTO messages
(uuid,parent_uuid,session_id,project_slug,cwd,git_branch,cc_version,entrypoint,
 type,is_sidechain,agent_id,timestamp,model,stop_reason,prompt_id,message_id,
 input_tokens,output_tokens,cache_read_tokens,cache_create_5m_tokens,cache_create_1h_tokens,
 prompt_text,prompt_chars,tool_calls_json)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

	_, err := conn.Exec(q,
		row.uuid, row.parentUUID, row.sessionID, row.projectSlug,
		row.cwd, row.gitBranch, row.ccVersion, row.entrypoint,
		row.msgType, row.isSidechain, row.agentID, row.timestamp,
		row.model, row.stopReason, row.promptID, row.messageID,
		row.inputTokens, row.outputTokens, row.cacheReadTokens,
		row.cacheCreate5mTokens, row.cacheCreate1hTokens,
		row.promptText, row.promptChars, row.toolCallsJSON,
	)
	if err != nil {
		return fmt.Errorf("insert message %s: %w", row.uuid, err)
	}
	return nil
}

// insertToolCall inserts one row into the tool_calls table.
func insertToolCall(conn *sql.DB, messageUUID, sessionID, projectSlug, timestamp string, tc toolCall) error {
	const q = `
INSERT INTO tool_calls
(message_uuid,session_id,project_slug,tool_name,target,result_tokens,is_error,timestamp)
VALUES (?,?,?,?,?,?,?,?)`

	_, err := conn.Exec(q,
		messageUUID, sessionID, projectSlug,
		tc.toolName, tc.target, tc.resultTokens, tc.isError, timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert tool_call for %s: %w", messageUUID, err)
	}
	return nil
}

// projectSlug returns the first path component of filePath relative to root.
func projectSlug(filePath, root string) string {
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return ""
	}
	parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
	return parts[0]
}

// loadFileState fetches the stored mtime and bytes_read for a file.
// Returns nil when no row exists yet.
func loadFileState(conn *sql.DB, path string) (*fileState, error) {
	var state fileState
	err := conn.QueryRow(
		`SELECT mtime, bytes_read FROM files WHERE path=?`, path,
	).Scan(&state.mtime, &state.bytesRead)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query files: %w", err)
	}
	return &state, nil
}

// saveFileState upserts the scan position for a file.
func saveFileState(conn *sql.DB, path string, mtime float64, bytesRead int64) error {
	_, err := conn.Exec(
		`INSERT OR REPLACE INTO files (path, mtime, bytes_read, scanned_at) VALUES (?,?,?,?)`,
		path, mtime, bytesRead, float64(time.Now().UnixNano())/1e9,
	)
	if err != nil {
		return fmt.Errorf("upsert files: %w", err)
	}
	return nil
}
