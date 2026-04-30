import type { Chunk, Session } from './lib/types'
import type { ModelRate, PlanEntry, PlanResponse, ServiceStatus, ScanResult } from './composables/useWails'

declare global {
  interface Window {
    go: {
      app: {
        App: {
          // Build info
          GetVersion(): Promise<string>
          // Sessions
          GetSessions(limit: number, since: string, until: string): Promise<Session[]>
          GetSessionsByProject(limit: number, projectSlug: string, since: string, until: string): Promise<Session[]>
          GetSessionChunks(sessionId: string): Promise<Chunk[]>
          SaveHTMLExport(html: string, filename: string): Promise<string>
          // Overview / dashboard data
          GetOverview(since: string, until: string): Promise<unknown>
          GetPrompts(limit: number, sort: string): Promise<unknown[]>
          GetProjects(since: string, until: string): Promise<unknown[]>
          GetTools(since: string, until: string): Promise<unknown[]>
          GetDaily(since: string, until: string): Promise<unknown[]>
          GetByModel(since: string, until: string): Promise<unknown[]>
          GetSkills(since: string, until: string): Promise<unknown[]>
          GetTips(): Promise<unknown[]>
          DismissTip(key: string): Promise<void>
          ScanNow(): Promise<ScanResult>
          // Plan
          GetPlan(): Promise<PlanResponse>
          SetPlan(plan: string): Promise<void>
          // Settings — pricing
          GetPricingModels(): Promise<ModelRate[]>
          GetPricingPlans(): Promise<PlanEntry[]>
          ResetPricingToDefaults(): Promise<void>
          UpsertPricingModel(
            name: string, tier: string, input: number, output: number,
            cacheRead: number, cache5m: number, cache1h: number
          ): Promise<void>
          DeletePricingModel(name: string): Promise<void>
          UpsertPricingPlan(key: string, label: string, monthly: number): Promise<void>
          DeletePricingPlan(key: string): Promise<void>
          // Settings — currency
          GetCurrency(): Promise<string>
          GetExchangeRates(): Promise<Record<string, number>>
          GetExchangeApiKey(): Promise<string>
          SetCurrency(cur: string): Promise<void>
          SetExchangeRate(currency: string, rate: number): Promise<void>
          SetExchangeApiKey(key: string): Promise<void>
          RefreshExchangeRates(): Promise<Record<string, number>>
          // Settings — data
          GetRetentionDays(): Promise<number>
          SetRetentionDays(days: number): Promise<void>
          PurgeOlderThan(days: number): Promise<number>
          // Settings — service
          GetServiceStatus(): Promise<ServiceStatus>
          InstallService(): Promise<void>
          UninstallService(): Promise<void>
        }
      }
    }
    runtime: {
      EventsOn(event: string, cb: (...args: unknown[]) => void): void
      EventsOff(event: string): void
    }
  }
}
