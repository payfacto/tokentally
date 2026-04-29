import { computed, type ComputedRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { RANGES } from '../lib/api'

export function useRange(): {
  range: ComputedRef<{ key: string; label: string; days: number | null }>
  rangeKey: ComputedRef<string>
  setRange: (key: string) => void
} {
  const route = useRoute()
  const router = useRouter()

  const DEFAULT_RANGE_KEY = '30d'

  const rangeKey = computed(() => (route.query.range as string) || DEFAULT_RANGE_KEY)

  const range = computed(
    () => RANGES.find(r => r.key === rangeKey.value) ?? RANGES.find(r => r.key === DEFAULT_RANGE_KEY)!,
  )

  function setRange(key: string) {
    router.push({ query: { ...route.query, range: key } })
  }

  return { range, rangeKey, setRange }
}
