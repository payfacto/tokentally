import { defineStore } from 'pinia'
import type { Pricing } from '../composables/useWails'

export const useAppStore = defineStore('app', {
  state: () => ({
    plan: 'api' as string,
    pricing: null as Pricing | null,
    currency: 'CAD' as string,
    exchangeRate: 1.0 as number,
    lastScan: 0 as number,
  }),
  actions: {
    async boot() {
      const resp = await window.go.app.App.GetPlan() as { plan: string; pricing: Pricing; currency: string; exchange_rate: number }
      this.plan = resp.plan
      this.pricing = resp.pricing
      this.currency = resp.currency || 'CAD'
      this.exchangeRate = resp.exchange_rate || 1.0
    },
  },
})
