import { describe, expect, it } from 'vitest'
import { clampFontScale, FONT_SCALE_DEFAULT, FONT_SCALE_MAX, FONT_SCALE_MIN, FONT_SCALE_STEP, parseFontScale, resolveFontScaleShortcut } from './fontScale'

describe('clampFontScale', () => {
  it('steps up by one increment', () => {
    expect(clampFontScale(1.0, FONT_SCALE_STEP)).toBeCloseTo(1.1)
  })

  it('steps down by one increment', () => {
    expect(clampFontScale(1.0, -FONT_SCALE_STEP)).toBeCloseTo(0.9)
  })

  it('clamps at the minimum', () => {
    expect(clampFontScale(FONT_SCALE_MIN, -FONT_SCALE_STEP)).toBeCloseTo(FONT_SCALE_MIN)
  })

  it('clamps at the maximum', () => {
    expect(clampFontScale(FONT_SCALE_MAX, FONT_SCALE_STEP)).toBeCloseTo(FONT_SCALE_MAX)
  })

  it('clamps an overshooting step down to the minimum', () => {
    expect(clampFontScale(0.75, -FONT_SCALE_STEP)).toBeCloseTo(FONT_SCALE_MIN)
  })

  it('clamps an overshooting step up to the maximum', () => {
    expect(clampFontScale(1.95, FONT_SCALE_STEP)).toBeCloseTo(FONT_SCALE_MAX)
  })

  it('exposes a default of 1.0', () => {
    expect(FONT_SCALE_DEFAULT).toBe(1.0)
  })
})

describe('parseFontScale', () => {
  it('returns the default for null (no stored value)', () => {
    expect(parseFontScale(null)).toBe(FONT_SCALE_DEFAULT)
  })

  it('returns the default for garbage input', () => {
    expect(parseFontScale('not-a-number')).toBe(FONT_SCALE_DEFAULT)
  })

  it('returns the default for an empty string', () => {
    expect(parseFontScale('')).toBe(FONT_SCALE_DEFAULT)
  })

  it('parses a valid in-range value', () => {
    expect(parseFontScale('1.3')).toBeCloseTo(1.3)
  })

  it('clamps an out-of-range stored value to the maximum', () => {
    expect(parseFontScale('5')).toBeCloseTo(FONT_SCALE_MAX)
  })

  it('clamps an out-of-range stored value to the minimum', () => {
    expect(parseFontScale('-2')).toBeCloseTo(FONT_SCALE_MIN)
  })
})

describe('resolveFontScaleShortcut', () => {
  it('returns null when no modifier key is held', () => {
    expect(resolveFontScaleShortcut({ ctrlKey: false, metaKey: false, key: '=' })).toBeNull()
  })

  it('resolves Ctrl+= to increase', () => {
    expect(resolveFontScaleShortcut({ ctrlKey: true, metaKey: false, key: '=' })).toBe('increase')
  })

  it('resolves Cmd++ to increase', () => {
    expect(resolveFontScaleShortcut({ ctrlKey: false, metaKey: true, key: '+' })).toBe('increase')
  })

  it('resolves Ctrl+- to decrease', () => {
    expect(resolveFontScaleShortcut({ ctrlKey: true, metaKey: false, key: '-' })).toBe('decrease')
  })

  it('resolves Cmd+0 to reset', () => {
    expect(resolveFontScaleShortcut({ ctrlKey: false, metaKey: true, key: '0' })).toBe('reset')
  })

  it('returns null for an unrelated key with a modifier held', () => {
    expect(resolveFontScaleShortcut({ ctrlKey: true, metaKey: false, key: 'a' })).toBeNull()
  })
})
