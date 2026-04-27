import { createApp, type App as VueApp } from 'vue'
import AppVue from './App.vue'

interface SessionInspectorAPI {
  mount(el: HTMLElement, hash: string): void
  unmount(): void
}

declare global {
  interface Window {
    SessionInspector: SessionInspectorAPI
  }
}

let instance: VueApp | null = null

window.SessionInspector = {
  mount(el: HTMLElement, hash: string) {
    instance = createApp(AppVue, { initialHash: hash })
    instance.mount(el)
  },
  unmount() {
    instance?.unmount()
    instance = null
  },
}
