<script setup lang="ts">
import type { ToolCallChunk } from '../../../lib/types'
const props = defineProps<{ toolCall: ToolCallChunk }>()
const path = () => (props.toolCall.input as Record<string, string>)?.file_path ?? ''
const truncate = (s: string) => s.length > 3000 ? s.slice(0, 3000) + '\n…' : s
</script>

<template>
  <div class="read-viewer">
    <div class="file-path muted">{{ path() }}</div>
    <pre v-if="toolCall.output" class="output-pre">{{ truncate(toolCall.output) }}</pre>
  </div>
</template>

<style scoped>
.read-viewer { display: flex; flex-direction: column; gap: 4px; }
.file-path { font-family: var(--mono); font-size: 11px; margin-bottom: 4px; }
.output-pre { font-family: var(--mono); font-size: 11px; margin: 0; white-space: pre-wrap; word-break: break-word; max-height: 300px; overflow-y: auto; }
.muted { color: var(--muted); }
</style>
