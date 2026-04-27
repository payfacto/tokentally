<script setup lang="ts">
import { ref } from 'vue'
import { useSessionChunks } from '../../../composables/useWails'
import SessionInspector from '../SessionInspector.vue'
import type { ToolCallChunk } from '../../../lib/types'

const props = defineProps<{ toolCall: ToolCallChunk; depth?: number }>()
const expanded = ref(false)
const subId = ref(props.toolCall.subagentId ?? '')
const { data: chunks, isLoading } = useSessionChunks(subId)
</script>

<template>
  <div class="subagent-tree">
    <div class="subagent-header" @click="expanded = !expanded">
      <span class="muted" style="font-size:11px">{{ expanded ? '▾' : '▸' }}</span>
      <span class="muted" style="font-size:11px;font-family:var(--mono)">
        {{ toolCall.subagentName || subId.slice(0, 8) }}
      </span>
    </div>
    <div v-if="expanded" class="subagent-body">
      <div v-if="isLoading" class="skeleton" style="height:40px" />
      <SessionInspector v-else :chunks="chunks" :depth="(props.depth ?? 0) + 1" />
    </div>
  </div>
</template>

<style scoped>
.subagent-tree { }
.subagent-header { display: flex; align-items: center; gap: 6px; cursor: pointer; padding: 4px 0; user-select: none; }
.subagent-body { padding-left: 16px; border-left: 2px solid var(--border); margin-left: 8px; margin-top: 6px; }
.skeleton { background: var(--panel); border-radius: 4px; animation: pulse 1.5s infinite; }
@keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.5; } }
.muted { color: var(--muted); }
</style>
