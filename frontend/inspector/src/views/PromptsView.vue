<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../lib/api'
import { fmt, SESSION_ID_PREFIX } from '../lib/fmt'
import { useSort, SORTS } from '../composables/useSort'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const { sort, sortKey, setSort } = useSort()

interface PromptRow {
  timestamp: string; prompt_text: string; model: string
  billable_tokens: number; cache_read_tokens: number
  session_id: string; estimated_cost_usd: number
}

const rows = ref<PromptRow[]>([])
const modalRow = ref<PromptRow | null>(null)
const copyFlash = ref(false)
let copyTimer: ReturnType<typeof setTimeout> | undefined

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
  rows.value = (await api('/api/prompts?limit=100&sort=' + encodeURIComponent(sort.value.key))) as PromptRow[]
}

async function copyPrompt() {
  if (!modalRow.value) return
  await navigator.clipboard.writeText(prettyPrompt(modalRow.value.prompt_text || ''))
  clearTimeout(copyTimer)
  copyFlash.value = true
  copyTimer = setTimeout(() => { copyFlash.value = false }, 1200)
}

const subtitle = computed(() =>
  sort.value.key === 'recent'
    ? 'Your latest prompts and the assistant turn each one triggered. Click a row to see the full prompt.'
    : 'The prompts that cost the most tokens. Click a row to see the full prompt.'
)

onMounted(fetchRows)
onUnmounted(() => clearTimeout(copyTimer))
watch([sortKey, () => store.lastScan], fetchRows)
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

    <div class="card">
      <p class="muted" style="margin:0 0 14px">{{ subtitle }}</p>
      <table>
        <thead>
          <tr>
            <th>{{ sort.key === 'recent' ? 'when' : 'cache cost' }}</th>
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
            v-for="r in rows"
            :key="r.session_id + r.timestamp"
            style="cursor:pointer"
            @click="modalRow = r"
          >
            <td :class="sort.key === 'recent' ? 'mono' : 'num mono'">
              {{ sort.key === 'recent' ? fmt.ts(r.timestamp) : fmt.money4(r.estimated_cost_usd, store.currency, store.exchangeRate) }}
            </td>
            <td style="color:var(--muted)">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="4"/><path d="M4 20c0-4 3.6-7 8-7s8 3 8 7"/></svg>
            </td>
            <td>{{ fmt.short(r.prompt_text, 110) }}</td>
            <td><span :class="'badge ' + fmt.modelClass(r.model)">{{ fmt.modelShort(r.model) }}</span></td>
            <td class="num">{{ fmt.int(r.billable_tokens) }}</td>
            <td class="num">{{ fmt.int(r.cache_read_tokens) }}</td>
            <td>
              <RouterLink :to="'/sessions/' + encodeURIComponent(r.session_id)" class="mono" @click.stop>
                {{ r.session_id.slice(0, SESSION_ID_PREFIX) }}…
              </RouterLink>
            </td>
          </tr>
          <tr v-if="!rows.length">
            <td colspan="7" class="muted">no prompts yet</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Detail modal -->
    <div v-if="modalRow" class="modal-overlay" @click.self="modalRow = null">
      <div class="modal" style="max-width:760px;width:90vw;max-height:80vh;display:flex;flex-direction:column">
        <div style="display:flex;align-items:center;margin-bottom:12px;flex-shrink:0">
          <strong style="font-size:14px">Prompt detail</strong>
          <span class="spacer"></span>
          <span :class="'badge ' + fmt.modelClass(modalRow.model)">{{ fmt.modelShort(modalRow.model) }}</span>
        </div>
        <div style="position:relative;flex:1;overflow:hidden;display:flex;flex-direction:column">
          <pre style="font-family:var(--mono);white-space:pre-wrap;word-break:break-word;background:var(--bg);padding:12px;padding-bottom:36px;border-radius:6px;border:1px solid var(--border);font-size:12px;line-height:1.5;overflow-y:auto;flex:1;margin:0">{{ fmt.htmlSafe(prettyPrompt(modalRow.prompt_text || '')) }}</pre>
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
          <span class="muted">{{ fmt.int(modalRow.billable_tokens) }} billable · {{ fmt.int(modalRow.cache_read_tokens) }} cache rd · ~{{ fmt.money4(modalRow.estimated_cost_usd, store.currency, store.exchangeRate) }} cache cost</span>
          <span class="spacer"></span>
          <RouterLink :to="'/sessions/' + encodeURIComponent(modalRow.session_id)" @click="modalRow = null">Open session →</RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.btn-copy {
  position: absolute; bottom: 8px; right: 8px;
  background: transparent; border: 1px solid var(--border); border-radius: 4px;
  padding: 5px 7px; cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center; line-height: 1;
  transition: color 120ms, border-color 120ms;
}
.btn-copy--flash { border-color: var(--good); color: var(--good); }
</style>
