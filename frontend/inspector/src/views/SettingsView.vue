<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { fmt } from '../lib/fmt'
import { useAppStore } from '../stores/app'
import type { ModelRate, PlanEntry } from '../composables/useWails'

const store = useAppStore()

const CURRENCIES = [
  { code: 'CAD', label: 'CAD — Canadian Dollar' },
  { code: 'USD', label: 'USD — US Dollar' },
  { code: 'EUR', label: 'EUR — Euro' },
  { code: 'GBP', label: 'GBP — British Pound' },
  { code: 'AUD', label: 'AUD — Australian Dollar' },
  { code: 'NZD', label: 'NZD — New Zealand Dollar' },
  { code: 'CHF', label: 'CHF — Swiss Franc' },
  { code: 'JPY', label: 'JPY — Japanese Yen' },
  { code: 'MXN', label: 'MXN — Mexican Peso' },
  { code: 'BRL', label: 'BRL — Brazilian Real' },
]

// General
const currentPlan = ref('api')
const plans = ref<PlanEntry[]>([])
const planMsg = ref('')
const selectedPlan = ref('api')

// Currency
const currency = ref('CAD')
const exchangeRate = ref(1.0)
const rates = ref<Record<string, number>>({})
const currencyMsg = ref('')

// Exchange Rate API
const apiKey = ref('')
const ratesMsg = ref('')

// Models
const models = ref<ModelRate[]>([])

// Plans table
const plansMsg = ref('')

// Data management
const retentionDays = ref(0)
const retentionMsg = ref('')
const purgeMsg = ref('')
const scanMsg = ref('')

// Service
const serviceStatus = ref('Checking…')
const SERVICE_STATUS_DELAY = 1500

// Modals
interface ModelModalState {
  show: boolean; title: string; isEdit: boolean
  model_name: string; tier: string; input: number; output: number
  cache_read: number; cache_create_5m: number; cache_create_1h: number
}
interface PlanModalState {
  show: boolean; title: string; isEdit: boolean
  plan_key: string; label: string; monthly: number
}

const modelModal = ref<ModelModalState>({
  show: false, title: 'Add model', isEdit: false,
  model_name: '', tier: '', input: 0, output: 0, cache_read: 0, cache_create_5m: 0, cache_create_1h: 0,
})
const planModal = ref<PlanModalState>({
  show: false, title: 'Add plan', isEdit: false,
  plan_key: '', label: '', monthly: 0,
})

function flash(msgRef: { value: string }, text: string, color = 'var(--good)', duration = 2500) {
  msgRef.value = text
  setTimeout(() => { msgRef.value = '' }, duration)
}

async function loadAll() {
  const [planResp, m, p, r, key, days] = await Promise.all([
    window.go.app.App.GetPlan(),
    window.go.app.App.GetPricingModels(),
    window.go.app.App.GetPricingPlans(),
    window.go.app.App.GetExchangeRates(),
    window.go.app.App.GetExchangeApiKey(),
    window.go.app.App.GetRetentionDays(),
  ])
  currentPlan.value = planResp.plan || 'api'
  selectedPlan.value = planResp.plan || 'api'
  currency.value = planResp.currency || 'CAD'
  plans.value = (p as PlanEntry[]).sort((a, b) => a.label.localeCompare(b.label))
  models.value = m as ModelRate[]
  rates.value = r
  apiKey.value = key || ''
  retentionDays.value = days > 0 ? days : 0
  exchangeRate.value = r[planResp.currency || 'CAD'] || 1.0
  refreshServiceStatus()
}

async function savePlan() {
  await window.go.app.App.SetPlan(selectedPlan.value)
  store.plan = selectedPlan.value
  currentPlan.value = selectedPlan.value
  const pill = document.getElementById('plan-pill')
  if (pill) pill.textContent = selectedPlan.value
  flash(planMsg, 'Saved.')
}

function onCurrencyChange() {
  exchangeRate.value = +(rates.value[currency.value] || 1.0).toFixed(4)
}

async function saveCurrency() {
  await window.go.app.App.SetCurrency(currency.value)
  await window.go.app.App.SetExchangeRate(currency.value, exchangeRate.value)
  store.currency = currency.value
  store.exchangeRate = exchangeRate.value
  flash(currencyMsg, 'Saved.')
}

async function saveApiKey() {
  await window.go.app.App.SetExchangeApiKey(apiKey.value.trim())
  flash(ratesMsg, 'API key saved.')
}

async function refreshRates() {
  ratesMsg.value = 'Fetching live rates…'
  try {
    if (apiKey.value.trim()) await window.go.app.App.SetExchangeApiKey(apiKey.value.trim())
    const updated = await window.go.app.App.RefreshExchangeRates()
    Object.assign(rates.value, updated)
    if (updated[currency.value] != null) {
      exchangeRate.value = +updated[currency.value].toFixed(4)
    }
    flash(ratesMsg, 'Rates updated from exchangerate-api.com.')
  } catch (e: unknown) {
    ratesMsg.value = 'Error: ' + ((e as Error).message || String(e))
  }
}

function openAddModel() {
  modelModal.value = {
    show: true, title: 'Add model', isEdit: false,
    model_name: '', tier: '', input: 0, output: 0, cache_read: 0, cache_create_5m: 0, cache_create_1h: 0,
  }
}

function openEditModel(m: ModelRate) {
  modelModal.value = {
    show: true, title: 'Edit model', isEdit: true,
    model_name: m.model_name, tier: m.tier,
    input: m.input, output: m.output,
    cache_read: m.cache_read, cache_create_5m: m.cache_create_5m, cache_create_1h: m.cache_create_1h,
  }
}

async function saveModel() {
  const mm = modelModal.value
  if (!mm.model_name.trim()) return
  await window.go.app.App.UpsertPricingModel(
    mm.model_name.trim(), mm.tier.trim(),
    mm.input, mm.output, mm.cache_read, mm.cache_create_5m, mm.cache_create_1h,
  )
  modelModal.value.show = false
  models.value = (await window.go.app.App.GetPricingModels()) as ModelRate[]
}

async function deleteModel(name: string) {
  if (!confirm(`Delete model "${name}"?`)) return
  await window.go.app.App.DeletePricingModel(name)
  models.value = (await window.go.app.App.GetPricingModels()) as ModelRate[]
}

async function resetPricing() {
  if (!confirm('Reset all model rates and plans to the built-in defaults? This cannot be undone.')) return
  await window.go.app.App.ResetPricingToDefaults()
  await loadAll()
}

function openAddPlan() {
  planModal.value = { show: true, title: 'Add plan', isEdit: false, plan_key: '', label: '', monthly: 0 }
}

function openEditPlan(p: PlanEntry) {
  planModal.value = { show: true, title: 'Edit plan', isEdit: true, plan_key: p.plan_key, label: p.label, monthly: p.monthly }
}

async function savePlanEntry() {
  const pm = planModal.value
  if (!pm.plan_key.trim() || !pm.label.trim()) return
  await window.go.app.App.UpsertPricingPlan(pm.plan_key.trim(), pm.label.trim(), pm.monthly)
  planModal.value.show = false
  plans.value = ((await window.go.app.App.GetPricingPlans()) as PlanEntry[]).sort((a, b) => a.label.localeCompare(b.label))
}

async function deletePlan(key: string) {
  if (!confirm(`Delete plan "${key}"?`)) return
  await window.go.app.App.DeletePricingPlan(key)
  plans.value = ((await window.go.app.App.GetPricingPlans()) as PlanEntry[]).sort((a, b) => a.label.localeCompare(b.label))
}

async function refreshServiceStatus() {
  try {
    const status = await window.go.app.App.GetServiceStatus() as { installed: boolean; state: string }
    if (!status.installed) {
      serviceStatus.value = '● Not installed'
    } else {
      serviceStatus.value = `● ${status.state}`
    }
  } catch {
    serviceStatus.value = '● Status unavailable'
  }
}

async function installService() {
  await window.go.app.App.InstallService().catch(() => {})
  setTimeout(refreshServiceStatus, SERVICE_STATUS_DELAY)
}

async function uninstallService() {
  await window.go.app.App.UninstallService().catch(() => {})
  setTimeout(refreshServiceStatus, SERVICE_STATUS_DELAY)
}

async function scanNow() {
  scanMsg.value = 'Scanning…'
  try {
    const result = await window.go.app.App.ScanNow() as { Messages: number; Files: number }
    scanMsg.value = result.Messages > 0 || result.Files > 0
      ? `Scanned ${result.Messages} messages in ${result.Files} files`
      : 'Nothing new'
  } catch (e: unknown) {
    scanMsg.value = 'Error: ' + ((e as Error).message || String(e))
  }
  setTimeout(() => { scanMsg.value = '' }, 2500)
}

async function saveRetention() {
  try {
    await window.go.app.App.SetRetentionDays(retentionDays.value)
    flash(retentionMsg, retentionDays.value > 0
      ? `Saved — auto-purge every scan (>${retentionDays.value} days)`
      : 'Saved — retention off')
  } catch (e: unknown) {
    flash(retentionMsg, 'Error: ' + ((e as Error).message || String(e)))
  }
}

async function purgeNow() {
  if (retentionDays.value <= 0) return
  if (!confirm(`Delete all TokenTally data older than ${retentionDays.value} days? This cannot be undone.`)) return
  purgeMsg.value = 'Purging…'
  try {
    const deleted = await window.go.app.App.PurgeOlderThan(retentionDays.value)
    purgeMsg.value = deleted > 0 ? `Deleted ${deleted.toLocaleString()} messages` : 'Nothing to purge'
  } catch (e: unknown) {
    purgeMsg.value = 'Error: ' + ((e as Error).message || String(e))
  }
  setTimeout(() => { purgeMsg.value = '' }, 2500)
}

onMounted(loadAll)
</script>

<template>
  <div style="padding:20px">

    <!-- General: Plan + Currency -->
    <div class="card">
      <h2>Settings</h2>

      <h3 style="margin-top:16px">Plan</h3>
      <p class="muted" style="margin:0 0 12px">Sets how cost is displayed. API mode shows pay-per-token rates. Subscription modes show what you actually pay each month.</p>
      <div class="flex" style="gap:10px;align-items:center">
        <select v-model="selectedPlan">
          <option v-for="p in plans" :key="p.plan_key" :value="p.plan_key">
            {{ p.label }}{{ p.monthly ? ` — ${fmt.money(p.monthly, store.currency, store.exchangeRate)}/mo` : '' }}
          </option>
        </select>
        <button class="primary" @click="savePlan">Save</button>
        <span class="muted" style="font-size:12px">{{ planMsg }}</span>
      </div>

      <hr class="divider" style="margin:20px 0">

      <h3>Currency &amp; Exchange Rate</h3>
      <p class="muted" style="margin:0 0 12px">All pricing is stored in USD. Enter the exchange rate so costs display in your currency.</p>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;max-width:480px">
        <div>
          <label class="form-label">Currency</label>
          <select v-model="currency" style="width:100%" @change="onCurrencyChange">
            <option v-for="c in CURRENCIES" :key="c.code" :value="c.code">{{ c.label }}</option>
          </select>
        </div>
        <div>
          <label class="form-label">Exchange rate (1 USD = ?)</label>
          <input v-model.number="exchangeRate" type="number" step="0.0001" min="0" class="form-input">
        </div>
      </div>
      <div class="flex" style="gap:10px;margin-top:10px;align-items:center">
        <button class="primary" @click="saveCurrency">Save</button>
        <span class="muted" style="font-size:12px">{{ currencyMsg }}</span>
      </div>

      <hr class="divider" style="margin:20px 0">

      <h3>Exchange Rate API</h3>
      <p class="muted" style="margin:0 0 12px">Connect to <strong>exchangerate-api.com</strong> to fetch live rates with one click.
        <a href="https://www.exchangerate-api.com/" target="_blank" style="margin-left:4px">Sign up for a free account →</a>
      </p>
      <div style="display:grid;grid-template-columns:1fr auto auto;gap:10px;max-width:600px;align-items:flex-end">
        <div>
          <label class="form-label">API key</label>
          <input v-model="apiKey" type="password" class="form-input" placeholder="Your exchangerate-api.com key">
        </div>
        <button @click="saveApiKey" style="white-space:nowrap">Save key</button>
        <button @click="refreshRates" style="display:flex;align-items:center;gap:5px;white-space:nowrap">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
          Refresh rates
        </button>
      </div>
      <span class="muted" style="font-size:12px;display:block;margin-top:6px">{{ ratesMsg }}</span>
    </div>

    <!-- Model Rates -->
    <div class="card" style="margin-top:16px">
      <div class="flex" style="align-items:center;margin-bottom:4px">
        <h2 style="margin:0">Model Rates</h2>
        <span class="spacer"></span>
        <button @click="resetPricing" style="display:flex;align-items:center;gap:5px;background:transparent;border:1px solid var(--border);color:var(--muted);padding:5px 10px;border-radius:6px;cursor:pointer;font-size:12px">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-5.1L1 10"/></svg>
          Reset to defaults
        </button>
        <button @click="openAddModel" style="display:flex;align-items:center;gap:5px;margin-left:8px">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
          Add model
        </button>
      </div>
      <p class="muted" style="margin:0 0 12px;font-size:12px">Rates per 1M tokens, USD. All pricing data is sourced from Anthropic's published rates.</p>
      <div v-if="!models.length"><p class="muted">No model rates configured.</p></div>
      <div v-else style="overflow-x:auto">
        <table>
          <thead>
            <tr>
              <th>model</th><th>tier</th>
              <th class="num">input</th><th class="num">output</th>
              <th class="num">cache read</th><th class="num">cache 5m</th><th class="num">cache 1h</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="m in models" :key="m.model_name">
              <td><span :class="'badge ' + m.tier">{{ m.model_name }}</span></td>
              <td class="muted" style="font-size:12px">{{ m.tier }}</td>
              <td class="num">{{ Number(m.input).toFixed(2) }}</td>
              <td class="num">{{ Number(m.output).toFixed(2) }}</td>
              <td class="num">{{ Number(m.cache_read).toFixed(2) }}</td>
              <td class="num">{{ Number(m.cache_create_5m).toFixed(2) }}</td>
              <td class="num">{{ Number(m.cache_create_1h).toFixed(2) }}</td>
              <td style="white-space:nowrap;text-align:right">
                <button class="icon-btn" title="Edit" @click="openEditModel(m)">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
                </button>
                <button class="icon-btn" title="Delete" style="color:var(--bad)" @click="deleteModel(m.model_name)">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Plans -->
    <div class="card" style="margin-top:16px">
      <div class="flex" style="align-items:center;margin-bottom:12px">
        <h2 style="margin:0">Plans</h2>
        <span class="spacer"></span>
        <button @click="openAddPlan" style="display:flex;align-items:center;gap:5px">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
          Add plan
        </button>
      </div>
      <table>
        <thead>
          <tr><th>key</th><th>label</th><th class="num">monthly (USD)</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="p in plans" :key="p.plan_key">
            <td class="mono" style="font-size:12px">{{ p.plan_key }}</td>
            <td>{{ p.label }}</td>
            <td class="num">
              <span v-if="p.monthly > 0">{{ fmt.money(p.monthly, store.currency, store.exchangeRate) }}</span>
              <span v-else class="muted">pay-per-token</span>
            </td>
            <td style="white-space:nowrap;text-align:right">
              <button class="icon-btn" title="Edit" @click="openEditPlan(p)">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
              </button>
              <button class="icon-btn" title="Delete" style="color:var(--bad)" @click="deletePlan(p.plan_key)">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Data Management -->
    <div class="card" style="margin-top:16px">
      <h2>Data Management</h2>

      <h3 style="margin-top:16px">Scanner</h3>
      <p class="muted" style="margin:0 0 12px;font-size:13px">The scanner runs automatically every 30 seconds. Use this to pick up new sessions immediately.</p>
      <div class="flex" style="gap:10px;align-items:center">
        <button @click="scanNow">Scan Now</button>
        <span class="muted" style="font-size:12px">{{ scanMsg }}</span>
      </div>

      <hr class="divider" style="margin:20px 0">

      <h3>Retention</h3>
      <p class="muted" style="margin:0 0 12px;font-size:13px">Automatically delete old data from TokenTally's database. Leave blank to keep data forever.</p>
      <div class="flex" style="gap:10px;align-items:center">
        <label class="form-label" style="margin:0;white-space:nowrap">Delete data older than</label>
        <input v-model.number="retentionDays" type="number" min="1" step="1" class="form-input" style="width:90px" placeholder="e.g. 90">
        <span style="color:var(--muted);font-size:13px">days</span>
        <button class="primary" @click="saveRetention">Save</button>
        <span class="muted" style="font-size:12px">{{ retentionMsg }}</span>
      </div>
      <div style="margin-top:10px">
        <button
          :disabled="retentionDays <= 0"
          style="background:var(--bad);color:#fff;border:none;padding:6px 14px;border-radius:6px;cursor:pointer"
          @click="purgeNow"
        >Purge Now</button>
        <span class="muted" style="font-size:12px;margin-left:10px">{{ purgeMsg }}</span>
      </div>
      <p class="muted" style="font-size:11px;margin-top:8px">Removes messages from TokenTally's database only. Your <code style="font-size:11px">~/.claude/projects/</code> files are not affected and won't be re-imported.</p>
    </div>

    <!-- Windows Service -->
    <div class="card" style="margin-top:16px">
      <h2>Windows Service</h2>
      <p class="muted" style="font-size:13px">The background scanner runs as a Windows service, keeping data up to date even when the dashboard is closed.</p>
      <div style="margin:12px 0;font-size:13px">{{ serviceStatus }}</div>
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        <button class="primary" @click="installService">Install Service</button>
        <button style="background:var(--bad);color:#fff;border:none;padding:6px 14px;border-radius:6px;cursor:pointer" @click="uninstallService">Uninstall Service</button>
      </div>
      <p class="muted" style="font-size:11px;margin-top:8px">Requires administrator rights (UAC prompt will appear).</p>
    </div>

    <!-- Model Modal -->
    <div v-if="modelModal.show" class="modal-overlay" @click.self="modelModal.show = false">
      <div class="modal" style="max-width:560px;width:90vw">
        <h3 style="margin:0 0 16px;font-size:15px">{{ modelModal.title }}</h3>
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">
          <div style="grid-column:1/-1">
            <label class="form-label">Model name</label>
            <input v-model="modelModal.model_name" class="form-input" :readonly="modelModal.isEdit" :style="modelModal.isEdit ? 'opacity:0.6' : ''" placeholder="claude-sonnet-4-6">
          </div>
          <div style="grid-column:1/-1">
            <label class="form-label">Tier (opus / sonnet / haiku)</label>
            <input v-model="modelModal.tier" class="form-input" placeholder="sonnet">
          </div>
          <div>
            <label class="form-label">Input (per 1M, USD)</label>
            <input v-model.number="modelModal.input" type="number" step="0.01" min="0" class="form-input">
          </div>
          <div>
            <label class="form-label">Output (per 1M, USD)</label>
            <input v-model.number="modelModal.output" type="number" step="0.01" min="0" class="form-input">
          </div>
          <div>
            <label class="form-label">Cache read (per 1M, USD)</label>
            <input v-model.number="modelModal.cache_read" type="number" step="0.01" min="0" class="form-input">
          </div>
          <div>
            <label class="form-label">Cache 5m (per 1M, USD)</label>
            <input v-model.number="modelModal.cache_create_5m" type="number" step="0.01" min="0" class="form-input">
          </div>
          <div>
            <label class="form-label">Cache 1h (per 1M, USD)</label>
            <input v-model.number="modelModal.cache_create_1h" type="number" step="0.01" min="0" class="form-input">
          </div>
        </div>
        <div style="margin-top:16px;display:flex;gap:8px;justify-content:flex-end">
          <button @click="modelModal.show = false">Cancel</button>
          <button class="primary" @click="saveModel">Save</button>
        </div>
      </div>
    </div>

    <!-- Plan Modal -->
    <div v-if="planModal.show" class="modal-overlay" @click.self="planModal.show = false">
      <div class="modal" style="max-width:400px;width:90vw">
        <h3 style="margin:0 0 16px;font-size:15px">{{ planModal.title }}</h3>
        <div style="display:flex;flex-direction:column;gap:12px">
          <div>
            <label class="form-label">Plan key (unique identifier)</label>
            <input v-model="planModal.plan_key" class="form-input" :readonly="planModal.isEdit" :style="planModal.isEdit ? 'opacity:0.6' : ''" placeholder="max">
          </div>
          <div>
            <label class="form-label">Label</label>
            <input v-model="planModal.label" class="form-input" placeholder="Max">
          </div>
          <div>
            <label class="form-label">Monthly cost in USD (0 = free or pay-per-token)</label>
            <input v-model.number="planModal.monthly" type="number" step="0.01" min="0" class="form-input">
          </div>
        </div>
        <div style="margin-top:16px;display:flex;gap:8px;justify-content:flex-end">
          <button @click="planModal.show = false">Cancel</button>
          <button class="primary" @click="savePlanEntry">Save</button>
        </div>
      </div>
    </div>

  </div>
</template>
