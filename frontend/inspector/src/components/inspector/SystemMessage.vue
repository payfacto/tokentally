<script setup lang="ts">
import type { Chunk } from '../../lib/types'
defineProps<{ chunk: Chunk }>()
</script>

<template>
  <div class="system-msg muted">
    <span style="font-size:10px;font-family:var(--mono)">system</span>
    <span style="font-size:12px;margin-left:8px">{{ chunk.text }}</span>
  </div>
</template>

<style scoped>
.system-msg { padding: 6px 0; border-bottom: 1px solid var(--border); display: flex; align-items: baseline; }
.muted { color: var(--muted); }
</style>
