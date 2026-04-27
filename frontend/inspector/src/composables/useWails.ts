import { ref, watch, type Ref } from 'vue'
import type { Chunk, Session } from '../lib/types'

declare global {
  interface Window {
    go: {
      app: {
        App: {
          GetSessions(limit: number, since: string, until: string): Promise<Session[]>
          GetSessionChunks(sessionId: string): Promise<Chunk[]>
          SaveHTMLExport(html: string): Promise<string>
        }
      }
    }
    runtime: {
      EventsOn(event: string, cb: (...args: unknown[]) => void): void
      EventsOff(event: string): void
    }
  }
}

function rangeToSince(range: string): string {
  const days: Record<string, number> = { today: 1, '7d': 7, '30d': 30 }
  const d = days[range]
  if (!d) return ''
  return new Date(Date.now() - d * 86_400_000).toISOString()
}

export function useSessionList(range: Ref<string>) {
  const data = ref<Session[]>([])
  const isLoading = ref(false)
  const error = ref<string | null>(null)

  async function refetch() {
    isLoading.value = true
    error.value = null
    try {
      const since = rangeToSince(range.value)
      data.value = (await window.go.app.App.GetSessions(200, since, '')) ?? []
    } catch (e) {
      error.value = String(e)
    } finally {
      isLoading.value = false
    }
  }

  watch(range, refetch, { immediate: true })
  return { data, isLoading, error, refetch }
}

export function useSessionChunks(id: Ref<string>) {
  const data = ref<Chunk[]>([])
  const visibleCount = ref(20)
  const isLoading = ref(false)
  const error = ref<string | null>(null)

  function revealProgressively(total: number) {
    if (visibleCount.value >= total) return
    visibleCount.value = Math.min(visibleCount.value + 20, total)
    requestAnimationFrame(() => revealProgressively(total))
  }

  async function refetch() {
    if (!id.value) { data.value = []; visibleCount.value = 20; return }
    isLoading.value = true
    error.value = null
    visibleCount.value = 20
    try {
      data.value = (await window.go.app.App.GetSessionChunks(id.value)) ?? []
      revealProgressively(data.value.length)
    } catch (e) {
      error.value = String(e)
    } finally {
      isLoading.value = false
    }
  }

  watch(id, refetch, { immediate: true })
  return { data, visibleCount, isLoading, error, refetch }
}
