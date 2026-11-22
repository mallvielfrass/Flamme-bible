<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSessionStore } from './stores/session'
import AiSettingsModal from './components/AiSettingsModal.vue'

const session = useSessionStore()
const route = useRoute()
const router = useRouter()
const aiOpen = ref(false)

const authScreen = computed(() => route.path === '/login' || route.path === '/register')
const songsTo = computed(() => (session.token ? '/songbooks' : '/catalog'))
const pendingAdmin = computed(() => {
  const summary = session.adminSummary
  if (!summary) return 0
  return Number(summary.pending_songbooks || 0) + Number(summary.pending_modules || 0)
})

function on(name) {
  const path = route.path
  if (name === 'songs') return path === '/songbooks' || path === '/catalog'
  if (name === 'plans') return path === '/plans'
  if (name === 'ai') return path === '/ai' || path.startsWith('/ai/')
  if (name === 'spaces') return path === '/communities' || path.startsWith('/dimensions/')
  if (name === 'download') return path === '/download'
  if (name === 'admin') return path.startsWith('/admin')
  return false
}

async function logout() {
  await session.logout()
  router.push('/catalog')
}
</script>

<template>
  <header class="topbar">
    <router-link class="brand" :to="songsTo">FlammeBible</router-link>
    <nav>
      <router-link class="nav-link" :class="{ 'router-link-active': on('songs') }" :to="songsTo">Сборники и поиск</router-link>
      <router-link v-if="session.token" class="nav-link" :class="{ 'router-link-active': on('plans') }" to="/plans">Планы</router-link>
      <router-link v-if="session.token" class="nav-link" :class="{ 'router-link-active': on('spaces') }" to="/communities">Общины</router-link>
      <router-link v-if="session.token" class="nav-link" :class="{ 'router-link-active': on('ai') }" to="/ai">AI Чат</router-link>
      <router-link class="nav-link" :class="{ 'router-link-active': on('download') }" to="/download">Скачать клиент</router-link>
      <router-link v-if="session.role === 'admin' || session.role === 'moderator'" class="nav-link" :class="{ 'router-link-active': on('admin') }" to="/admin">
        Админ-панель
        <span v-if="pendingAdmin" class="nav-badge">{{ pendingAdmin }}</span>
      </router-link>
      <div class="nav-end">
        <button v-if="session.token" class="logout" type="button" @click="logout">Выход</button>
        <template v-else>
          <router-link class="btn slim" to="/login">Войти</router-link>
          <router-link class="btn slim primary" to="/register">Регистрация</router-link>
        </template>
      </div>
    </nav>
  </header>
  <main :class="{ 'auth-main': authScreen, 'chat-main': on('ai') }">
    <router-view v-slot="{ Component }">
      <component :is="Component" @open-ai-settings="aiOpen = true" />
    </router-view>
  </main>
  <AiSettingsModal v-if="aiOpen" @close="aiOpen = false" />
</template>
