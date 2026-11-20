<script setup>
import { useRouter } from 'vue-router'
import { useSessionStore } from './stores/session'

const session = useSessionStore()
const router = useRouter()

async function logout() {
  await session.logout()
  router.push('/login')
}
</script>

<template>
  <header v-if="session.token" class="topbar">
    <router-link class="brand" to="/songbooks">FlammeBible</router-link>
    <nav>
      <router-link class="nav-link" to="/songbooks">Сборники</router-link>
      <router-link class="nav-link" to="/plans">Планы</router-link>
      <button class="logout" type="button" @click="logout">Выход</button>
    </nav>
  </header>
  <main :class="{ 'auth-main': !session.token }">
    <router-view />
  </main>
</template>
