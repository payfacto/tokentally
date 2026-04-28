import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { RANGES } from '../lib/api'

export function useRange() {
  const route = useRoute()
  const router = useRouter()

  const rangeKey = computed(() => (route.query.range as string) || '30d')

  const range = computed(() => RANGES.find(r => r.key === rangeKey.value) || RANGES[1])

  function setRange(key: string) {
    router.push({ query: { ...route.query, range: key } })
  }

  return { range, rangeKey, setRange }
}
