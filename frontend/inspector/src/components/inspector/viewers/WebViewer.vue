<script setup lang="ts">
import type { ToolCallChunk } from '../../../lib/types'
const props = defineProps<{ toolCall: ToolCallChunk }>()
const url = () => (props.toolCall.input as Record<string, string>)?.url ?? (props.toolCall.input as Record<string, string>)?.query ?? ''
const truncate = (s: string) => s.length > 2000 ? s.slice(0, 2000) + '\n…' : s
</script>

<template>
  <div class="web-viewer">
    <div class="url muted">{{ url() }}</div>
    <pre v-if="toolCall.output" class="output-pre">{{ truncate(toolCall.output) }}</pre>
  </div>
</template>

<style scoped>
.web-viewer { display: flex; flex-direction: column; gap: 4px; }
.url { font-family: var(--mono); font-size: 11px; margin-bottom: 4px; word-break: break-all; }
.output-pre { font-family: var(--mono); font-size: 11px; margin: 0; white-space: pre-wrap; word-break: break-word; max-height: 300px; overflow-y: auto; }
.muted { color: var(--muted); }
</style>
