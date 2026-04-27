<script setup lang="ts">
import { computed } from 'vue'
import type { Chunk, ToolCallChunk } from '../../lib/types'
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

    <div class="token-row muted">
      <span>in {{ fmtTok(chunk.inputTokens) }}</span>
      <span>out {{ fmtTok(chunk.outputTokens) }}</span>
      <span v-if="chunk.cacheRead">cache {{ fmtTok(chunk.cacheRead) }}</span>
    </div>
  </div>
</template>

<style scoped>
.ai-turn { padding: 12px 0; border-bottom: 1px solid var(--border); }
.turn-header { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.spacer { flex: 1; }
.token-row { display: flex; gap: 12px; font-family: var(--mono); font-size: 10px; margin-top: 8px; }
.muted { color: var(--muted); }
</style>
