# Session Inspector UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve the Session Inspector with sidebar active-scroll, N+1 batch query fix + progressive render, HTML export via Save-As dialog, and icon-only copy buttons on every card.

**Architecture:** Go batch query eliminates per-turn DB round-trips; Vue progressive reveal uses `requestAnimationFrame` to show first 20 chunks before the full list renders. HTML export is generated client-side from existing chunk data; Go provides only the Save-As dialog. Copy buttons use a shared `copyMarkdown` helper that writes Markdown to the clipboard and flashes green.

**Tech Stack:** Go 1.21 + Wails v2, Vue 3 `<script setup>` + TypeScript, Vite IIFE bundle, SQLite via `modernc.org/sqlite`

---

## File Map

### New files
| File | Purpose |
|---|---|
| `frontend/inspector/src/lib/clipboard.ts` | `copyMarkdown(text, btn)` — write to clipboard + flash green |
| `frontend/inspector/src/lib/export.ts` | `generateSessionHTML(chunks, meta)` — build offline HTML string |

### Modified files
| File | Change |
|---|---|
| `internal/db/chunks.go` | `batchQueryToolCalls` replaces per-message `queryToolCalls` loop |
| `internal/db/db_test.go` | Add `TestGetSessionChunks_MultipleTurnsWithTools` |
| `app/app.go` | Add `SaveHTMLExport(html string) (string, error)` |
| `frontend/inspector/src/composables/useWails.ts` | Add `visibleCount` ref + `revealProgressively` |
| `frontend/inspector/src/App.vue` | `scrollIntoView` on mount, `visibleCount` slice, export button |
| `frontend/inspector/src/components/inspector/UserTurn.vue` | Copy button |
| `frontend/inspector/src/components/inspector/AITurn.vue` | Copy button |
| `frontend/inspector/src/components/inspector/CompactBoundary.vue` | Copy button |
| `frontend/inspector/src/components/inspector/SystemMessage.vue` | Copy button |

---

## Task 1: Go — batch tool_calls query (N+1 fix)

**Files:**
- Modify: `internal/db/chunks.go`
- Modify: `internal/db/db_test.go`

- [ ] **Step 1.1: Add test for multiple assistant turns with tool calls**

Append to `internal/db/db_test.go`:

```go
func TestGetSessionChunks_MultipleTurnsWithTools(t *testing.T) {
	conn := openMem(t)
	for i := 0; i < 3; i++ {
		ts := fmt.Sprintf("2025-01-01T10:00:%02dZ", i)
		uuid := fmt.Sprintf("ai%d", i)
		conn.Exec(`INSERT INTO messages (uuid,session_id,project_slug,type,timestamp,input_tokens)
			VALUES (?,?,?,?,?,?)`, uuid, "sessM", "proj", "assistant", ts, 10) //nolint:errcheck
		conn.Exec(`INSERT INTO tool_calls
			(message_uuid,session_id,project_slug,tool_name,target,tool_use_id,input_json,output_text,is_error,timestamp)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			uuid, "sessM", "proj", "Bash", "", fmt.Sprintf("tu%d", i),
			`{"command":"ls"}`, "ok", 0, ts) //nolint:errcheck
	}

	chunks, err := db.GetSessionChunks(conn, "sessM")
	if err != nil {
		t.Fatalf("GetSessionChunks: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.Type != "ai" {
			t.Errorf("chunk %d type: want ai, got %q", i, c.Type)
		}
		if len(c.ToolCalls) != 1 {
			t.Errorf("chunk %d: want 1 tool call, got %d", i, len(c.ToolCalls))
		}
		if c.ToolCalls[0].Name != "Bash" {
			t.Errorf("chunk %d tool name: want Bash, got %q", i, c.ToolCalls[0].Name)
		}
	}
}
```

- [ ] **Step 1.2: Run test to confirm it passes with current code (baseline)**

```
go test ./internal/db/... -run TestGetSessionChunks_MultipleTurnsWithTools -v
```

Expected: `PASS` — this confirms the behavior we're about to preserve.

- [ ] **Step 1.3: Rewrite `internal/db/chunks.go` with batch query**

Replace the entire file content:

```go
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
	toolCallMap := batchQueryToolCalls(conn, assistantUUIDs)

	chunks := make([]SessionChunk, 0, len(msgs))
	for _, m := range msgs {
		chunk := buildChunk(m.msgType, m.ts, m.promptText, m.thinkingText,
			m.inputTok, m.outputTok, m.cacheRead, m.tokensBefore, m.tokensAfter,
			toolCallMap[m.uuid])
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// batchQueryToolCalls fetches all tool calls for the given message UUIDs in one
// query and returns them grouped by message_uuid.
func batchQueryToolCalls(conn *sql.DB, uuids []string) map[string][]ToolCallChunk {
	if len(uuids) == 0 {
		return map[string][]ToolCallChunk{}
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
		       is_error, COALESCE(duration_ms,0)
		FROM tool_calls
		WHERE message_uuid IN (`+placeholders+`) AND tool_name != '_tool_result'
		ORDER BY rowid ASC`, args...)
	if err != nil {
		return map[string][]ToolCallChunk{}
	}
	defer rows.Close()

	result := make(map[string][]ToolCallChunk)
	for rows.Next() {
		var msgUUID, id, name, inputJSON, outputText string
		var isErrInt, durMs int
		if err := rows.Scan(&msgUUID, &id, &name, &inputJSON, &outputText, &isErrInt, &durMs); err != nil {
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
		result[msgUUID] = append(result[msgUUID], tc)
	}
	_ = rows.Err()
	return result
}

func buildChunk(msgType, ts, promptText, thinkingText string,
	inputTok, outputTok, cacheRead int, tokensBefore, tokensAfter *int,
	tcs []ToolCallChunk) SessionChunk {

	switch msgType {
	case "user", "attachment":
		return SessionChunk{Type: "user", Timestamp: ts, Text: promptText}

	case "assistant":
		if tcs == nil {
			tcs = []ToolCallChunk{}
		}
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
```

- [ ] **Step 1.4: Run all chunk tests**

```
go test ./internal/db/... -run TestGetSessionChunks -v
```

Expected: all 4 tests (`_UserAndAI`, `_Compaction`, `_SubagentExtraction`, `_MultipleTurnsWithTools`) PASS.

- [ ] **Step 1.5: Run full test suite**

```
go test ./...
```

Expected: all tests PASS, no failures.

- [ ] **Step 1.6: Commit**

```bash
git add internal/db/chunks.go internal/db/db_test.go
git commit -m "perf: batch tool_calls query to eliminate N+1 in GetSessionChunks"
```

---

## Task 2: Vue — sidebar scroll into view on mount

**Files:**
- Modify: `frontend/inspector/src/App.vue` (script section only)

- [ ] **Step 2.1: Add `nextTick` import and `scrollIntoView` call**

In `App.vue`, update the import line and `onMounted`:

```ts
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
```

Replace the existing `onMounted` block:

```ts
onMounted(() => {
  window.addEventListener('hashchange', onHashChange)
  try { window.runtime.EventsOn('scan', refetchSessions) } catch { /* not in Wails env */ }
  nextTick(() => {
    document.querySelector('.session-row.active')?.scrollIntoView({ block: 'nearest' })
  })
})
```

- [ ] **Step 2.2: Commit**

```bash
git add frontend/inspector/src/App.vue
git commit -m "fix(inspector): scroll active session row into view on mount"
```

---

## Task 3: Vue — progressive render

**Files:**
- Modify: `frontend/inspector/src/composables/useWails.ts`
- Modify: `frontend/inspector/src/App.vue`

- [ ] **Step 3.1: Add `visibleCount` and progressive reveal to `useSessionChunks`**

Replace `useSessionChunks` in `frontend/inspector/src/composables/useWails.ts`:

```ts
export function useSessionChunks(id: Ref<string>) {
  const data = ref<Chunk[]>([])
  const visibleCount = ref(20)
  const isLoading = ref(false)
  const error = ref<string | null>(null)

  function revealProgressively(total: number) {
    if (visibleCount.value >= total) return
    visibleCount.value = Math.min(visibleCount.value + 20, total)
    requestAnimationFrame(() => revealProgressively(total))
  }

  async function refetch() {
    if (!id.value) { data.value = []; visibleCount.value = 20; return }
    isLoading.value = true
    error.value = null
    visibleCount.value = 20
    try {
      data.value = (await window.go.app.App.GetSessionChunks(id.value)) ?? []
      revealProgressively(data.value.length)
    } catch (e) {
      error.value = String(e)
    } finally {
      isLoading.value = false
    }
  }

  watch(id, refetch, { immediate: true })
  return { data, visibleCount, isLoading, error, refetch }
}
```

- [ ] **Step 3.2: Use `visibleCount` slice in `App.vue`**

In `App.vue` script, update the destructure:

```ts
const { data: chunks, visibleCount, isLoading, error } = useSessionChunks(selectedId)
```

In the template, replace the `<SessionInspector>` line:

```html
<SessionInspector :chunks="(chunks.slice(0, visibleCount) as Chunk[])" />
```

- [ ] **Step 3.3: Commit**

```bash
git add frontend/inspector/src/composables/useWails.ts frontend/inspector/src/App.vue
git commit -m "feat(inspector): progressive render — first 20 chunks paint immediately"
```

---

## Task 4: Create `clipboard.ts` helper

**Files:**
- Create: `frontend/inspector/src/lib/clipboard.ts`

- [ ] **Step 4.1: Create the file**

```ts
export async function copyMarkdown(text: string, btn: HTMLElement): Promise<void> {
  await navigator.clipboard.writeText(text)
  btn.style.color = 'var(--good)'
  btn.style.borderColor = 'var(--good)'
  setTimeout(() => { btn.style.color = ''; btn.style.borderColor = '' }, 1200)
}
```

- [ ] **Step 4.2: Commit**

```bash
git add frontend/inspector/src/lib/clipboard.ts
git commit -m "feat(inspector): add copyMarkdown clipboard helper"
```

---

## Task 5: Copy button on `UserTurn.vue`

**Files:**
- Modify: `frontend/inspector/src/components/inspector/UserTurn.vue`

- [ ] **Step 5.1: Replace the entire file**

```vue
<script setup lang="ts">
import type { Chunk } from '../../lib/types'
import { copyMarkdown } from '../../lib/clipboard'

const props = defineProps<{ chunk: Chunk }>()
const fmtTime = (ts: string) => new Date(ts).toLocaleTimeString()

function copyChunk(e: MouseEvent) {
  const md = `**User** · ${fmtTime(props.chunk.timestamp)}\n\n${props.chunk.text ?? ''}`
  copyMarkdown(md, e.currentTarget as HTMLElement)
}
</script>

<template>
  <div class="user-turn">
    <div class="turn-header">
      <span class="badge" style="font-size:10px">you</span>
      <span class="muted" style="font-family:var(--mono);font-size:11px">{{ fmtTime(chunk.timestamp) }}</span>
    </div>
    <div class="turn-text">{{ chunk.text }}</div>
    <button class="copy-btn" title="Copy as Markdown" @click="copyChunk">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
    </button>
  </div>
</template>

<style scoped>
.user-turn { padding: 12px 0; border-bottom: 1px solid var(--border); position: relative; }
.turn-header { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.turn-text { font-size: 13px; line-height: 1.6; white-space: pre-wrap; word-break: break-word; padding-bottom: 20px; }
.muted { color: var(--muted); }
.copy-btn {
  position: absolute; bottom: 6px; right: 0;
  background: transparent; border: 1px solid var(--border);
  border-radius: 4px; padding: 4px 5px;
  cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center;
  line-height: 1; transition: color 120ms, border-color 120ms;
}
.copy-btn:hover { color: var(--text); border-color: var(--text); }
</style>
```

- [ ] **Step 5.2: Commit**

```bash
git add frontend/inspector/src/components/inspector/UserTurn.vue
git commit -m "feat(inspector): copy-as-markdown button on UserTurn"
```

---

## Task 6: Copy button on `CompactBoundary.vue`

**Files:**
- Modify: `frontend/inspector/src/components/inspector/CompactBoundary.vue`

- [ ] **Step 6.1: Replace the entire file**

```vue
<script setup lang="ts">
import type { Chunk } from '../../lib/types'
import { copyMarkdown } from '../../lib/clipboard'

const props = defineProps<{ chunk: Chunk }>()
const fmtTok = (n?: number) => n ? (n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n)) : '?'

function copyChunk(e: MouseEvent) {
  const md = `**Context compacted** · ${fmtTok(props.chunk.tokensBefore)} → ${fmtTok(props.chunk.tokensAfter)} tokens`
  copyMarkdown(md, e.currentTarget as HTMLElement)
}
</script>

<template>
  <div class="compact-boundary">
    <div class="compact-line" />
    <div class="compact-label">
      ⚡ Context compacted — {{ fmtTok(chunk.tokensBefore) }} → {{ fmtTok(chunk.tokensAfter) }} tokens
    </div>
    <div class="compact-line" />
    <button class="copy-btn" title="Copy as Markdown" @click="copyChunk">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
    </button>
  </div>
</template>

<style scoped>
.compact-boundary { display: flex; align-items: center; gap: 10px; padding: 12px 0; position: relative; }
.compact-line { flex: 1; height: 1px; background: var(--accent); opacity: 0.4; }
.compact-label { font-size: 11px; font-family: var(--mono); color: var(--accent); white-space: nowrap; }
.copy-btn {
  position: absolute; bottom: 4px; right: 0;
  background: transparent; border: 1px solid var(--border);
  border-radius: 4px; padding: 4px 5px;
  cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center;
  line-height: 1; transition: color 120ms, border-color 120ms;
}
.copy-btn:hover { color: var(--text); border-color: var(--text); }
</style>
```

- [ ] **Step 6.2: Commit**

```bash
git add frontend/inspector/src/components/inspector/CompactBoundary.vue
git commit -m "feat(inspector): copy-as-markdown button on CompactBoundary"
```

---

## Task 7: Copy button on `SystemMessage.vue`

**Files:**
- Modify: `frontend/inspector/src/components/inspector/SystemMessage.vue`

- [ ] **Step 7.1: Replace the entire file**

```vue
<script setup lang="ts">
import type { Chunk } from '../../lib/types'
import { copyMarkdown } from '../../lib/clipboard'

const props = defineProps<{ chunk: Chunk }>()

function copyChunk(e: MouseEvent) {
  const ts = props.chunk.timestamp.slice(11, 19)
  const md = `**System** · ${ts}\n\n${props.chunk.text ?? ''}`
  copyMarkdown(md, e.currentTarget as HTMLElement)
}
</script>

<template>
  <div class="system-msg muted">
    <span style="font-size:10px;font-family:var(--mono)">system</span>
    <span style="font-size:12px;margin-left:8px;flex:1">{{ chunk.text }}</span>
    <button class="copy-btn" title="Copy as Markdown" @click="copyChunk">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
    </button>
  </div>
</template>

<style scoped>
.system-msg { padding: 6px 0; border-bottom: 1px solid var(--border); display: flex; align-items: center; }
.muted { color: var(--muted); }
.copy-btn {
  background: transparent; border: 1px solid var(--border);
  border-radius: 4px; padding: 4px 5px;
  cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center;
  line-height: 1; transition: color 120ms, border-color 120ms; flex-shrink: 0;
}
.copy-btn:hover { color: var(--text); border-color: var(--text); }
</style>
```

- [ ] **Step 7.2: Commit**

```bash
git add frontend/inspector/src/components/inspector/SystemMessage.vue
git commit -m "feat(inspector): copy-as-markdown button on SystemMessage"
```

---

## Task 8: Copy button on `AITurn.vue`

**Files:**
- Modify: `frontend/inspector/src/components/inspector/AITurn.vue`

- [ ] **Step 8.1: Add `copyMarkdown` import and `copyChunk` function to `AITurn.vue`**

Replace the `<script setup>` block:

```vue
<script setup lang="ts">
import { computed } from 'vue'
import type { Chunk, ToolCallChunk } from '../../lib/types'
import { copyMarkdown } from '../../lib/clipboard'
import ThinkingBlock from './ThinkingBlock.vue'
import ContextBadge from './ContextBadge.vue'
import ToolCallFrame from './ToolCallFrame.vue'
import GenericViewer from './viewers/GenericViewer.vue'
import ReadViewer from './viewers/ReadViewer.vue'
import WriteViewer from './viewers/WriteViewer.vue'
import DiffViewer from './viewers/DiffViewer.vue'
import BashViewer from './viewers/BashViewer.vue'
import SearchViewer from './viewers/SearchViewer.vue'
import WebViewer from './viewers/WebViewer.vue'
import SubagentTree from './viewers/SubagentTree.vue'

const props = defineProps<{ chunk: Chunk; depth?: number }>()

const fmtTime = (ts: string) => new Date(ts).toLocaleTimeString()
const fmtTok = (n?: number) => {
  if (!n) return '0'
  return n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n)
}

function viewerFor(tc: ToolCallChunk) {
  if (tc.name === 'Read') return ReadViewer
  if (tc.name === 'Write') return WriteViewer
  if (tc.name === 'Edit' || tc.name === 'MultiEdit') return DiffViewer
  if (tc.name === 'Bash') return BashViewer
  if (tc.name === 'Grep' || tc.name === 'Glob') return SearchViewer
  if (tc.name === 'WebFetch' || tc.name === 'WebSearch') return WebViewer
  if ((tc.name === 'Task' || tc.name === 'Agent') && tc.subagentId) return SubagentTree
  return GenericViewer
}

function buildMarkdown(chunk: Chunk): string {
  const ts = fmtTime(chunk.timestamp)
  let md = `**Assistant** · ${ts}\n${fmtTok(chunk.inputTokens)} in · ${fmtTok(chunk.outputTokens)} out`

  if (chunk.thinking) {
    md += `\n\n<details><summary>Thinking</summary>\n\n${chunk.thinking}\n</details>`
  }

  for (const tc of chunk.toolCalls ?? []) {
    const inputStr = JSON.stringify(tc.input, null, 2)
    const errorPrefix = tc.isError ? '⚠ Error:\n' : ''
    md += `\n\n**Tool: \`${tc.name}\`**\n\`\`\`json\n${inputStr}\n\`\`\`\n**Output:**\n\`\`\`\n${errorPrefix}${tc.output ?? ''}\n\`\`\``
  }

  return md
}

function copyChunk(e: MouseEvent) {
  copyMarkdown(buildMarkdown(props.chunk), e.currentTarget as HTMLElement)
}
</script>
```

- [ ] **Step 8.2: Add copy button to the template**

Replace the `<template>` block:

```vue
<template>
  <div class="ai-turn">
    <div class="turn-header">
      <span class="badge sonnet" style="font-size:10px">claude</span>
      <span class="muted" style="font-family:var(--mono);font-size:11px">{{ fmtTime(chunk.timestamp) }}</span>
      <span class="spacer" />
      <ContextBadge
        v-if="chunk.contextAttrib && chunk.inputTokens"
        :attrib="chunk.contextAttrib"
        :inputTokens="chunk.inputTokens"
      />
    </div>

    <ThinkingBlock v-if="chunk.thinking" :text="chunk.thinking" />

    <div v-for="tc in (chunk.toolCalls ?? [])" :key="tc.id">
      <ToolCallFrame :toolCall="tc">
        <component :is="viewerFor(tc)" :toolCall="tc" :depth="depth ?? 0" />
      </ToolCallFrame>
    </div>

    <div class="turn-footer">
      <div class="token-row muted">
        <span>in {{ fmtTok(chunk.inputTokens) }}</span>
        <span>out {{ fmtTok(chunk.outputTokens) }}</span>
        <span v-if="chunk.cacheRead">cache {{ fmtTok(chunk.cacheRead) }}</span>
      </div>
      <button class="copy-btn" title="Copy as Markdown" @click="copyChunk">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
      </button>
    </div>
  </div>
</template>
```

- [ ] **Step 8.3: Update the `<style scoped>` block**

Replace styles:

```vue
<style scoped>
.ai-turn { padding: 12px 0; border-bottom: 1px solid var(--border); }
.turn-header { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.spacer { flex: 1; }
.turn-footer { display: flex; align-items: center; margin-top: 8px; }
.token-row { display: flex; gap: 12px; font-family: var(--mono); font-size: 10px; flex: 1; }
.muted { color: var(--muted); }
.copy-btn {
  background: transparent; border: 1px solid var(--border);
  border-radius: 4px; padding: 4px 5px;
  cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center;
  line-height: 1; transition: color 120ms, border-color 120ms; flex-shrink: 0;
}
.copy-btn:hover { color: var(--text); border-color: var(--text); }
</style>
```

- [ ] **Step 8.4: Commit**

```bash
git add frontend/inspector/src/components/inspector/AITurn.vue
git commit -m "feat(inspector): copy-as-markdown button on AITurn"
```

---

## Task 9: Build Vue bundle and verify copy buttons

**Files:** (build artifacts only)

- [ ] **Step 9.1: Build the inspector bundle**

```bash
npm run build --prefix frontend/inspector
```

Expected output ends with:
```
frontend/web/inspector/index.js  (some size)
frontend/web/inspector/index.css (some size)
✓ built in ...ms
```

- [ ] **Step 9.2: Build the Wails app**

```bash
wails build -platform windows/amd64
```

Expected: `build/bin/tokentally.exe` produced with no errors.

- [ ] **Step 9.3: Smoke test copy buttons**

Run `build/bin/tokentally.exe`, navigate to Sessions, open a session, and:
- Verify a copy icon appears in the bottom-right of each turn card
- Click a copy button on a user turn — paste into a text editor and confirm Markdown format
- Click a copy button on an AI turn with tool calls — paste and confirm tool call blocks are present
- Confirm the button briefly turns green then resets

- [ ] **Step 9.4: Commit build artifacts**

```bash
git add frontend/web/inspector/index.js frontend/web/inspector/index.css
git commit -m "build: rebuild inspector bundle with copy buttons and progressive render"
```

---

## Task 10: Create `export.ts`

**Files:**
- Create: `frontend/inspector/src/lib/export.ts`

- [ ] **Step 10.1: Create the file**

```ts
import type { Chunk, ToolCallChunk } from './types'

export interface SessionMeta {
  sessionId: string
  projectName: string
  started: string
  ended: string
}

function escHtml(s: string): string {
  return (s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function fmtTok(n?: number): string {
  if (!n) return '0'
  return n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n)
}

function fmtDate(ts: string): string {
  return ts ? ts.slice(0, 16).replace('T', ' ') : '—'
}

function renderToolCall(tc: ToolCallChunk): string {
  const inputStr = escHtml(JSON.stringify(tc.input, null, 2))
  const outputStr = escHtml(tc.output ?? '')
  const errorBadge = tc.isError ? '<span class="error-badge">⚠ Error</span> ' : ''
  return `<div class="tool-call">
  <div class="tool-name">⚙ ${escHtml(tc.name)}</div>
  <pre class="tool-pre">${inputStr}</pre>
  <div class="tool-output-label">${errorBadge}Output</div>
  <pre class="tool-pre">${outputStr}</pre>
</div>`
}

function renderChunk(chunk: Chunk): string {
  const ts = chunk.timestamp.slice(11, 19)
  switch (chunk.type) {
    case 'user':
      return `<div class="turn user-turn">
  <div class="turn-header"><span class="badge">you</span><span class="ts">${ts}</span></div>
  <div class="turn-text">${escHtml(chunk.text ?? '')}</div>
</div>`

    case 'ai': {
      const thinking = chunk.thinking
        ? `<details class="thinking"><summary>Thinking</summary><pre>${escHtml(chunk.thinking)}</pre></details>`
        : ''
      const tools = (chunk.toolCalls ?? []).map(renderToolCall).join('\n')
      return `<div class="turn ai-turn">
  <div class="turn-header"><span class="badge ai-badge">claude</span><span class="ts">${ts}</span></div>
  ${thinking}
  ${tools}
  <div class="token-row">${fmtTok(chunk.inputTokens)} in · ${fmtTok(chunk.outputTokens)} out${chunk.cacheRead ? ` · ${fmtTok(chunk.cacheRead)} cache` : ''}</div>
</div>`
    }

    case 'compact':
      return `<div class="compact-boundary">⚡ Context compacted — ${fmtTok(chunk.tokensBefore)} → ${fmtTok(chunk.tokensAfter)} tokens</div>`

    case 'system':
      return `<div class="turn system-turn"><span class="system-label">system</span> ${escHtml(chunk.text ?? '')}</div>`

    default:
      return ''
  }
}

const CSS = `
*{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#fff;--text:#1a1a1a;--muted:#666;--border:#e0e0e0;--panel:#f5f5f5;--accent:#7c3aed;--mono:'Menlo','Consolas',monospace}
@media(prefers-color-scheme:dark){:root{--bg:#1a1a1a;--text:#e0e0e0;--muted:#999;--border:#333;--panel:#252525;--accent:#9d72ff}}
body{background:var(--bg);color:var(--text);font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:13px;line-height:1.5;padding:16px}
header{max-width:860px;margin:0 auto 24px;padding-bottom:12px;border-bottom:2px solid var(--accent)}
h1{font-size:18px;font-weight:600;margin-bottom:6px}
.meta{display:flex;gap:16px;font-size:11px;font-family:var(--mono);color:var(--muted)}
main{max-width:860px;margin:0 auto}
.turn{padding:12px 0;border-bottom:1px solid var(--border)}
.turn-header{display:flex;align-items:center;gap:8px;margin-bottom:8px}
.badge{font-size:10px;font-family:var(--mono);background:var(--panel);border:1px solid var(--border);border-radius:3px;padding:2px 6px}
.ai-badge{background:rgba(124,58,237,.15);border-color:var(--accent);color:var(--accent)}
.ts{font-size:10px;font-family:var(--mono);color:var(--muted)}
.turn-text{white-space:pre-wrap;word-break:break-word;font-size:13px;line-height:1.6}
.thinking{margin:8px 0;border:1px solid var(--border);border-radius:4px;padding:6px 10px;font-size:11px;color:var(--muted)}
.thinking summary{cursor:pointer;font-family:var(--mono)}
.thinking pre{margin-top:6px;white-space:pre-wrap;font-size:11px}
.tool-call{margin:6px 0;border:1px solid var(--border);border-radius:4px;overflow:hidden;font-size:11px}
.tool-name{padding:4px 8px;font-family:var(--mono);font-size:11px;background:var(--panel);color:var(--muted);border-bottom:1px solid var(--border)}
.tool-pre{padding:8px;background:var(--bg);font-family:var(--mono);font-size:11px;white-space:pre-wrap;word-break:break-word;max-height:300px;overflow:auto}
.tool-output-label{padding:2px 8px;font-size:10px;font-family:var(--mono);color:var(--muted);background:var(--panel);border-top:1px solid var(--border);border-bottom:1px solid var(--border)}
.error-badge{color:#e53e3e}
.token-row{margin-top:6px;font-size:10px;font-family:var(--mono);color:var(--muted)}
.compact-boundary{padding:10px 0;font-size:11px;font-family:var(--mono);color:var(--accent);text-align:center;border-bottom:1px solid var(--border)}
.system-turn{padding:6px 0;border-bottom:1px solid var(--border);font-size:12px;color:var(--muted)}
.system-label{font-family:var(--mono);font-size:10px;margin-right:8px}
`

export function generateSessionHTML(chunks: Chunk[], meta: SessionMeta): string {
  const totalIn = chunks.reduce((s, c) => s + (c.inputTokens ?? 0), 0)
  const totalOut = chunks.reduce((s, c) => s + (c.outputTokens ?? 0), 0)
  const title = escHtml(meta.projectName || meta.sessionId.slice(0, 8))
  const body = chunks.map(renderChunk).join('\n')

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>${title} — TokenTally Session</title>
  <style>${CSS}</style>
</head>
<body>
  <header>
    <h1>${title}</h1>
    <div class="meta">
      <span>${escHtml(meta.sessionId.slice(0, 8))}</span>
      <span>${fmtDate(meta.started)} → ${fmtDate(meta.ended)}</span>
      <span>${fmtTok(totalIn)} in · ${fmtTok(totalOut)} out</span>
    </div>
  </header>
  <main>
${body}
  </main>
</body>
</html>`
}
```

- [ ] **Step 10.2: Commit**

```bash
git add frontend/inspector/src/lib/export.ts
git commit -m "feat(inspector): generateSessionHTML for offline export"
```

---

## Task 11: Go — `SaveHTMLExport` method

**Files:**
- Modify: `app/app.go`

- [ ] **Step 11.1: Add `"os"` to imports in `app/app.go`**

The existing import block starts with:
```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
```

Add `"os"` to the list (alphabetical order, between `"net/http"` and `"time"`):
```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
```

- [ ] **Step 11.2: Add `SaveHTMLExport` after `PurgeOlderThan`**

Insert after the `PurgeOlderThan` method (around line 446):

```go
// SaveHTMLExport opens a native Save-As dialog and writes the provided HTML
// to the chosen path. Returns the saved path, or empty string if the user cancelled.
func (a *App) SaveHTMLExport(html string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export session as HTML",
		DefaultFilename: "session.html",
		Filters: []runtime.FileFilter{
			{DisplayName: "HTML files (*.html)", Pattern: "*.html"},
		},
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(html), 0644); err != nil {
		return "", fmt.Errorf("SaveHTMLExport: %w", err)
	}
	return path, nil
}
```

- [ ] **Step 11.3: Add `SaveHTMLExport` to the TypeScript declaration in `useWails.ts`**

In `frontend/inspector/src/composables/useWails.ts`, extend the `App` type inside the `declare global` block:

```ts
App: {
  GetSessions(limit: number, since: string, until: string): Promise<Session[]>
  GetSessionChunks(sessionId: string): Promise<Chunk[]>
  SaveHTMLExport(html: string): Promise<string>
}
```

- [ ] **Step 11.4: Compile to verify no errors**

```bash
go build ./...
```

Expected: no output (clean build).

- [ ] **Step 11.4: Commit**

```bash
git add app/app.go
git commit -m "feat: add SaveHTMLExport method with native Save-As dialog"
```

---

## Task 12: Wire export button in `App.vue` and final build

**Files:**
- Modify: `frontend/inspector/src/App.vue`

- [ ] **Step 12.1: Add `generateSessionHTML` import and export state to `App.vue`**

Add to the imports at the top of `<script setup>`:

```ts
import { generateSessionHTML } from './lib/export'
import type { SessionMeta } from './lib/export'
```

Add after the existing `ref` declarations:

```ts
const exportMsg = ref('')
```

Add after the `exportMsg` declaration:

```ts
async function exportHTML() {
  const meta: SessionMeta = {
    sessionId: selectedId.value,
    projectName: selectedSession.value?.project_name ?? '',
    started: selectedSession.value?.started ?? '',
    ended: (chunks.value as Chunk[]).at(-1)?.timestamp ?? '',
  }
  const html = generateSessionHTML(chunks.value as Chunk[], meta)
  const path = await window.go.app.App.SaveHTMLExport(html)
  if (path) {
    exportMsg.value = 'Saved'
    setTimeout(() => { exportMsg.value = '' }, 2000)
  }
}
```

- [ ] **Step 12.2: Add export button to the inspector header in the template**

Replace the existing `<div class="inspector-header">` block:

```html
<div class="inspector-header">
  <span style="font-weight:600;font-size:14px">
    {{ selectedSession?.project_name || selectedId.slice(0, 8) }}
  </span>
  <span class="muted" style="font-size:11px;font-family:var(--mono);margin-left:8px">
    {{ selectedId.slice(0, 8) }}
  </span>
  <span class="spacer" />
  <span v-if="exportMsg" class="export-msg muted" style="font-size:11px;font-family:var(--mono)">{{ exportMsg }}</span>
  <button class="btn-export" title="Export as HTML" @click="exportHTML">
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
  </button>
</div>
```

- [ ] **Step 12.3: Add `.spacer` and `.btn-export` styles to `<style scoped>`**

Add to the style block (`.spacer` may already exist in the overall app CSS — add only what's missing):

```css
.spacer { flex: 1; }
.btn-export {
  background: transparent; border: 1px solid var(--border);
  border-radius: 4px; padding: 4px 6px;
  cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center;
  line-height: 1; transition: color 120ms, border-color 120ms;
}
.btn-export:hover { color: var(--text); border-color: var(--text); }
.export-msg { margin-right: 8px; }
```

- [ ] **Step 12.4: Rebuild the Vue bundle**

```bash
npm run build --prefix frontend/inspector
```

Expected: clean build, `frontend/web/inspector/index.js` updated.

- [ ] **Step 12.5: Rebuild the Wails app**

```bash
wails build -platform windows/amd64
```

Expected: `build/bin/tokentally.exe` produced with no errors. If you need binding regeneration (new Go method `SaveHTMLExport`), run without `-skipbindings`:

```bash
wails build -platform windows/amd64
```

- [ ] **Step 12.6: Smoke test export**

Run `build/bin/tokentally.exe`, navigate to a session, click the download icon in the header:
- Native Save-As dialog should appear with default filename `session.html`
- Save to Desktop, open in a browser — confirm session renders correctly with dark/light theme
- Confirm `Saved` flash appears briefly in the header after saving

- [ ] **Step 12.7: Commit everything**

```bash
git add frontend/inspector/src/App.vue frontend/inspector/src/lib/export.ts frontend/web/inspector/index.js frontend/web/inspector/index.css
git commit -m "feat(inspector): HTML export via Save-As dialog"
```
