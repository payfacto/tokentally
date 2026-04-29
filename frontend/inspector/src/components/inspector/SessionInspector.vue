<script setup lang="ts">
import type { Chunk } from '../../lib/types'
import UserTurn from './UserTurn.vue'
import AITurn from './AITurn.vue'
import CompactBoundary from './CompactBoundary.vue'
import SystemMessage from './SystemMessage.vue'

defineProps<{ chunks: Chunk[]; depth?: number }>()
</script>

<template>
  <div class="session-inspector">
    <template v-for="(chunk, i) in chunks" :key="i">
      <UserTurn v-if="chunk.type === 'user' && chunk.text?.trim()" :chunk="chunk" />
      <AITurn v-else-if="chunk.type === 'ai'" :chunk="chunk" :depth="depth" />
      <CompactBoundary v-else-if="chunk.type === 'compact'" :chunk="chunk" />
      <SystemMessage v-else-if="chunk.type === 'system'" :chunk="chunk" />
    </template>
  </div>
</template>

<style scoped>
.session-inspector { display: flex; flex-direction: column; }
</style>
