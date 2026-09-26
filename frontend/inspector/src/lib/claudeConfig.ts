// Pure helpers for the Claude Config editor view - no Vue/Wails dependency,
// so they're cheap to unit test in isolation (see claudeConfig.test.ts).

export interface ClaudeJSONProjectStat {
  path: string
  size_bytes: number
  message_count: number
}

export type ProjectSortKey = 'path' | 'size_bytes' | 'message_count'

/** Overview stats for ~/.claude.json, as returned by GetClaudeJSONOverview. */
export interface ClaudeJsonOverview {
  version: string
  total_size_bytes: number
  project_count: number
  mcp_count: number
  num_startups: number
  health: 'healthy' | 'moderate' | 'warning'
}

const BYTE_UNITS = ['KB', 'MB', 'GB', 'TB']

/** Formats a byte count as a human-readable size, one decimal past bytes. */
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  let value = n / 1024
  for (const unit of BYTE_UNITS) {
    if (value < 1024 || unit === BYTE_UNITS[BYTE_UNITS.length - 1]) {
      return `${value.toFixed(1)} ${unit}`
    }
    value /= 1024
  }
  return `${value.toFixed(1)} ${BYTE_UNITS[BYTE_UNITS.length - 1]}`
}

/**
 * Returns a new, sorted copy of projects - never mutates the input, since
 * callers hold this array as reactive Vue state. `path` always sorts
 * ascending (alphabetical); numeric keys default to descending (largest
 * first, matching claude-config-editor's default "biggest projects on top").
 */
export function sortProjects(
  projects: ClaudeJSONProjectStat[],
  key: ProjectSortKey,
  descending = true,
): ClaudeJSONProjectStat[] {
  const sorted = [...projects].sort((a, b) => {
    if (key === 'path') return a.path.localeCompare(b.path)
    return a[key] - b[key]
  })
  if (descending && key !== 'path') sorted.reverse()
  return sorted
}

/** Reports whether text parses as valid JSON, for the Hooks/Raw JSON textareas. */
export function isValidJson(text: string): boolean {
  try {
    JSON.parse(text)
    return true
  } catch {
    return false
  }
}

/** Splits a whitespace-separated args string (no shell-quoting support in v1). */
export function parseArgs(text: string): string[] {
  const trimmed = text.trim()
  return trimmed === '' ? [] : trimmed.split(/\s+/)
}

/** Parses one "KEY=value" pair per line into an env map. Blank/malformed lines are skipped. */
export function parseEnv(text: string): Record<string, string> {
  const env: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const eq = trimmed.indexOf('=')
    if (eq === -1) continue
    env[trimmed.slice(0, eq)] = trimmed.slice(eq + 1)
  }
  return env
}

/**
 * Builds the confirmation-dialog message shown before an MCP server is
 * added or replaced. Adding a server writes a command Claude Code will run
 * automatically at its next session, so the message names the exact
 * command and file, and calls out explicitly when it will replace an
 * existing entry of the same name.
 */
export function buildMcpServerConfirmMessage(
  name: string,
  command: string,
  fileLabel: string,
  replacingExisting: boolean,
): string {
  if (replacingExisting) {
    return `A server named "${name}" already exists in ${fileLabel} and will be replaced with this new command. Continue?`
  }
  return `Add "${name}" (${command}) to ${fileLabel}? Claude Code will run this command at its next session.`
}
