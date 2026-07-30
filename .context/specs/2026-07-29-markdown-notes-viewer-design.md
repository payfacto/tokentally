# Markdown Notes Viewer — Design

## Problem

Two Claude Code skills used against this and other repos — `handoff` and `afk`
— write markdown files outside any git repo (`~/.claude/handoffs/*.md`,
`~/.claude/afk/*.md`), one flat file per repo, named `<reponame>-<hash>.md`.
There is currently no way to browse or read these except by opening them in a
text editor or `cat`-ing them from a terminal. TokenTally already reads
Claude Code data outside its own repo (`~/.claude/projects/`), so it's a
natural home for a small viewer.

## Scope

- Browse and render markdown files from a small set of **configured, flat**
  (non-recursive) folders. Ships pre-configured with the two known folders
  (`~/.claude/handoffs`, `~/.claude/afk`).
- Folder list is **user-configurable** via Settings (add/remove), so future
  skills that write out-of-repo notes can be added without a code change.
- Shows files from **all repos**, not just the current one — consistent with
  TokenTally already being a global, not per-repo, dashboard.
- Out of scope: recursive/nested directory browsing, editing files, search
  across file contents, deleting/renaming files from the UI.

## Architecture overview

```text
~/.claude/handoffs/*.md  ─┐
~/.claude/afk/*.md       ─┼─ configured folders (DB table: markdown_folders)
(future folders)          ┘
         │
         ▼ os.ReadDir (flat, *.md only)
   app/app.go: ListMarkdownFiles / ReadMarkdownFile
         │
         ▼ window.go.app.App.*()
   frontend/inspector/src/views/NotesView.vue
         │  (file list, left pane)      (rendered content, right pane)
         ▼
   lib/markup.ts: renderDocument()  →  marked.parse → DOMPurify.sanitize
```

## Backend / data model

### Schema

New table, added via the standard migration mechanism (append a
`func(tx *sql.Tx) error` to `migrations` in `internal/db/db.go`, bump
`targetSchemaVersion`, add a migration test per `migrations_test.go`
convention):

```sql
CREATE TABLE markdown_folders (
  path  TEXT PRIMARY KEY,
  label TEXT NOT NULL
);
```

### Seeding

Follows the existing `SeedExchangeRate` pattern (`INSERT OR IGNORE`, so a
user-deleted default doesn't reappear on restart):

- `db.SeedMarkdownFolder(p *Pool, path, label string) error` — `INSERT OR
  IGNORE INTO markdown_folders (path, label) VALUES (?, ?)`.
- Called once from `app.seedFromDefaults()` for `~/.claude/handoffs` →
  "Handoffs" and `~/.claude/afk` → "AFK Notes", gated by its own
  `markdown_folders_seeded` marker in the `plan` table (independent of the
  existing pricing-seed gate — different concern, shouldn't be coupled).

### DB helpers (`internal/db/db.go`)

```go
type MarkdownFolder struct {
    Path  string
    Label string
}

func GetMarkdownFolders(p *Pool) ([]MarkdownFolder, error)
func AddMarkdownFolder(p *Pool, path, label string) error   // INSERT OR REPLACE
func DeleteMarkdownFolder(p *Pool, path string) error
```

### App methods (`app/app.go`)

```go
func (a *App) GetMarkdownFolders() ([]map[string]any, error)
func (a *App) AddMarkdownFolder(path, label string) error
func (a *App) DeleteMarkdownFolder(path string) error
func (a *App) ListMarkdownFiles() ([]map[string]any, error)
func (a *App) ReadMarkdownFile(path string) (string, error)
```

- `AddMarkdownFolder` expands a leading `~` via `os.UserHomeDir()`, resolves
  to an absolute path, verifies the path exists and is a directory, and
  rejects duplicates (primary key conflict surfaces as an error, not a
  silent overwrite of the label — the caller decides whether to retry with
  delete+add).
- `ListMarkdownFiles` iterates `GetMarkdownFolders()`, and for each calls
  `os.ReadDir` (non-recursive). Entries are filtered to non-directory files
  whose extension case-insensitively matches `.md`. A folder that no longer
  exists on disk (`os.ReadDir` error) is **skipped, not an error** — the call
  still succeeds and returns files from the folders that do exist. Each
  returned row: `{folder_path, folder_label, filename, full_path, size,
  mod_time}`.
- `ReadMarkdownFile` is the security-sensitive one: it cleans the requested
  path (`filepath.Clean`) and requires `filepath.Dir(cleanedPath)` to be
  **exactly equal** to one of the configured folder paths (not a prefix
  check — prefix checks are vulnerable to sibling-folder confusion, e.g.
  `~/.claude/handoffs-evil` matching a prefix of `~/.claude/handoffs`).
  Anything that doesn't resolve to a direct child of a configured folder is
  rejected before the file is read. Returns raw markdown content; no
  server-side rendering.

## Frontend

### Route + nav

- New route `/notes` in `frontend/inspector/src/router/*.ts` →
  `NotesView.vue`.
- Added to the `NAV_ROUTES` array in `App.vue` — nav label auto-derives from
  the path (`"notes"`), same as every other tab.

### `NotesView.vue`

Two-pane layout (list/detail split, in the spirit of `SessionsView`):

- **Left pane** — file list grouped by folder label (section header per
  configured folder, e.g. "Handoffs", "AFK Notes"). Within each group, sorted
  by `mod_time` descending (most recent first). Each row shows filename,
  relative time, and size.
- **Right pane** — rendered markdown of the selected file via
  `renderDocument()`. Empty state ("select a file to view it") when nothing
  is selected.
- **Empty states**: zero configured folders → "No folders configured — add
  one in Settings." Configured folders but no `.md` files found in any of
  them → "No files found."

### Rendering: `renderDocument()` (new, in `lib/markup.ts`)

The existing `renderMarkdown()` in that file is tuned for chat-transcript
previews — it pill-ifies XML-ish tags (`preprocessText`/`spaceXmlTags`) and
has a narrow `ALLOWED_TAGS` allow-list with no table support. Handoff/AFK
files are full documents (headers, GFM tables, code fences, links,
bold/italic), so a second function is added rather than overloading the
first:

```ts
const DOCUMENT_SANITIZE_CFG: DOMPurify.Config = {
  ALLOWED_TAGS: [
    'p', 'br', 'hr', 'em', 'strong', 'b', 'i', 'del',
    'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
    'ul', 'ol', 'li', 'code', 'pre', 'blockquote', 'a', 'span', 'div',
    'table', 'thead', 'tbody', 'tr', 'td', 'th',
  ],
  ALLOWED_ATTR: ['class', 'href'],
  ALLOW_DATA_ATTR: false,
}

export function renderDocument(text: string): string {
  if (!text) return ''
  const raw = marked.parse(text, { async: false }) as string
  return DOMPurify.sanitize(raw, DOCUMENT_SANITIZE_CFG) as string
}
```

No XML preprocessing — runs `marked.parse()` directly on the raw file
content. Still no `img`, `script`, or inline event handler attributes
allowed.

### Auto-refresh

- The file list polls `ListMarkdownFiles()` on an interval (~15s) while
  `NotesView` is mounted, cleared in `onUnmounted` — same
  `onMounted`/`onUnmounted`/interval shape already used elsewhere (e.g.
  `SkillsView`'s `watch([rangeKey, () => store.lastScan], fetchAll)`, though
  this is a plain timer rather than the scan-event watcher since folder
  listing is unrelated to the JSONL scan loop).
- If the currently-open file's `mod_time` advances on a poll (e.g. a handoff
  gets appended to mid-session), its content is refetched via
  `ReadMarkdownFile` and re-rendered automatically, so the view stays live
  without the user re-clicking it.

### Settings integration

New card in `SettingsView.vue`, "Markdown Folders", following the existing
pricing-models table pattern (table + delete button, form to add a new row
below — see the models table around
[SettingsView.vue:340](../../frontend/inspector/src/views/SettingsView.vue#L340)):

- **Table**: path, label, delete button → `DeleteMarkdownFolder(path)`.
- **Add form**: path + label inputs, "Add" button → `AddMarkdownFolder(path,
  label)`. Backend validation errors (folder doesn't exist, not a directory,
  duplicate path) surface inline; nothing is added on failure.
- No guard against deleting both defaults down to zero — a valid, if
  unhelpful, state. The Notes tab's empty state already covers it.

## Testing

- **`internal/db`**: migration test for `markdown_folders` (per
  `migrations_test.go` convention); `GetMarkdownFolders` /
  `AddMarkdownFolder` / `DeleteMarkdownFolder` round-trip; `SeedMarkdownFolder`
  idempotency (a second seed call doesn't resurrect a row the user deleted).
- **`app`**: `ReadMarkdownFile` rejects both classic traversal
  (`../../etc/passwd`-style) and sibling-folder-prefix confusion (a path
  whose parent merely starts with a configured folder's path but isn't
  exactly it); `ListMarkdownFiles` skips a configured folder that doesn't
  exist on disk instead of failing the whole call.
- **Manual**: `wails dev` — add/remove a folder in Settings; confirm the
  Notes tab lists real files from `~/.claude/handoffs`/`~/.claude/afk` on
  this machine; open one and confirm it renders; edit a handoff file on disk
  while the tab is open and confirm auto-refresh picks up the change.
