<script setup lang="ts">
import type { Chunk } from '../../lib/types'
defineProps<{ chunk: Chunk }>()
const fmtTime = (ts: string) => new Date(ts).toLocaleTimeString()
</script>

<template>
  <div class="user-turn">
    <div class="turn-header">
      <span class="badge" style="font-size:10px">you</span>
      <span class="muted" style="font-family:var(--mono);font-size:11px">{{ fmtTime(chunk.timestamp) }}</span>
    </div>
    <div class="turn-text">{{ chunk.text }}</div>
  </div>
</template>

<style scoped>
.user-turn { padding: 12px 0; border-bottom: 1px solid var(--border); }
.turn-header { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.turn-text { font-size: 13px; line-height: 1.6; white-space: pre-wrap; word-break: break-word; }
.muted { color: var(--muted); }
</style>
