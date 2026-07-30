#!/usr/bin/env node
// scripts/doc-lint.mjs
// Enforces TokenTally's doc-first contract: CLAUDE.md must stay wired to
// .context/INDEX.md, every doc under .context/ must be reachable from the
// index (and vice versa), and docs shouldn't silently go stale relative to
// the code they describe. See .context/reference/CLAUDE-CONTEXT-PATTERN.md
// for the convention this enforces. Zero dependencies.

import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { join, resolve, dirname, relative, extname } from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const ROOT = resolve(dirname(__filename), '..')
const CONTEXT_DIR = join(ROOT, '.context')

const errors = []
const warnings = []

function err(msg) {
  errors.push(msg)
}

function warn(msg) {
  warnings.push(msg)
}

function readIfExists(path) {
  try {
    return readFileSync(path, 'utf8')
  } catch {
    return null
  }
}

// Recursively collect files under `dir`, optionally filtered by `filterFn(path, dirent)`.
function walk(dir, filterFn) {
  const out = []
  if (!existsSync(dir)) return out
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, entry.name)
    if (entry.isDirectory()) {
      out.push(...walk(p, filterFn))
    } else if (!filterFn || filterFn(p, entry)) {
      out.push(p)
    }
  }
  return out
}

// ---------------- Rule 1: CLAUDE.md must @-import .context/INDEX.md ----------------

const claudeMdPath = join(ROOT, 'CLAUDE.md')
const claudeMd = readIfExists(claudeMdPath)

if (claudeMd === null) {
  err('CLAUDE.md is missing')
} else if (!/^@\.context\/INDEX\.md\s*$/m.test(claudeMd)) {
  err(
    'CLAUDE.md must @-import .context/INDEX.md (add "@.context/INDEX.md" as its own line, near the top)',
  )
}

// ---------------- Rule 2: required root + .context files exist ----------------

const REQUIRED_ROOT_FILES = ['CLAUDE.md', 'README.md']
for (const f of REQUIRED_ROOT_FILES) {
  if (!existsSync(join(ROOT, f))) {
    err(`Missing required root file: ${f}`)
  }
}

const REQUIRED_CONTEXT_DOCS = ['INDEX.md', 'TECHSTACK.md', 'ARCHITECTURE.md']
for (const f of REQUIRED_CONTEXT_DOCS) {
  if (!existsSync(join(CONTEXT_DIR, f))) {
    err(`Missing required context doc: .context/${f}`)
  }
}

// ---------------- Rule 3: INDEX.md <-> .context/ two-way consistency ----------------
// Every markdown file physically present under .context/ (besides INDEX.md
// itself) must be linked from INDEX.md, and every relative link INDEX.md
// makes must resolve to a file that actually exists. This is what keeps
// "every doc has exactly one home, and INDEX.md is the map" (see
// .context/reference/CLAUDE-CONTEXT-PATTERN.md) from silently rotting.

const indexPath = join(CONTEXT_DIR, 'INDEX.md')
const indexSrc = readIfExists(indexPath)

if (indexSrc === null) {
  err('.context/INDEX.md is missing')
} else {
  const linkRe = /]\(([^)]+)\)/g
  const linkedPaths = new Set()
  let match
  while ((match = linkRe.exec(indexSrc))) {
    const target = match[1].trim()
    if (/^[a-z][a-z0-9+.-]*:\/\//i.test(target)) continue // external URL, not a local doc
    linkedPaths.add(target)
  }

  for (const link of linkedPaths) {
    if (!existsSync(join(CONTEXT_DIR, link))) {
      err(`.context/INDEX.md links to "${link}", which does not exist`)
    }
  }

  const docsOnDisk = walk(CONTEXT_DIR, (p) => extname(p) === '.md').filter(
    (p) => resolve(p) !== resolve(indexPath),
  )
  for (const doc of docsOnDisk) {
    const rel = relative(CONTEXT_DIR, doc)
    if (!linkedPaths.has(rel)) {
      err(`.context/${rel} exists but is not linked from .context/INDEX.md`)
    }
  }
}

// ---------------- Rule 4: freshness (warning only) ----------------
// If Go or frontend sources have changed more recently than every doc that
// describes them, nudge to update CLAUDE.md / .context/ before merging.
// Mtimes are meaningless right after a fresh `git checkout` (everything
// lands at the same time), so this only bites in a normal working tree —
// which is exactly when it's useful.

const SOURCE_DIRS = ['internal', 'app', 'cmd', 'svc', 'frontend']
const GENERATED_MARKERS = [
  'node_modules',
  'wailsjs',
  join('frontend', 'web', 'app.bundle.js'),
  join('frontend', 'web', 'app.css'),
  join('frontend', 'web', 'inspector'),
]

const mainEntryPoints = readdirSync(ROOT, { withFileTypes: true })
  .filter((e) => e.isFile() && /^main_.*\.go$/.test(e.name))
  .map((e) => join(ROOT, e.name))

const sourceFiles = [
  ...mainEntryPoints,
  ...SOURCE_DIRS.flatMap((d) =>
    walk(join(ROOT, d), (p) => {
      if (!/\.(go|vue|ts|js|mjs)$/.test(p)) return false
      return !GENERATED_MARKERS.some((marker) => p.includes(marker))
    }),
  ),
]

const docFiles = [claudeMdPath, ...walk(CONTEXT_DIR, (p) => extname(p) === '.md')].filter(
  existsSync,
)

if (sourceFiles.length > 0 && docFiles.length > 0) {
  const newestDocMtime = Math.max(...docFiles.map((p) => statSync(p).mtimeMs))

  let newestSourceMtime = 0
  let newestSourcePath = null
  for (const s of sourceFiles) {
    const mtime = statSync(s).mtimeMs
    if (mtime > newestSourceMtime) {
      newestSourceMtime = mtime
      newestSourcePath = s
    }
  }

  if (newestSourcePath && newestSourceMtime > newestDocMtime) {
    warn(
      `Stale docs: ${relative(ROOT, newestSourcePath)} is newer than every file in CLAUDE.md / .context/ — consider updating docs`,
    )
  }
}

// ---------------- Report ----------------

if (warnings.length > 0) {
  console.log('doc-lint warnings:')
  for (const w of warnings) console.log(`  ! ${w}`)
  console.log('')
}

if (errors.length > 0) {
  console.log('doc-lint errors:')
  for (const e of errors) console.log(`  x ${e}`)
  console.log(`\ndoc-lint failed: ${errors.length} error(s)`)
  process.exit(1)
}

console.log(`doc-lint passed (${warnings.length} warning(s))`)
