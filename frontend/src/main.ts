import { createPinia } from 'pinia'
import { createApp } from 'vue'

import './styles/main.css'
import './themes'

import App from './App.vue'
import { setUnauthorizedHandler } from './api/client'
import { i18n } from './i18n'
import { router } from './router'
import { useAuth } from './stores/auth'

const app = createApp(App)
app.use(createPinia())
app.use(i18n)
app.use(router)

setUnauthorizedHandler(() => {
  useAuth().clear()
  if (router.currentRoute.value.path !== '/login') {
    router.push({ path: '/login', query: { next: router.currentRoute.value.fullPath } })
  }
})

app.mount('#app')
