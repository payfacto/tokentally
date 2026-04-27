<script setup lang="ts">
import { ref, computed } from 'vue'
const props = defineProps<{ text: string }>()
const expanded = ref(false)
const charCount = computed(() => props.text.length.toLocaleString())
</script>

<template>
  <div class="thinking-block">
    <div class="thinking-toggle" @click="expanded = !expanded">
      <span class="thinking-icon">◈</span>
      <span class="muted" style="font-size:12px">Thinking ({{ charCount }} chars)</span>
      <span class="spacer" />
      <span class="muted" style="font-size:11px">{{ expanded ? '▾' : '▸' }}</span>
    </div>
    <div v-if="expanded" class="thinking-body">
      <pre class="thinking-text">{{ text }}</pre>
    </div>
  </div>
</template>

<style scoped>
.thinking-block { border: 1px solid var(--border); border-radius: 6px; margin-bottom: 8px; overflow: hidden; }
.thinking-toggle { display: flex; align-items: center; gap: 6px; padding: 6px 10px; cursor: pointer; background: var(--panel); user-select: none; }
.thinking-toggle:hover { background: var(--panel-2, var(--panel)); }
.thinking-icon { color: var(--muted); font-size: 14px; }
.thinking-body { padding: 10px; background: var(--bg); }
.thinking-text { font-family: var(--mono); font-size: 11px; white-space: pre-wrap; word-break: break-word; margin: 0; color: var(--muted); }
.spacer { flex: 1; }
.muted { color: var(--muted); }
</style>
