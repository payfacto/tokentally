<script setup lang="ts">
import { ref } from 'vue'
import { fmt } from '../lib/fmt'

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

interface RTKCommandRow {
  rank: number
  command: string
  count: number
  saved: string
  avg_pct: number
  time: string
  impact: number // 0.0–1.0
}

interface RTKGainResult {
  efficiency: number
  total_commands: number
  input_tokens: string
  output_tokens: string
  tokens_saved: string
  total_exec_time: string
  commands?: RTKCommandRow[]
  raw_output?: string[]
  not_found?: boolean
  error?: string
}

const info = ref<OverageInfo | null>(null)
const loading = ref(false)
const fetchError = ref<string | null>(null)

const rtkResult = ref<RTKGainResult | null>(null)
const rtkLoading = ref(false)
const rtkError = ref<string | null>(null)

async function check() {
  loading.value = true
  fetchError.value = null
  info.value = null
  try {
    info.value = await window.go.app.App.GetOverageInfo()
  } catch (e) {
    fetchError.value = String(e)
  } finally {
    loading.value = false
  }
}

async function checkRTK() {
  rtkLoading.value = true
  rtkError.value = null
  rtkResult.value = null
  try {
    rtkResult.value = await window.go.app.App.GetRTKGain()
  } catch (e) {
    rtkError.value = String(e)
  } finally {
    rtkLoading.value = false
  }
}

function efficiencyColor(pct: number): string {
  if (pct >= 75) return '#2d8a5e'
  if (pct >= 50) return '#3b82f6'
  if (pct >= 25) return '#b07800'
  return '#c03030'
}

function efficiencyLabel(pct: number): string {
  if (pct >= 75) return 'Excellent'
  if (pct >= 50) return 'Good'
  if (pct >= 25) return 'Needs Work'
  return 'Poor'
}

function rtkDonutDash(pct: number): string {
  // r=42, circumference = 2π×42 ≈ 263.89
  const c = 263.89
  const filled = (pct / 100) * c
  return `${filled} ${c}`
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

    <!-- RTK Section -->
    <div class="card rtk-card">
      <div class="rtk-header">
        <div>
          <h2 style="margin:0 0 4px">⚡ RTK Token Savings</h2>
          <p class="muted" style="margin:0 0 2px;font-size:13px">
            CLI proxy that reduces LLM token consumption by 60–90% on common dev commands.
          </p>
          <a href="https://www.rtk-ai.app/" target="_blank" style="font-size:12px;color:var(--accent)">rtk-ai.app →</a>
        </div>
        <button class="primary" :disabled="rtkLoading" @click="checkRTK" style="white-space:nowrap">
          <span v-if="rtkLoading" class="btn-spinner" aria-hidden="true"></span>
          {{ rtkLoading ? 'Checking…' : 'Check RTK status' }}
        </button>
      </div>

      <p v-if="rtkError" style="color:var(--error,#c03030);margin-top:14px">{{ rtkError }}</p>

      <p v-if="rtkResult && rtkResult.not_found" style="color:var(--warn,#b07800);margin-top:14px">
        RTK not found — install it from
        <a href="https://www.rtk-ai.app/" target="_blank" style="color:var(--accent)">rtk-ai.app</a>
      </p>

      <p v-if="rtkResult && rtkResult.error" style="color:var(--error,#c03030);margin-top:14px">
        {{ rtkResult.error }}
      </p>

      <!-- Parsed stats display -->
      <div v-if="rtkResult && !rtkResult.not_found && !rtkResult.error" class="rtk-body">

        <!-- Stats + Donut row -->
        <div class="rtk-summary-row">
          <div class="rtk-stats">
            <div class="rtk-stat">
              <span class="rtk-stat-icon">✕</span>
              <span class="rtk-stat-label">Total commands</span>
              <span class="rtk-stat-value">{{ rtkResult.total_commands }}</span>
            </div>
            <div class="rtk-stat">
              <span class="rtk-stat-icon">›</span>
              <span class="rtk-stat-label">Input tokens</span>
              <span class="rtk-stat-value">{{ rtkResult.input_tokens }}</span>
            </div>
            <div class="rtk-stat">
              <span class="rtk-stat-icon">›</span>
              <span class="rtk-stat-label">Output tokens</span>
              <span class="rtk-stat-value">{{ rtkResult.output_tokens }}</span>
            </div>
            <div class="rtk-stat">
              <span class="rtk-stat-icon">›</span>
              <span class="rtk-stat-label">Tokens saved</span>
              <span class="rtk-stat-value" :style="{ color: efficiencyColor(rtkResult.efficiency) }">
                {{ rtkResult.tokens_saved }}
                <span style="font-weight:400;font-size:12px;opacity:0.85">({{ rtkResult.efficiency.toFixed(1) }}%)</span>
              </span>
            </div>
            <div class="rtk-stat">
              <span class="rtk-stat-icon">⏱</span>
              <span class="rtk-stat-label">Total exec time</span>
              <span class="rtk-stat-value">{{ rtkResult.total_exec_time }}</span>
            </div>
          </div>

          <!-- Circular efficiency meter -->
          <div class="rtk-donut-wrap">
            <div class="rtk-donut-label-top">Efficiency Meter</div>
            <svg width="110" height="110" viewBox="0 0 110 110">
              <circle cx="55" cy="55" r="42" fill="none" stroke="#2a2a2a" stroke-width="11"/>
              <circle
                cx="55" cy="55" r="42" fill="none"
                :stroke="efficiencyColor(rtkResult.efficiency)"
                stroke-width="11"
                stroke-linecap="round"
                :stroke-dasharray="rtkDonutDash(rtkResult.efficiency)"
                transform="rotate(-90 55 55)"
              />
              <text x="55" y="50" text-anchor="middle" dominant-baseline="middle"
                font-size="17" font-weight="700"
                :fill="efficiencyColor(rtkResult.efficiency)">
                {{ rtkResult.efficiency.toFixed(1) }}%
              </text>
              <text x="55" y="67" text-anchor="middle" dominant-baseline="middle"
                font-size="10" fill="#888">
                {{ efficiencyLabel(rtkResult.efficiency) }}
              </text>
            </svg>
          </div>
        </div>

        <!-- By Command table -->
        <div v-if="rtkResult.commands && rtkResult.commands.length" class="rtk-cmd-section">
          <div class="rtk-cmd-title">› By Command</div>
          <table class="rtk-cmd-table">
            <thead>
              <tr>
                <th>#</th>
                <th>Command</th>
                <th>Count</th>
                <th>Saved</th>
                <th>Avg%</th>
                <th>Time</th>
                <th style="width:120px">Impact</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in rtkResult.commands" :key="row.rank">
                <td class="col-rank">{{ row.rank }}</td>
                <td class="col-cmd">{{ row.command }}</td>
                <td class="col-num">{{ row.count }}</td>
                <td class="col-num">{{ row.saved }}</td>
                <td class="col-num" :style="{ color: efficiencyColor(row.avg_pct) }">{{ row.avg_pct.toFixed(1) }}%</td>
                <td class="col-num">{{ row.time }}</td>
                <td class="col-impact">
                  <div class="impact-track">
                    <div class="impact-fill" :style="{ width: (row.impact * 100) + '%' }"></div>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <details style="margin-top:14px">
          <summary style="cursor:pointer;font-size:12px;color:var(--muted)">Raw output ({{ (rtkResult.raw_output || []).length }} lines)</summary>
          <pre style="font-size:11px;overflow:auto;max-height:300px;background:#1a1a1a;color:#d4d4d4;padding:10px;border-radius:4px;margin-top:8px">{{ (rtkResult.raw_output || []).join('\n') }}</pre>
        </details>
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

/* Donut */
.rtk-donut-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding-top: 2px;
}
.rtk-donut-label-top {
  font-size: 11px;
  color: var(--muted, #888);
  letter-spacing: 0.03em;
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
.col-rank { color: var(--muted, #666); width: 28px; }
.col-cmd  { font-family: monospace; font-size: 11px; }
.col-num  { text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }

.col-impact { padding-left: 10px; }
.impact-track {
  height: 8px;
  background: #2a2a2a;
  border-radius: 4px;
  overflow: hidden;
  width: 100%;
}
.impact-fill {
  height: 100%;
  background: #f97316;
  border-radius: 4px;
  transition: width 0.4s ease;
}
</style>
