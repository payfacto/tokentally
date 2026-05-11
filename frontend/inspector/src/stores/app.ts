import { defineStore } from 'pinia'
import type { Pricing, PlanResponse } from '../composables/useWails'

const SHOW_LMSGO_KEY = 'tt.showLmsgo'

function readShowLmsgo(): boolean {
  try {
    return localStorage.getItem(SHOW_LMSGO_KEY) === '1'
  } catch {
    return false
  }
}

export const useAppStore = defineStore('app', {
  state: () => ({
    plan: 'api' as string,
    pricing: null as Pricing | null,
    currency: 'CAD' as string,
    exchangeRate: 1.0,
    lastScan: 0,
    showLmsgo: readShowLmsgo(),
  }),
  actions: {
    recordScan() {
      this.lastScan = Date.now()
    },
    setShowLmsgo(value: boolean) {
      this.showLmsgo = value
      try {
        localStorage.setItem(SHOW_LMSGO_KEY, value ? '1' : '0')
      } catch {
        // localStorage unavailable — keep the in-memory value, lose persistence
      }
    },
    async boot() {
      const resp = await window.go.app.App.GetPlan()
      this.plan = resp.plan
      this.pricing = resp.pricing
      this.currency = resp.currency || 'CAD'
      this.exchangeRate = resp.exchange_rate ?? 1.0
    },
  },
})
