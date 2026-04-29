import { defineStore } from 'pinia'
import type { Pricing, PlanResponse } from '../composables/useWails'

export const useAppStore = defineStore('app', {
  state: () => ({
    plan: 'api' as string,
    pricing: null as Pricing | null,
    currency: 'CAD' as string,
    exchangeRate: 1.0,
    lastScan: 0,
  }),
  actions: {
    recordScan() {
      this.lastScan = Date.now()
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
