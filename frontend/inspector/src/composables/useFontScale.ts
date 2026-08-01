import { computed, onMounted, onUnmounted, readonly, ref, type ComputedRef, type DeepReadonly, type Ref } from 'vue'
import { clampFontScale, FONT_SCALE_DEFAULT, FONT_SCALE_STEP, parseFontScale, resolveFontScaleShortcut } from '../lib/fontScale'

export function useFontScale(storageKey: string): {
  fontScale: DeepReadonly<Ref<number>>
  fontScalePercent: ComputedRef<number>
  stepFontScale: (delta: number) => void
  resetFontScale: () => void
} {
  function readFontScale(): number {
    try {
      return parseFontScale(localStorage.getItem(storageKey))
    } catch {
      return FONT_SCALE_DEFAULT
    }
  }

  const fontScale = ref(readFontScale())
  const fontScalePercent = computed(() => Math.round(fontScale.value * 100))

  function setFontScale(value: number) {
    fontScale.value = value
    try {
      localStorage.setItem(storageKey, String(value))
    } catch {
      // localStorage unavailable — keep the in-memory value, lose persistence
    }
  }

  function stepFontScale(delta: number) {
    setFontScale(clampFontScale(fontScale.value, delta))
  }

  function resetFontScale() {
    setFontScale(FONT_SCALE_DEFAULT)
  }

  function handleKeydown(e: KeyboardEvent) {
    const action = resolveFontScaleShortcut(e)
    if (!action) return
    e.preventDefault()
    switch (action) {
      case 'increase': stepFontScale(FONT_SCALE_STEP); break
      case 'decrease': stepFontScale(-FONT_SCALE_STEP); break
      case 'reset': resetFontScale(); break
    }
  }

  onMounted(() => window.addEventListener('keydown', handleKeydown))
  onUnmounted(() => window.removeEventListener('keydown', handleKeydown))

  return { fontScale: readonly(fontScale), fontScalePercent, stepFontScale, resetFontScale }
}
