# TokenTally — Context Index

## Root Files

- [TECHSTACK.md](TECHSTACK.md) — Stack reference: Go 1.25 + Wails v2 backend, Vue 3/Vite inspector SPA, vanilla-JS main SPA, SQLite (`modernc.org/sqlite`), build/CI tooling.
- [ARCHITECTURE.md](ARCHITECTURE.md) — Deeper architecture reference: tech stack detail, the 10 core architectural patterns/decisions (build constraints, JSONL scanner, schema migrations, systray threading, etc.), and full directory layout.

## Subfolders

### `reference/`

- [reference/CLAUDE-CONTEXT-PATTERN.md](reference/CLAUDE-CONTEXT-PATTERN.md) — The `.context/` convention this repo follows for durable, version-controlled project knowledge (this index is generated from it).

### `specs/`

- _(none yet — create `YYYY-MM-DD-<topic>-design.md` here before starting non-trivial feature work; the last one, the Findings subsystem design, was removed after the feature shipped)_

### `plans/`

- _(none yet — create `YYYY-MM-DD-plan-<#>.<sub>-<topic>.md` here to break a spec into TDD tasks; the last one, the Findings subsystem plan, was removed after the feature shipped)_
