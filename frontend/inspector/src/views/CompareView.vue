<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { api, withSince, sinceIso, RANGES } from '../lib/api'
import { fmt } from '../lib/fmt'
import { useRange } from '../composables/useRange'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const { range, rangeKey, setRange } = useRange()

interface ModelRow {
  model: string
  turns: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_create_tokens: number
  avg_output_per_turn: number
  cache_hit_rate: number
  cost_usd?: number
  cost_per_turn?: number
}

const rows = ref<ModelRow[]>([])

async function fetchAll() {
  const since = sinceIso(range.value)
  rows.value = await api<ModelRow[]>(withSince('/api/compare', since)) ?? []
}

onMounted(fetchAll)
watch([rangeKey, () => store.lastScan], fetchAll)
</script>

<template>
  <div style="padding:20px">
    <div class="flex" style="margin-bottom:14px">
      <h2 style="margin:0;font-size:16px;letter-spacing:-0.01em">Model Comparison</h2>
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

    <div v-if="!rows.length" class="card">
      <p class="muted">No data for this period. Model comparison requires at least one assistant turn.</p>
    </div>

    <div v-else class="card">
      <table>
        <thead>
          <tr>
            <th>Model</th>
            <th class="num">Turns</th>
            <th class="num">Cache hit %</th>
            <th class="num">Avg output/turn</th>
            <th class="num">Cost/turn</th>
            <th class="num">Total cost</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="r.model">
            <td class="mono">{{ fmt.modelShort(r.model) }}</td>
            <td class="num">{{ fmt.int(r.turns) }}</td>
            <td class="num">{{ (r.cache_hit_rate * 100).toFixed(1) }}%</td>
            <td class="num">{{ fmt.compact(r.avg_output_per_turn) }}</td>
            <td class="num">{{ r.cost_per_turn != null ? fmt.money(r.cost_per_turn, store.currency, store.exchangeRate) : '–' }}</td>
            <td class="num">{{ r.cost_usd != null ? fmt.money(r.cost_usd, store.currency, store.exchangeRate) : '–' }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="card" style="margin-top:16px">
      <h3>What to look for</h3>
      <table>
        <thead><tr><th>Signal</th><th>What it might mean</th></tr></thead>
        <tbody>
          <tr><td>Cache hit &lt; 60%</td><td>Context is unstable and costly to re-send</td></tr>
          <tr><td>High avg output/turn</td><td>Model is verbose; consider tighter response constraints</td></tr>
          <tr><td>High cost/turn vs peers</td><td>Expensive model may be overused for simple tasks</td></tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
