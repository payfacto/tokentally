<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { api } from '../lib/api'
import { fmt } from '../lib/fmt'
import { useAppStore } from '../stores/app'

const store = useAppStore()

interface ProjectRow {
  project_slug: string; project_name: string
  sessions: number; turns: number; billable_tokens: number; cache_read_tokens: number
  last_active: string
}

const rows = ref<ProjectRow[]>([])

async function fetchRows() {
  rows.value = await api<ProjectRow[]>('/api/projects')
}

function cacheHitPct(r: ProjectRow): string {
  const total = r.billable_tokens + r.cache_read_tokens
  if (!total) return '—'
  return ((r.cache_read_tokens / total) * 100).toFixed(0) + '%'
}

onMounted(fetchRows)
watch(() => store.lastScan, fetchRows)
</script>

<template>
  <div style="padding:20px">
    <div class="card">
      <h2>Projects</h2>
      <p class="muted" style="margin:-8px 0 14px">Sorted by billable token spend. Cache reads are billed cheaper, so high cache-read columns are good.</p>
      <table>
        <thead>
          <tr>
            <th>project</th>
            <th class="num">sessions</th>
            <th class="num">turns</th>
            <th class="num">billable tokens</th>
            <th class="num">cache reads</th>
            <th class="num">cache hit %</th>
            <th class="num">last active</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="r.project_slug">
            <td :title="r.project_slug">{{ r.project_name || r.project_slug }}</td>
            <td class="num">
              <router-link :to="'/sessions?project=' + encodeURIComponent(r.project_slug) + '&name=' + encodeURIComponent(r.project_name || r.project_slug)" class="sessions-link">
                {{ fmt.int(r.sessions) }}
              </router-link>
            </td>
            <td class="num">{{ fmt.int(r.turns) }}</td>
            <td class="num">{{ fmt.int(r.billable_tokens) }}</td>
            <td class="num">{{ fmt.int(r.cache_read_tokens) }}</td>
            <td class="num">{{ cacheHitPct(r) }}</td>
            <td class="num mono">{{ fmt.date(r.last_active) }}</td>
          </tr>
          <tr v-if="!rows.length">
            <td colspan="7" class="muted">no projects</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.sessions-link { color: var(--accent, #e8956d); text-decoration: none; }
.sessions-link:hover { text-decoration: underline; }
.mono { font-family: var(--mono); }
</style>
