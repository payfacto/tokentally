<script setup lang="ts">
defineProps<{ percent: number }>()
const emit = defineEmits<{ decrease: []; increase: []; reset: [] }>()
</script>

<template>
  <div class="font-scale-controls">
    <button class="font-scale-btn" title="Decrease font size (Ctrl/Cmd -)" @click="emit('decrease')">A-</button>
    <button class="font-scale-pct" title="Reset font size (Ctrl/Cmd 0)" @click="emit('reset')">{{ percent }}%</button>
    <button class="font-scale-btn" title="Increase font size (Ctrl/Cmd +)" @click="emit('increase')">A+</button>
  </div>
</template>

<style scoped>
.font-scale-controls { display: flex; align-items: center; gap: 2px; }
.font-scale-btn, .font-scale-pct {
  background: transparent; border: 1px solid var(--border);
  border-radius: 4px; padding: 3px 7px;
  cursor: pointer; color: var(--muted); font-size: 11px;
  line-height: 1; transition: color 120ms, border-color 120ms;
}
.font-scale-btn:hover, .font-scale-pct:hover { color: var(--text); border-color: var(--text); }
.font-scale-pct { font-family: var(--mono); min-width: 36px; text-align: center; }
</style>
