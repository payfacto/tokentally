# Claude Code + `.context/` — Project Knowledge Convention

A reproducible pattern for giving Claude Code (and any other AI coding agent that respects `CLAUDE.md`) durable, curated, version-controlled project knowledge. Drop this whole layout into a fresh repo and the next agent that opens the project gets a coherent starting state in seconds.

The pattern is language-agnostic — adapt the standards/specs to whatever the project uses.

---

## TL;DR

```
repo/
├── CLAUDE.md                       ← project memory, auto-loaded by Claude Code
└── .context/                       ← curated, committed knowledge base
    ├── INDEX.md                    ← table of contents (imported by CLAUDE.md)
    ├── TECHSTACK.md                ← stack reference
    ├── DESIGN.md                   ← design system / brand reference (UI projects)
    ├── RELEASE.md                  ← release runbook
    ├── CODESTYLE.md                ← code style / lint crib sheet (code projects; per-language, Java so far)
    ├── specs/                      ← design specs (YYYY-MM-DD-<topic>.md)
    ├── plans/                      ← TDD implementation plans
    └── reference/                  ← external docs / vendor APIs / snapshots
        └── <topic>/
```

The two load-bearing ideas:

1. **`CLAUDE.md` at the repo root** is auto-loaded by Claude Code on every session and contains the project's stable description plus `@`-imports that pull in the rest of `.context/` only as needed.
2. **`.context/INDEX.md`** is a single navigation document the agent reads first to learn what's available without front-loading every file's content into its context window.

---

## CLAUDE.md (root)

Claude Code reads `CLAUDE.md` from the working directory automatically at session start. Keep it short — under 200 lines — and use `@`-imports for everything else.

**Template:**

```markdown
@.context/INDEX.md

# <Project Name>

<One-paragraph description: what it is, who uses it, what makes it
non-obvious.>

## Tech stack

<One sentence. Full details live in TECHSTACK.md, which INDEX.md links.>

Always use the `<framework-skill>` skill when writing or reviewing
<language> code that uses <framework>.

## First-time setup after clone

```bash
# Concrete commands a new developer (or agent) runs to bring the repo
# up. Aim for copy-pasteable.
```

## Project layout

```
<tree of the 5-15 most important paths with one-line annotations>
```

## <Project-specific domain note>

<Any non-obvious convention specific to this codebase. E.g. for
Orchestra: how "agent-enabled" issues are detected via a Jira label;
how the Confluence per-project mapping works. Keep it brief; defer to
.context/specs/ for details.>

## Language

<Language>. Always use the `plugin-clean-code:<lang>` skill when
writing or reviewing <Language> code. For a project with a house style
beyond linter defaults, capture it in `.context/CODESTYLE.md` and
`@`-import it here so the rules load every session. (CODESTYLE.md is
per-language; a Java version exists so far.)
```

**The `@` syntax** — Claude Code recursively expands `@path/to/file.md` at session start. It's how a small `CLAUDE.md` pulls in the full `.context/INDEX.md` content, which in turn references (via prose links) the rest of `.context/`. The agent doesn't auto-expand every link in `INDEX.md` — only `@`-prefixed paths get loaded.

**Rule of thumb:** `@`-import only the things the agent should see on **every** session. Everything else is reachable via INDEX.md links, which the agent reads only when it needs them.

---

## .context/INDEX.md

This is the navigation hub. Every file in `.context/` gets exactly one one-line entry. The agent reads `INDEX.md` (because `CLAUDE.md` `@`-imports it) and learns the shape of the knowledge base without paying the token cost of loading every file.

**Template:**

```markdown
# <Project Name> — Context Index

## Root Files

- [@TECHSTACK.md](TECHSTACK.md) — Tech stack reference (auto-imported).
- [@DESIGN.md](DESIGN.md) — Design system / brand reference
  (auto-imported, UI projects only).
- [@RELEASE.md](RELEASE.md) — Release runbook (auto-imported when
  build/release work is common).
- [@CODESTYLE.md](CODESTYLE.md) - Code style / lint crib sheet
  (auto-imported on code-heavy projects). Per-language; only a Java
  version exists so far.

## Subfolders

### `reference/`

- [reference/<topic>.md](reference/<topic>.md) — <one-line summary>.

### `specs/`

- [specs/2026-05-14-<topic>.md](specs/2026-05-14-<topic>.md) — <design spec summary>.

### `plans/`

- [plans/2026-05-14-plan-1.1-<topic>.md](plans/...) — <TDD plan summary>.

```

**Convention notes:**
- The `@` prefix in `[@TECHSTACK.md](TECHSTACK.md)` is a **convention used inside INDEX.md** to signal "this file is also `@`-imported by CLAUDE.md". The agent doesn't parse it — it's a hint to humans (and to future-you).
- Entries should be one line each, ~150 chars max. INDEX.md gets loaded on every session; bloat costs tokens.
- Reorganize semantically by topic, not chronologically. Specs/plans are date-prefixed but stay grouped together.

---

## Standing documents

### `.context/TECHSTACK.md`

Stack reference. Sections cover language + runtime, frameworks, data layer, APIs, secrets, build/dependency mgmt, testing, CI/CD, infra/deployment, frontend stack, dev-experience tooling. Bullets cite version numbers — that's the whole point. The agent answers "what library does this project use for X?" by reading this file instead of grepping the lockfile.

### `.context/DESIGN.md` (UI projects)

Brand colors, typography, spacing scale, component vocabulary, do's and don'ts. The agent uses this to keep new UI work consistent with the existing design language. Skip for non-UI projects.

### `.context/RELEASE.md`

The release runbook. Pre-flight checks, tag-and-push flow, troubleshooting table. Saves the agent from having to read CI configs to answer "how do we ship a release?"

### `.context/CODESTYLE.md` (code projects; per-language)

A language-specific coding-standard crib sheet: the rules that bite in practice (formatter settings, lint/Checkstyle rules, package layout, naming, layering conventions) written out in human-readable form for the times no IDE is in the loop - Claude Code edits, ad-hoc patches, code review. `@`-import it in CLAUDE.md on code-heavy projects so it loads every session, and have the agent read it before any code edit.

**So far only a Java version exists** (PayFacto IntelliJ formatter + gecko-plugin-codequality Checkstyle rules). The same file shape works for any language - add a TypeScript/Go/Python equivalent the same way. For a polyglot repo, split per language (`CODESTYLE-java.md`, `CODESTYLE-go.md`, ...) and `@`-import each. Keep it grounded in the repo's *actual* conventions (real class names, real package tree), not a generic style guide - a borrowed CODESTYLE from another repo will carry stale examples and wrong CI claims.

---

## `.context/specs/` — design specs

Pre-implementation design docs. Date-prefixed filenames:

```
specs/YYYY-MM-DD-<short-feature-name>-design.md
```

Each spec covers: problem, goals, non-goals, design, alternatives considered, open questions, references. Specs are written **before** plans (which are written before code). The agent should produce a spec when starting non-trivial work; the human reviews it; only then does a plan or code follow.

---

## `.context/plans/` — TDD implementation plans

Numbered, task-by-task plans for executing a spec. Filenames:

```
plans/YYYY-MM-DD-plan-<#>.<sub>-<topic>.md
```

Format: numbered tasks, each task with explicit test → impl → verify steps. The agent works through tasks one at a time, reporting completion. Useful when handing implementation work to a subagent or another session — the plan is the contract.

---

## `.context/reference/` — external knowledge

Snapshots of external docs that don't change often: vendor API references, CLI reference manuals, historical-decision PDFs, articles you keep coming back to. Subfolder per topic:

```
reference/oauth/<provider>-oauth-notes.md
reference/jira/<workspace>-summary.md
reference/<cli-tool>/CLI-REFERENCE.md
```

The agent reads these when working on that area. INDEX.md lists them with one-line summaries so the agent knows what's available.

---

## `.gitignore` additions for the pattern

```gitignore
# .claude/ is committed (skills, slash commands, settings.json define the
# agent contract for this repo). Only the per-contributor and per-session
# files are ignored.
.claude/settings.local.json
.claude/.current-tool

# If using the superpowers harness:
.superpowers/
```

---

## Bootstrap script — drop this pattern into a new repo

Save as `bootstrap-claude-context.sh` and run from the repo root:

```bash
#!/usr/bin/env bash
set -euo pipefail

PROJECT_NAME="${1:-MyProject}"
LANG="${2:-go}"   # or python, typescript, etc.

mkdir -p .context/{reference,specs,plans,tools}

# --- CLAUDE.md ---
cat > CLAUDE.md <<EOF
@.context/INDEX.md

# ${PROJECT_NAME}

<One-paragraph description goes here.>

## Tech stack

<One sentence. Full details in TECHSTACK.md.>

## First-time setup after clone

\`\`\`bash
# concrete commands
\`\`\`

## Project layout

\`\`\`
src/    — production code
tests/  — tests
\`\`\`

## Language

${LANG}. Always use the \`plugin-clean-code:${LANG}\` skill when
writing or reviewing ${LANG} code.
EOF

# --- INDEX.md ---
cat > .context/INDEX.md <<'EOF'
# Context Index

## Root Files

- [@TECHSTACK.md](TECHSTACK.md) — Stack reference.

## Subfolders

### `reference/`
- (populate with vendor / API references)

### `specs/`
- (populate with YYYY-MM-DD-<topic>-design.md)

### `plans/`
- (populate with YYYY-MM-DD-plan-#-<topic>.md)

EOF

# --- TECHSTACK.md (skeleton) ---
cat > .context/TECHSTACK.md <<EOF
# TECHSTACK — ${PROJECT_NAME}

## 1. Language and Runtime
- ${LANG} <version> — primary application language.

## 2. Core Frameworks and Libraries
- _(populate)_

## 3. Data and Persistence
- _(populate)_

## 4. API and Contract Tooling
- _(populate)_

## 5. Security and Secrets
- _(populate)_

## 6. Build and Dependency Management
- _(populate)_

## 7. Testing Stack
- _(populate)_

## 8. CI/CD and Delivery
- _(populate)_

## 9. Infrastructure and Deployment
- _(populate)_
EOF

# --- .gitignore additions ---
{
  echo ''
  echo '# .claude/ is committed (skills, slash commands, settings.json define the'
  echo '# agent contract for this repo). Only the per-contributor and per-session'
  echo '# files are ignored.'
  echo '.claude/settings.local.json'
  echo '.claude/.current-tool'
  echo ''  
  echo '.superpowers/'
} >> .gitignore

echo "Done. Edit CLAUDE.md and the .context/ files to fit ${PROJECT_NAME}."
```

Run with:

```bash
bash bootstrap-claude-context.sh "MyProject" go
```

Then the agent's first task in the new repo is to populate `TECHSTACK.md` and `INDEX.md` from the actual codebase — `techstack-review-summarizer` and similar codebase-inspection skills handle this in one shot. For a code-heavy repo, also add a `.context/CODESTYLE.md` grounded in the repo's real conventions and `@`-import it from CLAUDE.md (a Java template exists today; mirror it for other languages).

---

## How an agent actually uses this in a session

1. Session opens → Claude Code reads `CLAUDE.md` → `@`-expands `.context/INDEX.md`.
2. Agent now knows: project description, stack at a glance, every file available in `.context/`.
3. User asks a question or assigns a task.
4. Agent decides which `.context/` files it needs:
   - Designing a feature? → read recent `specs/` for adjacent work, then write a new spec.
   - Hit a bug? → read the relevant `reference/` doc for the area.
   - Editing code? → read `CODESTYLE.md` (if present) before writing, so edits match house style and pass the linter.
   - About to ship? → read `RELEASE.md`.

(Cross-session handoff is separate — see "What this pattern doesn't cover" below.)

---

## Anti-patterns (don't do these)

- **Putting everything in `CLAUDE.md`.** Bloats every session. Use `@`-imports and INDEX.md links instead.
- **`@`-importing every `.context/` file.** Same problem — only `@`-import the things the agent needs every session.
- **Storing secrets, tokens, or PII anywhere in `.context/`.** It's all committed. Keep them in `.env`, OS keychains, or CI secrets.
- **Generating docs the agent doesn't read.** Every file in `.context/` should map to either INDEX.md (general reference) or be auto-imported in CLAUDE.md (always loaded). Orphan files are dead weight.

---

## What this pattern doesn't cover

- **Session handoffs.** Cross-session handoff notes are handled by the `handoff` skill, which writes to an out-of-repo file (`~/.claude/handoffs/`, one per repo, never committed) — *not* to `.context/`. Handoffs are ephemeral working state (what's running, what's half-done, inferred next steps), not curated project knowledge, so they stay out of the committed tree. Durable, review-worthy context (decisions, specs, gotchas) still belongs in `.context/`.
- **User-level memory.** That's a per-developer Claude Code feature (`~/.claude/projects/<project>/memory/MEMORY.md`). It's separate from `.context/` and shouldn't be checked in — it's individual context (preferences, OS quirks, personal shortcuts).
- **Skills and plugins.** Those live under `~/.claude/` and Claude Code's plugin marketplace. The pattern here just *references* skill names in CLAUDE.md ("use the `plugin-clean-code:go` skill") — installing them is out of scope.
- **Subagents.** When an agent dispatches a subagent, the subagent reads `CLAUDE.md` too (so it inherits `.context/INDEX.md`). The agent's prompt to the subagent should mention specific `.context/` files when relevant.

---

## Why this works

- **Discoverable**: every piece of project knowledge has exactly one home, and INDEX.md is the map.
- **Bounded context window**: only `CLAUDE.md` + `@`-imported files load every session. Specifics load on demand.
- **Reproducible across sessions**: any agent (or human) opening the repo gets the same starting state.
- **Reviewable**: `.context/` is git-tracked, so changes are visible in PRs.
- **Composable with Claude Code defaults**: works with the `@`-import mechanism Claude Code already implements. No custom tooling required.
