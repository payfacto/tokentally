# Handoff — tokentally

## Session — 2026-04-22 10:30

### What Was Done
- Fixed tray "Quit TokenTally" staying in tray — removed `systray.Quit()` from menu handler (was deadlocking Win32 message loop); replaced with direct `os.Exit(0)`
- Fixed Ctrl+C hanging — flipped threading model so Wails owns the main goroutine and systray runs in a locked goroutine; `os.Exit(0)` after `wails.Run()` returns now kills everything cleanly
- Fixed "Open Dashboard" tray button not showing window — added `HideWindowOnClose: true` to keep runtime alive, added always-on-top pulse trick (`WindowSetAlwaysOnTop` briefly true then false) to force foreground focus
- Fixed "TokenTally (Not Responding)" at startup — root cause was Wails in a goroutine with new Go WebView2Loader; moving Wails back to main goroutine resolved it
- Fixed UI stuck on "loading…" forever — JS bindings used `window.go.App.*` but Wails routes via package name: correct path is `window.go.app.App.*` (lowercase `app` = Go package); every API call was silently hanging
- Created `~/.claude/skills/wails-desktop` skill documenting all Wails lessons

### Files Changed
- `app/tray_windows.go` — quit uses `os.Exit(0)` directly; open-dashboard adds always-on-top pulse; `time` import added
- `main.go` — Wails on main goroutine, systray in `go a.StartTray()`; `os.Exit(0)` after `wails.Run()` returns; removed old signal handler approach
- `frontend/web/app.js` — `const App = window.go.app.App` alias; all `window.go.App.*` calls replaced with `App.*`
- `frontend/web/routes/settings.js` — `window.go.App.*` → `window.go.app.App.*`
- `frontend/web/routes/tips.js` — `window.go.App.*` → `window.go.app.App.*`
- `~/.claude/skills/wails-desktop/SKILL.md` — new skill created (not in repo)

### Decisions Made
- Wails on main goroutine, systray in non-main goroutine — `getlantern/systray` calls `runtime.LockOSThread()` internally on Windows so it works in any goroutine; Wails/WebView2 is more stable on the main OS thread with the new Go WebView2Loader
- No `systray.Quit()` in quit handler — can deadlock; OS removes tray icon automatically when process exits via `os.Exit(0)`
- JS bindings alias pattern (`const App = window.go.app.App`) — single place to fix if package name ever changes; cleaner call sites than `window['go']['app']['App']` everywhere
- Binary is built with `go build -tags production -o tokentally.exe .` for iteration; `wails build` for final release (adds proper Windows manifest + DPI settings)

### Inferred Next Steps
- **Smoke-test all 7 tabs** — Overview, Prompts, Sessions, Projects, Skills, Tips, Settings — confirm data renders correctly now that the namespace fix is in
- **Verify tray fully works** — Open Dashboard (focus), Scan Now, Quit — all three menu items
- **Checkpoint the WAL** — DB is 42 MB with a 4.1 MB WAL from a force-killed previous session; run `PRAGMA wal_checkpoint(FULL)` or restart the app cleanly to flush it
- **Consider `wails build` for distribution** — `go build -tags production` skips Windows manifest embedding (DPI awareness, UAC level); use `wails build -platform windows/amd64` for the distributable `.exe`
- **Service install/uninstall** — Settings tab has Install/Uninstall Service buttons; these haven't been tested yet this session

### Open Questions / Blockers
- The `wails-desktop` skill notes the "Open Dashboard" always-on-top trick but it hasn't been confirmed working in this session (window refused to appear before the threading fix; test now that threading is correct)
- `GetServiceStatus` / `InstallService` / `UninstallService` in `app/service_windows.go` — these use PowerShell elevation; not tested
- go.mod declares Wails v2.12.0 but the Wails CLI might be a different patch version — run `wails build` and check for version warnings before shipping

## Session — 2026-04-27 16:00

### What Was Done
- Merged `feature/session-inspector` (16 commits) into `main` — full Session Inspector feature landed
- Fixed `wails.json` `frontend:build` to use `npm run build --prefix` instead of `cd && npm run build` (Wails on Windows can't exec shell builtins like `cd`)
- Fixed `frontend/inspector/vite.config.ts` to define `process.env.NODE_ENV: "production"` — without this the Vue IIFE bundle silently failed in the browser because Vue's dev checks reference `process.env.NODE_ENV` which doesn't exist in a bare browser context; bundle shrank from 115 kB to 79 kB after fix
- Fixed `wails.json` prefix path: Wails runs `frontend:install`/`frontend:build` from the `frontend/` directory, so the path must be `--prefix inspector` (not `--prefix frontend/inspector` which doubled the directory)
- Added Node.js prerequisite and `npm install --prefix frontend/inspector` step to `README.md` and `CLAUDE.md`
- Smoke-tested the Vue inspector via Chrome DevTools MCP: sidebar renders session list, clicking a session loads chunks, user/AI turns render correctly, thinking blocks collapse, tool call frames (Bash, Read, Write) display with output, compact boundary shows correctly, zero JS errors

### Files Changed
- `wails.json` — two fixes: `cd` → `--prefix` for Windows compat (`697de90`), then `frontend/inspector` → `inspector` for correct Wails CWD (`5f0c83a`)
- `frontend/inspector/vite.config.ts` — added `define: { 'process.env.NODE_ENV': '"production"' }` (`7293a47`)
- `README.md` — added Node.js to prerequisites, added `npm install --prefix frontend/inspector` step (`8adb469`)
- `CLAUDE.md` — added `npm install --prefix frontend/inspector` step to Commands section (`8adb469`)

### Decisions Made
- `process.env.NODE_ENV` define in vite.config — required for IIFE builds targeting bare browsers; Vite doesn't automatically replace it in library/IIFE mode
- `wails.json` prefix is `inspector` not `frontend/inspector` — Wails v2 runs frontend lifecycle commands from the `frontend/` directory, not the project root
- README/CLAUDE.md use `--prefix frontend/inspector` (project-root-relative for manual use) while `wails.json` uses `--prefix inspector` (frontend-dir-relative for Wails) — intentionally different

### Inferred Next Steps
- Confirm `wails build -platform windows/amd64 -skipbindings` succeeds end-to-end now that the prefix path is fixed (`5f0c83a`)
- Run the built `tokentally.exe` and do a live smoke test: navigate to Sessions tab, select a real session, verify chunks render with real DB data
- The inspector backfill runs on first launch (clears `files` table, rescans all JSONL, sets `inspector_backfill_done` flag) — verify this completes without errors and the Sessions tab loads after a few seconds

### Open Questions / Blockers
- `wails build` with the corrected `wails.json` has not yet been confirmed working — the path fix (`5f0c83a`) was committed immediately after the build error; needs a fresh build run to verify

## Session — 2026-04-27 22:00

### What Was Done

- Removed the "What do these numbers mean?" glossary `<details>` card from the Overview page — KPI tooltips (added in previous session) made it redundant; committed `8d88a92`
- Brainstormed and designed "Scan Now + Data Retention" feature: Settings page gets a new "Data Management" card with a manual scan trigger and a configurable retention purge (auto on every scan tick, manual on demand)
- Implemented full feature via subagent-driven development across 4 tasks and 9 commits:
  - `GetRetentionDays` / `SetRetentionDays` DB helpers (plan k/v table, `strconv.Atoi` for safe parse, error wrapping) — `224da66`, `2ce4918`
  - `PurgeMessages` DB helper (transactional DELETE of `tool_calls` then `messages` by ISO8601 timestamp cutoff; `files` table intentionally untouched) — `f660897`, `a31b3df`
  - `App.GetRetentionDays`, `App.SetRetentionDays`, `App.PurgeOlderThan` Wails-bound wrappers; `scanLoop` auto-purge on each tick — `4df678b`, `651a82e`
  - Frontend "Data Management" card in Settings: Scan Now button, retention-days input + Save, Purge Now (red, confirm dialog, disabled when days=0) — `f6bd0e8`, `971f4e6`
- Added files-table invariant test and `scanLoop` intent comment — `115e14e`

### Files Changed

- `frontend/web/routes/overview.js` — removed glossary `<details>` card (`8d88a92`)
- `frontend/web/style.css` — removed `.glossary` CSS rules (`8d88a92`)
- `internal/db/db.go` — added `GetRetentionDays`, `SetRetentionDays`, `PurgeMessages`
- `internal/db/db_test.go` — added `TestGetSetRetentionDays`, `TestPurgeMessages`, `TestPurgeMessages_ZeroDaysIsNoop` (including files-table invariant assertion)
- `app/app.go` — added `GetRetentionDays`, `SetRetentionDays`, `PurgeOlderThan` methods; updated `scanLoop` with auto-purge + intent comment
- `frontend/web/routes/settings.js` — added Data Management card HTML + `bindDataManagement` function
- `docs/superpowers/specs/2026-04-27-scan-and-retention-design.md` — design spec (committed)
- `docs/superpowers/plans/2026-04-27-scan-and-retention.md` — implementation plan (committed)

### Decisions Made

- "Stay pruned" invariant: `PurgeMessages` deletes `tool_calls` then `messages` but leaves `files` table intact — the scanner uses `files` rows as "already processed" markers so purged sessions are not re-imported from disk
- `PurgeMessages` wraps both DELETEs in a transaction — prevents partial purge if the second DELETE fails mid-operation
- Auto-purge fires unconditionally after each scan tick regardless of whether `ScanDir` returned an error — purge and scan are independent DB operations; documented with inline comment
- Default retention is off (blank/0) — no data is ever deleted without explicit user opt-in
- `strconv.Atoi` instead of `fmt.Sscanf` in `GetRetentionDays` — surfaces corrupt stored values rather than silently returning 0
- Wails binding stubs (`wailsjs/`) not regenerated — build uses `-skipbindings`; `window.go.app.App.*` direct calls work at runtime without stubs

### Inferred Next Steps

- **Build and live-test** — run `wails build -platform windows/amd64 -skipbindings`, launch `tokentally.exe`, go to Settings and verify the Data Management card renders correctly
- **Test Scan Now** — click it right after launching (before first auto-scan tick) to verify it picks up new sessions immediately and shows the correct count
- **Test retention save + purge** — enter 90, save, verify it persists after navigating away and back; then test Purge Now shows a confirm dialog and reports a deleted count
- **Regenerate Wails bindings** — run `wails build` (without `-skipbindings`) once to update `frontend/wailsjs/go/app/App.js` and `App.d.ts` with the three new methods

### Open Questions / Blockers

- The `-skipbindings` build has not been run since the new Go methods were added — confirm `go build ./...` passes (it does per CI in the subagent runs) and the Wails build succeeds
- `wails build` (full, with binding generation) has not been tested this session

## Session — 2026-04-27 23:30

### What Was Done

- Implemented 4 Session Inspector UX improvements via subagent-driven development (15 commits):
  - **N+1 fix**: `batchQueryToolCalls` replaces per-turn `queryToolCalls` loop — single `WHERE message_uuid IN (...)` query; added `TestGetSessionChunks_MultipleTurnsWithTools` with tool-ID assertion — `937ecbd`, `250046b`
  - **Progressive render**: `useSessionChunks` adds `visibleCount` ref (starts at 20); `revealProgressively` schedules `requestAnimationFrame` increments of 20; first 20 chunks paint immediately — `3ff5813`
  - **Sidebar scroll**: `onMounted` calls `nextTick(() => .session-row.active?.scrollIntoView({ block: 'nearest' }))` — `9a05b45`
  - **Copy buttons**: `clipboard.ts` helper (`copyMarkdown` with try/catch + green flash); icon-only copy SVG button added to `UserTurn`, `CompactBoundary`, `SystemMessage`, `AITurn` — `28b5125`, `397c498`, `bfc8d67`, `3e16e99`, `b691eb2`, `82c559f`, `4cd0fbe`
  - **HTML export**: `export.ts` (`generateSessionHTML` — self-contained HTML with `@media prefers-color-scheme:dark` CSS, all chunks rendered, no external CDN); `app.go` `SaveHTMLExport` method (native Save-As dialog via `runtime.SaveFileDialog` + `os.WriteFile`); download icon button in inspector header — `51580b0`, `3da79ee`, `5a86f08`
- Final code review identified silent scan-error swallow in `batchQueryToolCalls`; fixed `continue` → `return nil, fmt.Errorf("batchQueryToolCalls scan: %w", err)` — `21bd7c3`
- Wails app fully built (`tokentally.exe`, 75 MB) and Vue bundle verified (88 kB)

### Files Changed

- `internal/db/chunks.go` — `batchQueryToolCalls` replaces N+1 loop; `buildChunk` signature cleaned; silent scan error fixed (`21bd7c3`)
- `internal/db/db_test.go` — added `TestGetSessionChunks_MultipleTurnsWithTools` with tool-ID cross-contamination assertion
- `app/app.go` — added `"os"` import + `SaveHTMLExport(html string) (string, error)` method after `PurgeOlderThan`
- `frontend/inspector/src/lib/clipboard.ts` — new file: `copyMarkdown(text, btn)` helper with try/catch
- `frontend/inspector/src/lib/export.ts` — new file: `generateSessionHTML(chunks, meta)` offline HTML generator
- `frontend/inspector/src/composables/useWails.ts` — `visibleCount` ref + `revealProgressively`; `SaveHTMLExport` TypeScript declaration added
- `frontend/inspector/src/App.vue` — `scrollIntoView` on mount; `visibleCount` slice; export button + `exportHTML()` handler; `.spacer`/`.btn-export`/`.export-msg` styles
- `frontend/inspector/src/components/inspector/UserTurn.vue` — icon-only copy button (bottom-right absolute)
- `frontend/inspector/src/components/inspector/AITurn.vue` — copy button in turn-footer flex row; `buildMarkdown` helper
- `frontend/inspector/src/components/inspector/CompactBoundary.vue` — icon-only copy button (bottom-right absolute)
- `frontend/inspector/src/components/inspector/SystemMessage.vue` — icon-only copy button (flex sibling)
- `docs/superpowers/specs/2026-04-27-inspector-ux-design.md` — design spec
- `docs/superpowers/plans/2026-04-27-inspector-ux.md` — implementation plan

### Decisions Made

- Progressive reveal is client-side only (RAF); Go still returns all chunks in one Wails call — simpler than server-side streaming, adequate for typical session sizes
- HTML export CSS uses `:root` vars + `@media(prefers-color-scheme:dark)` override — works offline in both browser themes without a `<script>` toggle
- `copyMarkdown` silently no-ops on clipboard failure (permission denied, no focus) — green flash only fires on success; deliberate UX choice
- `clipboard.ts` uses inline `style` mutation rather than class toggle — avoids global CSS dependency; acceptable for a small utility
- `SaveHTMLExport` returns `("", nil)` on user cancel (empty path from dialog) — silent ignore; `App.vue` checks `if (path)` before showing "Saved" toast
- `batchQueryToolCalls` scan errors now propagate instead of silently skipping rows — consistent with the rest of `chunks.go`; schema is stable so this was a latent risk, not an active bug

### Inferred Next Steps

- **Live smoke test** — launch the built `tokentally.exe`, open Sessions tab, verify progressive render (first 20 chunks appear before full load), test copy buttons (User + AI + Compact + System turns), test export (click download icon, save, open in browser)
- **Regenerate Wails bindings** — run `wails build -platform windows/amd64` (without `-skipbindings`) to update `frontend/wailsjs/go/app/App.js` and `App.d.ts` with `SaveHTMLExport`
- **Consider disabling export button on empty session** — reviewer noted the export button is reachable even when `chunks.length === 0`; could add `:disabled="!chunks.length"` to the button
- **Consider `<details>` accordion for tool calls in export.ts** — design spec mentioned "tool call accordions" but plan code (and implementation) uses plain visible divs; could wrap `renderToolCall` output in `<details>`/`<summary>` if users want collapsible tool calls in the exported HTML

### Open Questions / Blockers

- Wails binding stubs still not regenerated for `SaveHTMLExport` — runtime calls work without stubs, but `frontend/wailsjs/go/app/App.d.ts` is stale
- The `started` field on `selectedSession` may be undefined for sessions that lack it — `exportHTML()` uses `selectedSession.value?.started ?? ''`; the export will show `— → —` for date range in that case (harmless)

## Session — 2026-04-29 (afternoon)

### What Was Done

- Added loading spinner to Overage & Auth Status "Check Now" button — CSS `@keyframes spin` + `.btn-spinner` element shown while `loading` ref is true; self-contained in `OverageView.vue` scoped styles
- Investigated Prompts page "synthetic" entries — confirmed `<synthetic>` is a literal model name written by Claude Code into JSONL for subagent sidechain records (`isSidechain: true`); hook/attachment records are `type='attachment'` in the DB
- Extended `ExpensivePrompts` query in `internal/db/db.go` to expose `u.is_sidechain` and `u.type AS msg_type`, and widened `WHERE` to include `type='attachment'` rows (hook prompts) alongside `type='user'`
- Added conditional row icons in `PromptsView.vue`: zap SVG for hook rows, bot SVG for subagent rows, person SVG for regular user rows; each has a `title` tooltip
- Added `tag-hook` / `tag-subagent` pill annotations in the Prompts modal header using inline SVG stroke icons (no emoji) matching the app's existing icon style

### Files Changed

- `frontend/inspector/src/views/OverageView.vue` — added `.btn-spinner` CSS + spinner element inside Check Now button
- `frontend/inspector/src/views/PromptsView.vue` — added `is_sidechain`/`msg_type` to `PromptRow` interface; conditional icons in table; hook/subagent tag pills in modal header; scoped tag styles
- `internal/db/db.go` — `ExpensivePrompts`: added `u.is_sidechain, u.type AS msg_type` to SELECT; widened WHERE to `IN ('user','attachment')` with `AND prompt_text != ''`
- `frontend/web/app.bundle.js` + `frontend/web/app.css` — rebuilt artifacts (do not edit directly)
- `build/bin/tokentally.exe` — rebuilt binary

### Decisions Made

- Skill name for subagent prompts is **not capturable** — it is never written into the JSONL; the `agentId` and `isSidechain` flag are the only metadata available on sidechain records
- Used inline SVG stroke icons (not emoji) throughout, matching the existing Lucide-style icon set already in use
- Hook rows included in Prompts view only when `prompt_text != ''` to avoid showing empty hook records (many attachment records have no meaningful text)

### Open Questions / Blockers

- None

## Running state

- Background processes: none
- Dev servers / ports: none
- Open worktrees / branches: none

### Inferred Next Steps

- The Prompts page description still reads "Your latest prompts" — could add a note that subagent and hook entries are also included
- Hook row `session_id` links may not navigate correctly if the attachment record's `session_id` belongs to a parent session that has no entries in the Sessions view — worth testing the link behaviour on a hook row
- The `is_sidechain` field is now exposed but not used as a filter — consider adding a toggle in the Prompts view to show/hide subagent prompts for users who only want to see their own input

## Session — 2026-04-30 07:13

### What Was Done

- **Fixed search hang** (db.go) — `SetMaxOpenConns(1)` was serializing every DB op through one connection; while the scanner held it in a per-file write tx, `SearchPrompts` blocked indefinitely at the Go pool layer (never reaching SQLite, so `busy_timeout` never fired). Raised limit to 4 so WAL-mode concurrent readers can proceed during writes. (commit `d5187a4`)
- **Fixed blank-screen on Search tab** (PromptsView.vue) — Go's `scanMaps` returns a `nil` slice on zero results, which Wails serializes as JSON `null`. Assigning `null` to `searchRows`/`rows` then evaluating `null.length` in the `v-if` guard threw a TypeError that Vue silently caught and rendered as blank. Patched at the assignment sites (`?? []`) and added optional chaining on the two template guards. (commit `2c2d62c`)
- **G25 cleanup** — extracted `1e9` (nanos→sec) to `nanosPerSec` constant in db.go for parity with scanner.go.
- **Rebuilt Windows binary** at `build/bin/tokentally.exe` with both fixes.
- **Comprehensive database review** (read-only, no code changes) — surfaced 14 issues across schema, queries, concurrency, and minor cleanup. Top findings recorded in "Inferred Next Steps" below.

### Files Changed

- `internal/db/db.go` — `SetMaxOpenConns(1)` → `SetMaxOpenConns(4)`; added `nanosPerSec` constant; committed
- `frontend/inspector/src/views/PromptsView.vue` — null-coalesce in `fetchRows()` / `doSearch()`; optional chaining on `displayRows?.length` in two `v-if` guards; committed

### Decisions Made

- **Pool size of 4, not unlimited** — WAL allows N readers + 1 writer; 4 is enough for the UI's concurrent reads while keeping the pool bounded. Did not split into separate read/write pools yet — that's a deferred refactor (see Next Steps #4).
- **Frontend defensive fix instead of Go-side fix for `nil` slice** — fixed at the consumer because the same bug affects all 8+ callers of `scanMaps`. The proper fix is to make `scanMaps` return `[]map[string]any{}` (item #1 in next steps), but the frontend guard is now in place as defense-in-depth.

### Open Questions / Blockers

- None

## Running state

- Background processes: none
- Dev servers / ports: none
- Open worktrees / branches: none
- Unstaged working tree: `frontend/inspector/src/views/OverageView.vue` (pre-existing, untouched this session)

### Inferred Next Steps

The user requested an expert DB review and asked whether to apply the low-risk fixes. The review identified the following, **in priority order**:

**Quick wins (one commit, all additive, low risk):**

1. **Add three missing indexes** to the schema in `internal/db/db.go`:
   - `CREATE INDEX IF NOT EXISTS idx_tools_message_uuid ON tool_calls(message_uuid)` — fixes scanner O(N²) on rescans, used by `batchQueryToolCalls`, `processLine` DELETE, `evictPriorSnapshots` DELETE
   - `CREATE INDEX IF NOT EXISTS idx_tools_use_id ON tool_calls(tool_use_id)` — fixes `pairToolResults` UPDATE full-scan
   - `CREATE INDEX IF NOT EXISTS idx_messages_parent ON messages(parent_uuid)` — fixes the `LEFT JOIN ... ON a.parent_uuid = u.uuid` in `SearchPrompts` and `ExpensivePrompts`
2. **Fix `scanMaps` to return `[]map[string]any{}` not `nil`** — root-cause fix for the blank-screen class of bugs; affects all callers.
3. **Add `synchronous=NORMAL`** to the DSN in `Open()` — official WAL recommendation, ~2× write speedup, durable across app crashes.
4. **Make `SearchPrompts` case-insensitive** — add `COLLATE NOCASE` to the `LIKE` clause; users expect case-insensitive search.

**Larger refactors (each its own PR):**

5. **Split read/write pools** — current `SetMaxOpenConns(4)` allows concurrent writers, which will hit `database is locked` after 5s if the scanner overlaps with any UI write (e.g., `UpsertPricingModel`). Two `*sql.DB` handles: read pool with N=4, write pool with N=1. Forces serialization at the Go layer, eliminates BUSY surprises.
6. **FTS5 virtual table** for `messages.prompt_text` — current `LIKE '%query%'` cannot use any index; FTS5 with `content=messages, content_rowid=rowid` plus sync triggers takes search from O(N) to O(matches).
7. **Batch `distinctCWDs`** — N+1 query in `ProjectSummary` and `RecentSessions` (one extra query per project/session). Replace with a single `GROUP BY project_slug` query and assemble in Go.
8. **Disambiguate the `LEFT JOIN` in `SearchPrompts`/`ExpensivePrompts`** — when a user message has multiple assistant children (streaming-snapshot replay edge case), the join picks an arbitrary one. Pick the earliest by `MIN(timestamp)` or use a correlated subquery.
9. **Schema version tracking** — add a `schema_version` row in `plan` table so future migrations know what's applied.
10. **Drop `AUTOINCREMENT` from `tool_calls.id`** — `INTEGER PRIMARY KEY` alone is sufficient and avoids the `sqlite_sequence` write-amplification.
11. **Periodic `PRAGMA wal_checkpoint(TRUNCATE)`** — call at end of each scan loop iteration to keep `.wal` file size bounded for long-running processes.

**Verification before merging quick wins:**

```sql
EXPLAIN QUERY PLAN SELECT * FROM tool_calls WHERE message_uuid='abc';
-- before: SCAN tool_calls   after: SEARCH tool_calls USING INDEX idx_tools_message_uuid

EXPLAIN QUERY PLAN
SELECT u.uuid FROM messages u LEFT JOIN messages a ON a.parent_uuid=u.uuid AND a.type='assistant' LIMIT 1;
-- before: SCAN a   after: SEARCH a USING INDEX idx_messages_parent
```

The user asked "Want me to apply 1-6 now?" — this is the open question to pick up at the start of the next session.

## Session — 2026-04-30 09:30

### What Was Done

This session closed out **all 11 items from the database review** (started in the previous session), shipped as separate focused commits. Started from 44 passing tests, ended at 57.

- **`aa318f0` — Quick-win batch (review items #1–4):** added `idx_messages_parent`, `idx_tools_message_uuid`, `idx_tools_use_id`; switched DSN to `synchronous=NORMAL`; added `COLLATE NOCASE` to `SearchPrompts`; fixed `scanMaps` to return `make([]map[string]any, 0)` instead of nil so Wails serializes `[]` not `null`.
- **`ef6a733` — Read/write pool split (#5):** introduced `db.Pool` with separate Read (`MaxOpenConns=4`) and Write (`MaxOpenConns=1`) handles to the same SQLite file. Eliminates the "database is locked" class of errors when UI writes overlap the scanner's per-file transaction — losers now wait at the Go pool layer instead of failing after `busy_timeout`. Touched 18 files; every helper signature flipped from `*sql.DB` to `*Pool`. `:memory:` paths share one handle so tests still work.
- **`b0b39e9` — JOIN disambiguation (#8):** correlated subquery using `MIN(rowid)` makes the user→assistant join in `SearchPrompts`/`ExpensivePrompts` deterministic when a user message has multiple assistant children. Added regression test.
- **`3431511` — Untrack scratch files:** dropped `calculator-todo.md`, `rtk-feature.md`, `image-1/2.png` from the index; live working files moved under `.superpowers/jay-todo/` (already gitignored via `.superpowers/`).
- **`c47fe1c` — Batch lookup + WAL checkpoint (#7, #11):** replaced N+1 `distinctCWDs` calls in `ProjectSummary` and `RecentSessions` with a single `cwdsForSlugs(p, slugs)` query. Added `Pool.CheckpointWAL()` calling `PRAGMA wal_checkpoint(TRUNCATE)` after each scan-loop tick so the `.wal` sidecar does not grow unbounded over a long session. Removed dead `distinctCWDs` helper.
- **`c3f624c` — FTS5 full-text search (#6):** added `messages_fts` virtual table with the **trigram tokenizer** (preserves the `LIKE` substring-match UX with index support). AFTER INSERT and AFTER DELETE triggers keep the index in sync; WHEN clauses skip empty `prompt_text` rows. One-time `applyFTSBackfill` populates existing messages. New `sanitizeFTSQuery` escapes user input — quoted-phrase-AND so multi-word queries match in any order. Frontend min-length guard raised from 1 to 3 (trigram requires ≥3 chars per token). 8 new test sub-cases.
- **`762fe43` — Version-tracked migrations + drop AUTOINCREMENT (#9, #10):** replaced the per-migration gate-flag pattern with a single `schema_version` row in `plan` and a numbered `migrations` slice. `targetSchemaVersion = 3`. `readSchemaVersion` infers the starting version from the legacy gate flags so existing DBs do not repeat already-applied work. Migration 3 (`migrateDropToolCallsAutoincrement`) recreates `tool_calls` without `AUTOINCREMENT` only when the table actually has it. Static schema for fresh DBs drops the keyword entirely. Exported `SchemaVersion(p)` for diagnostics. 4 internal-package tests cover fresh, legacy-detection, recreate, and no-op-when-clean paths.

### Files Changed

- `internal/db/db.go` — `Pool` type, `Open` returns `*Pool`, `initSchema` extracted, FTS5 schema + triggers, `applyMigrations` runner, three numbered migration funcs, batch `cwdsForSlugs`, `Pool.CheckpointWAL`, `sanitizeFTSQuery`, `SchemaVersion`, `MIN(rowid)` JOIN subqueries, three new indexes, every helper signature now `*Pool`
- `internal/db/chunks.go` — read helpers take `*Pool`
- `internal/db/db_test.go` — `openMem` returns `*Pool`; new tests for FTS5, JOIN disambiguation, FTS backfill
- `internal/db/migrations_test.go` — new internal-package file: `TestSchemaVersion_FreshDB`, `TestSchemaVersion_LegacyDBInferredFromGateFlags`, `TestMigrateDropToolCallsAutoincrement`, `TestMigrateDropToolCallsAutoincrement_NoOpOnFreshDB`
- `internal/scanner/scanner.go` — `ScanDir` takes `*Pool`; `scanFile` uses `p.Write.Begin()`; `processLine` no longer takes a separate `conn` (skill upserts now go through the open tx — fixes a deadlock under the single Write-pool slot)
- `internal/scanner/scanner_test.go`, `internal/tips/tips.go`, `internal/tips/tips_test.go` — adapted to `*Pool`
- `app/app.go` — `App.conn` is now `*db.Pool`; `runInspectorBackfill` and `scanLoop` route through `.Write` for raw exec; periodic `CheckpointWAL` in scan loop
- `app/service_linux.go`, `svc/service_windows.go`, `svc/service_linux.go`, `cmd/backfill-skills/main.go` — match new signatures
- `frontend/inspector/src/views/PromptsView.vue` — search min-length guard now 3 chars (trigram floor)

### Decisions Made

- **Trigram tokenizer over default FTS5 tokenizer.** Why: preserves the substring-match UX users had with LIKE — default tokenizer would have been a regression where "dep" no longer found "deploy". How to apply: bigger index on disk is the cost; if disk usage becomes a concern, revisit by switching to `unicode61` with prefix-matching `*` suffix on the last token.
- **Multi-word FTS queries are AND'd, not phrase-matched.** Why: users typing two terms usually mean "find anything mentioning both," not "find this exact phrase." How to apply: if anyone reports `"foo bar"` no longer finding the exact contiguous string, revisit `sanitizeFTSQuery` and consider supporting a `"..."` literal-phrase syntax.
- **Migration runner uses legacy-gate inference.** Why: existing DBs already have `fix_user_string_content=1` and `fts_backfill_done=1`; running them again would be wasted work. How to apply: when adding migration N, append to `migrations` slice and bump `targetSchemaVersion`. No separate gate flag needed for new migrations.
- **AUTOINCREMENT removal detects-then-recreates.** Why: fresh DBs ship without AUTOINCREMENT in the static schema, so the migration must no-op for them. How to apply: any future "drop a feature from an existing table" migration should follow the same `sqlite_master` inspection pattern.
- **Frontend min-length 3 instead of LIKE fallback for short queries.** Why: 2-char substring searches are rare and the code complexity of a fallback path is not worth it. How to apply: if anyone asks, add a LIKE fallback in `SearchPrompts` when sanitized query has tokens shorter than 3.

### Open Questions / Blockers

- None. All 11 review items closed.

## Running state

- Background processes: none
- Dev servers / ports: none
- Open worktrees / branches: none
- Unstaged working tree: clean (apart from untracked notes under `.superpowers/jay-todo/` which are gitignored)
- Local commits ahead of `main`: 7 (`aa318f0` through `762fe43`); not pushed

### Inferred Next Steps

The DB front is fully closed out. Reasonable next directions, in roughly descending value:

1. **Push the branch and open a PR.** 7 commits is a decent batch. Each is independent enough that the PR could be reviewed by reading commit messages alone.
2. **Verify migration 3 against a production-sized DB.** The AUTOINCREMENT recreate dance has only been tested on `:memory:`. Worth running once against a copy of `~/.claude/tokentally.db` before merging if the user has a sizable history. Quick check: copy the DB, run the new binary against it, confirm the app starts and search works.
3. **Document the new schema migration pattern in CLAUDE.md.** The project instructions mention SQL conventions but not how to add a migration. A short note pointing to `targetSchemaVersion` + `migrations` slice in `db.go` would help future-you.
4. **Frontend feedback for "too-short query" state.** Currently a 2-char query just shows nothing happening. Could show a hint like "type 3 or more characters to search". Small UX polish.
5. **Resume any non-DB work.** The inspector / tray / pricing / overage areas have not been touched in this DB-focused arc. If there's a feature backlog, this is a good time to switch.
