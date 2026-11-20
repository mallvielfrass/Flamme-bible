<script setup>
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { useSessionStore } from '../stores/session'
import InvitePanel from '../components/InvitePanel.vue'

const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const space = ref(null)
const error = ref('')
const name = ref('')
const address = ref('')
const email = ref('')
const confession = ref('')
const description = ref('')
const inviteToken = ref('')

async function load() {
  error.value = ''
  space.value = null
  inviteToken.value = ''
  try {
    const view = await api(`/dimensions/${route.params.id}`, { token: session.token })
    space.value = view
    session.remember(view)
  } catch (err) {
    error.value = err.message
  }
}

async function createCommunity() {
  error.value = ''
  try {
    const view = await api('/dimensions', {
      method: 'POST',
      token: session.token,
      body: {
        name: name.value.trim(),
        address: address.value.trim(),
        email: email.value.trim(),
        confession: confession.value.trim(),
        description: description.value.trim(),
      },
    })
    session.remember(view)
    router.push(`/dimensions/${view.id}`)
  } catch (err) {
    error.value = err.message
  }
}

function rememberInvite(invite) {
  inviteToken.value = invite?.token || ''
}

async function acceptInvite() {
  if (!inviteToken.value) return
  error.value = ''
  try {
    const data = await api(`/invites/${encodeURIComponent(inviteToken.value)}/accept`, {
      method: 'POST',
      token: session.token,
    })
    const view = await api(`/dimensions/${data.dimension_id}`, { token: session.token })
    session.remember(view)
    router.push(`/dimensions/${data.dimension_id}`)
  } catch (err) {
    error.value = err.message
  }
}

onMounted(load)
watch(() => route.params.id, load)
</script>

<template>
  <h1>Пространство</h1>
  <p v-if="error" class="error">{{ error }}</p>
  <div class="split">
    <section v-if="space" class="card" :data-kind="space.kind">
      <p class="badge">{{ space.kind === 'personal' ? 'Личное' : 'Община' }}</p>
      <h2 v-if="space.name" class="space-name">{{ space.name }}</h2>
      <ul class="facts">
        <li v-if="space.address">
          <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 21s6-5.2 6-10a6 6 0 1 0-12 0c0 4.8 6 10 6 10z" fill="none" stroke="currentColor" stroke-width="1.6" /><circle cx="12" cy="11" r="1.6" fill="currentColor" /></svg>
          <span>Адрес: {{ space.address }}</span>
        </li>
        <li v-if="space.email">
          <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3.5" y="6" width="17" height="12" rx="1.5" fill="none" stroke="currentColor" stroke-width="1.6" /><path d="M4 7l8 6 8-6" fill="none" stroke="currentColor" stroke-width="1.6" /></svg>
          <span>Почта: {{ space.email }}</span>
        </li>
        <li v-if="space.confession">
          <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v18M7 8h10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" /></svg>
          <span>Конфессия: {{ space.confession }}</span>
        </li>
        <li v-if="space.description">
          <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 3.5h7l4 4V20a1.5 1.5 0 0 1-1.5 1.5h-9.5A1.5 1.5 0 0 1 5.5 20V5A1.5 1.5 0 0 1 7 3.5z" fill="none" stroke="currentColor" stroke-width="1.6" /><path d="M14 3.5V8h4.5M8 12h8M8 16h6" fill="none" stroke="currentColor" stroke-width="1.6" /></svg>
          <span>Описание: {{ space.description }}</span>
        </li>
      </ul>
      <nav class="menu-links">
        <router-link :to="`/dimensions/${space.id}/invite`">
          <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="9" cy="9" r="2.2" fill="none" stroke="currentColor" stroke-width="1.6" /><circle cx="16" cy="10" r="1.7" fill="none" stroke="currentColor" stroke-width="1.6" /><path d="M4.5 18c.8-2.2 2.4-3.3 4.5-3.3s3.7 1.1 4.5 3.3M14 14.8c1.2-.5 2.3-.6 3.4-.2 1.3.6 2.1 1.7 2.6 3.4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" /></svg>
          Приглашение
        </router-link>
        <router-link :to="`/dimensions/${space.id}/sync`">
          <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 8h11l-2.2-2.2M17 16H6l2.2 2.2" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /><path d="M18 8a6.5 6.5 0 0 0-11.2-2M6 16a6.5 6.5 0 0 0 11.2 2" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" /></svg>
          Состояние синхронизации
        </router-link>
      </nav>
      <h3 class="gold-title">Новая община</h3>
      <form @submit.prevent="createCommunity">
        <div class="fields">
          <label class="wide">Имя <input v-model="name" name="name" placeholder="Имя" required /></label>
          <label>Адрес <input v-model="address" name="address" placeholder="Адрес" /></label>
          <label>Почта <input v-model="email" name="email" type="email" placeholder="Почта" /></label>
          <label>Конфессия <input v-model="confession" name="confession" placeholder="Конфессия" /></label>
          <label>Описание <input v-model="description" name="description" placeholder="Описание" /></label>
        </div>
        <button class="btn primary" type="submit">Создать общину</button>
      </form>
    </section>
    <section v-else class="card">
      <h2 class="gold-title">Новая община</h2>
      <form @submit.prevent="createCommunity">
        <div class="fields">
          <label class="wide">Имя <input v-model="name" name="name" placeholder="Имя" required /></label>
          <label>Адрес <input v-model="address" name="address" placeholder="Адрес" /></label>
          <label>Почта <input v-model="email" name="email" type="email" placeholder="Почта" /></label>
          <label>Конфессия <input v-model="confession" name="confession" placeholder="Конфессия" /></label>
          <label>Описание <input v-model="description" name="description" placeholder="Описание" /></label>
        </div>
        <button class="btn primary" type="submit">Создать общину</button>
      </form>
    </section>
    <InvitePanel @created="rememberInvite" />
  </div>
  <section class="join-strip">
    <p>
      <svg class="icon" viewBox="0 0 24 24" aria-hidden="true">
        <path d="M12 3l7 3v6c0 4.2-2.8 7.2-7 9-4.2-1.8-7-4.8-7-9V6l7-3z" fill="none" stroke="currentColor" stroke-width="1.6" />
        <path d="M9 12l2 2 4-4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
      </svg>
      <span><strong>Вступление.</strong> Одноразовая ссылка в это пространство.</span>
    </p>
    <button class="btn primary slim" type="button" :disabled="!inviteToken" @click="acceptInvite">Принять приглашение</button>
  </section>
</template>
