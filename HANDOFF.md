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
