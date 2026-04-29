<script setup lang="ts">
import type { ToolCallChunk } from '../../../lib/types'
import { inputStr } from '../../../lib/types'
const props = defineProps<{ toolCall: ToolCallChunk }>()
const path = () => inputStr(props.toolCall.input, 'file_path')
const oldStr = () => inputStr(props.toolCall.input, 'old_string')
const newStr = () => inputStr(props.toolCall.input, 'new_string')
</script>

<template>
  <div class="diff-viewer">
    <div class="file-path muted">{{ path() }}</div>
    <div v-if="oldStr()" class="diff-block">
      <pre class="diff-pre removed">{{ oldStr().slice(0, 1000) }}</pre>
      <pre class="diff-pre added">{{ newStr().slice(0, 1000) }}</pre>
    </div>
    <pre v-else class="diff-pre">{{ toolCall.output?.slice(0, 1000) }}</pre>
  </div>
</template>

<style scoped>
.diff-viewer { display: flex; flex-direction: column; gap: 4px; }
.file-path { font-family: var(--mono); font-size: 11px; margin-bottom: 4px; }
.diff-block { display: flex; flex-direction: column; gap: 4px; }
.diff-pre { font-family: var(--mono); font-size: 11px; margin: 0; white-space: pre-wrap; word-break: break-word; padding: 4px 8px; border-radius: 3px; max-height: 200px; overflow-y: auto; }
.diff-pre.removed { background: rgba(192,57,43,0.08); border-left: 2px solid rgba(192,57,43,0.4); }
.diff-pre.added   { background: rgba(39,174,96,0.08);  border-left: 2px solid rgba(39,174,96,0.4); }
.muted { color: var(--muted); }
</style>
