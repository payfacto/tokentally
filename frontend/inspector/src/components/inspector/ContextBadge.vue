<script setup lang="ts">
import type { ContextAttrib } from '../../lib/types'
import { fmt } from '../../lib/fmt'
const props = defineProps<{ attrib: ContextAttrib; inputTokens: number }>()
const pct = (n: number) => props.inputTokens > 0
  ? Math.max(2, Math.round(n / props.inputTokens * 100)) + '%'
  : '0%'
</script>

<template>
  <div class="ctx-badge" :title="`tool:${fmt.tok(attrib.toolOutput)} think:${fmt.tok(attrib.thinking)} user:${fmt.tok(attrib.userText)}`">
    <span class="ctx-seg tool" :style="{ width: pct(attrib.toolOutput) }" />
    <span class="ctx-seg think" :style="{ width: pct(attrib.thinking) }" />
    <span class="ctx-seg user" :style="{ width: pct(attrib.userText) }" />
    <span class="muted" style="font-size:10px;font-family:var(--mono);margin-left:6px">{{ fmt.tok(inputTokens) }}</span>
  </div>
</template>

<style scoped>
.ctx-badge { display: flex; align-items: center; width: 80px; }
.ctx-seg { height: 5px; display: inline-block; }
.ctx-seg.tool  { background: var(--accent); }
.ctx-seg.think { background: var(--muted); }
.ctx-seg.user  { background: var(--good); }
.muted { color: var(--muted); }
</style>
