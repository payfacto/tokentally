<script setup lang="ts">
import type { Chunk } from '../../lib/types'
import { copyMarkdown } from '../../lib/clipboard'
import { fmt } from '../../lib/fmt'

const props = defineProps<{ chunk: Chunk }>()

function copyChunk(e: MouseEvent) {
  if (!(e.currentTarget instanceof HTMLElement)) return
  const md = `**User** · ${fmt.time(props.chunk.timestamp)}\n\n${props.chunk.text ?? ''}`
  copyMarkdown(md, e.currentTarget)
}
</script>

<template>
  <div class="user-turn">
    <div class="turn-header">
      <span class="badge" style="font-size:10px">you</span>
      <span class="muted" style="font-family:var(--mono);font-size:11px">{{ fmt.time(chunk.timestamp) }}</span>
    </div>
    <div class="turn-text">{{ chunk.text }}</div>
    <button class="copy-btn" title="Copy as Markdown" @click="copyChunk">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
    </button>
  </div>
</template>

<style scoped>
.user-turn { padding: 12px 0; border-bottom: 1px solid var(--border); position: relative; }
.turn-header { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.turn-text { font-size: 13px; line-height: 1.6; white-space: pre-wrap; word-break: break-word; padding-bottom: 20px; }
.muted { color: var(--muted); }
.copy-btn {
  position: absolute; bottom: 6px; right: 0;
  background: transparent; border: 1px solid var(--border);
  border-radius: 4px; padding: 4px 5px;
  cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center;
  line-height: 1; transition: color 120ms, border-color 120ms;
}
.copy-btn:hover { color: var(--text); border-color: var(--text); }
</style>
