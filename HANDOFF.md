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
