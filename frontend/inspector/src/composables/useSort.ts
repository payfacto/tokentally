import { computed, type ComputedRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'

export const SORTS = [
  { key: 'tokens', label: 'Most tokens' },
  { key: 'recent', label: 'Most recent' },
]

export function useSort(): {
  sort: ComputedRef<{ key: string; label: string }>
  sortKey: ComputedRef<string>
  setSort: (key: string) => void
} {
  const route = useRoute()
  const router = useRouter()

  const sortKey = computed(() => (route.query.sort as string) || 'tokens')

  const sort = computed(() => SORTS.find(s => s.key === sortKey.value) || SORTS[0])

  function setSort(key: string) {
    router.push({ query: { ...route.query, sort: key } })
  }

  return { sort, sortKey, setSort }
}
