<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { api, sinceIso, RANGES } from '../lib/api'
import { barChart, disposeChart } from '../lib/charts'
import { fmt } from '../lib/fmt'
import { useRange } from '../composables/useRange'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const { range, rangeKey, setRange } = useRange()

const SKILL_NAME_MAX = 25
const TOP_SKILLS_LIMIT = 12

interface SkillRow {
  skill: string; invocations: number; tokens_per_call: number | null
  sessions: number; last_used: string
}

const skills = ref<SkillRow[]>([])
const chSkills = ref<HTMLElement | null>(null)

const totalInvocations = computed(() => skills.value.reduce((s, r) => s + r.invocations, 0))

async function fetchAll() {
  const since = sinceIso(range.value)
  const url = '/api/skills' + (since ? '?since=' + encodeURIComponent(since) : '')
  skills.value = await api<SkillRow[]>(url)
  await nextTick()
  if (chSkills.value) {
    const top = skills.value.slice(0, TOP_SKILLS_LIMIT)
    barChart(chSkills.value, {
      categories: top.map(t => t.skill.length > SKILL_NAME_MAX ? t.skill.slice(0, SKILL_NAME_MAX) + '…' : t.skill),
      values: top.map(t => t.invocations),
      color: '#3FB68B',
    })
  }
}

onMounted(fetchAll)
onUnmounted(() => disposeChart(chSkills.value))
watch([rangeKey, () => store.lastScan], fetchAll)
</script>

<template>
  <div style="padding:20px">
    <div class="flex" style="margin-bottom:14px">
      <h2 style="margin:0;font-size:16px;letter-spacing:-0.01em">Skills</h2>
      <span class="muted" style="font-size:12px">{{ range.days ? `last ${range.days} days` : 'all time' }}</span>
      <div class="spacer"></div>
      <div class="range-tabs" role="tablist">
        <button
          v-for="r in RANGES"
          :key="r.key"
          :class="{ active: r.key === range.key }"
          @click="setRange(r.key)"
        >{{ r.label }}</button>
      </div>
    </div>

    <div class="row cols-2">
      <div class="card kpi"><div class="label">Unique skills used</div><div class="value">{{ fmt.int(skills.length) }}</div></div>
      <div class="card kpi"><div class="label">Total invocations</div><div class="value">{{ fmt.int(totalInvocations) }}</div></div>
    </div>

    <div class="card" style="margin-top:16px">
      <h3>Top skills (by invocations)</h3>
      <div ref="chSkills" style="height:320px"></div>
    </div>

    <div class="card" style="margin-top:16px">
      <h3>All skills</h3>
      <p class="muted" style="margin:-4px 0 14px;font-size:12px">"Tokens per call" is the size of the skill's <code>SKILL.md</code> file — what Claude Code loads into context each time the skill is invoked.</p>
      <table>
        <thead>
          <tr>
            <th>skill</th>
            <th class="num">invocations</th>
            <th class="num">tokens per call</th>
            <th class="num">sessions</th>
            <th>last used</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in skills" :key="s.skill">
            <td><span class="badge">{{ s.skill }}</span></td>
            <td class="num">{{ fmt.int(s.invocations) }}</td>
            <td class="num">
              <span v-if="s.tokens_per_call == null" class="muted">—</span>
              <span v-else>{{ fmt.int(s.tokens_per_call) }}</span>
            </td>
            <td class="num">{{ fmt.int(s.sessions) }}</td>
            <td class="mono">{{ fmt.ts(s.last_used) }}</td>
          </tr>
          <tr v-if="!skills.length">
            <td colspan="5" class="muted">no skills invoked in this range</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
