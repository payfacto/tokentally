<script setup lang="ts">
import type { ToolCallChunk } from '../../../lib/types'
const props = defineProps<{ toolCall: ToolCallChunk }>()
const cmd = () => (props.toolCall.input as Record<string, string>)?.command ?? ''
const truncate = (s: string) => s.length > 2000 ? s.slice(0, 2000) + '\n…' : s
</script>

<template>
  <div class="bash-viewer">
    <pre class="cmd-pre">$ {{ cmd() }}</pre>
    <pre v-if="toolCall.output" class="output-pre" :class="{ error: toolCall.isError }">{{ truncate(toolCall.output) }}</pre>
  </div>
</template>

<style scoped>
.bash-viewer { display: flex; flex-direction: column; gap: 4px; }
.cmd-pre   { font-family: var(--mono); font-size: 11px; margin: 0; color: var(--good); white-space: pre-wrap; }
.output-pre { font-family: var(--mono); font-size: 11px; margin: 0; white-space: pre-wrap; word-break: break-word; max-height: 300px; overflow-y: auto; }
.output-pre.error { color: var(--bad, #c0392b); }
</style>
