<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { api, withSince, sinceIso, RANGES } from '../lib/api'
import { fmt } from '../lib/fmt'
import { KIND_LABELS, sevClass } from '../lib/findings'
import { useRange } from '../composables/useRange'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const { range, rangeKey, setRange } = useRange()

interface KindRow { kind: string; est_tokens: number; occurrences: number; sessions: number; sev_rank: number; est_cost_usd: number }
interface LowRow { session_id: string; project_slug: string; score: number; grade: string }

const kinds = ref<KindRow[]>([])
const lowest = ref<LowRow[]>([])
const totalSessionsCount = ref<number>(0)

const totalTokens   = computed(() => kinds.value.reduce((s, k) => s + (k.est_tokens || 0), 0))
const totalCost     = computed(() => kinds.value.reduce((s, k) => s + (k.est_cost_usd || 0), 0))
const totalSessions = computed(() => totalSessionsCount.value)

async function fetchAll() {
  const since = sinceIso(range.value)
  kinds.value  = (await api<KindRow[]>(withSince('/api/findings', since)))        ?? []
  lowest.value = (await api<LowRow[]>(withSince('/api/findings/lowest', since)))  ?? []
  totalSessionsCount.value = (await api<number>(withSince('/api/findings/total-sessions', since))) ?? 0
}

onMounted(fetchAll)
watch([rangeKey, () => store.lastScan], fetchAll)
</script>

<template>
  <div style="padding:20px">
    <div class="flex" style="margin-bottom:14px">
      <h2 style="margin:0;font-size:16px;letter-spacing:-0.01em">Findings</h2>
      <span class="muted" style="font-size:12px">{{ range.days ? `last ${range.days} days` : 'all time' }}</span>
      <div class="spacer"></div>
      <div class="range-tabs" role="tablist">
        <button
          v-for="r in RANGES"
          :key="r.key"
          :data-range="r.key"
          :class="{ active: r.key === range.key }"
          @click="setRange(r.key)"
        >{{ r.label }}</button>
      </div>
    </div>

    <div class="card banner" v-if="kinds.length">
      <span class="big">~{{ fmt.tok(totalTokens) }}</span>
      <span class="sub">est. recoverable ·
        <b class="money">{{ fmt.money(totalCost, store.currency, store.exchangeRate) }}</b>
        across {{ totalSessions }} session{{ totalSessions === 1 ? '' : 's' }}</span>
    </div>

    <div class="card" style="margin-top:16px">
      <h3>Findings by kind</h3>
      <p v-if="!kinds.length" class="muted">No findings in this range. Clean sessions, or not enough activity yet.</p>
      <div v-for="k in kinds" :key="k.kind" class="finding-card">
        <div class="bar" :class="sevClass(k.sev_rank)"></div>
        <div class="body">
          <div class="name">{{ KIND_LABELS[k.kind] ?? k.kind }}</div>
          <div class="meta">{{ k.sessions }} session{{ k.sessions === 1 ? '' : 's' }} · {{ k.occurrences }} occurrence{{ k.occurrences === 1 ? '' : 's' }}</div>
        </div>
        <div class="amt">
          <div class="tok">~{{ fmt.tok(k.est_tokens) }}</div>
          <div class="usd">{{ fmt.money(k.est_cost_usd, store.currency, store.exchangeRate) }}</div>
        </div>
      </div>
    </div>

    <div class="card" v-if="lowest.length" style="margin-top:16px">
      <h3>Lowest-scoring sessions</h3>
      <table>
        <thead>
          <tr><th>Project</th><th>Session</th><th class="num">Grade</th><th class="num">Score</th></tr>
        </thead>
        <tbody>
          <tr v-for="s in lowest" :key="s.session_id">
            <td>{{ s.project_slug }}</td>
            <td><RouterLink :to="`/sessions/${encodeURIComponent(s.session_id)}`">{{ s.session_id.slice(0, 8) }}</RouterLink></td>
            <td class="num"><span class="grade" :class="`grade-${s.grade}`">{{ s.grade }}</span></td>
            <td class="num">{{ s.score }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.banner { display:flex; align-items:baseline; gap:14px; }
.banner .big { font-size:26px; font-weight:700; color:var(--accent); }
.banner .sub { color:var(--muted); font-size:13px; }
.banner .money { color:#7fd49a; }
.finding-card { display:flex; align-items:center; gap:12px; padding:10px 0; border-bottom:1px solid var(--border); }
.finding-card:last-child { border-bottom:none; }
.finding-card .bar { width:4px; align-self:stretch; min-height:32px; border-radius:3px; }
.sev-high { background:#e5534b; }
.sev-med  { background:#e0a23a; }
.sev-low  { background:#4a90d9; }
.finding-card .name { font-weight:600; }
.finding-card .meta { color:var(--muted); font-size:12px; }
.finding-card .amt { margin-left:auto; text-align:right; }
.finding-card .amt .tok { font-weight:700; color:var(--accent); }
.finding-card .amt .usd { color:#7fd49a; font-size:12px; }
.grade-A { color:#7fd49a; }
.grade-B { color:#7fd49a; }
.grade-C { color:#e0a23a; }
.grade-D { color:#e0a23a; }
.grade-F { color:#e5534b; }
</style>
