<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { encode, decode } from 'gpt-tokenizer'
import { useAppStore } from '../stores/app'
import { fmt } from '../lib/fmt'
import type { ModelRate } from '../composables/useWails'

const store = useAppStore()
const text = ref('')
const models = ref<ModelRate[]>([])

const ctxMenu = ref({ visible: false, x: 0, y: 0 })
const isDragging = ref(false)
const fileError = ref('')

onMounted(async () => {
  try {
    models.value = await window.go.app.App.GetPricingModels()
  } catch { /* not in Wails env */ }
  document.addEventListener('mousedown', hideCtxMenu)
})

onUnmounted(() => {
  document.removeEventListener('mousedown', hideCtxMenu)
})

function showCtxMenu(e: MouseEvent) {
  ctxMenu.value = { visible: true, x: e.clientX, y: e.clientY }
}

function hideCtxMenu() {
  ctxMenu.value.visible = false
}

async function pasteFromClipboard() {
  hideCtxMenu()
  try {
    const clip = await navigator.clipboard.readText()
    text.value += clip
  } catch { /* clipboard access denied */ }
}

function onDragOver() { isDragging.value = true }
function onDragLeave() { isDragging.value = false }

async function onDrop(e: DragEvent) {
  isDragging.value = false
  const file = e.dataTransfer?.files[0]
  if (file) await loadFile(file)
}

async function onFileSelect(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (file) await loadFile(file)
  ;(e.target as HTMLInputElement).value = ''
}

function looksLikeBinary(content: string): boolean {
  const sample = content.slice(0, 8000)
  let bad = 0
  for (let i = 0; i < sample.length; i++) {
    const c = sample.charCodeAt(i)
    if (c === 0 || (c < 32 && c !== 9 && c !== 10 && c !== 13)) bad++
  }
  return sample.length > 0 && bad / sample.length > 0.02
}

function loadFile(file: File): Promise<void> {
  fileError.value = ''
  return new Promise((resolve) => {
    const reader = new FileReader()
    reader.onload = (e) => {
      const content = e.target?.result as string
      if (looksLikeBinary(content)) {
        fileError.value = `"${file.name}" looks like a binary file — only text files are supported.`
      } else {
        text.value = content
      }
      resolve()
    }
    reader.onerror = () => {
      fileError.value = `Could not read "${file.name}".`
      resolve()
    }
    reader.readAsText(file)
  })
}

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

const TOKENS_PER_MILLION = 1_000_000
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

      <div
        class="input-wrapper"
        :class="{ 'drop-active': isDragging }"
        @dragover.prevent="onDragOver"
        @dragleave="onDragLeave"
        @drop.prevent="onDrop"
      >
        <textarea
          v-model="text"
          class="calc-input"
          placeholder="Paste or type your text here to count tokens…"
          spellcheck="false"
          @contextmenu.prevent="showCtxMenu"
        />
        <div v-if="isDragging" class="drop-overlay">Drop file to load text</div>
      </div>

      <div class="input-actions">
        <label class="btn-upload">
          <input type="file" style="display:none" @change="onFileSelect" />
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
          Upload file
        </label>
        <span v-if="fileError" class="file-error">{{ fileError }}</span>
      </div>

      <div
        v-if="ctxMenu.visible"
        class="ctx-menu"
        :style="{ top: ctxMenu.y + 'px', left: ctxMenu.x + 'px' }"
        @mousedown.stop
      >
        <button class="ctx-item" @mousedown.prevent="pasteFromClipboard">Paste</button>
      </div>

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

.input-wrapper { position: relative; }
.input-wrapper.drop-active .calc-input { border-color: var(--accent); opacity: 0.5; }
.drop-overlay {
  position: absolute; inset: 0;
  display: flex; align-items: center; justify-content: center;
  font-size: 14px; font-weight: 600; color: var(--accent);
  background: color-mix(in srgb, var(--accent) 8%, transparent);
  border: 2px dashed var(--accent);
  border-radius: 6px;
  pointer-events: none;
}

.input-actions {
  display: flex; align-items: center; gap: 10px; margin-top: 8px;
}
.btn-upload {
  display: inline-flex; align-items: center; gap: 5px;
  font-size: 12px; color: var(--muted); cursor: pointer;
  padding: 4px 10px;
  border: 1px solid var(--border); border-radius: 5px;
  background: transparent; font-family: inherit;
  transition: color 120ms, border-color 120ms;
  user-select: none;
}
.btn-upload:hover { color: var(--text); border-color: var(--text); }
.file-error { font-size: 12px; color: var(--warn, #e07b39); font-family: var(--mono); }

.ctx-menu {
  position: fixed; z-index: 9999;
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: 5px;
  padding: 3px 0;
  box-shadow: 0 4px 12px rgba(0,0,0,0.3);
  min-width: 100px;
}
.ctx-item {
  display: block; width: 100%;
  padding: 6px 14px; text-align: left;
  background: transparent; border: none;
  font-size: 13px; color: var(--text); font-family: inherit;
  cursor: pointer;
}
.ctx-item:hover { background: color-mix(in srgb, var(--accent) 12%, transparent); }

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
