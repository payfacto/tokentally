<script setup lang="ts">
import type { Chunk } from '../../lib/types'
import { copyMarkdown } from '../../lib/clipboard'

const props = defineProps<{ chunk: Chunk }>()

function copyChunk(e: MouseEvent) {
  if (!(e.currentTarget instanceof HTMLElement)) return
  const ts = props.chunk.timestamp.slice(11, 19)
  const md = `**System** · ${ts}\n\n${props.chunk.text ?? ''}`
  copyMarkdown(md, e.currentTarget)
}
</script>

<template>
  <div class="system-msg muted">
    <span style="font-size:10px;font-family:var(--mono)">system</span>
    <span style="font-size:12px;margin-left:8px;flex:1">{{ chunk.text }}</span>
    <button class="copy-btn" title="Copy as Markdown" @click="copyChunk">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
    </button>
  </div>
</template>

<style scoped>
.system-msg { padding: 6px 0; border-bottom: 1px solid var(--border); display: flex; align-items: center; }
.muted { color: var(--muted); }
.copy-btn {
  background: transparent; border: 1px solid var(--border);
  border-radius: 4px; padding: 4px 5px;
  cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center;
  line-height: 1; transition: color 120ms, border-color 120ms; flex-shrink: 0;
}
.copy-btn:hover { color: var(--text); border-color: var(--text); }
</style>
