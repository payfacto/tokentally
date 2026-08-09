<script setup lang="ts">
import { ref } from 'vue'
import { fmt } from '../lib/fmt'
import { useAppStore } from '../stores/app'
import { App } from '../bindings/tokentally/app'

const store = useAppStore()

interface OverageInfo {
  model: string
  service_tier: string
  rate_limit_type: string
  overage_status: string
  overage_disabled_reason: string
  is_using_overage: boolean
  error?: string
  raw_output?: string[]
}

interface LmsgoSubRow { subcommand: string; calls: number; tokens_saved: number }
interface LmsgoSavings {
  total_calls: number
  successful_calls: number
  error_calls: number
  input_tokens_apx: number
  response_tokens_apx: number
  tokens_saved_apx: number
  files_resolved: number
  files_missing: number
  by_subcommand: LmsgoSubRow[]
}

const info = ref<OverageInfo | null>(null)
const loading = ref(false)
const fetchError = ref<string | null>(null)

const lmsgoResult = ref<LmsgoSavings | null>(null)
const lmsgoLoading = ref(false)
const lmsgoError = ref<string | null>(null)

async function check() {
  loading.value = true
  fetchError.value = null
  info.value = null
  try {
    info.value = await App.GetOverageInfo()
  } catch (e) {
    fetchError.value = String(e)
  } finally {
    loading.value = false
  }
}

async function checkLmsgo() {
  lmsgoLoading.value = true
  lmsgoError.value = null
  lmsgoResult.value = null
  try {
    lmsgoResult.value = await App.GetLmsgoSavings('', '')
  } catch (e) {
    lmsgoError.value = String(e)
  } finally {
    lmsgoLoading.value = false
  }
}

</script>

<template>
  <div style="padding:20px">

    <!-- Overage Checker -->
    <div class="card" style="max-width:560px;margin-bottom:20px">
      <h2 style="margin-top:0">Overage &amp; Auth Status</h2>
      <p class="muted" style="margin:-4px 0 16px;font-size:13px">
        Makes a quick test call to the Claude CLI to reveal your current auth mode,
        model, and rate-limit / overage settings.
      </p>

      <button class="primary" :disabled="loading" @click="check">
        <span v-if="loading" class="btn-spinner" aria-hidden="true"></span>
        {{ loading ? 'Checking…' : 'Check Now' }}
      </button>

      <p v-if="fetchError" style="color:var(--error,#c03030);margin-top:14px">{{ fetchError }}</p>

      <table v-if="info" style="margin-top:20px;width:100%;border-collapse:collapse">
        <tbody>
          <tr>
            <td class="row-label">Model</td>
            <td class="row-value">
              <span v-if="info.model" :class="'badge ' + fmt.modelClass(info.model)">{{ info.model }}</span>
              <span v-else class="muted">—</span>
            </td>
          </tr>
          <tr>
            <td class="row-label">Service Tier</td>
            <td class="row-value mono">{{ info.service_tier || '—' }}</td>
          </tr>
          <tr>
            <td class="row-label">Rate Limit Type</td>
            <td class="row-value mono">{{ info.rate_limit_type || '—' }}</td>
          </tr>
          <tr>
            <td class="row-label">Overage Status</td>
            <td class="row-value mono">{{ info.overage_status || '—' }}</td>
          </tr>
          <tr>
            <td class="row-label">Overage Disabled Reason</td>
            <td class="row-value mono">{{ info.overage_disabled_reason || '—' }}</td>
          </tr>
          <tr>
            <td class="row-label">Using Overage?</td>
            <td class="row-value">
              <span :class="'badge ' + (info.is_using_overage ? 'badge-warn' : 'badge-ok')">
                {{ info.is_using_overage ? 'yes' : 'no' }}
              </span>
            </td>
          </tr>
          <tr v-if="info.error && info.error !== 'none'">
            <td class="row-label">Error</td>
            <td class="row-value" style="color:var(--error,#c03030)">{{ info.error }}</td>
          </tr>
        </tbody>
      </table>

      <details v-if="info && info.raw_output && info.raw_output.length" style="margin-top:16px">
        <summary style="cursor:pointer;font-size:12px;color:var(--muted)">Raw CLI output ({{ info.raw_output.length }} lines)</summary>
        <pre style="font-size:11px;overflow:auto;max-height:300px;background:#1a1a1a;color:#d4d4d4;padding:10px;border-radius:4px;margin-top:8px">{{ info.raw_output.join('\n') }}</pre>
      </details>
    </div>

    <!-- lmsgo Section (opt-in via Settings → Beta Features) -->
    <div v-if="store.showLmsgo" class="card rtk-card" style="margin-top:20px">
      <div class="rtk-header">
        <div>
          <h2 style="margin:0 0 4px">📂 lmsgo Token Savings</h2>
          <p class="muted" style="margin:0 0 2px;font-size:13px">
            Delegate bulk file reads to a local LM Studio model so file contents never enter Claude's context.
          </p>
          <a href="https://github.com/payfacto/lmsgo" target="_blank" style="font-size:12px;color:var(--accent)">github.com/payfacto/lmsgo →</a>
        </div>
        <button class="primary" :disabled="lmsgoLoading" @click="checkLmsgo" style="white-space:nowrap">
          <span v-if="lmsgoLoading" class="btn-spinner" aria-hidden="true"></span>
          {{ lmsgoLoading ? 'Loading…' : 'Check lmsgo savings' }}
        </button>
      </div>

      <p v-if="lmsgoError" style="color:var(--error,#c03030);margin-top:14px">{{ lmsgoError }}</p>

      <p v-if="lmsgoResult && lmsgoResult.total_calls === 0" style="color:var(--warn,#b07800);margin-top:14px">
        No lmsgo invocations recorded yet — install it from
        <a href="https://github.com/payfacto/lmsgo" target="_blank" style="color:var(--accent)">github.com/payfacto/lmsgo</a>
        and use it from Claude Code.
      </p>

      <div v-if="lmsgoResult && lmsgoResult.total_calls > 0" class="rtk-body">

        <div class="rtk-summary-row">
          <div class="rtk-stats">
            <div class="rtk-stat">
              <span class="rtk-stat-icon">✕</span>
              <span class="rtk-stat-label">Total calls</span>
              <span class="rtk-stat-value">{{ lmsgoResult.total_calls }}</span>
            </div>
            <div class="rtk-stat">
              <span class="rtk-stat-icon">›</span>
              <span class="rtk-stat-label">Input tokens avoided</span>
              <span class="rtk-stat-value">~{{ fmt.compact(lmsgoResult.input_tokens_apx) }}</span>
            </div>
            <div class="rtk-stat">
              <span class="rtk-stat-icon">›</span>
              <span class="rtk-stat-label">Response tokens (returned)</span>
              <span class="rtk-stat-value">~{{ fmt.compact(lmsgoResult.response_tokens_apx) }}</span>
            </div>
            <div class="rtk-stat">
              <span class="rtk-stat-icon">›</span>
              <span class="rtk-stat-label">Tokens saved</span>
              <span class="rtk-stat-value" style="color:#2d8a5e">
                ~{{ fmt.compact(lmsgoResult.tokens_saved_apx) }}
                <span style="font-weight:400;font-size:12px;opacity:0.85">(approx)</span>
              </span>
            </div>
            <div v-if="lmsgoResult.error_calls > 0" class="rtk-stat">
              <span class="rtk-stat-icon">!</span>
              <span class="rtk-stat-label">Errored calls</span>
              <span class="rtk-stat-value">{{ lmsgoResult.error_calls }}</span>
            </div>
            <div v-if="lmsgoResult.files_missing > 0" class="rtk-stat">
              <span class="rtk-stat-icon">?</span>
              <span class="rtk-stat-label">Files missing on disk</span>
              <span class="rtk-stat-value">
                {{ lmsgoResult.files_missing }}
                <span style="font-weight:400;font-size:11px;opacity:0.7;margin-left:6px">est. is a lower bound</span>
              </span>
            </div>
          </div>
        </div>

        <div v-if="lmsgoResult.by_subcommand.length > 0" class="rtk-cmd-section">
          <div class="rtk-cmd-title">› By Subcommand</div>
          <table class="rtk-cmd-table">
            <thead>
              <tr>
                <th>Subcommand</th>
                <th>Calls</th>
                <th>Tokens saved</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in lmsgoResult.by_subcommand" :key="row.subcommand">
                <td class="col-cmd">lmsgo {{ row.subcommand }}</td>
                <td class="col-num">{{ row.calls }}</td>
                <td class="col-num">~{{ fmt.compact(row.tokens_saved) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <p class="muted" style="margin-top:14px;font-size:11px">
          Approximation: ~4 chars/token. Saved = on-disk size of input files − size of the lmsgo response.
        </p>
      </div>
    </div>

  </div>
</template>

<style scoped>
@keyframes spin {
  to { transform: rotate(360deg); }
}
.btn-spinner {
  display: inline-block;
  width: 12px;
  height: 12px;
  border: 2px solid rgba(255,255,255,0.4);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
  vertical-align: middle;
  margin-right: 6px;
}
.row-label {
  padding: 7px 16px 7px 0;
  font-size: 13px;
  color: var(--muted);
  white-space: nowrap;
  vertical-align: middle;
  border-bottom: 1px solid var(--border);
}
.row-value {
  padding: 7px 0;
  font-size: 13px;
  vertical-align: middle;
  border-bottom: 1px solid var(--border);
}
.badge-ok   { background: var(--good, #2d8a5e); color: #fff; }
.badge-warn { background: var(--warn, #b07800); color: #fff; }

/* RTK card */
.rtk-card { max-width: 760px; }

.rtk-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
}

.rtk-body { margin-top: 4px; }

.rtk-summary-row {
  display: flex;
  gap: 24px;
  align-items: flex-start;
}

.rtk-stats { flex: 1; }

.rtk-stat {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px solid var(--border, #2a2a2a);
  font-size: 13px;
}
.rtk-stat:last-child { border-bottom: none; }

.rtk-stat-icon {
  width: 16px;
  text-align: center;
  color: var(--muted, #666);
  font-size: 12px;
  flex-shrink: 0;
}
.rtk-stat-label {
  flex: 1;
  color: var(--muted, #888);
}
.rtk-stat-value {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

/* By Command */
.rtk-cmd-section { margin-top: 24px; }
.rtk-cmd-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--muted, #888);
  margin-bottom: 10px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.rtk-cmd-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}
.rtk-cmd-table th {
  text-align: left;
  padding: 5px 8px;
  color: var(--muted, #666);
  border-bottom: 1px solid var(--border, #2a2a2a);
  font-weight: 500;
  white-space: nowrap;
}
.rtk-cmd-table td {
  padding: 5px 8px;
  border-bottom: 1px solid var(--border, #1e1e1e);
  vertical-align: middle;
}
.col-cmd  { font-family: monospace; font-size: 11px; }
.col-num  { text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
</style>
