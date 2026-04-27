<script setup lang="ts">
import type { ToolCallChunk } from '../../../lib/types'
defineProps<{ toolCall: ToolCallChunk }>()
</script>

<template>
  <div v-if="toolCall.output" class="generic-output">
    <pre class="output-pre">{{ toolCall.output.slice(0, 2000) }}</pre>
    <span v-if="toolCall.output.length > 2000" class="muted" style="font-size:10px">
      … {{ (toolCall.output.length - 2000).toLocaleString() }} more chars
    </span>
  </div>
</template>

<style scoped>
.generic-output { overflow: hidden; }
.output-pre { font-family: var(--mono); font-size: 11px; white-space: pre-wrap; word-break: break-word; margin: 0; max-height: 300px; overflow-y: auto; }
.muted { color: var(--muted); }
</style>
