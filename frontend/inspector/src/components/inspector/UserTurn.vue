<script setup lang="ts">
import { computed } from 'vue'
import type { Chunk } from '../../lib/types'
import { copyMarkdown } from '../../lib/clipboard'
import { fmt } from '../../lib/fmt'
import { renderMarkdown } from '../../lib/markup'

const props = defineProps<{ chunk: Chunk }>()

function copyChunk(e: MouseEvent) {
  if (!(e.currentTarget instanceof HTMLElement)) return
  const md = `**User** · ${fmt.time(props.chunk.timestamp)}\n\n${props.chunk.text ?? ''}`
  copyMarkdown(md, e.currentTarget)
}

const renderedText = computed<string>(() => renderMarkdown(props.chunk.text ?? ''))
</script>

<template>
  <div class="user-turn">
    <div class="turn-header">
      <!-- hook -->
      <span v-if="chunk.msgType === 'attachment'" class="tag-hook">
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
        hook result
      </span>
      <!-- subagent -->
      <span v-else-if="chunk.isSidechain" class="tag-subagent">
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 8V4H8"/><rect width="16" height="12" x="4" y="8" rx="2"/><path d="M2 14h2"/><path d="M20 14h2"/><path d="M15 13v2"/><path d="M9 13v2"/></svg>
        subagent
      </span>
      <!-- regular user -->
      <span v-else class="badge" style="font-size:10px">you</span>
      <span class="muted" style="font-family:var(--mono);font-size:11px">{{ fmt.time(chunk.timestamp) }}</span>
    </div>
    <div class="turn-text markdown-body" v-html="renderedText"></div>
    <button class="copy-btn" title="Copy as Markdown" @click="copyChunk">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
    </button>
  </div>
</template>

<style scoped>
.user-turn { padding: 12px 0; border-bottom: 1px solid var(--border); position: relative; }
.turn-header { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.turn-text { font-size: 13px; line-height: 1.6; word-break: break-word; padding-bottom: 20px; }
.muted { color: var(--muted); }
.tag-subagent, .tag-hook {
  display: inline-flex; align-items: center; gap: 4px;
  font-size: 11px; padding: 2px 7px; border-radius: 10px;
  font-weight: 500; white-space: nowrap;
}
.tag-subagent { background: color-mix(in srgb, var(--accent) 12%, transparent); color: var(--accent-2); }
.tag-hook     { background: color-mix(in srgb, var(--warn)   12%, transparent); color: var(--warn); }
.copy-btn {
  position: absolute; bottom: 6px; right: 0;
  background: transparent; border: 1px solid var(--border);
  border-radius: 4px; padding: 4px 5px;
  cursor: pointer; color: var(--muted);
  display: flex; align-items: center; justify-content: center;
  line-height: 1; transition: color 120ms, border-color 120ms;
}
.copy-btn:hover { color: var(--text); border-color: var(--text); }

/* Markdown prose styles — compact for 13px UI */
.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3),
.markdown-body :deep(h4) {
  font-weight: 600; line-height: 1.3; margin: 10px 0 4px; color: var(--text);
}
.markdown-body :deep(h1) { font-size: 16px; }
.markdown-body :deep(h2) { font-size: 15px; }
.markdown-body :deep(h3) { font-size: 14px; }
.markdown-body :deep(h4) { font-size: 13px; }

.markdown-body :deep(p) { margin: 0 0 8px; }
.markdown-body :deep(p:last-child) { margin-bottom: 0; }
.markdown-body :deep(strong) { font-weight: 600; }
.markdown-body :deep(em) { font-style: italic; }

.markdown-body :deep(code) {
  font-family: var(--mono); font-size: 11.5px;
  background: rgba(0, 0, 0, 0.06); border-radius: 3px; padding: 1px 4px;
}
.markdown-body :deep(pre) {
  background: #2a1f14; border-radius: 6px; padding: 10px 12px; margin: 6px 0; overflow-x: auto;
}
.markdown-body :deep(pre > code) {
  background: none; padding: 0; border-radius: 0; font-size: 11.5px; color: #e8d5bc; display: block;
}
.markdown-body :deep(ul),
.markdown-body :deep(ol) { padding-left: 20px; margin: 4px 0 8px; }
.markdown-body :deep(li) { margin: 2px 0; }
.markdown-body :deep(li:last-child) { margin-bottom: 0; }
.markdown-body :deep(blockquote) {
  border-left: 3px solid var(--border); color: var(--muted); padding: 2px 0 2px 10px; margin: 6px 0;
}
.markdown-body :deep(a) { color: var(--accent); text-decoration: none; }
.markdown-body :deep(a:hover) { text-decoration: underline; }

/* System injection tags — muted pill */
.markdown-body :deep(.sys-tag) {
  display: inline-block; font-family: var(--mono); font-size: 10px;
  color: var(--muted); background: rgba(122, 92, 58, 0.08);
  border: 1px solid var(--border); border-radius: 3px;
  padding: 1px 5px; margin: 1px 0; opacity: 0.75;
}

/* System-injected content block — dimmed to distinguish from user-typed text */
.markdown-body :deep(.sys-block) {
  opacity: 0.55;
  font-size: 11.5px;
  border-left: 2px solid var(--border);
  padding-left: 8px;
  margin: 4px 0 8px;
}
.markdown-body :deep(.sys-block .sys-tag) { opacity: 1; }
</style>
