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

interface RTKGainResult {
  efficiency: number
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
  if (pct >= 75) return 'var(--good, #2d8a5e)'
  if (pct >= 50) return 'var(--accent, #3b82f6)'
  if (pct >= 25) return 'var(--warn, #b07800)'
  return 'var(--error, #c03030)'
}

function efficiencyLabel(pct: number): string {
  if (pct >= 75) return 'Excellent'
  if (pct >= 50) return 'Good'
  if (pct >= 25) return 'Needs Work'
  return 'Poor'
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
    <div class="card" style="max-width:560px">
      <h2 style="margin-top:0">RTK — Token Killer</h2>
      <p class="muted" style="margin:-4px 0 4px;font-size:13px">
        RTK is a CLI proxy that reduces LLM token consumption by 60-90% on common dev commands.
      </p>
      <p style="margin:0 0 16px;font-size:12px">
        <a href="https://www.rtk-ai.app/" target="_blank" style="color:var(--accent)">rtk-ai.app →</a>
      </p>

      <button class="primary" :disabled="rtkLoading" @click="checkRTK">
        <span v-if="rtkLoading" class="btn-spinner" aria-hidden="true"></span>
        {{ rtkLoading ? 'Checking…' : 'Check RTK Gain' }}
      </button>

      <p v-if="rtkError" style="color:var(--error,#c03030);margin-top:14px">{{ rtkError }}</p>

      <p v-if="rtkResult && rtkResult.not_found" style="color:var(--warn,#b07800);margin-top:14px">
        RTK not found — install it from
        <a href="https://www.rtk-ai.app/" target="_blank" style="color:var(--accent)">rtk-ai.app</a>
      </p>

      <p v-if="rtkResult && rtkResult.error" style="color:var(--error,#c03030);margin-top:14px">
        {{ rtkResult.error }}
      </p>

      <div v-if="rtkResult && !rtkResult.not_found && !rtkResult.error" style="margin-top:20px">
        <div style="margin-bottom:10px;font-size:13px;color:var(--muted)">Token Efficiency</div>
        <div class="eff-meter-wrap">
          <div class="eff-bar-track">
            <div
              class="eff-bar-fill"
              :style="{ width: rtkResult.efficiency + '%', background: efficiencyColor(rtkResult.efficiency) }"
            ></div>
          </div>
          <span class="eff-label" :style="{ color: efficiencyColor(rtkResult.efficiency) }">
            {{ rtkResult.efficiency.toFixed(1) }}% — {{ efficiencyLabel(rtkResult.efficiency) }}
          </span>
        </div>
      </div>

      <details v-if="rtkResult && rtkResult.raw_output && rtkResult.raw_output.length" style="margin-top:16px">
        <summary style="cursor:pointer;font-size:12px;color:var(--muted)">Raw output ({{ rtkResult.raw_output.length }} lines)</summary>
        <pre style="font-size:11px;overflow:auto;max-height:300px;background:#1a1a1a;color:#d4d4d4;padding:10px;border-radius:4px;margin-top:8px">{{ rtkResult.raw_output.join('\n') }}</pre>
      </details>
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

.eff-meter-wrap {
  display: flex;
  align-items: center;
  gap: 14px;
}
.eff-bar-track {
  flex: 1;
  height: 10px;
  background: var(--surface2, #2a2a2a);
  border-radius: 5px;
  overflow: hidden;
}
.eff-bar-fill {
  height: 100%;
  border-radius: 5px;
  transition: width 0.4s ease;
}
.eff-label {
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
  min-width: 180px;
}
</style>
