<script setup lang="ts">
import type { ToolCallChunk } from '../../../lib/types'
import { inputStr } from '../../../lib/types'
const props = defineProps<{ toolCall: ToolCallChunk }>()
const pattern = () => inputStr(props.toolCall.input, 'pattern') || inputStr(props.toolCall.input, 'query')
const truncate = (s: string) => s.length > 2000 ? s.slice(0, 2000) + '\n…' : s
</script>

<template>
  <div class="search-viewer">
    <div class="pattern muted">{{ pattern() }}</div>
    <pre v-if="toolCall.output" class="output-pre">{{ truncate(toolCall.output) }}</pre>
  </div>
</template>

<style scoped>
.search-viewer { display: flex; flex-direction: column; gap: 4px; }
.pattern { font-family: var(--mono); font-size: 11px; margin-bottom: 4px; }
.output-pre { font-family: var(--mono); font-size: 11px; margin: 0; white-space: pre-wrap; word-break: break-word; max-height: 300px; overflow-y: auto; }
.muted { color: var(--muted); }
</style>
