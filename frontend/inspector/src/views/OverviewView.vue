<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { api, withSince, sinceIso } from '../lib/api'
import { stackedBarChart, donutChart, groupedBarChart, barChart } from '../lib/charts'
import { fmt } from '../lib/fmt'
import { useRange } from '../composables/useRange'
import { useAppStore } from '../stores/app'
import { RANGES } from '../lib/api'

const store = useAppStore()
const { range, rangeKey, setRange } = useRange()

const totals  = ref<Record<string, number>>({})
const projects = ref<Array<Record<string, unknown>>>([])
const sessions = ref<Array<Record<string, unknown>>>([])
const tools    = ref<Array<Record<string, unknown>>>([])
const daily    = ref<Array<Record<string, number>>>([])
const byModel  = ref<Array<Record<string, number>>>([])

const chDailyBillable = ref<HTMLElement | null>(null)
const chDailyCache    = ref<HTMLElement | null>(null)
const chModel         = ref<HTMLElement | null>(null)
const chProjects      = ref<HTMLElement | null>(null)
const chTools         = ref<HTMLElement | null>(null)

const TOP_CHART_LIMIT = 8

async function fetchAll() {
  const since = sinceIso(range.value)
  const [t, p, s, tl, d, bm] = await Promise.all([
    api(withSince('/api/overview', since)),
    api(withSince('/api/projects', since)),
    api(withSince('/api/sessions?limit=10', since)),
    api(withSince('/api/tools', since)),
    api(withSince('/api/daily', since)),
    api(withSince('/api/by-model', since)),
  ])
  totals.value   = t as Record<string, number>
  projects.value = p as Array<Record<string, unknown>>
  sessions.value = s as Array<Record<string, unknown>>
  tools.value    = tl as Array<Record<string, unknown>>
  daily.value    = d as Array<Record<string, number>>
  byModel.value  = bm as Array<Record<string, number>>
  await nextTick()
  renderCharts()
}

function renderCharts() {
  if (chDailyBillable.value) {
    stackedBarChart(chDailyBillable.value, {
      categories: daily.value.map(d => d.day as unknown as string),
      series: [
        { name: 'input',        values: daily.value.map(d => d.input_tokens || 0),        color: '#eb733b' },
        { name: 'output',       values: daily.value.map(d => d.output_tokens || 0),       color: '#b04e20' },
        { name: 'cache create', values: daily.value.map(d => d.cache_create_tokens || 0), color: '#b07800' },
      ],
    })
  }
  if (chDailyCache.value) {
    stackedBarChart(chDailyCache.value, {
      categories: daily.value.map(d => d.day as unknown as string),
      series: [
        { name: 'cache read', values: daily.value.map(d => d.cache_read_tokens || 0), color: '#2d8a5e' },
      ],
    })
  }
  if (chModel.value) {
    donutChart(chModel.value,
      byModel.value
        .map(m => ({
          name: fmt.modelShort(m.model as string) || 'unknown',
          value: (m.input_tokens || 0) + (m.output_tokens || 0)
               + (m.cache_create_5m_tokens || 0) + (m.cache_create_1h_tokens || 0),
        }))
        .filter(d => d.value > 0)
    )
  }
  const topProjects = projects.value.slice(0, TOP_CHART_LIMIT)
  if (chProjects.value && topProjects.length) {
    groupedBarChart(chProjects.value, {
      categories: topProjects.map(p => {
        const name = (p.project_name || p.project_slug) as string
        return name.length > 20 ? name.slice(0, 19) + '…' : name
      }),
      series: [
        { name: 'input',  values: topProjects.map(p => (p.input_tokens  as number) || 0), color: '#eb733b' },
        { name: 'output', values: topProjects.map(p => (p.output_tokens as number) || 0), color: '#4ab0c0' },
      ],
    })
  }
  const topTools = tools.value.slice(0, TOP_CHART_LIMIT)
  if (chTools.value && topTools.length) {
    barChart(chTools.value, {
      categories: topTools.map(t => t.tool_name as string),
      values: topTools.map(t => t.calls as number),
      color: '#b04e20',
    })
  }
}

const cacheCreate = computed(() =>
  (totals.value.cache_create_5m_tokens || 0) + (totals.value.cache_create_1h_tokens || 0)
)

const planEntry = computed(() => store.pricing?.plans?.[store.plan])

onMounted(fetchAll)
watch([rangeKey, () => store.lastScan], fetchAll)
</script>

<template>
  <div style="padding:20px">
    <div class="flex" style="margin-bottom:14px">
      <h2 style="margin:0;font-size:16px;letter-spacing:-0.01em">Overview</h2>
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

    <div style="display:flex;gap:16px;align-items:stretch">
      <div style="display:flex;align-items:center;justify-content:center;flex-shrink:0;width:90px">
        <img :src="'/web/mascot.png'" alt="" style="width:100%;height:100%;object-fit:contain;display:block">
      </div>
      <div class="kpi-row" style="flex:1">
        <div class="card kpi" data-tooltip="One run of Claude Code (from claude to exit). Each session is a single .jsonl file.">
          <div class="label">Sessions</div>
          <div class="value" :title="fmt.int(totals.sessions)">{{ fmt.int(totals.sessions) }}</div>
        </div>
        <div class="card kpi" data-tooltip="One message you sent to Claude. Each turn triggers a response (possibly with tool calls in between).">
          <div class="label">Turns</div>
          <div class="value" :title="fmt.int(totals.turns)">{{ fmt.int(totals.turns) }}</div>
        </div>
        <div class="card kpi" data-tooltip="The new text you (and tool results) sent to Claude this turn. Billed at the full input rate.">
          <div class="label">Input</div>
          <div class="value" :title="fmt.int(totals.input_tokens) + ' tokens'">{{ fmt.compact(totals.input_tokens) }}</div>
        </div>
        <div class="card kpi" data-tooltip="The text Claude wrote back. Billed at the highest rate — usually the biggest cost driver per turn.">
          <div class="label">Output</div>
          <div class="value" :title="fmt.int(totals.output_tokens) + ' tokens'">{{ fmt.compact(totals.output_tokens) }}</div>
        </div>
        <div class="card kpi" data-tooltip="Tokens Claude re-used from a cache (your CLAUDE.md, previously-read files, the conversation so far). ~10× cheaper than fresh input. High counts = good cost hygiene.">
          <div class="label">Cache read</div>
          <div class="value" :title="fmt.int(totals.cache_read_tokens) + ' tokens'">{{ fmt.compact(totals.cache_read_tokens) }}</div>
        </div>
        <div class="card kpi" data-tooltip="Writing something into the cache for the first time. One-time cost; pays off on the next turn.">
          <div class="label">Cache create</div>
          <div class="value" :title="fmt.int(cacheCreate) + ' tokens'">{{ fmt.compact(cacheCreate) }}</div>
        </div>
        <!-- Cost KPI -->
        <div class="card kpi cost" data-tooltip="Estimated spend based on token counts and current API pricing. Subscription plan cost shown as a flat monthly fee with token-equivalent below.">
          <div class="label">Est. cost</div>
          <template v-if="planEntry && planEntry.monthly > 0">
            <div class="value" :title="planEntry.label">
              {{ fmt.money(planEntry.monthly, store.currency, store.exchangeRate) }}<span style="font-size:11px;opacity:0.6">/mo</span>
            </div>
            <div class="sub">{{ fmt.money(totals.cost_usd, store.currency, store.exchangeRate) }} token equiv</div>
          </template>
          <template v-else>
            <div class="value" :title="fmt.money(totals.cost_usd, store.currency, store.exchangeRate)">
              {{ fmt.money(totals.cost_usd, store.currency, store.exchangeRate) }}
            </div>
          </template>
        </div>
      </div>
    </div>

    <div class="row cols-2" style="margin-top:16px">
      <div class="card">
        <h3>Your daily work</h3>
        <p class="muted" style="margin:-4px 0 10px;font-size:12px">Tokens you paid for: what you sent (<b>input</b>), what Claude wrote (<b>output</b>), and what got stored for re-use (<b>cache create</b>).</p>
        <div ref="chDailyBillable" style="height:260px"></div>
      </div>
      <div class="card">
        <h3>Daily cache reads</h3>
        <p class="muted" style="margin:-4px 0 10px;font-size:12px"><b>Cache reads</b> are cheap re-uses of things Claude already saw (like your CLAUDE.md). They cost ~10× less than regular input tokens — high numbers here are a good thing.</p>
        <div ref="chDailyCache" style="height:260px"></div>
      </div>
    </div>

    <div class="row cols-2" style="margin-top:16px">
      <div class="card"><h3>Tokens by project</h3><div ref="chProjects" style="height:320px"></div></div>
      <div class="card">
        <h3>Token usage by model</h3>
        <p class="muted" style="margin:-4px 0 4px;font-size:12px">Share of billable tokens per Claude model.</p>
        <div ref="chModel" style="height:300px"></div>
      </div>
    </div>

    <div class="row cols-2" style="margin-top:16px">
      <div class="card"><h3>Top tools (by call count)</h3><div ref="chTools" style="height:320px"></div></div>
      <div class="card">
        <h3 style="display:flex;align-items:center">
          <span>Recent sessions</span><span class="spacer"></span>
          <a href="#/sessions" style="font-weight:400;font-size:12px">all →</a>
        </h3>
        <table>
          <thead><tr><th>started</th><th>project</th><th class="num">tokens</th></tr></thead>
          <tbody>
            <tr v-for="s in sessions" :key="(s.session_id as string)">
              <td class="mono">{{ fmt.ts(s.started as string) }}</td>
              <td><a :href="'#/sessions/' + encodeURIComponent(s.session_id as string)">{{ fmt.htmlSafe((s.project_name || s.project_slug) as string) }}</a></td>
              <td class="num">{{ fmt.compact(s.tokens as number) }}</td>
            </tr>
            <tr v-if="!sessions.length"><td colspan="3" class="muted">no sessions in this range</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
