<script setup lang="ts">
import { ref, computed } from 'vue'
import type { ToolCallChunk } from '../../lib/types'
import { inputStr } from '../../lib/types'

const props = defineProps<{ toolCall: ToolCallChunk }>()
const expanded = ref(true)

const categoryClass = computed(() => {
  const n = props.toolCall.name
  if (['Read', 'Edit', 'Write', 'MultiEdit'].includes(n)) return 'file-op'
  if (n === 'Bash') return 'shell'
  if (['Grep', 'Glob', 'WebSearch'].includes(n)) return 'search'
  if (['Task', 'Agent'].includes(n)) return 'agent'
  if (n === 'WebFetch') return 'web'
  return 'other'
})

const fmtDuration = (ms?: number) => {
  if (!ms) return ''
  return ms < 1000 ? ms + 'ms' : (ms / 1000).toFixed(1) + 's'
}

const targetLabel = computed(() =>
  inputStr(props.toolCall.input, 'file_path') ||
  inputStr(props.toolCall.input, 'command') ||
  inputStr(props.toolCall.input, 'url') ||
  inputStr(props.toolCall.input, 'pattern')
)
</script>

<template>
  <div class="tool-frame" :class="categoryClass">
    <div class="tool-header" @click="expanded = !expanded">
      <span class="tool-badge" :class="categoryClass">{{ toolCall.name }}</span>
      <span v-if="toolCall.isError" class="badge bad" style="font-size:10px">error</span>
      <span class="tool-target muted">{{ targetLabel }}</span>
      <span class="spacer" />
      <span v-if="toolCall.durationMs" class="muted" style="font-size:10px;font-family:var(--mono)">{{ fmtDuration(toolCall.durationMs) }}</span>
      <span class="expand-btn">{{ expanded ? '▾' : '▸' }}</span>
    </div>
    <div v-if="expanded" class="tool-body">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.tool-frame { border: 1px solid var(--border); border-radius: 6px; margin: 6px 0; overflow: hidden; }
.tool-header { display: flex; align-items: center; gap: 6px; padding: 6px 10px; cursor: pointer; background: var(--panel); user-select: none; }
.tool-header:hover { background: var(--panel-2, var(--panel)); }
.tool-body { padding: 10px; background: var(--bg); }
.tool-badge { font-family: var(--mono); font-size: 10px; padding: 2px 6px; border-radius: 3px; border: 1px solid; }
.tool-badge.file-op { color: var(--accent); border-color: rgba(235,115,59,0.3); background: rgba(235,115,59,0.08); }
.tool-badge.shell   { color: var(--good);   border-color: rgba(45,138,94,0.3);   background: rgba(45,138,94,0.08); }
.tool-badge.search  { color: var(--warn);   border-color: rgba(176,120,0,0.3);   background: rgba(176,120,0,0.08); }
.tool-badge.agent   { color: var(--accent); border-color: rgba(176,78,32,0.3);   background: rgba(176,78,32,0.08); }
.tool-badge.web     { color: var(--muted);  border-color: var(--border); background: var(--panel); }
.tool-badge.other   { color: var(--muted);  border-color: var(--border); background: var(--panel); }
.tool-target { font-family: var(--mono); font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 300px; }
.spacer { flex: 1; }
.expand-btn { color: var(--muted); font-size: 12px; }
.muted { color: var(--muted); }
</style>
