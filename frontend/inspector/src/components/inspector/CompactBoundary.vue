<script setup lang="ts">
import type { Chunk } from '../../lib/types'
import { copyMarkdown } from '../../lib/clipboard'
import { fmt } from '../../lib/fmt'

const props = defineProps<{ chunk: Chunk }>()

function copyChunk(e: MouseEvent) {
  if (!(e.currentTarget instanceof HTMLElement)) return
  const md = `**Context compacted** · ${fmt.tok(props.chunk.tokensBefore)} → ${fmt.tok(props.chunk.tokensAfter)} tokens`
  copyMarkdown(md, e.currentTarget)
}
</script>

<template>
  <div class="compact-boundary">
    <div class="compact-line" />
    <div class="compact-label">
      ⚡ Context compacted — {{ fmt.tok(chunk.tokensBefore) }} → {{ fmt.tok(chunk.tokensAfter) }} tokens
    </div>
    <div class="compact-line" />
    <button class="copy-btn" title="Copy as Markdown" @click="copyChunk">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
    </button>
  </div>
</template>

<style scoped>
.compact-boundary { display: flex; align-items: center; gap: 10px; padding: 12px 0; position: relative; }
.compact-line { flex: 1; height: 1px; background: var(--accent); opacity: 0.4; }
.compact-label { font-size: 11px; font-family: var(--mono); color: var(--accent); white-space: nowrap; }
.copy-btn {
  position: absolute; bottom: 4px; right: 0;
  background: transparent; border: 1px solid var(--border);
  border-radius: 4px; padding: 4px 5px;
  cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center;
  line-height: 1; transition: color 120ms, border-color 120ms;
}
.copy-btn:hover { color: var(--text); border-color: var(--text); }
</style>
