<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useSessionList, useSessionChunks } from './composables/useWails'
import SessionInspector from './components/inspector/SessionInspector.vue'
import type { Chunk, Session } from './lib/types'

const props = defineProps<{ initialHash: string }>()

const range = ref<string>('7d')
const selectedId = ref<string>('')

// Pre-select from initial hash: #/sessions/UUID
const hashId = props.initialHash.split('/')[2]
if (hashId) selectedId.value = decodeURIComponent(hashId)

const { data: sessions, refetch: refetchSessions } = useSessionList(range)
const { data: chunks, visibleCount, isLoading, error } = useSessionChunks(selectedId)

function pick(session: Session) {
  selectedId.value = session.session_id
  window.location.hash = '#/sessions/' + encodeURIComponent(session.session_id)
}

function onHashChange() {
  const id = window.location.hash.split('/')[2]
  selectedId.value = id ? decodeURIComponent(id) : ''
}

onMounted(() => {
  window.addEventListener('hashchange', onHashChange)
  try { window.runtime.EventsOn('scan', refetchSessions) } catch { /* not in Wails env */ }
  nextTick(() => {
    document.querySelector('.session-row.active')?.scrollIntoView({ block: 'nearest' })
  })
})

onUnmounted(() => {
  window.removeEventListener('hashchange', onHashChange)
  try { window.runtime.EventsOff('scan') } catch { /* not in Wails env */ }
})

const selectedSession = computed(() =>
  sessions.value.find((s: Session) => s.session_id === selectedId.value)
)
const fmtDate = (ts: string) => ts ? ts.slice(0, 10) : '—'
const fmtTok = (n: number) => n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n)
</script>

<template>
  <div class="sessions-page">
    <!-- Sidebar -->
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

    <!-- Inspector pane -->
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
        </div>
        <div v-if="isLoading" class="skeleton" style="height:80px;margin:16px" />
        <div v-else-if="error" class="empty" style="padding:16px">
          <span>⚠</span> {{ error }}
        </div>
        <div v-else-if="!chunks.length" class="empty" style="padding:16px">
          <span>○</span> No turns found for this session.
        </div>
        <div v-else class="inspector-scroll">
          <SessionInspector :chunks="(chunks.slice(0, visibleCount) as Chunk[])" />
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
</style>
