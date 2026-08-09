<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSessionList, useSessionChunks } from '../composables/useWails'
import { useAppStore } from '../stores/app'
import SessionInspector from '../components/inspector/SessionInspector.vue'
import type { Chunk, Session } from '../lib/types'
import { generateSessionHTML } from '../lib/export'
import type { SessionMeta } from '../lib/export'
import { fmt } from '../lib/fmt'
import { App } from '../bindings/tokentally/app'
import { api } from '../lib/api'
import { KIND_LABELS, sevClass } from '../lib/findings'
import { FONT_SCALE_STEP } from '../lib/fontScale'
import { useFontScale } from '../composables/useFontScale'
import FontScaleControls from '../components/FontScaleControls.vue'

const route = useRoute()
const router = useRouter()
const store = useAppStore()

const range = ref<string>('7d')

const { fontScale, fontScalePercent, stepFontScale, resetFontScale } = useFontScale('tt.sessionsFontScale')

const selectedId = computed(() =>
  route.params.id ? decodeURIComponent(route.params.id as string) : ''
)

const projectFilter = computed(() => (route.query.project as string) || '')
const projectName = computed(() => (route.query.name as string) || projectFilter.value)

const { data: sessions, refetch: refetchSessions } = useSessionList(range, projectFilter)
const { data: chunks, visibleCount, isLoading, error, cancelReveal } = useSessionChunks(selectedId)

interface BadgeEntry { grade: string; score: number; findings: number; sev_rank: number }
const badges = ref<Record<string, BadgeEntry>>({})

interface FindingRow { kind: string; severity: number; est_tokens: number; detail: string }
interface SessionFindings { score: number | null; grade: string | null; findings: FindingRow[] }

const sessionFindings = ref<SessionFindings | null>(null)

async function fetchFindings(id: string) {
  if (!id) { sessionFindings.value = null; return }
  try {
    sessionFindings.value = await api<SessionFindings>('/api/findings/session/' + encodeURIComponent(id))
  } catch (err) {
    console.error('[findings] session fetch:', err)
    sessionFindings.value = null
  }
}

async function fetchBadges(list: Session[]) {
  if (!list.length) { badges.value = {}; return }
  try {
    const ids = list.map((s) => s.session_id)
    const result = await App.GetSessionBadges(ids)
    badges.value = result ?? {}
  } catch (err) {
    console.error('[findings] badges fetch:', err)
    badges.value = {}
  }
}

watch(sessions, fetchBadges, { immediate: true })
watch(selectedId, fetchFindings, { immediate: true })

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
  const dateStr = fmt.date(selectedSession.value?.started ?? '')
  const idPrefix = selectedId.value.slice(0, 8)
  const filename = `session-${idPrefix}-${dateStr}.html`
  const path = await App.SaveHTMLExport(html, filename)
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
      <div v-if="projectFilter" class="project-filter-bar">
        <span class="project-filter-label">{{ projectName }}</span>
        <router-link to="/sessions" class="project-filter-clear" title="Clear filter">✕</router-link>
      </div>
      <div class="session-list">
        <div
          v-for="s in sessions"
          :key="s.session_id"
          class="session-row"
          :class="{ active: s.session_id === selectedId }"
          @click="pick(s)"
        >
          <div class="session-title-row">
            <span class="session-title">{{ s.project_name || s.session_id.slice(0, 8) }}</span>
            <span v-if="badges[s.session_id]"
                  class="sess-badge"
                  :class="`grade-${badges[s.session_id].grade}`">
              {{ badges[s.session_id].grade }} · {{ badges[s.session_id].findings }} finding{{ badges[s.session_id].findings === 1 ? '' : 's' }}
            </span>
          </div>
          <div class="session-meta">
            <span class="muted mono">{{ fmt.tok(s.tokens) }} tok</span>
            <span class="muted mono">{{ fmt.date(s.started) }}</span>
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
            {{ fmt.tok(selectedSession.tokens) }} tokens · {{ fmt.date(selectedSession.started) }}
          </span>
          <span class="spacer" />
          <FontScaleControls
            class="font-scale-slot"
            :percent="fontScalePercent"
            @decrease="stepFontScale(-FONT_SCALE_STEP)"
            @increase="stepFontScale(FONT_SCALE_STEP)"
            @reset="resetFontScale"
          />
          <span v-if="exportMsg" class="export-msg muted" style="font-size:11px;font-family:var(--mono)">{{ exportMsg }}</span>
          <button class="btn-export" title="Export as HTML" :disabled="!chunks.length" @click="exportHTML">
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
        <div v-else class="inspector-scroll" :style="{ '--sessions-font-scale': fontScale }">
          <SessionInspector :chunks="chunks.slice(0, visibleCount)" />
          <div
            v-if="sessionFindings && (sessionFindings.grade || sessionFindings.findings.length)"
            class="findings-section"
          >
            <div class="findings-header">
              <span class="findings-title">Findings</span>
              <span
                v-if="sessionFindings.grade"
                class="grade-chip"
                :class="`grade-${sessionFindings.grade}`"
              >{{ sessionFindings.grade }}{{ sessionFindings.score != null ? ' · ' + sessionFindings.score : '' }}</span>
              <span v-else class="muted" style="font-size:11px">No quality score yet</span>
            </div>
            <div v-if="!sessionFindings.findings.length" class="muted" style="font-size:12px;padding:6px 0">
              No findings for this session.
            </div>
            <div v-for="(f, i) in sessionFindings.findings" :key="i" class="finding-row">
              <div class="finding-bar" :class="sevClass(f.severity)"></div>
              <div class="finding-body">
                <span class="finding-kind">{{ KIND_LABELS[f.kind] ?? f.kind }}</span>
                <span class="finding-detail muted">{{ f.detail }}</span>
              </div>
              <span class="finding-tok muted mono">~{{ fmt.tok(f.est_tokens) }}</span>
            </div>
          </div>
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
.session-title-row { display: flex; align-items: center; gap: 4px; margin-bottom: 4px; overflow: hidden; }
.session-title { font-size: 12px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex-shrink: 1; }
.sess-badge { font-size: 10px; padding: 1px 6px; border-radius: 10px; white-space: nowrap; flex-shrink: 0; }
.grade-A, .grade-B { color: #7fd49a; }
.grade-C, .grade-D { color: #e0a23a; }
.grade-F { color: #e5534b; }
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
.font-scale-slot { margin-right: 8px; }
.btn-export { background: transparent; border: 1px solid var(--border); border-radius: 4px; padding: 4px 6px; cursor: pointer; color: var(--muted); display: flex; align-items: center; justify-content: center; line-height: 1; transition: color 120ms, border-color 120ms; }
.btn-export:hover:not(:disabled) { color: var(--text); border-color: var(--text); }
.btn-export:disabled { opacity: 0.35; cursor: default; }
.export-msg { margin-right: 8px; }
.project-filter-bar { display: flex; align-items: center; gap: 6px; padding: 5px 12px; background: var(--panel); border-bottom: 1px solid var(--border); font-size: 11px; font-family: var(--mono); }
.project-filter-label { color: var(--accent, #e8956d); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; }
.project-filter-clear { color: var(--muted); text-decoration: none; flex-shrink: 0; }
.project-filter-clear:hover { color: var(--text); }
.findings-section { margin-top: 20px; padding: 12px 0 4px; border-top: 1px solid var(--border); }
.findings-header { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.findings-title { font-size: 12px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--muted); }
.grade-chip { font-size: 11px; font-weight: 700; padding: 2px 8px; border-radius: 10px; }
.finding-row { display: flex; align-items: center; gap: 10px; padding: 7px 0; border-bottom: 1px solid var(--border); }
.finding-row:last-child { border-bottom: none; }
.finding-bar { width: 3px; align-self: stretch; min-height: 28px; border-radius: 2px; flex-shrink: 0; }
.sev-high { background: #e5534b; }
.sev-med  { background: #e0a23a; }
.sev-low  { background: #4a90d9; }
.finding-body { flex: 1; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.finding-kind { font-size: 12px; font-weight: 600; }
.finding-detail { font-size: 11px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.finding-tok { font-size: 11px; flex-shrink: 0; }
</style>
