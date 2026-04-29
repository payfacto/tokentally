<script setup lang="ts">
import type { ToolCallChunk } from '../../../lib/types'
import { inputStr } from '../../../lib/types'
const props = defineProps<{ toolCall: ToolCallChunk }>()
const cmd = () => inputStr(props.toolCall.input, 'command')
const truncate = (s: string) => s.length > 2000 ? s.slice(0, 2000) + '\n…' : s
</script>

<template>
  <div class="terminal">
    <div class="terminal-titlebar">
      <span class="dot dot-red" />
      <span class="dot dot-yellow" />
      <span class="dot dot-green" />
    </div>
    <div class="terminal-body">
      <pre class="cmd-pre"><span class="prompt">$</span> {{ cmd() }}</pre>
      <template v-if="toolCall.output">
        <hr class="divider" />
        <pre class="output-pre" :class="{ error: toolCall.isError }">{{ truncate(toolCall.output) }}</pre>
      </template>
    </div>
  </div>
</template>

<style scoped>
.terminal {
  background: #1e1e1e;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid #333;
}

.terminal-titlebar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  background: #2a2a2a;
  border-bottom: 1px solid #333;
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  display: inline-block;
}

.dot-red    { background: #ff5f57; }
.dot-yellow { background: #febc2e; }
.dot-green  { background: #28c840; }

.terminal-body {
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 0;
}

.cmd-pre {
  font-family: var(--mono);
  font-size: 11px;
  margin: 0;
  color: #4ec994;
  white-space: pre-wrap;
  word-break: break-word;
}

.prompt {
  color: #6a9f6a;
  user-select: none;
  margin-right: 4px;
}

.divider {
  border: none;
  border-top: 1px solid #333;
  margin: 8px 0;
}

.output-pre {
  font-family: var(--mono);
  font-size: 11px;
  margin: 0;
  color: #d4d4d4;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 300px;
  overflow-y: auto;
}

.output-pre.error { color: #f47070; }
</style>
