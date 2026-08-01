// @vitest-environment jsdom
import { defineComponent, h } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it } from 'vitest'
import { useFontScale } from './useFontScale'

const STORAGE_KEY = 'tt.testFontScale'

function mountFontScale(): { wrapper: VueWrapper; api: ReturnType<typeof useFontScale> } {
  let api!: ReturnType<typeof useFontScale>
  const wrapper = mount(
    defineComponent({
      setup() {
        api = useFontScale(STORAGE_KEY)
        return () => h('div')
      },
    }),
  )
  return { wrapper, api }
}

function pressCtrl(key: string) {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, ctrlKey: true }))
}

describe('useFontScale', () => {
  beforeEach(() => localStorage.clear())

  it('falls back to the default scale when localStorage holds garbage', () => {
    localStorage.setItem(STORAGE_KEY, 'not-a-number')
    const { api } = mountFontScale()
    expect(api.fontScale.value).toBe(1.0)
  })

  it('increases and persists the scale on Ctrl+= after mount', () => {
    const { api } = mountFontScale()
    pressCtrl('=')
    expect(api.fontScale.value).toBeCloseTo(1.1)
    expect(localStorage.getItem(STORAGE_KEY)).toBe('1.1')
  })

  it('resets to the default on Ctrl+0', () => {
    const { api } = mountFontScale()
    pressCtrl('+')
    pressCtrl('0')
    expect(api.fontScale.value).toBe(1.0)
  })

  it('stops responding to the keyboard shortcut after unmount', () => {
    const { wrapper, api } = mountFontScale()
    wrapper.unmount()
    pressCtrl('=')
    expect(api.fontScale.value).toBe(1.0)
  })
})
