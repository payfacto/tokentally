<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSessionList, useSessionChunks } from '../composables/useWails'
import { useAppStore } from '../stores/app'
import SessionInspector from '../components/inspector/SessionInspector.vue'
import type { Chunk, Session } from '../lib/types'
import { generateSessionHTML } from '../lib/export'
import type { SessionMeta } from '../lib/export'

const route = useRoute()
const router = useRouter()
const store = useAppStore()

const range = ref<string>('7d')

const selectedId = computed(() =>
  route.params.id ? decodeURIComponent(route.params.id as string) : ''
)

const { data: sessions, refetch: refetchSessions } = useSessionList(range)
const { data: chunks, visibleCount, isLoading, error, cancelReveal } = useSessionChunks(selectedId)

function pick(session: Session) {
  router.push('/sessions/' + encodeURIComponent(session.session_id))
}

onMounted(() => {
  nextTick(() => {
    document.querySelector('.session-row.active')?.scrollIntoView({ block: 'nearest' })
  })
})

watch(() => store.lastScan, refetchSessions)

const selectedSession = computed(() =>
  sessions.value.find((s: Session) => s.session_id === selectedId.value)
)
const fmtDate = (ts: string) => ts ? ts.slice(0, 10) : '—'
const fmtTok = (n: number) => n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n)

const exportMsg = ref('')
let exportTimer: ReturnType<typeof setTimeout> | undefined

async function exportHTML() {
  const meta: SessionMeta = {
    sessionId: selectedId.value,
    projectName: selectedSession.value?.project_name ?? '',
    started: selectedSession.value?.started ?? '',
    ended: chunks.value.at(-1)?.timestamp ?? '',
  }
  const html = generateSessionHTML(chunks.value, meta)
  const dateStr = fmtDate(selectedSession.value?.started ?? '')
  const idPrefix = selectedId.value.slice(0, 8)
  const filename = `session-${idPrefix}-${dateStr}.html`
  const path = await window.go.app.App.SaveHTMLExport(html, filename)
  if (path) {
    clearTimeout(exportTimer)
    exportMsg.value = 'Saved'
    exportTimer = setTimeout(() => { exportMsg.value = '' }, 2000)
  }
}

onUnmounted(() => { cancelReveal(); clearTimeout(exportTimer) })
</script>

<template>
  <div class="sessions-page">
    <div class="sessions-sidebar">
      <div class="sidebar-header">
        <select v-model="range" class="range-select">
          <option value="today">Today</option>
          <option value="7d">7 days</option>
          <option value="30d">30 days</option>
        </select>
        <span class="muted" style="font-size:11px">{{ sessions.length }} sessions</span>
      </div>
      <div class="session-list">
        <div
          v-for="s in sessions"
          :key="s.session_id"
          class="session-row"
          :class="{ active: s.session_id === selectedId }"
          @click="pick(s)"
        >
          <div class="session-title">{{ s.project_name || s.session_id.slice(0, 8) }}</div>
          <div class="session-meta">
            <span class="muted mono">{{ fmtTok(s.tokens) }} tok</span>
            <span class="muted mono">{{ fmtDate(s.started) }}</span>
          </div>
          <div class="muted" style="font-size:10px;font-family:var(--mono)">{{ s.session_id.slice(0, 8) }}</div>
        </div>
        <div v-if="!sessions.length" class="empty">
          <span>○</span>
          No sessions in range.
        </div>
      </div>
    </div>

    <div class="sessions-main">
      <div v-if="!selectedId" class="empty" style="height:100%;justify-content:center">
        <span>◎</span>
        Select a session to inspect
      </div>
      <template v-else>
        <div class="inspector-header">
          <span style="font-weight:600;font-size:14px">
            {{ selectedSession?.project_name || selectedId.slice(0, 8) }}
          </span>
          <span class="muted" style="font-size:11px;font-family:var(--mono);margin-left:8px">
            {{ selectedId.slice(0, 8) }}
          </span>
          <span v-if="selectedSession" class="muted" style="font-size:11px;font-family:var(--mono);margin-left:12px">
            {{ fmtTok(selectedSession.tokens) }} tokens · {{ fmtDate(selectedSession.started) }}
          </span>
          <span class="spacer" />
          <span v-if="exportMsg" class="export-msg muted" style="font-size:11px;font-family:var(--mono)">{{ exportMsg }}</span>
          <button class="btn-export" title="Export as HTML" @click="exportHTML">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
          </button>
        </div>
        <div v-if="isLoading" class="skeleton" style="height:80px;margin:16px" />
        <div v-else-if="error" class="empty" style="padding:16px">
          <span>⚠</span> {{ error }}
        </div>
        <div v-else-if="!chunks.length" class="empty" style="padding:16px">
          <span>○</span> No turns found for this session.
        </div>
        <div v-else class="inspector-scroll">
          <SessionInspector :chunks="chunks.slice(0, visibleCount)" />
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.sessions-page { display: flex; height: calc(100vh - 48px); overflow: hidden; background: var(--bg); }
.sessions-sidebar { width: 280px; flex-shrink: 0; border-right: 1px solid var(--border); display: flex; flex-direction: column; overflow: hidden; }
.sidebar-header { display: flex; align-items: center; gap: 8px; padding: 10px 12px; border-bottom: 1px solid var(--border); }
.range-select { font-size: 12px; border: 1px solid var(--border); background: var(--panel); color: var(--text); border-radius: 4px; padding: 2px 6px; }
.session-list { overflow-y: auto; flex: 1; }
.session-row { padding: 10px 12px; cursor: pointer; border-bottom: 1px solid var(--border); }
.session-row:hover { background: var(--panel); }
.session-row.active { background: var(--panel-2, var(--panel)); border-left: 2px solid var(--accent); }
.session-title { font-size: 12px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-bottom: 4px; }
.session-meta { display: flex; align-items: center; gap: 6px; font-family: var(--mono); font-size: 10px; margin-bottom: 2px; }
.sessions-main { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
.inspector-header { padding: 10px 16px; border-bottom: 1px solid var(--border); display: flex; align-items: center; flex-shrink: 0; }
.inspector-scroll { flex: 1; overflow-y: auto; padding: 0 16px 16px; }
.empty { display: flex; flex-direction: column; align-items: center; gap: 8px; color: var(--muted); font-size: 13px; padding: 32px; }
.skeleton { background: var(--panel); border-radius: 4px; animation: pulse 1.5s infinite; }
.muted { color: var(--muted); }
.mono { font-family: var(--mono); }
@keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.5; } }
.spacer { flex: 1; }
.btn-export { background: transparent; border: 1px solid var(--border); border-radius: 4px; padding: 4px 6px; cursor: pointer; color: var(--muted); display: flex; align-items: center; justify-content: center; line-height: 1; transition: color 120ms, border-color 120ms; }
.btn-export:hover { color: var(--text); border-color: var(--text); }
.export-msg { margin-right: 8px; }
</style>
