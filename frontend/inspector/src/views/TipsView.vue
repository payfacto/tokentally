<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { api } from '../lib/api'
import { useAppStore } from '../stores/app'

const store = useAppStore()

interface TipRow { key: string; title: string; body: string }

const tips = ref<TipRow[]>([])

async function fetchTips() {
  tips.value = ((await api('/api/tips')) ?? []) as TipRow[]
}

async function dismiss(key: string) {
  await api('/api/tips/dismiss', { method: 'POST', body: JSON.stringify({ key }) })
  await fetchTips()
}

onMounted(fetchTips)
watch(() => store.lastScan, fetchTips)
</script>

<template>
  <div style="padding:20px">
    <div class="card">
      <h2>Suggestions</h2>
      <p v-if="!tips.length" class="muted">No suggestions right now. Token Dashboard surfaces patterns weekly — check back after more activity.</p>
      <p v-else class="muted" style="margin:-8px 0 14px">Rule-based pattern detection over the last 7 days. Dismissed tips re-appear after 14 days.</p>

      <div v-for="t in tips" :key="t.key" class="tip">
        <div class="tip-head">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" style="flex-shrink:0;color:var(--accent)"><path d="M9 18h6"/><path d="M10 22h4"/><path d="M12 2a7 7 0 0 1 7 7c0 2.6-1.4 4.9-3.5 6.2-.5.3-.5.8-.5 1.3V17H9v-.5c0-.5 0-1-.5-1.3A7 7 0 0 1 12 2z"/></svg>
          <strong>{{ t.title }}</strong>
          <span class="spacer"></span>
          <button class="ghost" @click="dismiss(t.key)">dismiss</button>
        </div>
        <p class="tip-body">{{ t.body }}</p>
      </div>
    </div>
  </div>
</template>
