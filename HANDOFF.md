# Handoff — tokentally

## Historical context (discoveries not obvious from the code)

- `<synthetic>` is a literal model name Claude Code writes into JSONL for subagent sidechain records (`isSidechain: true`); skill name is never written to JSONL and is not recoverable
- Hook records have `type='attachment'` in the DB; included in Prompts view only when `prompt_text != ''` (many attachment records have no meaningful text)
- FTS5 trigram tokenizer chosen over default tokenizer to preserve substring-match UX — "dep" still finds "deploy"; multi-word queries are AND'd (not phrase-matched)

---

## Session — 2026-04-30 (afternoon)

### What Was Done

- **Pushed all 12 commits** to `origin/main` (DB arc + HANDOFF update)
- **CLAUDE.md** — added "Schema migrations" section under SQL conventions documenting `targetSchemaVersion` / `migrations` slice pattern and the `sqlite_master` inspect-before-recreate technique
- **PromptsView search hint** — typing 1–2 chars now clears stale results and shows "type 3 or more characters to search" instead of misleading "no results"; `doSearch` early-exits cleanly with `searchRows.value = []`
- **Wails bindings regenerated** — ran `wails build -platform windows/amd64`; `frontend/web/wailsjs/go/app/App.js` + `App.d.ts` now include `SaveHTMLExport`, `GetRetentionDays`, `SetRetentionDays`, `PurgeOlderThan` (bindings are gitignored — regenerated on each full build)
- **Export button disabled on empty session** — `SessionsView.vue`: `:disabled="!chunks.length"` + `.btn-export:disabled { opacity: 0.35; cursor: default }`
- **Prompts subtitle** — both "recent" and "tokens" subtitles now mention "including subagent and hook entries"
- **Calculator: right-click paste + file load** — custom `contextmenu` handler shows a "Paste" item (WebView2 native context menu is disabled in production); drag-and-drop or "Upload file" button loads text files into the textarea; `looksLikeBinary()` checks first 8 kB for null bytes / control chars (>2% threshold) and shows an inline error for binary files

### Files Changed

- `CLAUDE.md` — schema migration section added
- `frontend/inspector/src/views/PromptsView.vue` — short-query hint + stale-result clear
- `frontend/inspector/src/views/SessionsView.vue` — export button disabled state
- `frontend/inspector/src/views/CalculatorView.vue` — context menu, drag-and-drop, file upload, binary guard

### Decisions Made

- Custom context menu rather than enabling `EnableDefaultContextMenu` in Wails options — scoped fix to the textarea; enabling globally would change UX for the whole app
- `looksLikeBinary` uses >2% non-printable threshold on first 8 kB — catches typical binary files while accepting files with occasional special chars; not a security boundary, just UX

### Running state

- Background processes: none
- Dev servers / ports: none
- Open worktrees / branches: none
- Working tree: clean
- `origin/main` is up to date

### Inferred Next Steps

- **Service install/uninstall smoke test** — Settings tab Install/Uninstall Service buttons use PowerShell elevation; not yet tested in any recent session
- **Hook row session link** — attachment records' `session_id` is a parent session; worth testing whether clicking a hook row's session link navigates correctly in the Sessions tab
- **Subagent filter toggle** — `is_sidechain` is exposed but unused as a filter; could add a toggle to show/hide subagent prompts for users who only want their own input
- **`wails build` for distribution** — latest committed source has not been built into a distributable `.exe` with Windows manifest (DPI awareness, UAC); use `wails build -platform windows/amd64`
- **`rtk-feature.md`** — user has a todo in `.superpowers/jay-todo/rtk-feature.md`; not yet reviewed

---

## Session — 2026-04-30 12:53

### What Was Done

- **RTK gain parser rewrite** — replaced single "highest %" scrape with a structured parser that extracts all summary fields (total commands, input/output/tokens saved with %, exec time) and the full "By Command" table (rank, command, count, saved, avg%, time, impact fraction from `█░` blocks). Fixed bug where 100.0% from a per-command row was returned instead of the global 89.7% from "Tokens saved" line.
- **RTK frontend redesign** — rewrote the RTK section in `OverageView.vue` to match the design spec in `.superpowers/jay-todo/image-3.png`: stats row with icons, SVG donut efficiency gauge (r=42, `stroke-dasharray` arc), "By Command" table with orange impact bars, card widened to 760px.
- **RTK install link line-break fix** — separated the `rtk-ai.app →` link from the description paragraph so it starts on its own line instead of wrapping inline.
- **README overhaul** — added all missing features: Calculator tab, Tools tab, RTK Token Savings section, currency/exchange rates, HTML export, data retention, daily trends, sessions turn-by-turn, platform feature matrix table. Corrected tab count (9, not 7). Fixed MD060 linter warnings on table separator rows.

### Files Changed

- `app/app.go` — added `RTKCommandRow` struct; expanded `RTKGainResult` with all parsed fields; replaced `rtkPctRe` with 6 targeted regexes; rewrote `GetRTKGain()` to parse all summary lines and "By Command" table rows
- `frontend/inspector/src/views/OverageView.vue` — new `RTKCommandRow` + expanded `RTKGainResult` interfaces; full template rewrite for RTK section (stats row, SVG donut, table with impact bars); install link on own line; CSS expanded
- `frontend/web/app.bundle.js` / `frontend/web/app.css` — rebuilt (gitignored, not committed)
- `README.md` — near-complete rewrite of features section; added platform matrix table; building section unchanged

### Decisions Made

- **SVG donut not canvas/ECharts** — inline SVG `stroke-dasharray` approach; keeps zero extra dependencies and matches the existing ECharts pattern of self-contained rendering
- **efficiencyColor returns hex not CSS vars** — SVG `stroke` attribute doesn't resolve CSS custom properties; concrete hex values (`#2d8a5e` etc.) used throughout
- **Impact bar width from `█░` block chars** — `strings.Count(s, "█") / len([]rune(s))` gives a 0–1 fraction directly from RTK's own visual output; no separate calculation needed
- **Separator row fixed to `| --- |` style** — MD060 linter requires consistent spacing around pipes; all table separator rows updated to match content row style

### Open Questions / Blockers

- RTK section rendered correctly in parser test but not verified in the live Wails app (no browser page was open during session); recommend opening Tools tab and clicking "Check RTK status" to confirm layout

### Running state

- Background processes: none
- Dev servers / ports: none
- Open worktrees / branches: none
- Working tree: clean, `origin/main` up to date (commits `2b2e7e2`, `16a5ddd`)

### Inferred Next Steps

- **Smoke test RTK dashboard** — open the Wails app, navigate to Tools tab, click "Check RTK status", verify: stats row shows correct values, donut shows ~89.7%, "By Command" table has rows with orange impact bars
- **`rtk-feature.md` review** — `.superpowers/jay-todo/rtk-feature.md` was noted as unreviewed in previous session; check if anything there wasn't yet implemented
- **`wails build` for distribution** — source is up to date but no fresh Windows `.exe` has been produced this session; build with `wails build -platform windows/amd64`
