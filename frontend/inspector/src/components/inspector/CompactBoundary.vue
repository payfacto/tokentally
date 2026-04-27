<script setup lang="ts">
import type { Chunk } from '../../lib/types'
const props = defineProps<{ chunk: Chunk }>()
const fmtTok = (n?: number) => n ? (n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n)) : '?'
</script>

<template>
  <div class="compact-boundary">
    <div class="compact-line" />
    <div class="compact-label">
      ⚡ Context compacted — {{ fmtTok(chunk.tokensBefore) }} → {{ fmtTok(chunk.tokensAfter) }} tokens
    </div>
    <div class="compact-line" />
  </div>
</template>

<style scoped>
.compact-boundary { display: flex; align-items: center; gap: 10px; padding: 12px 0; }
.compact-line { flex: 1; height: 1px; background: var(--accent); opacity: 0.4; }
.compact-label { font-size: 11px; font-family: var(--mono); color: var(--accent); white-space: nowrap; }
</style>
