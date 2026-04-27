<script setup lang="ts">
import { computed } from 'vue'
import type { Chunk, ToolCallChunk } from '../../lib/types'
import { copyMarkdown } from '../../lib/clipboard'
import ThinkingBlock from './ThinkingBlock.vue'
import ContextBadge from './ContextBadge.vue'
import ToolCallFrame from './ToolCallFrame.vue'
import GenericViewer from './viewers/GenericViewer.vue'
import ReadViewer from './viewers/ReadViewer.vue'
import WriteViewer from './viewers/WriteViewer.vue'
import DiffViewer from './viewers/DiffViewer.vue'
import BashViewer from './viewers/BashViewer.vue'
import SearchViewer from './viewers/SearchViewer.vue'
import WebViewer from './viewers/WebViewer.vue'
import SubagentTree from './viewers/SubagentTree.vue'

const props = defineProps<{ chunk: Chunk; depth?: number }>()

const fmtTime = (ts: string) => new Date(ts).toLocaleTimeString()
const fmtTok = (n?: number) => {
  if (!n) return '0'
  return n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n)
}

function viewerFor(tc: ToolCallChunk) {
  if (tc.name === 'Read') return ReadViewer
  if (tc.name === 'Write') return WriteViewer
  if (tc.name === 'Edit' || tc.name === 'MultiEdit') return DiffViewer
  if (tc.name === 'Bash') return BashViewer
  if (tc.name === 'Grep' || tc.name === 'Glob') return SearchViewer
  if (tc.name === 'WebFetch' || tc.name === 'WebSearch') return WebViewer
  if ((tc.name === 'Task' || tc.name === 'Agent') && tc.subagentId) return SubagentTree
  return GenericViewer
}

function buildMarkdown(chunk: Chunk): string {
  const ts = fmtTime(chunk.timestamp)
  let md = `**Assistant** · ${ts}\n${fmtTok(chunk.inputTokens)} in · ${fmtTok(chunk.outputTokens)} out`

  if (chunk.thinking) {
    md += `\n\n<details><summary>Thinking</summary>\n\n${chunk.thinking}\n</details>`
  }

  for (const tc of chunk.toolCalls ?? []) {
    const inputStr = JSON.stringify(tc.input, null, 2)
    const errorPrefix = tc.isError ? '⚠ Error:\n' : ''
    md += `\n\n**Tool: \`${tc.name}\`**\n\`\`\`json\n${inputStr}\n\`\`\`\n**Output:**\n\`\`\`\n${errorPrefix}${tc.output ?? ''}\n\`\`\``
  }

  return md
}

function copyChunk(e: MouseEvent) {
  copyMarkdown(buildMarkdown(props.chunk), e.currentTarget as HTMLElement)
}
</script>

<template>
  <div class="ai-turn">
    <div class="turn-header">
      <span class="badge sonnet" style="font-size:10px">claude</span>
      <span class="muted" style="font-family:var(--mono);font-size:11px">{{ fmtTime(chunk.timestamp) }}</span>
      <span class="spacer" />
      <ContextBadge
        v-if="chunk.contextAttrib && chunk.inputTokens"
        :attrib="chunk.contextAttrib"
        :inputTokens="chunk.inputTokens"
      />
    </div>

    <ThinkingBlock v-if="chunk.thinking" :text="chunk.thinking" />

    <div v-for="tc in (chunk.toolCalls ?? [])" :key="tc.id">
      <ToolCallFrame :toolCall="tc">
        <component :is="viewerFor(tc)" :toolCall="tc" :depth="depth ?? 0" />
      </ToolCallFrame>
    </div>

    <div class="turn-footer">
      <div class="token-row muted">
        <span>in {{ fmtTok(chunk.inputTokens) }}</span>
        <span>out {{ fmtTok(chunk.outputTokens) }}</span>
        <span v-if="chunk.cacheRead">cache {{ fmtTok(chunk.cacheRead) }}</span>
      </div>
      <button class="copy-btn" title="Copy as Markdown" @click="copyChunk">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
      </button>
    </div>
  </div>
</template>

<style scoped>
.ai-turn { padding: 12px 0; border-bottom: 1px solid var(--border); }
.turn-header { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.spacer { flex: 1; }
.turn-footer { display: flex; align-items: center; margin-top: 8px; }
.token-row { display: flex; gap: 12px; font-family: var(--mono); font-size: 10px; flex: 1; }
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
