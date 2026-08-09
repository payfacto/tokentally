import { ref, watch, type Ref } from 'vue'
import type { Chunk, Session } from '../lib/types'
import { App } from '../bindings/tokentally/app'

export interface OverviewTotals {
  sessions: number; turns: number; input_tokens: number; output_tokens: number
  cache_read_tokens: number; cache_create_5m_tokens: number; cache_create_1h_tokens: number
  cost_usd: number
}

export interface PricingPlan { monthly: number; label: string }
export interface Pricing { plans: Record<string, PricingPlan> }

export interface PlanResponse {
  plan: string; pricing: Pricing; currency: string; exchange_rate: number
}

export interface ModelRate {
  model_name: string; tier: string; input: number; output: number
  cache_read: number; cache_create_5m: number; cache_create_1h: number
}

export interface PlanEntry { plan_key: string; label: string; monthly: number }

export interface ServiceStatus { installed: boolean; state: string }

export interface ScanResult { Messages: number; Files: number }

export interface MarkdownFolder { path: string; label: string }

export interface MarkdownFile {
  folder_path: string; folder_label: string; filename: string
  full_path: string; size: number; mod_time: string
}

function rangeToSince(range: string): string {
  const days: Record<string, number> = { today: 1, '7d': 7, '30d': 30 }
  const d = days[range]
  if (!d) return ''
  return new Date(Date.now() - d * 86_400_000).toISOString()
}

export function useSessionList(range: Ref<string>, project?: Ref<string>): {
  data: Ref<Session[]>; isLoading: Ref<boolean>; error: Ref<string | null>; refetch: () => Promise<void>
} {
  const data = ref<Session[]>([])
  const isLoading = ref(false)
  const error = ref<string | null>(null)

  async function refetch() {
    isLoading.value = true
    error.value = null
    try {
      const since = rangeToSince(range.value)
      const slug = project?.value || ''
      // These v3 bindings return a generic map shape (Go's return type is
      // map[string]any); cast to the concrete Session shape this app relies on.
      data.value = slug
        ? ((await App.GetSessionsByProject(200, slug, since, '')) as unknown as Session[] | null) ?? []
        : ((await App.GetSessions(200, since, '')) as unknown as Session[] | null) ?? []
    } catch (e) {
      error.value = String(e)
    } finally {
      isLoading.value = false
    }
  }

  watch(range, refetch, { immediate: true })
  if (project) watch(project, refetch)
  return { data, isLoading, error, refetch }
}

export function useSessionChunks(id: Ref<string>): {
  data: Ref<Chunk[]>; visibleCount: Ref<number>; isLoading: Ref<boolean>
  error: Ref<string | null>; refetch: () => Promise<void>; cancelReveal: () => void
} {
  const data = ref<Chunk[]>([])
  const visibleCount = ref(20)
  const isLoading = ref(false)
  const error = ref<string | null>(null)
  let rafHandle: number | undefined

  function cancelReveal() {
    if (rafHandle !== undefined) { cancelAnimationFrame(rafHandle); rafHandle = undefined }
  }

  function revealProgressively(total: number) {
    if (visibleCount.value >= total) return
    visibleCount.value = Math.min(visibleCount.value + 20, total)
    rafHandle = requestAnimationFrame(() => revealProgressively(total))
  }

  async function refetch() {
    cancelReveal()
    if (!id.value) { data.value = []; visibleCount.value = 20; return }
    isLoading.value = true
    error.value = null
    visibleCount.value = 20
    try {
      // GetSessionChunks' generated model has a bare `type: string` field;
      // cast to the narrower Chunk union this app's rendering code switches on.
      data.value = ((await App.GetSessionChunks(id.value)) as unknown as Chunk[] | null) ?? []
      revealProgressively(data.value.length)
    } catch (e) {
      error.value = String(e)
    } finally {
      isLoading.value = false
    }
  }

  watch(id, refetch, { immediate: true })
  return { data, visibleCount, isLoading, error, refetch, cancelReveal }
}
