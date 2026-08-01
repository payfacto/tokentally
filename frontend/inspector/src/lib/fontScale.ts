export const FONT_SCALE_DEFAULT = 1.0
export const FONT_SCALE_MIN = 0.7
export const FONT_SCALE_MAX = 2.0
export const FONT_SCALE_STEP = 0.1

export function clampFontScale(current: number, delta: number): number {
  const next = Math.round((current + delta) * 10) / 10
  return Math.min(FONT_SCALE_MAX, Math.max(FONT_SCALE_MIN, next))
}

export function parseFontScale(raw: string | null): number {
  const parsed = raw === null || raw === '' ? NaN : parseFloat(raw)
  return Number.isFinite(parsed) ? clampFontScale(parsed, 0) : FONT_SCALE_DEFAULT
}

export type FontScaleAction = 'increase' | 'decrease' | 'reset'

export function resolveFontScaleShortcut(event: { ctrlKey: boolean; metaKey: boolean; key: string }): FontScaleAction | null {
  if (!(event.ctrlKey || event.metaKey)) return null
  if (event.key === '=' || event.key === '+') return 'increase'
  if (event.key === '-') return 'decrease'
  if (event.key === '0') return 'reset'
  return null
}
