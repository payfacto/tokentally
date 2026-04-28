// Temporary stub — replaced in Task 4
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHashHistory } from 'vue-router'
import { defineComponent } from 'vue'

const Stub = defineComponent({ template: '<div>loading</div>' })
const router = createRouter({ history: createWebHashHistory(), routes: [{ path: '/:p(.*)', component: Stub }] })
createApp(Stub).use(createPinia()).use(router).mount('#app')
