<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { encode, decode } from 'gpt-tokenizer'
import { useAppStore } from '../stores/app'
import { fmt } from '../lib/fmt'
import type { ModelRate } from '../composables/useWails'

const store = useAppStore()
const text = ref('')
const models = ref<ModelRate[]>([])

onMounted(async () => {
  try {
    models.value = (await window.go.app.App.GetPricingModels()) as ModelRate[]
  } catch { /* not in Wails env */ }
})

const tokenIds = computed(() => text.value ? encode(text.value) : [])

const stats = computed(() => {
  const t = text.value
  const wordMatches = t.replace(/['";:,.?¿\-!¡]+/g, '').match(/\S+/g)
  return {
    tokenCount: tokenIds.value.length,
    wordCount: wordMatches ? wordMatches.length : 0,
    charsNoSpaces: t.replace(/\s/g, '').length,
    charsTotal: t.length,
  }
})

// Colorized token visualization (capped for performance)
const VIZ_LIMIT = 800
const tokenSpans = computed(() =>
  tokenIds.value.slice(0, VIZ_LIMIT).map((id, i) => ({
    text: decode([id]),
    cls: `tok-${i % 6}`,
  }))
)

const vizTruncated = computed(() => tokenIds.value.length > VIZ_LIMIT)

function fmtNum(n: number): string {
  return n.toLocaleString()
}

const TOKENS_PER_MILLION = TOKENS_PER_MILLION
const COST_DECIMALS = 4

function fmtCurrency(amount: number, decimals = COST_DECIMALS): string {
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: store.currency || 'USD',
    minimumFractionDigits: decimals,
    maximumFractionDigits: Math.max(decimals, 2),
  }).format(amount)
}

function fmtMoney(usd: number, decimals = COST_DECIMALS): string {
  return fmtCurrency(usd * store.exchangeRate, decimals)
}

function roundTo(n: number, decimals: number): number {
  const f = 10 ** decimals
  return Math.round(n * f) / f
}

function inputCostUSD(m: ModelRate): number {
  return (stats.value.tokenCount / TOKENS_PER_MILLION) * m.input
}

function outputCostUSD(m: ModelRate): number {
  return (stats.value.tokenCount / TOKENS_PER_MILLION) * m.output
}

// Sum the two values after rounding to display precision so total always
// matches what the user would get by adding the two visible numbers.
function totalCost(m: ModelRate): string {
  const inputRounded  = roundTo(inputCostUSD(m)  * store.exchangeRate, COST_DECIMALS)
  const outputRounded = roundTo(outputCostUSD(m) * store.exchangeRate, COST_DECIMALS)
  return fmtCurrency(inputRounded + outputRounded)
}

const sortedModels = computed(() =>
  [...models.value]
    .filter(m => m.input > 0 || m.output > 0)
    .sort((a, b) => a.model_name.localeCompare(b.model_name))
)
</script>

<template>
  <div class="calc-page">
    <div class="calc-inner">
      <h2 class="page-title">Token Calculator</h2>

      <textarea
        v-model="text"
        class="calc-input"
        placeholder="Paste or type your text here to count tokens…"
        spellcheck="false"
      />

      <div class="stats-row">
        <div class="stat-card accent">
          <span class="stat-value">{{ fmtNum(stats.tokenCount) }}</span>
          <span class="stat-label">tokens</span>
        </div>
        <div class="stat-card">
          <span class="stat-value">{{ fmtNum(stats.wordCount) }}</span>
          <span class="stat-label">words</span>
        </div>
        <div class="stat-card">
          <span class="stat-value">{{ fmtNum(stats.charsNoSpaces) }}</span>
          <span class="stat-label">chars (no spaces)</span>
        </div>
        <div class="stat-card">
          <span class="stat-value">{{ fmtNum(stats.charsTotal) }}</span>
          <span class="stat-label">total chars</span>
        </div>
      </div>

      <div v-if="text" class="viz-block">
        <div class="viz-header">
          <span class="section-label">Token visualization</span>
          <span v-if="vizTruncated" class="muted" style="font-size:11px">showing first {{ VIZ_LIMIT }} of {{ fmtNum(tokenIds.length) }}</span>
        </div>
        <div class="viz-tokens">
          <span
            v-for="(tok, i) in tokenSpans"
            :key="i"
            :class="['tok', tok.cls]"
          >{{ tok.text }}</span>
        </div>
      </div>

      <div v-if="sortedModels.length" class="pricing-block">
        <div class="section-label" style="margin-bottom:10px">Pricing by model</div>
        <div class="pricing-note muted">
          Rates per 1M tokens · estimated input cost for {{ fmtNum(stats.tokenCount) }} tokens
        </div>
        <table class="pricing-table">
          <thead>
            <tr>
              <th>model</th>
              <th class="num">input / 1M</th>
              <th class="num">output / 1M</th>
              <th class="num">input cost</th>
              <th class="num">output cost</th>
              <th class="num total-col">total cost</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="m in sortedModels" :key="m.model_name">
              <td><span :class="'badge ' + fmt.modelClass(m.model_name)">{{ fmt.modelShort(m.model_name) }}</span></td>
              <td class="num mono">{{ fmtMoney(m.input, 2) }}</td>
              <td class="num mono">{{ fmtMoney(m.output, 2) }}</td>
              <td class="num mono" :class="{ dim: stats.tokenCount === 0 }">
                {{ stats.tokenCount ? fmtMoney(inputCostUSD(m)) : '—' }}
              </td>
              <td class="num mono" :class="{ dim: stats.tokenCount === 0 }">
                {{ stats.tokenCount ? fmtMoney(outputCostUSD(m)) : '—' }}
              </td>
              <td class="num mono total-col" :class="{ dim: stats.tokenCount === 0 }">
                {{ stats.tokenCount ? totalCost(m) : '—' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.calc-page { padding: 20px; overflow-y: auto; height: calc(100vh - 48px); background: var(--bg); }
.calc-inner { max-width: 960px; margin: 0 auto; }
.page-title { margin: 0 0 16px; font-size: 16px; letter-spacing: -0.01em; }

.calc-input {
  width: 100%;
  min-height: 180px;
  box-sizing: border-box;
  font-family: var(--mono);
  font-size: 13px;
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--panel);
  color: var(--text);
  resize: vertical;
  outline: none;
  line-height: 1.5;
}
.calc-input:focus { border-color: var(--accent); }

.stats-row { display: flex; gap: 12px; margin-top: 14px; flex-wrap: wrap; }
.stat-card {
  flex: 1; min-width: 100px;
  padding: 12px 16px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--panel);
  display: flex; flex-direction: column; gap: 4px;
}
.stat-card.accent { border-color: var(--accent); }
.stat-value { font-size: 22px; font-weight: 600; font-family: var(--mono); }
.stat-label { font-size: 11px; color: var(--muted); text-transform: uppercase; letter-spacing: 0.05em; }
.stat-card.accent .stat-value { color: var(--accent); }

.section-label { font-size: 12px; font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: 0.05em; }

.viz-block { margin-top: 20px; }
.viz-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.viz-tokens {
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--panel);
  font-family: var(--mono);
  font-size: 13px;
  line-height: 1.8;
  max-height: 220px;
  overflow-y: auto;
  word-break: break-all;
}
.tok { border-radius: 2px; padding: 1px 0; }
.tok-0 { background: rgba(255,  99,  71, 0.15); }
.tok-1 { background: rgba(255, 165,   0, 0.15); }
.tok-2 { background: rgba( 50, 205,  50, 0.15); }
.tok-3 { background: rgba( 30, 144, 255, 0.15); }
.tok-4 { background: rgba(147, 112, 219, 0.15); }
.tok-5 { background: rgba( 32, 178, 170, 0.15); }

.pricing-block { margin-top: 24px; }
.pricing-note { font-size: 11px; margin-bottom: 10px; font-family: var(--mono); }

.pricing-table { width: 100%; border-collapse: collapse; font-size: 12px; }
.pricing-table th { text-align: left; padding: 6px 10px; color: var(--muted); font-weight: 500; border-bottom: 1px solid var(--border); font-size: 11px; text-transform: uppercase; letter-spacing: 0.04em; }
.pricing-table th.num, .pricing-table td.num { text-align: right; }
.pricing-table td { padding: 7px 10px; border-bottom: 1px solid var(--border); }
.pricing-table tr:last-child td { border-bottom: none; }
.pricing-table tr:hover td { background: var(--panel); }

.total-col { color: #e07b39; font-weight: 500; }
.mono { font-family: var(--mono); }
.muted { color: var(--muted); }
.dim { color: var(--muted); }
</style>
