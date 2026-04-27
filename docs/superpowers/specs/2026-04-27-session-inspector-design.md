# Session Inspector — Design Spec

**Date:** 2026-04-27
**Status:** Approved

## Summary

Port the Session Inspector feature from `claude-command-center` into TokenTally. The inspector renders a Claude Code session as a rich, structured conversation: user turns, AI turns with collapsible thinking blocks and per-tool-call viewers, context compaction boundaries, and recursive subagent trees — all backed by the existing SQLite DB (no JSONL reads at query time).

---

## Architecture Overview

Four bounded layers of change:

```text
internal/scanner  →  extract thinking, compaction, tool inputs/outputs during JSONL walk
internal/db       →  schema migration + GetSessionChunks query helper
app/app.go        →  new GetSessionChunks Wails-bound method returning []SessionChunk
frontend/         →  new Vite sub-project (frontend/inspector/); builds into frontend/web/inspector/
```

**Data flow:**

```text
JSONL → scanner (enhanced) → messages + tool_calls (extended schema)
                                      ↓
                          db.GetSessionChunks → []SessionChunk
                                      ↓
                        app.GetSessionChunks (Wails binding)
                                      ↓
                        Vue inspector (frontend/inspector/)
```

**Frontend integration:** The existing vanilla JS hash router in `app.js` remains untouched for all non-sessions routes. For `#/sessions` and `#/sessions/:id`, it dynamically loads the compiled Vue IIFE bundle (`/web/inspector/index.js`) on first visit and calls `window.SessionInspector.mount(root, hash)`. On navigate away, it calls `window.SessionInspector.unmount()`.

**Build integration:** `wails.json` gains `frontend:install` and `frontend:build` entries that run inside `frontend/inspector/`. Wails embeds the compiled bundle alongside existing vanilla JS. End users receive a single binary — no change to the install process.

---

## DB Schema Changes

Two tables gain new columns via `ALTER TABLE ADD COLUMN` migrations appended to the existing DDL block in `db.Open()`. SQLite's `ALTER TABLE ADD COLUMN` is idempotent-safe when wrapped with a helper that ignores `duplicate column name` errors.

### `messages` — 3 new columns

```sql
ALTER TABLE messages ADD COLUMN thinking_text  TEXT;
ALTER TABLE messages ADD COLUMN tokens_before  INTEGER;
ALTER TABLE messages ADD COLUMN tokens_after   INTEGER;
```

- `thinking_text`: concatenated content of all `"thinking"` blocks in an assistant message.
- `tokens_before` / `tokens_after`: populated only on `type = 'summary'` rows that carry a `<compacted_context>` marker (compaction events).

### `tool_calls` — 4 new columns

```sql
ALTER TABLE tool_calls ADD COLUMN tool_use_id  TEXT;
ALTER TABLE tool_calls ADD COLUMN input_json   TEXT;
ALTER TABLE tool_calls ADD COLUMN output_text  TEXT;
ALTER TABLE tool_calls ADD COLUMN duration_ms  INTEGER;
```

- `tool_use_id`: the `id` field from the `tool_use` content block; used to pair with `tool_result` in the subsequent user message.
- `input_json`: full tool input as a raw JSON object.
- `output_text`: the text content of the matching `tool_result` block.
- `duration_ms`: wall time from the assistant message timestamp to the user message containing the matching result (approximate; sufficient for the UI badge).

### Migration strategy

The four `ALTER TABLE` statements are appended to the DDL block in `db.Open()`. A helper `addColumnIfMissing(db, table, column, def)` runs each statement and swallows `duplicate column name` errors. No versioning table required.

### Backfill

Existing rows lack the new data. A one-time backfill is triggered at startup: if `plan` table key `inspector_backfill_done` is absent, the scanner performs a full rescan of all known JSONL files (ignoring `mtime`/`bytes_read` guards), then sets the flag. Subsequent startups skip the backfill.

---

## Scanner Changes (`internal/scanner`)

The scanner walks JSONL records and upserts into `messages` and `tool_calls`. Four additions:

### 1. Extract thinking blocks (assistant records)

When parsing `message.content` blocks of type `"thinking"`, concatenate their `thinking` field. Store the result as `thinking_text` on the upserted `messages` row.

### 2. Store full tool call inputs (assistant records)

For each `tool_use` content block, store `tool_use_id = block.id` and `input_json = block.input` on the `tool_calls` row. The existing `tool_name` and `target` extraction logic is unchanged.

### 3. Pair tool results (user records)

When a `user` record contains `tool_result` blocks, match each by `tool_use_id` to its corresponding `tool_calls` row (same session, same `tool_use_id`). UPDATE `output_text`, `is_error`, and `duration_ms` (delta between the assistant message timestamp and this user message timestamp, in milliseconds).

### 4. Extract compaction markers (system/summary records)

When a record has `type = "system"` and the message text contains `<compacted_context`, extract `previous_tokens` and `new_tokens` XML attributes. Store as `tokens_before` / `tokens_after` on the `messages` row. The `type` column remains `"summary"` (existing convention).

---

## New Wails Method — `GetSessionChunks`

### Types (`internal/db/chunks.go`)

Types live in `internal/db` — not `app` — to avoid a circular import (`app` already imports `internal/db`). Wails serializes the return value via JSON marshaling regardless of which package defines the types.

```go
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

type ContextAttrib struct {
    ToolOutput int `json:"toolOutput"`
    Thinking   int `json:"thinking"`
    UserText   int `json:"userText"`
}

type SessionChunk struct {
    Type          string           `json:"type"`
    Timestamp     string           `json:"timestamp"`
    Text          string           `json:"text,omitempty"`
    Thinking      string           `json:"thinking,omitempty"`
    ToolCalls     []ToolCallChunk  `json:"toolCalls,omitempty"`
    InputTokens   int              `json:"inputTokens,omitempty"`
    OutputTokens  int              `json:"outputTokens,omitempty"`
    CacheRead     int              `json:"cacheRead,omitempty"`
    ContextAttrib *ContextAttrib   `json:"contextAttrib,omitempty"`
    TokensBefore  int              `json:"tokensBefore,omitempty"`
    TokensAfter   int              `json:"tokensAfter,omitempty"`
}
```

`SessionChunk.Type` values: `"user"` | `"ai"` | `"compact"` | `"system"`. This matches the TypeScript `Chunk` interface in CCC exactly so Vue components require no adaptation to the data shape.

### `db.GetSessionChunks(conn, sessionID) ([]SessionChunk, error)` (`internal/db/db.go`)

1. Query `messages` for the session ordered by `timestamp`, selecting all relevant columns.
2. For each `assistant` row, query `tool_calls WHERE message_uuid = ?` for full tool call detail.
3. Map each row:
   - `type=user` → `SessionChunk{Type:"user", Text:prompt_text, ...}`
   - `type=assistant` → `SessionChunk{Type:"ai", ...}` with thinking, toolCalls, tokens, and heuristic `ContextAttrib`:
     - `ToolOutput = Σ len(output_text)/4` across tool calls
     - `Thinking = len(thinking_text)/4`
     - `UserText = max(0, inputTokens - ToolOutput - Thinking)`
   - `type=summary` with non-nil `tokens_before` → `SessionChunk{Type:"compact", ...}`
   - `type=summary` without compaction data → `SessionChunk{Type:"system", ...}`
4. For `Task`/`Agent` tool calls: extract `SubagentID` from `output_text` JSON (`session_id` field) and `SubagentName` from `input_json` (`description` or `subagent_type` field), truncated to 60 chars.

### `app.GetSessionChunks(sessionID string) ([]SessionChunk, error)` (`app/app.go`)

Thin wrapper: validates `sessionID` is non-empty, delegates to `db.GetSessionChunks`.

---

## Frontend — Vue Micro-App

### Directory structure

```text
frontend/
  web/
    inspector/          ← Vite build output (gitignored)
      index.js
      index.css
  inspector/            ← Vite project source
    src/
      main.ts           ← IIFE entry; exposes window.SessionInspector
      App.vue           ← two-pane layout: sidebar + inspector
      composables/
        useWails.ts     ← wraps window.go.App.* with Vue ref/watch
      components/
        inspector/
          SessionInspector.vue
          UserTurn.vue
          AITurn.vue
          ThinkingBlock.vue
          ContextBadge.vue
          ToolCallFrame.vue
          CompactBoundary.vue
          SystemMessage.vue
          viewers/
            BashViewer.vue
            ReadViewer.vue
            WriteViewer.vue
            DiffViewer.vue
            SearchViewer.vue
            WebViewer.vue
            SubagentTree.vue
            GenericViewer.vue
    package.json
    vite.config.ts
    tsconfig.json
```

### Build configuration

**`vite.config.ts`** — IIFE library mode, single output file, no externals:

```ts
build: {
  lib: { entry: 'src/main.ts', name: 'SessionInspector', formats: ['iife'] },
  outDir: '../web/inspector',
  rollupOptions: { output: { entryFileNames: 'index.js', assetFileNames: 'index.[ext]' } }
}
```

**`wails.json`** additions:

```json
"frontend:install": "cd frontend/inspector && npm install",
"frontend:build":   "cd frontend/inspector && npm run build"
```

**`.gitignore`** addition:

```text
frontend/web/inspector/
```

### `src/main.ts` — global mount/unmount API

```ts
import { createApp, type App as VueApp } from 'vue'
import App from './App.vue'

let instance: VueApp | null = null

window.SessionInspector = {
  mount(el: HTMLElement, hash: string) {
    instance = createApp(App, { initialHash: hash })
    instance.mount(el)
  },
  unmount() {
    instance?.unmount()
    instance = null
  }
}
```

### `composables/useWails.ts`

Thin composable wrapping Wails bindings with Vue reactivity (no Vue Query dependency):

```ts
export function useSessionList(range: Ref<string>) { ... }   // window.go.App.GetSessions
export function useSessionChunks(id: Ref<string>) { ... }    // window.go.App.GetSessionChunks
```

Each returns `{ data, isLoading, error }` backed by `ref` + `watch` + `window.go.App.*`.

### Component source

All inspector components are copied from `claude-command-center/frontend/src/components/inspector/` and `views/Sessions.vue` with the following adaptations:

- Data fetching: replace `useQuery` / REST calls with `useWails.ts` composables.
- Routing: replace Vue Router `useRoute`/`useRouter` with hash parsing from the `initialHash` prop and `window.location.hash` assignments.
- CSS variables: reuse the existing TokenTally CSS custom properties (`--bg`, `--panel`, `--border`, `--text`, `--muted`, `--accent`, `--good`, `--warn`, `--mono`) — they match CCC's variable names.
- `SubagentTree.vue`: calls `window.go.App.GetSessionChunks` recursively for the subagent session ID.

### `App.vue` — two-pane layout

- **Left sidebar (280px):** range selector (Today / 7d / 30d) backed by `useSessionList`; scrollable list of session rows (title, model badge, tokens, date, cwd short). Active row highlighted. Clicking a row sets `#/sessions/:id` via `window.location.hash`.
- **Right pane:** empty state until selection; loading skeleton; `SessionInspector` with the chunk array.
- **Live refresh:** `App.vue` calls `window.runtime.EventsOn('scan', refetch)` on mount and `EventsOff` on unmount, so the session list refreshes after a background scan — consistent with all other routes.

---

## Hash Router Integration (`frontend/web/app.js`)

Two additions to the existing `render()` function:

### 1. Lazy bundle loader (one-time)

```js
let inspectorReady = false;
async function ensureInspector() {
  if (inspectorReady) return;
  await new Promise(resolve => {
    const s = document.createElement('script');
    s.src = '/web/inspector/index.js';
    s.onload = resolve;
    document.head.appendChild(s);
  });
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = '/web/inspector/index.css';
  document.head.appendChild(link);
  inspectorReady = true;
}
```

### 2. Route handler changes

`prevPath` tracking is added to the render loop (new variable, updated at end of each render call).

```js
// Unmount Vue when navigating away from sessions
if (prevPath?.startsWith('/sessions') && !path.startsWith('/sessions')) {
  window.SessionInspector?.unmount();
}

// Sessions routes: load bundle and mount
if (path.startsWith('/sessions')) {
  await ensureInspector();
  window.SessionInspector.mount(root, location.hash);
  return;
}
```

The Vue app reads `initialHash` on mount and subscribes to `hashchange` internally for in-pane navigation (sidebar clicks updating `#/sessions/:id`).

---

## Out of Scope

- Migrating other routes (overview, prompts, projects, skills, tips, settings) to Vue.
- Real-time streaming of in-progress sessions (the existing 30s scan loop is sufficient).
- Tool call diff syntax highlighting (plain text rendering is sufficient for v1).
- Search or filtering within the inspector pane.
