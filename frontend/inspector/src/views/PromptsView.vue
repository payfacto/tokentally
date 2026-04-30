<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../lib/api'
import { fmt, SESSION_ID_PREFIX } from '../lib/fmt'
import { renderMarkdown, stripTagsForPreview } from '../lib/markup'
import { useSort, SORTS } from '../composables/useSort'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const { sort, sortKey, setSort } = useSort()

interface PromptRow {
  timestamp: string; prompt_text: string; model: string
  billable_tokens: number; cache_read_tokens: number
  session_id: string; estimated_cost_usd: number
  is_sidechain: number; msg_type: string
}

const rows = ref<PromptRow[]>([])
const searchRows = ref<PromptRow[]>([])
const modalRow = ref<PromptRow | null>(null)
const copyFlash = ref(false)
const searchPending = ref(false)

const searchQuery = ref('')
const searchTypes = ref<string[]>(['user'])
const searchFrom = ref(todayISO())
const searchTo = ref(todayISO())

let copyTimer: ReturnType<typeof setTimeout> | undefined
let searchTimer: ReturnType<typeof setTimeout> | undefined
let searchSeq = 0

function todayISO(): string {
  return new Date().toISOString().slice(0, 10)
}

function toggleType(t: string) {
  const idx = searchTypes.value.indexOf(t)
  if (idx >= 0) {
    if (searchTypes.value.length > 1) {
      searchTypes.value = searchTypes.value.filter(x => x !== t)
    }
  } else {
    searchTypes.value = [...searchTypes.value, t]
  }
}

function prettyPrompt(text: string): string {
  if (!text) return ''
  if (!/<[a-z_][a-z_0-9]*[^>]*>/.test(text)) return text
  return text
    .replace(/(<[a-z_][a-z_0-9]*(?:\s[^>]*)?>)/g, '\n$1\n')
    .replace(/(<\/[a-z_][a-z_0-9]*>)/g, '\n$1\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

async function fetchRows() {
  if (sort.value.key === 'search') return
  rows.value = (await api<PromptRow[]>('/api/prompts?limit=100&sort=' + encodeURIComponent(sort.value.key))) ?? []
}

async function doSearch() {
  if (sort.value.key !== 'search') return
  const trimmed = searchQuery.value.trim()
  if (trimmed.length > 0 && trimmed.length < 3) {
    searchRows.value = []
    return
  }
  const seq = ++searchSeq
  searchPending.value = true
  try {
    const types = searchTypes.value.join(',')
    const params = new URLSearchParams({ q: searchQuery.value, types, from: searchFrom.value, to: searchTo.value })
    const result = await api<PromptRow[]>('/api/prompts/search?' + params.toString())
    if (seq === searchSeq) searchRows.value = result ?? []
  } finally {
    if (seq === searchSeq) searchPending.value = false
  }
}

function scheduleSearch() {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(doSearch, 200)
}

async function copyPrompt() {
  if (!modalRow.value) return
  await navigator.clipboard.writeText(prettyPrompt(modalRow.value.prompt_text || ''))
  clearTimeout(copyTimer)
  copyFlash.value = true
  copyTimer = setTimeout(() => { copyFlash.value = false }, 1200)
}

const displayRows = computed<PromptRow[]>(() =>
  sort.value.key === 'search' ? searchRows.value : rows.value
)

const subtitle = computed(() => {
  if (sort.value.key === 'search') return null
  return sort.value.key === 'recent'
    ? 'Your latest prompts (including subagent and hook entries) and the assistant turn each one triggered. Click a row to see the full prompt.'
    : 'The prompts that cost the most tokens, including subagent and hook entries. Click a row to see the full prompt.'
})

onMounted(fetchRows)
onUnmounted(() => { clearTimeout(copyTimer); clearTimeout(searchTimer) })

watch(sortKey, async (newKey) => {
  if (newKey === 'search') {
    await nextTick()
    const el = document.querySelector<HTMLInputElement>('.search-input')
    el?.focus()
    doSearch()
  } else {
    fetchRows()
  }
})

watch(() => store.lastScan, () => {
  if (sort.value.key !== 'search') fetchRows()
})

watch([searchQuery, searchTypes, searchFrom, searchTo], scheduleSearch)
</script>

<template>
  <div style="padding:20px">
    <div class="flex" style="margin-bottom:14px">
      <h2 style="margin:0;font-size:16px;letter-spacing:-0.01em">Prompts</h2>
      <div class="spacer"></div>
      <div class="range-tabs" role="tablist">
        <button
          v-for="s in SORTS"
          :key="s.key"
          :class="{ active: s.key === sort.key }"
          @click="setSort(s.key)"
        >{{ s.label }}</button>
      </div>
    </div>

    <!-- Search tab -->
    <template v-if="sort.key === 'search'">
      <div class="card" style="margin-bottom:12px">
        <div class="search-controls">
          <div style="display:flex;align-items:center;gap:8px;flex:1;min-width:0">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="flex-shrink:0;color:var(--muted)"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
            <input
              v-model="searchQuery"
              class="search-input"
              type="text"
              placeholder="Search prompt text…"
              @input="scheduleSearch"
            />
          </div>
          <div class="type-chips">
            <button
              v-for="t in [['user','User'],['subagent','Subagents'],['hook','Hooks']]"
              :key="t[0]"
              :class="['type-chip', { active: searchTypes.includes(t[0]) }]"
              @click="toggleType(t[0])"
            >{{ t[1] }}</button>
          </div>
          <div class="date-range">
            <input type="date" v-model="searchFrom" class="date-input" @change="scheduleSearch" />
            <span class="muted" style="font-size:12px">–</span>
            <input type="date" v-model="searchTo" class="date-input" @change="scheduleSearch" />
          </div>
        </div>
      </div>

      <div class="card">
        <div v-if="searchPending" class="muted" style="margin:0 0 10px;font-size:12px">Searching…</div>
        <table>
          <thead>
            <tr>
              <th>when</th>
              <th></th>
              <th>prompt</th>
              <th>model</th>
              <th class="num">tokens</th>
              <th class="num">cache rd</th>
              <th>session</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="r in displayRows"
              :key="r.session_id + r.timestamp"
              style="cursor:pointer"
              @click="modalRow = r"
            >
              <td class="mono">{{ fmt.ts(r.timestamp) }}</td>
              <td style="color:var(--muted)" :title="r.msg_type === 'attachment' ? 'hook' : r.is_sidechain ? 'subagent' : 'user'">
                <svg v-if="r.msg_type === 'attachment'" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
                <svg v-else-if="r.is_sidechain" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 8V4H8"/><rect width="16" height="12" x="4" y="8" rx="2"/><path d="M2 14h2"/><path d="M20 14h2"/><path d="M15 13v2"/><path d="M9 13v2"/></svg>
                <svg v-else width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="4"/><path d="M4 20c0-4 3.6-7 8-7s8 3 8 7"/></svg>
              </td>
              <td>{{ fmt.short(stripTagsForPreview(r.prompt_text), 110) }}</td>
              <td><span v-if="r.model" :class="'badge ' + fmt.modelClass(r.model)">{{ fmt.modelShort(r.model) }}</span></td>
              <td class="num">{{ fmt.int(r.billable_tokens) }}</td>
              <td class="num">{{ fmt.int(r.cache_read_tokens) }}</td>
              <td>
                <RouterLink :to="'/sessions/' + encodeURIComponent(r.session_id)" class="mono" @click.stop>
                  {{ r.session_id.slice(0, SESSION_ID_PREFIX) }}…
                </RouterLink>
              </td>
            </tr>
            <tr v-if="!searchPending && !displayRows?.length">
              <td colspan="7" class="muted">{{
                searchQuery.trim().length > 0 && searchQuery.trim().length < 3
                  ? 'type 3 or more characters to search'
                  : searchQuery || searchTypes.length < 3 ? 'no results' : 'enter a search term or adjust filters'
              }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <!-- Tokens / Recent tabs -->
    <template v-else>
      <div class="card">
        <p class="muted" style="margin:0 0 14px">{{ subtitle }}</p>
        <table>
          <thead>
            <tr>
              <th>{{ sort.key === 'recent' ? 'when' : 'est. cost' }}</th>
              <th></th>
              <th>prompt</th>
              <th>model</th>
              <th class="num">tokens</th>
              <th class="num">cache rd</th>
              <th>session</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="r in displayRows"
              :key="r.session_id + r.timestamp"
              style="cursor:pointer"
              @click="modalRow = r"
            >
              <td :class="sort.key === 'recent' ? 'mono' : 'num mono'">
                {{ sort.key === 'recent' ? fmt.ts(r.timestamp) : fmt.money4(r.estimated_cost_usd, store.currency, store.exchangeRate) }}
              </td>
              <td style="color:var(--muted)" :title="r.msg_type === 'attachment' ? 'hook' : r.is_sidechain ? 'subagent' : 'user'">
                <svg v-if="r.msg_type === 'attachment'" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
                <svg v-else-if="r.is_sidechain" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 8V4H8"/><rect width="16" height="12" x="4" y="8" rx="2"/><path d="M2 14h2"/><path d="M20 14h2"/><path d="M15 13v2"/><path d="M9 13v2"/></svg>
                <svg v-else width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="4"/><path d="M4 20c0-4 3.6-7 8-7s8 3 8 7"/></svg>
              </td>
              <td>{{ fmt.short(stripTagsForPreview(r.prompt_text), 110) }}</td>
              <td><span :class="'badge ' + fmt.modelClass(r.model)">{{ fmt.modelShort(r.model) }}</span></td>
              <td class="num">{{ fmt.int(r.billable_tokens) }}</td>
              <td class="num">{{ fmt.int(r.cache_read_tokens) }}</td>
              <td>
                <RouterLink :to="'/sessions/' + encodeURIComponent(r.session_id)" class="mono" @click.stop>
                  {{ r.session_id.slice(0, SESSION_ID_PREFIX) }}…
                </RouterLink>
              </td>
            </tr>
            <tr v-if="!displayRows?.length">
              <td colspan="7" class="muted">no prompts yet</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <!-- Detail modal -->
    <div v-if="modalRow" class="modal-overlay" @click.self="modalRow = null">
      <div class="modal" style="max-width:760px;width:90vw;max-height:80vh;display:flex;flex-direction:column">
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:12px;flex-shrink:0">
          <strong style="font-size:14px">Prompt detail</strong>
          <span v-if="modalRow.msg_type === 'attachment'" class="tag-hook"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg> hook result</span>
          <span v-else-if="modalRow.is_sidechain" class="tag-subagent"><svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 8V4H8"/><rect width="16" height="12" x="4" y="8" rx="2"/><path d="M2 14h2"/><path d="M20 14h2"/><path d="M15 13v2"/><path d="M9 13v2"/></svg> subagent response</span>
          <span class="spacer"></span>
          <span :class="'badge ' + fmt.modelClass(modalRow.model)">{{ fmt.modelShort(modalRow.model) }}</span>
        </div>
        <div style="position:relative;flex:1;overflow:hidden;display:flex;flex-direction:column">
          <div class="prompt-body markdown-body" v-html="renderMarkdown(modalRow.prompt_text || '')"></div>
          <button
            class="btn-copy"
            :class="{ 'btn-copy--flash': copyFlash }"
            title="Copy to clipboard"
            @click="copyPrompt"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
          </button>
        </div>
        <div class="flex" style="margin-top:12px;flex-wrap:wrap;gap:14px;flex-shrink:0">
          <span class="muted">{{ fmt.ts(modalRow.timestamp) }}</span>
          <span class="muted">{{ fmt.int(modalRow.billable_tokens) }} billed tokens · {{ fmt.int(modalRow.cache_read_tokens) }} cache reads · ~{{ fmt.money4(modalRow.estimated_cost_usd, store.currency, store.exchangeRate) }} est. cost</span>
          <span class="spacer"></span>
          <RouterLink :to="'/sessions/' + encodeURIComponent(modalRow.session_id)" @click="modalRow = null">Open session →</RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tag-subagent, .tag-hook {
  display: inline-flex; align-items: center; gap: 4px;
  font-size: 11px;
  padding: 2px 7px;
  border-radius: 10px;
  font-weight: 500;
  white-space: nowrap;
}
.tag-subagent { background: color-mix(in srgb, var(--accent) 12%, transparent); color: var(--accent-2); }
.tag-hook     { background: color-mix(in srgb, var(--warn)   12%, transparent); color: var(--warn); }

/* Search controls */
.search-controls {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.search-input {
  flex: 1;
  min-width: 160px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 5px;
  padding: 5px 8px;
  font-size: 13px;
  color: var(--text);
  font-family: inherit;
  outline: none;
  transition: border-color 120ms;
}
.search-input:focus { border-color: var(--accent); }
.search-input::placeholder { color: var(--muted); }

.type-chips {
  display: flex;
  gap: 4px;
}
.type-chip {
  font-size: 11px;
  padding: 3px 9px;
  border-radius: 10px;
  border: 1px solid var(--border);
  background: transparent;
  color: var(--muted);
  cursor: pointer;
  font-family: inherit;
  font-weight: 500;
  transition: background 120ms, color 120ms, border-color 120ms;
}
.type-chip.active {
  background: color-mix(in srgb, var(--accent) 14%, transparent);
  color: var(--accent-2);
  border-color: color-mix(in srgb, var(--accent) 40%, transparent);
}

.date-range {
  display: flex;
  align-items: center;
  gap: 6px;
}
.date-input {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 5px;
  padding: 4px 6px;
  font-size: 12px;
  color: var(--text);
  font-family: inherit;
  outline: none;
  transition: border-color 120ms;
}
.date-input:focus { border-color: var(--accent); }

.prompt-body {
  background: var(--bg);
  padding: 12px 12px 40px;
  border-radius: 6px;
  border: 1px solid var(--border);
  font-size: 12px;
  line-height: 1.6;
  overflow-y: auto;
  flex: 1;
  margin: 0;
  word-break: break-word;
}

.markdown-body :deep(p) { margin: 0 0 6px; }
.markdown-body :deep(p:last-child) { margin-bottom: 0; }
.markdown-body :deep(strong) { font-weight: 600; }
.markdown-body :deep(em) { font-style: italic; }
.markdown-body :deep(h1), .markdown-body :deep(h2),
.markdown-body :deep(h3), .markdown-body :deep(h4) {
  font-weight: 600; margin: 8px 0 4px; line-height: 1.3;
}
.markdown-body :deep(code) {
  font-family: var(--mono); font-size: 11px;
  background: rgba(0,0,0,0.05); border-radius: 3px; padding: 1px 4px;
}
.markdown-body :deep(pre) {
  background: #2a1f14; border-radius: 6px; padding: 8px 10px; margin: 4px 0; overflow-x: auto;
}
.markdown-body :deep(pre > code) {
  background: none; padding: 0; font-size: 11px; color: #e8d5bc; display: block;
}
.markdown-body :deep(ul), .markdown-body :deep(ol) { padding-left: 18px; margin: 2px 0 6px; }
.markdown-body :deep(li) { margin: 1px 0; }
.markdown-body :deep(blockquote) {
  border-left: 3px solid var(--border); color: var(--muted); padding: 2px 0 2px 8px; margin: 4px 0;
}
.markdown-body :deep(a) { color: var(--accent); text-decoration: none; }
.markdown-body :deep(a:hover) { text-decoration: underline; }

.markdown-body :deep(.sys-tag) {
  display: inline-block;
  font-family: var(--mono); font-size: 10px;
  color: var(--muted);
  background: rgba(122, 92, 58, 0.08);
  border: 1px solid var(--border);
  border-radius: 3px; padding: 1px 5px; margin: 1px 0; opacity: 0.75;
}
.markdown-body :deep(.sys-block) {
  opacity: 0.55;
  font-size: 11px;
  border-left: 2px solid var(--border);
  padding-left: 8px;
  margin: 4px 0 8px;
}
.markdown-body :deep(.sys-block .sys-tag) { opacity: 1; }

.btn-copy {
  position: absolute; bottom: 8px; right: 8px;
  background: transparent; border: 1px solid var(--border); border-radius: 4px;
  padding: 5px 7px; cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center; line-height: 1;
  transition: color 120ms, border-color 120ms;
}
.btn-copy--flash { border-color: var(--good); color: var(--good); }
</style>
