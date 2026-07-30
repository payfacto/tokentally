<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import type { MarkdownFolder, MarkdownFile } from '../composables/useWails'
import { renderDocument } from '../lib/markup'
import { fmt } from '../lib/fmt'
import { copyMarkdown } from '../lib/clipboard'

const POLL_INTERVAL_MS = 15000

const folders = ref<MarkdownFolder[]>([])
const files = ref<MarkdownFile[]>([])
const selected = ref<MarkdownFile | null>(null)
const content = ref('')
const loadError = ref('')

const renderedContent = computed(() => renderDocument(content.value))

const groups = computed(() => {
  const byLabel = new Map<string, MarkdownFile[]>()
  for (const f of files.value) {
    const list = byLabel.get(f.folder_label) ?? []
    list.push(f)
    byLabel.set(f.folder_label, list)
  }
  for (const list of byLabel.values()) {
    list.sort((a, b) => b.mod_time.localeCompare(a.mod_time))
  }
  return Array.from(byLabel.entries()).map(([label, list]) => ({ label, files: list }))
})

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

async function fetchFolders() {
  try {
    folders.value = await window.go.app.App.GetMarkdownFolders()
  } catch (err) {
    console.error('[notes] fetchFolders:', err)
  }
}

async function fetchFiles() {
  try {
    const next = await window.go.app.App.ListMarkdownFiles()

    // If the open file changed on disk (e.g. a handoff got appended to),
    // refetch its content so the reader stays live.
    const current = selected.value
    if (current) {
      const updated = next.find((f) => f.full_path === current.full_path)
      if (updated && updated.mod_time !== current.mod_time) {
        selected.value = updated
        await loadContent(updated)
      }
    }

    files.value = next
  } catch (err) {
    console.error('[notes] fetchFiles:', err)
  }
}

// Guards against the poll's refresh and a user click resolving out of
// order: only the load that matches the current selection when it
// resolves is allowed to write content/loadError.
let loadRequestId = 0

async function loadContent(file: MarkdownFile) {
  const requestId = ++loadRequestId
  loadError.value = ''
  try {
    const text = await window.go.app.App.ReadMarkdownFile(file.full_path)
    if (requestId !== loadRequestId) return
    content.value = text
  } catch (err) {
    if (requestId !== loadRequestId) return
    content.value = ''
    loadError.value = String(err)
  }
}

async function select(file: MarkdownFile) {
  selected.value = file
  await loadContent(file)
}

async function copyFile(file: MarkdownFile, e: MouseEvent) {
  if (!(e.currentTarget instanceof HTMLElement)) return
  const btn = e.currentTarget
  try {
    const text = await window.go.app.App.ReadMarkdownFile(file.full_path)
    await copyMarkdown(text, btn)
  } catch (err) {
    console.error('[notes] copyFile:', err)
  }
}

let pollTimer: ReturnType<typeof setInterval> | undefined

onMounted(async () => {
  await Promise.all([fetchFolders(), fetchFiles()])
  pollTimer = setInterval(fetchFiles, POLL_INTERVAL_MS)
})
onUnmounted(() => clearInterval(pollTimer))
</script>

<template>
  <div class="notes-page">
    <div class="notes-sidebar">
      <div class="sidebar-header">
        <span class="muted" style="font-size:11px">{{ files.length }} file{{ files.length === 1 ? '' : 's' }}</span>
      </div>
      <div class="notes-list">
        <template v-for="group in groups" :key="group.label">
          <div class="group-header">{{ group.label }}</div>
          <div
            v-for="f in group.files"
            :key="f.full_path"
            class="note-row"
            :class="{ active: selected?.full_path === f.full_path }"
            @click="select(f)"
          >
            <div class="note-row-main">
              <div class="note-title">{{ f.filename }}</div>
              <div class="note-meta">
                <span class="muted mono">{{ fmt.ts(f.mod_time) }}</span>
                <span class="muted mono">{{ formatSize(f.size) }}</span>
              </div>
            </div>
            <button class="copy-btn" title="Copy as Markdown" @click.stop="copyFile(f, $event)">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
            </button>
          </div>
        </template>
        <div v-if="!folders.length" class="empty">
          <span>○</span>
          No folders configured — add one in Settings.
        </div>
        <div v-else-if="!files.length" class="empty">
          <span>○</span>
          No files found.
        </div>
      </div>
    </div>

    <div class="notes-main">
      <div v-if="!selected" class="empty" style="height:100%;justify-content:center">
        <span>◎</span>
        Select a file to view it
      </div>
      <template v-else>
        <div class="notes-header">
          <span style="font-weight:600;font-size:14px">{{ selected.filename }}</span>
          <span class="muted" style="font-size:11px;font-family:var(--mono);margin-left:8px">{{ selected.folder_path }}</span>
        </div>
        <div v-if="loadError" class="empty" style="padding:16px">
          <span>⚠</span> {{ loadError }}
        </div>
        <div v-else class="notes-scroll markdown-body" v-html="renderedContent" />
      </template>
    </div>
  </div>
</template>

<style scoped>
.notes-page { display: flex; height: calc(100vh - 48px); overflow: hidden; background: var(--bg); }
.notes-sidebar { width: 280px; flex-shrink: 0; border-right: 1px solid var(--border); display: flex; flex-direction: column; overflow: hidden; }
.sidebar-header { display: flex; align-items: center; gap: 8px; padding: 10px 12px; border-bottom: 1px solid var(--border); }
.notes-list { overflow-y: auto; flex: 1; }
.group-header { padding: 8px 12px 4px; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--muted); }
.note-row { display: flex; align-items: center; gap: 8px; padding: 8px 12px; cursor: pointer; border-bottom: 1px solid var(--border); }
.note-row:hover { background: var(--panel); }
.note-row.active { background: var(--panel-2, var(--panel)); border-left: 2px solid var(--accent); }
.note-row-main { flex: 1; min-width: 0; }
.note-title { font-size: 12px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.note-meta { display: flex; align-items: center; gap: 8px; font-family: var(--mono); font-size: 10px; margin-top: 2px; }
.copy-btn {
  background: transparent; border: 1px solid var(--border);
  border-radius: 4px; padding: 4px 5px;
  cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center;
  line-height: 1; transition: color 120ms, border-color 120ms; flex-shrink: 0;
}
.copy-btn:hover { color: var(--text); border-color: var(--text); }
.notes-main { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
.notes-header { padding: 10px 16px; border-bottom: 1px solid var(--border); display: flex; align-items: center; flex-shrink: 0; }
.notes-scroll { flex: 1; overflow-y: auto; padding: 16px 20px; }
.empty { display: flex; flex-direction: column; align-items: center; gap: 8px; color: var(--muted); font-size: 13px; padding: 32px; }
.muted { color: var(--muted); }
.mono { font-family: var(--mono); }

.markdown-body :deep(h1) { font-size: 20px; margin: 0 0 12px; }
.markdown-body :deep(h2) { font-size: 16px; margin: 20px 0 8px; }
.markdown-body :deep(h3) { font-size: 14px; margin: 16px 0 6px; }
.markdown-body :deep(p) { margin: 0 0 10px; line-height: 1.6; font-size: 13px; }
.markdown-body :deep(ul), .markdown-body :deep(ol) { margin: 0 0 10px; padding-left: 22px; font-size: 13px; line-height: 1.6; }
.markdown-body :deep(code) { font-family: var(--mono); background: var(--panel); padding: 1px 4px; border-radius: 3px; font-size: 12px; }
.markdown-body :deep(pre) { background: var(--panel); padding: 10px 12px; border-radius: 6px; overflow-x: auto; margin: 0 0 10px; }
.markdown-body :deep(pre code) { background: none; padding: 0; }
.markdown-body :deep(blockquote) { border-left: 2px solid var(--border); margin: 0 0 10px; padding-left: 12px; color: var(--muted); }
.markdown-body :deep(a) { color: var(--accent); }
.markdown-body :deep(table) { border-collapse: collapse; margin: 0 0 10px; font-size: 12px; }
.markdown-body :deep(th), .markdown-body :deep(td) { border: 1px solid var(--border); padding: 4px 8px; }
</style>
