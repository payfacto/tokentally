# Session Inspector UX — Design Spec
**Date:** 2026-04-27  
**Status:** Approved

## Overview

Four UX improvements to the Vue 3 Session Inspector:

1. Sidebar active-session highlight (scroll into view on mount)
2. Loading performance: N+1 query fix + client-side progressive render
3. HTML export of entire session as a shareable offline file
4. Copy-as-Markdown button on each inspector card

All changes are scoped to `internal/db/chunks.go`, `app/app.go`, and the `frontend/inspector/` Vue bundle.

---

## Feature 1 — Sidebar active highlight

**Problem:** When navigating from Prompts → a specific session (via `#/sessions/<id>`), the Vue inspector mounts with `selectedId` pre-populated from the URL hash. The `.session-row.active` CSS (border-left + background) already marks the correct row, but if the session is not in the top of the list the row may be off-screen and the user cannot tell which session is selected.

**Fix:** In `App.vue`, after the session list renders, scroll the active row into view.

```ts
// App.vue — onMounted
onMounted(() => {
  window.addEventListener('hashchange', onHashChange)
  try { window.runtime.EventsOn('scan', refetchSessions) } catch {}
  nextTick(() => {
    document.querySelector('.session-row.active')?.scrollIntoView({ block: 'nearest' })
  })
})
```

No new API, no new state. One call, wrapped in `nextTick` so the `v-for` list has rendered before the scroll runs.

---

## Feature 2 — Loading performance

### Root cause

`GetSessionChunks` (`internal/db/chunks.go`) calls `queryToolCalls(conn, uuid)` once per assistant message. A session with 50 AI turns makes 50+ synchronous SQLite round-trips in a for-loop. For large sessions this is the dominant latency.

### Go: batch tool_calls query

Replace the per-message `queryToolCalls` call with a single batch fetch keyed by `message_uuid IN (...)`.

```go
// Instead of: for each msg → queryToolCalls(conn, msg.uuid)
// Do:
func batchQueryToolCalls(conn *sql.DB, uuids []string) map[string][]ToolCallChunk {
    // Build placeholders, run one query, group results by message_uuid
}
```

`GetSessionChunks` collects all message UUIDs in the first pass, calls `batchQueryToolCalls` once, then calls `buildChunk` with the pre-fetched slice for each message. No change to `SessionChunk` shape or the `GetSessionChunks` signature — the optimisation is entirely internal.

### Vue: progressive render

`useSessionChunks` adds a `visibleCount` ref (initial value: 20). After `data.value = chunks`, it schedules incremental reveals:

```ts
function revealProgressively(total: number) {
  if (visibleCount.value >= total) return
  visibleCount.value = Math.min(visibleCount.value + 20, total)
  requestAnimationFrame(() => revealProgressively(total))
}
```

`App.vue` passes `chunks.slice(0, visibleCount)` to `<SessionInspector>`. The first 20 chunks paint immediately; the rest fill in over the next few animation frames (~16 ms each). `visibleCount` resets to 20 on every `id` change.

---

## Feature 3 — HTML export

### Frontend: `export.ts`

New file `frontend/inspector/src/lib/export.ts` exports:

```ts
export function generateSessionHTML(
  chunks: Chunk[],
  meta: { sessionId: string; projectName: string; started: string; ended: string }
): string
```

Produces a fully self-contained HTML string:

- `<style>` block with inline CSS; uses `@media (prefers-color-scheme: dark)` so it renders correctly in both browser themes
- Session header: project name, session ID (truncated), date range, token totals computed from `ai` chunks
- Each chunk rendered as a styled `<div>`: user bubbles, AI turns with tool call accordions, compact-boundary pills, system message blocks
- No external CDN links, no fonts fetched over network — works offline

### Go: `SaveHTMLExport`

New method on `App`:

```go
func (a *App) SaveHTMLExport(html string) (string, error) {
    path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
        Title:           "Export session as HTML",
        DefaultFilename: "session.html",
        Filters: []runtime.FileFilter{
            {DisplayName: "HTML files", Pattern: "*.html"},
        },
    })
    if err != nil || path == "" {
        return "", err // user cancelled → empty path, nil error
    }
    if err := os.WriteFile(path, []byte(html), 0644); err != nil {
        return "", fmt.Errorf("SaveHTMLExport: %w", err)
    }
    return path, nil
}
```

### Wiring in `App.vue`

Export button in `.inspector-header`:

```html
<button class="btn-icon" title="Export as HTML" @click="exportHTML">
  <!-- download SVG icon -->
</button>
```

```ts
async function exportHTML() {
  const html = generateSessionHTML(chunks.value as Chunk[], {
    sessionId: selectedId.value,
    projectName: selectedSession.value?.project_name ?? '',
    started: selectedSession.value?.started ?? '',
    ended: (chunks.value as Chunk[]).at(-1)?.timestamp ?? '',
  })
  const path = await window.go.app.App.SaveHTMLExport(html)
  if (path) exportMsg.value = 'Saved'
  setTimeout(() => { exportMsg.value = '' }, 2000)
}
```

User-cancelled save (empty `path`, no error) is silently ignored.

---

## Feature 4 — Copy-as-Markdown buttons

### Shared helper: `clipboard.ts`

New file `frontend/inspector/src/lib/clipboard.ts`:

```ts
export async function copyMarkdown(text: string, btn: HTMLElement) {
  await navigator.clipboard.writeText(text)
  btn.classList.add('copied')
  setTimeout(() => btn.classList.remove('copied'), 1200)
}
```

`.copied` class sets `color: var(--good)` and `border-color: var(--good)` (same flash pattern as the existing copy button in the sessions turn modal).

### Button placement

Each card component gets an `absolute`-positioned icon-only button at `bottom: 6px; right: 8px`. The card container needs `position: relative` (add if not already set).

```html
<button class="copy-btn" title="Copy as Markdown" @click="copy($event.currentTarget)">
  <!-- copy SVG (same icon as sessions.js) -->
</button>
```

### Markdown format per chunk type

| Component | Output |
|---|---|
| `UserTurn` | `**User** · {timestamp}\n\n{text}` |
| `AITurn` | `**Assistant** · {timestamp} · {model}\n{inputTok} in · {outputTok} out\n\n{text}` + tool calls (see below) |
| `CompactBoundary` | `**Context compacted** · {tokensBefore} → {tokensAfter} tokens` |
| `SystemMessage` | `**System** · {timestamp}\n\n{text}` |

Tool calls within an AI turn (appended after the response text):

```
**Tool: `{name}`**
```json
{input (pretty-printed)}
```
**Output:**
```
{output}
```
```

If the tool call `isError`, prefix the output block with `⚠ Error:\n`.

### Components modified

- `UserTurn.vue` — add copy button, format: user markdown
- `AITurn.vue` — add copy button, format: AI + tools markdown
- `CompactBoundary.vue` — add copy button, format: compact markdown
- `SystemMessage.vue` — add copy button, format: system markdown

---

## File inventory

### New files
| File | Purpose |
|---|---|
| `frontend/inspector/src/lib/export.ts` | `generateSessionHTML` — session → offline HTML |
| `frontend/inspector/src/lib/clipboard.ts` | `copyMarkdown` — shared copy helper |

### Modified files
| File | Change |
|---|---|
| `internal/db/chunks.go` | `batchQueryToolCalls` replaces per-message `queryToolCalls` loop |
| `app/app.go` | Add `SaveHTMLExport(html string) (string, error)` |
| `frontend/inspector/src/composables/useWails.ts` | Add `visibleCount` ref + `revealProgressively` |
| `frontend/inspector/src/App.vue` | `scrollIntoView` on mount, export button + handler, pass `visibleCount` slice |
| `frontend/inspector/src/components/inspector/UserTurn.vue` | Copy button |
| `frontend/inspector/src/components/inspector/AITurn.vue` | Copy button |
| `frontend/inspector/src/components/inspector/CompactBoundary.vue` | Copy button |
| `frontend/inspector/src/components/inspector/SystemMessage.vue` | Copy button |

---

## Out of scope

- Virtual scrolling / windowed rendering (not needed; progressive RAF render is sufficient for typical session sizes)
- Go-side streaming via Wails events (Approach B — complexity not justified by gain)
- Paginated "load more" (Approach C — UX friction for no real benefit after N+1 fix)
- Editing or deleting turns from the inspector
