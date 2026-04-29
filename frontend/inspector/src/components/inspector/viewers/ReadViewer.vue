<script setup lang="ts">
import type { ToolCallChunk } from '../../../lib/types'
import { inputStr } from '../../../lib/types'
const props = defineProps<{ toolCall: ToolCallChunk }>()
const path = () => inputStr(props.toolCall.input, 'file_path')
const truncate = (s: string) => s.length > 3000 ? s.slice(0, 3000) + '\n…' : s
</script>

<template>
  <div class="read-viewer">
    <div class="file-header">
      <span class="file-path">{{ path() }}</span>
    </div>
    <pre v-if="toolCall.output" class="output-pre">{{ truncate(toolCall.output) }}</pre>
  </div>
</template>

<style scoped>
.read-viewer {
  background: #1e1e1e;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid #333;
}

.file-header {
  padding: 6px 12px;
  background: #252525;
  border-bottom: 1px solid #333;
}

.file-path {
  font-family: var(--mono);
  font-size: 11px;
  color: #888;
}

.output-pre {
  font-family: var(--mono);
  font-size: 11px;
  margin: 0;
  padding: 10px 12px;
  color: #d4d4d4;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 300px;
  overflow-y: auto;
}
</style>
