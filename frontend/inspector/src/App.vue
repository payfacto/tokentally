<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { RouterView, RouterLink } from 'vue-router'
import { useAppStore } from './stores/app'
import { fmt } from './lib/fmt'

const store = useAppStore()

const NAV_ROUTES = ['/overview', '/prompts', '/sessions', '/projects', '/skills', '/tips', '/tools', '/calculator', '/settings']

const showFirstRun = ref(false)
const firstRunPlan = ref('api')
const plans = ref<Array<[string, { monthly: number; label: string }]>>([])
const version = ref('')

onMounted(async () => {
  await store.boot()

  try {
    version.value = await window.go.app.App.GetVersion()
  } catch { /* not in Wails env */ }

  if (store.pricing) {
    plans.value = Object.entries(store.pricing.plans)
    firstRunPlan.value = plans.value[0]?.[0] ?? 'api'
  }

  if (!localStorage.getItem('td.plan-set') && plans.value.length) {
    showFirstRun.value = true
  }

  try {
    window.runtime.EventsOn('scan', () => store.recordScan())
  } catch { /* not in Wails env */ }
})

async function saveFirstRun() {
  await window.go.app.App.SetPlan(firstRunPlan.value)
  localStorage.setItem('td.plan-set', '1')
  store.plan = firstRunPlan.value
  showFirstRun.value = false
}
</script>

<template>
  <header class="topbar">
    <div class="brand">
      <img :src="'/web/icon.svg'" class="mascot-logo" alt="">
      <span>Token<span style="color:var(--accent)">Tally</span></span>
      <span v-if="version" class="brand-version">{{ version }}</span>
    </div>
    <nav>
      <RouterLink
        v-for="p in NAV_ROUTES"
        :key="p"
        :to="p"
        active-class="active"
      >{{ p.slice(1) }}</RouterLink>
    </nav>
    <div class="spacer"></div>
    <span class="pill" id="plan-pill">{{ store.pricing?.plans?.[store.plan]?.label ?? store.plan }}</span>
  </header>

  <RouterView />

  <div v-if="showFirstRun" class="modal-overlay">
    <div class="modal first-run-modal">
      <img :src="'/web/mascot.png'" class="first-run-mascot" alt="" />
      <div class="first-run-body">
        <h2>Welcome — pick your plan</h2>
        <p>This sets how costs are displayed. Change it later in Settings.</p>
        <select v-model="firstRunPlan" style="width:100%">
          <option
            v-for="[k, v] in plans"
            :key="k"
            :value="k"
          >{{ v.label }}{{ v.monthly ? ` — ${fmt.money(v.monthly, store.currency, store.exchangeRate)}/mo` : '' }}</option>
        </select>
        <div class="actions">
          <div class="spacer"></div>
          <button class="primary" @click="saveFirstRun">Continue</button>
        </div>
      </div>
    </div>
  </div>
</template>
