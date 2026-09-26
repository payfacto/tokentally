<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { App } from '../bindings/tokentally/app'
import {
  formatBytes, sortProjects, isValidJson, parseArgs, parseEnv, buildMcpServerConfirmMessage,
  type ClaudeJSONProjectStat, type ProjectSortKey, type ClaudeJsonOverview,
} from '../lib/claudeConfig'

type Tab = 'overview' | 'history' | 'mcp-claude-json' | 'mcp-settings' | 'hooks' | 'claude-md' | 'raw'
const TABS: Array<{ key: Tab; label: string }> = [
  { key: 'overview', label: 'Overview' },
  { key: 'history', label: 'Project History' },
  { key: 'mcp-claude-json', label: 'MCP (~/.claude.json)' },
  { key: 'mcp-settings', label: 'MCP (settings.json)' },
  { key: 'hooks', label: 'Hooks' },
  { key: 'claude-md', label: 'CLAUDE.md' },
  { key: 'raw', label: 'Raw JSON' },
]
const activeTab = ref<Tab>('overview')
const tabError = ref('')

interface McpServer { name: string; command: string; args: string[] | null; env: Record<string, string> | null; cwd: string; type: string; url: string }

// Wails v3's generated bindings return this Go method's map[string]any as a
// loosely-typed object; every `as <Type>` cast below bridges that generic
// shape to the concrete type this view relies on (same pattern already used
// in composables/useWails.ts for other map[string]any-returning bindings).
const CONFLICT_MARKER = 'changed on disk'
const FLASH_DURATION_MS = 3000
const timers: ReturnType<typeof setTimeout>[] = []

function flash(msgRef: { value: string }, text: string, duration = FLASH_DURATION_MS): void {
  msgRef.value = text
  timers.push(setTimeout(() => { msgRef.value = '' }, duration))
}

/**
 * Runs a write action, then reloads on success. On a version conflict
 * (SafeWrite's ErrConflict - Claude Code itself rewrote the file since we
 * last loaded it), reloads anyway so the section shows the current state,
 * rather than silently overwriting it.
 */
async function runWrite(
  action: () => Promise<void>,
  reload: () => Promise<void>,
  msgRef: { value: string },
  successText = 'Saved.',
): Promise<void> {
  try {
    await action()
    await reload()
    flash(msgRef, successText)
  } catch (e: unknown) {
    const text = e instanceof Error ? e.message : String(e)
    if (text.includes(CONFLICT_MARKER)) {
      msgRef.value = 'This file changed on disk since it was loaded - showing the latest version. Please retry.'
      await reload()
    } else {
      msgRef.value = 'Error: ' + text
    }
  }
}

// --- Overview ---
const overview = ref<Partial<ClaudeJsonOverview>>({})
async function loadOverview(): Promise<void> {
  overview.value = (await App.GetClaudeJSONOverview()) as unknown as ClaudeJsonOverview
}

// --- Project History ---
const projects = ref<ClaudeJSONProjectStat[]>([])
const projectsVersion = ref('')
const sortKey = ref<ProjectSortKey>('size_bytes')
const sortDesc = ref(true)
const searchQuery = ref('')
const selected = ref<Set<string>>(new Set())
const historyMsg = ref('')
const exportModal = ref<{ show: boolean; path: string; content: string }>({ show: false, path: '', content: '' })

const filteredSortedProjects = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  const filtered = q ? projects.value.filter(p => p.path.toLowerCase().includes(q)) : projects.value
  return sortProjects(filtered, sortKey.value, sortDesc.value)
})

async function loadProjects(): Promise<void> {
  const result = await App.ListClaudeJSONProjects()
  projects.value = (result.projects as ClaudeJSONProjectStat[]) ?? []
  projectsVersion.value = (result.version as string) ?? ''
}

// Selection is made against the full project list (e.g. "select top 10
// largest"), independent of the current filter. Clearing it whenever the
// search text changes means "Delete selected" can never remove a project the
// user can no longer see in the table.
watch(searchQuery, () => { selected.value = new Set() })

function toggleSort(key: ProjectSortKey) {
  if (sortKey.value === key) { sortDesc.value = !sortDesc.value } else { sortKey.value = key; sortDesc.value = true }
}

function toggleSelected(path: string) {
  const next = new Set(selected.value)
  if (next.has(path)) next.delete(path); else next.add(path)
  selected.value = next
}

function selectAll() { selected.value = new Set(filteredSortedProjects.value.map(p => p.path)) }
function selectNone() { selected.value = new Set() }
function selectTop10Largest() {
  const top10 = sortProjects(projects.value, 'size_bytes', true).slice(0, 10)
  selected.value = new Set(top10.map(p => p.path))
}

async function deleteSelected(): Promise<void> {
  if (selected.value.size === 0) return
  const paths = [...selected.value]
  if (!confirm(`Delete project history for ${paths.length} project(s) from ~/.claude.json? A backup is made first, but this removes the history entries.`)) return
  await runWrite(
    () => App.DeleteClaudeJSONProjects(paths, projectsVersion.value),
    async () => { await loadProjects(); await loadOverview(); selected.value = new Set() },
    historyMsg,
    `Deleted ${paths.length} project(s).`,
  )
}

async function exportProject(path: string): Promise<void> {
  const result = await App.ExportClaudeJSONProject(path)
  exportModal.value = { show: true, path, content: (result.content as string) ?? '' }
}

async function copyExport(): Promise<void> {
  await navigator.clipboard.writeText(exportModal.value.content)
  flash(historyMsg, 'Copied to clipboard.')
}

// --- MCP servers (shared logic for both ~/.claude.json and settings.json) ---
function emptyMcpModal() {
  return { show: false, target: 'claudeJson' as 'claudeJson' | 'settings', name: '', command: '', argsText: '', envText: '' }
}
const mcpModal = ref(emptyMcpModal())

const claudeJsonServers = ref<McpServer[]>([])
const claudeJsonServersVersion = ref('')
const claudeJsonMcpMsg = ref('')

const settingsServers = ref<McpServer[]>([])
const settingsServersVersion = ref('')
const settingsMcpMsg = ref('')

async function loadClaudeJsonServers(): Promise<void> {
  const result = await App.ListClaudeJSONMCPServers()
  claudeJsonServers.value = (result.servers as McpServer[]) ?? []
  claudeJsonServersVersion.value = (result.version as string) ?? ''
}

async function loadSettingsServers(): Promise<void> {
  const result = await App.ListSettingsMCPServers()
  settingsServers.value = (result.servers as McpServer[]) ?? []
  settingsServersVersion.value = (result.version as string) ?? ''
}

function openAddMcpModal(target: 'claudeJson' | 'settings') {
  mcpModal.value = { ...emptyMcpModal(), show: true, target }
}

async function saveMcpServer(): Promise<void> {
  const m = mcpModal.value
  const name = m.name.trim()
  const command = m.command.trim()
  if (!name || !command) return

  const existing = m.target === 'claudeJson' ? claudeJsonServers.value : settingsServers.value
  const replacing = existing.some(s => s.name === name)
  const fileLabel = m.target === 'claudeJson' ? '~/.claude.json' : 'settings.json'
  if (!confirm(buildMcpServerConfirmMessage(name, command, fileLabel, replacing))) return

  const args = parseArgs(m.argsText)
  const env = parseEnv(m.envText)
  if (m.target === 'claudeJson') {
    await runWrite(
      () => App.AddClaudeJSONMCPServer(name, command, args, env, claudeJsonServersVersion.value),
      loadClaudeJsonServers, claudeJsonMcpMsg, 'Added.',
    )
  } else {
    await runWrite(
      () => App.AddSettingsMCPServer(name, command, args, env, settingsServersVersion.value),
      loadSettingsServers, settingsMcpMsg, 'Added.',
    )
  }
  mcpModal.value.show = false
}

async function deleteMcpServer(target: 'claudeJson' | 'settings', name: string): Promise<void> {
  if (!confirm(`Delete MCP server "${name}"?`)) return
  if (target === 'claudeJson') {
    await runWrite(
      () => App.DeleteClaudeJSONMCPServer(name, claudeJsonServersVersion.value),
      loadClaudeJsonServers, claudeJsonMcpMsg, 'Deleted.',
    )
  } else {
    await runWrite(
      () => App.DeleteSettingsMCPServer(name, settingsServersVersion.value),
      loadSettingsServers, settingsMcpMsg, 'Deleted.',
    )
  }
}

// --- Hooks (raw JSON textarea over settings.json's "hooks" key) ---
const hooksContent = ref('')
const hooksVersion = ref('')
const hooksMsg = ref('')
const hooksValid = computed(() => isValidJson(hooksContent.value))

async function loadHooks(): Promise<void> {
  const result = await App.GetSettingsHooksRaw()
  hooksContent.value = (result.content as string) ?? '{}'
  hooksVersion.value = (result.version as string) ?? ''
}

async function saveHooks(): Promise<void> {
  if (!hooksValid.value) { hooksMsg.value = 'Not valid JSON.'; return }
  if (!confirm('Save hooks? These commands run automatically on matching tool calls the next time Claude Code uses them.')) return
  await runWrite(
    () => App.SaveSettingsHooksRaw(hooksContent.value, hooksVersion.value),
    loadHooks, hooksMsg,
  )
}

// --- CLAUDE.md ---
const claudeMdContent = ref('')
const claudeMdVersion = ref('')
const claudeMdMsg = ref('')

async function loadClaudeMd(): Promise<void> {
  const result = await App.GetClaudeMdContent()
  claudeMdContent.value = (result.content as string) ?? ''
  claudeMdVersion.value = (result.version as string) ?? ''
}

async function saveClaudeMd(): Promise<void> {
  await runWrite(
    () => App.SaveClaudeMdContent(claudeMdContent.value, claudeMdVersion.value),
    loadClaudeMd, claudeMdMsg,
  )
}

// --- Raw JSON (~/.claude.json, read-only view) ---
const rawContent = ref('')
async function loadRaw(): Promise<void> {
  const result = await App.GetClaudeJSONRaw()
  rawContent.value = (result.content as string) ?? '{}'
}
async function copyRaw(): Promise<void> {
  await navigator.clipboard.writeText(rawContent.value)
}

async function loadTab(tab: Tab): Promise<void> {
  switch (tab) {
    case 'overview': await loadOverview(); break
    case 'history': await Promise.all([loadProjects(), loadOverview()]); break
    case 'mcp-claude-json': await loadClaudeJsonServers(); break
    case 'mcp-settings': await loadSettingsServers(); break
    case 'hooks': await loadHooks(); break
    case 'claude-md': await loadClaudeMd(); break
    case 'raw': await loadRaw(); break
  }
}

async function selectTab(tab: Tab): Promise<void> {
  activeTab.value = tab
  tabError.value = ''
  try {
    await loadTab(tab)
  } catch (e: unknown) {
    tabError.value = 'Failed to load: ' + (e instanceof Error ? e.message : String(e))
  }
}

onMounted(() => selectTab('overview'))
onUnmounted(() => timers.forEach(clearTimeout))
</script>

<template>
  <div style="padding:20px">
    <div class="flex" style="margin-bottom:14px;align-items:center">
      <h2 style="margin:0;font-size:16px;letter-spacing:-0.01em">Config Editor</h2>
      <span class="muted" style="font-size:12px">Edits Claude Code's own on-disk config files directly. Every save keeps a timestamped backup and refuses to overwrite a file that changed since it was loaded.</span>
    </div>

    <div class="range-tabs" role="tablist" style="margin-bottom:16px;flex-wrap:wrap">
      <button v-for="t in TABS" :key="t.key" :class="{ active: activeTab === t.key }" @click="selectTab(t.key)">{{ t.label }}</button>
    </div>

    <div v-if="tabError" class="card" style="margin-bottom:16px;border-color:var(--bad);color:var(--bad)">{{ tabError }}</div>

    <!-- Overview -->
    <div v-if="activeTab === 'overview'" class="card">
      <h2>~/.claude.json Overview</h2>
      <div style="display:grid;grid-template-columns:repeat(4,1fr);gap:16px;margin-top:12px">
        <div><div class="muted" style="font-size:12px">Total size</div><div style="font-size:18px;font-weight:700">{{ formatBytes(overview.total_size_bytes ?? 0) }}</div></div>
        <div><div class="muted" style="font-size:12px">Projects</div><div style="font-size:18px;font-weight:700">{{ overview.project_count ?? 0 }}</div></div>
        <div><div class="muted" style="font-size:12px">MCP servers</div><div style="font-size:18px;font-weight:700">{{ overview.mcp_count ?? 0 }}</div></div>
        <div><div class="muted" style="font-size:12px">Startups</div><div style="font-size:18px;font-weight:700">{{ overview.num_startups ?? 0 }}</div></div>
      </div>
      <p style="margin-top:16px">
        <span class="badge" :class="overview.health === 'warning' ? 'opus' : overview.health === 'healthy' ? 'haiku' : 'sonnet'">{{ overview.health ?? ' - ' }}</span>
        <span class="muted" style="font-size:12px;margin-left:8px">Healthy: under 1MB and 10 projects. Warning: 5MB+ or 20+ projects. Use Project History to prune old sessions.</span>
      </p>
    </div>

    <!-- Project History -->
    <div v-if="activeTab === 'history'" class="card">
      <div class="flex" style="align-items:center;margin-bottom:12px;flex-wrap:wrap;gap:8px">
        <h2 style="margin:0">Project History</h2>
        <span class="spacer"></span>
        <input v-model="searchQuery" class="form-input" placeholder="Search by path" style="max-width:240px">
        <button @click="selectTop10Largest">Select top 10 largest</button>
        <button @click="selectAll">Select all</button>
        <button @click="selectNone">Select none</button>
        <button style="background:var(--bad);color:#fff;border:none;padding:6px 14px;border-radius:6px;cursor:pointer" :disabled="!selected.size" @click="deleteSelected">Delete selected ({{ selected.size }})</button>
      </div>
      <p v-if="historyMsg" class="muted" style="font-size:12px;margin-bottom:8px">{{ historyMsg }}</p>
      <div v-if="!filteredSortedProjects.length"><p class="muted">No projects found.</p></div>
      <table v-else>
        <thead>
          <tr>
            <th></th>
            <th style="cursor:pointer" @click="toggleSort('path')">path</th>
            <th class="num" style="cursor:pointer" @click="toggleSort('size_bytes')">size</th>
            <th class="num" style="cursor:pointer" @click="toggleSort('message_count')">messages</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in filteredSortedProjects" :key="p.path">
            <td><input type="checkbox" :checked="selected.has(p.path)" @change="toggleSelected(p.path)"></td>
            <td class="mono" style="font-size:12px">{{ p.path }}</td>
            <td class="num">{{ formatBytes(p.size_bytes) }}</td>
            <td class="num">{{ p.message_count }}</td>
            <td style="text-align:right"><button class="icon-btn" title="Export as JSON" @click="exportProject(p.path)">⇩</button></td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- MCP servers: ~/.claude.json -->
    <div v-if="activeTab === 'mcp-claude-json'" class="card">
      <div class="flex" style="align-items:center;margin-bottom:12px">
        <h2 style="margin:0">MCP Servers - ~/.claude.json</h2>
        <span class="spacer"></span>
        <button class="primary" @click="openAddMcpModal('claudeJson')">Add server</button>
      </div>
      <p v-if="claudeJsonMcpMsg" class="muted" style="font-size:12px;margin-bottom:8px">{{ claudeJsonMcpMsg }}</p>
      <div v-if="!claudeJsonServers.length"><p class="muted">No MCP servers configured here.</p></div>
      <table v-else>
        <thead><tr><th>name</th><th>command / url</th><th>args</th><th></th></tr></thead>
        <tbody>
          <tr v-for="s in claudeJsonServers" :key="s.name">
            <td>{{ s.name }}</td>
            <td class="mono" style="font-size:12px">{{ s.command || s.url }}</td>
            <td class="mono" style="font-size:12px">{{ (s.args || []).join(' ') }}</td>
            <td style="text-align:right"><button class="icon-btn" title="Delete" style="color:var(--bad)" @click="deleteMcpServer('claudeJson', s.name)">✕</button></td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- MCP servers: settings.json -->
    <div v-if="activeTab === 'mcp-settings'" class="card">
      <div class="flex" style="align-items:center;margin-bottom:12px">
        <h2 style="margin:0">MCP Servers - ~/.claude/settings.json</h2>
        <span class="spacer"></span>
        <button class="primary" @click="openAddMcpModal('settings')">Add server</button>
      </div>
      <p v-if="settingsMcpMsg" class="muted" style="font-size:12px;margin-bottom:8px">{{ settingsMcpMsg }}</p>
      <div v-if="!settingsServers.length"><p class="muted">No MCP servers configured here.</p></div>
      <table v-else>
        <thead><tr><th>name</th><th>command / url</th><th>args</th><th></th></tr></thead>
        <tbody>
          <tr v-for="s in settingsServers" :key="s.name">
            <td>{{ s.name }}</td>
            <td class="mono" style="font-size:12px">{{ s.command || s.url }}</td>
            <td class="mono" style="font-size:12px">{{ (s.args || []).join(' ') }}</td>
            <td style="text-align:right"><button class="icon-btn" title="Delete" style="color:var(--bad)" @click="deleteMcpServer('settings', s.name)">✕</button></td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Hooks -->
    <div v-if="activeTab === 'hooks'" class="card">
      <h2>Hooks - ~/.claude/settings.json</h2>
      <p class="muted" style="font-size:12px;margin:4px 0 12px">Raw JSON for the "hooks" key. Other settings.json keys are untouched by Save.</p>
      <textarea v-model="hooksContent" class="form-input mono" style="width:100%;height:320px;font-size:12px" spellcheck="false"></textarea>
      <div class="flex" style="gap:10px;align-items:center;margin-top:10px">
        <button class="primary" :disabled="!hooksValid" @click="saveHooks">Save</button>
        <span v-if="!hooksValid" style="color:var(--bad);font-size:12px">Not valid JSON</span>
        <span class="muted" style="font-size:12px">{{ hooksMsg }}</span>
      </div>
    </div>

    <!-- CLAUDE.md -->
    <div v-if="activeTab === 'claude-md'" class="card">
      <h2>~/.claude/CLAUDE.md</h2>
      <textarea v-model="claudeMdContent" class="form-input mono" style="width:100%;height:400px;font-size:12px" spellcheck="false"></textarea>
      <div class="flex" style="gap:10px;align-items:center;margin-top:10px">
        <button class="primary" @click="saveClaudeMd">Save</button>
        <span class="muted" style="font-size:12px">{{ claudeMdMsg }}</span>
      </div>
    </div>

    <!-- Raw JSON -->
    <div v-if="activeTab === 'raw'" class="card">
      <div class="flex" style="align-items:center;margin-bottom:8px">
        <h2 style="margin:0">~/.claude.json - Raw JSON</h2>
        <span class="spacer"></span>
        <button @click="copyRaw">Copy to clipboard</button>
      </div>
      <p class="muted" style="font-size:12px;margin-bottom:8px">Read-only. Use the Project History and MCP Servers tabs to make changes.</p>
      <pre class="mono" style="font-size:11px;white-space:pre-wrap;max-height:500px;overflow:auto;background:var(--panel-2);padding:12px;border-radius:6px">{{ rawContent }}</pre>
    </div>

    <!-- Add MCP server modal -->
    <div v-if="mcpModal.show" class="modal-overlay" @click.self="mcpModal.show = false">
      <div class="modal" style="max-width:480px;width:90vw">
        <h3 style="margin:0 0 16px;font-size:15px">Add MCP server ({{ mcpModal.target === 'claudeJson' ? '~/.claude.json' : 'settings.json' }})</h3>
        <div style="display:flex;flex-direction:column;gap:12px">
          <div>
            <label class="form-label">Name</label>
            <input v-model="mcpModal.name" class="form-input" placeholder="my-server">
          </div>
          <div>
            <label class="form-label">Command</label>
            <input v-model="mcpModal.command" class="form-input" placeholder="npx">
          </div>
          <div>
            <label class="form-label">Args (space-separated)</label>
            <input v-model="mcpModal.argsText" class="form-input" placeholder="-y some-package">
          </div>
          <div>
            <label class="form-label">Env (one KEY=value per line)</label>
            <textarea v-model="mcpModal.envText" class="form-input mono" style="height:80px;font-size:12px"></textarea>
          </div>
        </div>
        <div style="margin-top:16px;display:flex;gap:8px;justify-content:flex-end">
          <button @click="mcpModal.show = false">Cancel</button>
          <button class="primary" @click="saveMcpServer">Save</button>
        </div>
      </div>
    </div>

    <!-- Export project modal -->
    <div v-if="exportModal.show" class="modal-overlay" @click.self="exportModal.show = false">
      <div class="modal" style="max-width:640px;width:90vw">
        <h3 style="margin:0 0 12px;font-size:15px">Export - {{ exportModal.path }}</h3>
        <pre class="mono" style="font-size:11px;white-space:pre-wrap;max-height:400px;overflow:auto;background:var(--panel-2);padding:12px;border-radius:6px">{{ exportModal.content }}</pre>
        <div style="margin-top:12px;display:flex;gap:8px;justify-content:flex-end">
          <button @click="exportModal.show = false">Close</button>
          <button class="primary" @click="copyExport">Copy to clipboard</button>
        </div>
      </div>
    </div>
  </div>
</template>
