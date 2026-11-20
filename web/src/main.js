import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import { useSessionStore } from './stores/session'
import './style.css'

const pinia = createPinia()
const app = createApp(App)
app.use(pinia)

useSessionStore(pinia).bootstrap().finally(() => {
  app.use(router)
  app.mount('#app')
})
