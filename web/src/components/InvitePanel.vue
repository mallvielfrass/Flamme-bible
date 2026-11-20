<script setup>
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api'
import { useSessionStore } from '../stores/session'

const emit = defineEmits(['created'])
const route = useRoute()
const session = useSessionStore()
const ttl = ref(86400)
const invite = ref(null)
const userId = ref('')
const excluded = ref(false)
const error = ref('')

async function createInvite() {
  error.value = ''
  invite.value = null
  try {
    invite.value = await api(`/dimensions/${route.params.id}/invites`, {
      method: 'POST',
      token: session.token,
      body: { ttl_seconds: Number(ttl.value) },
    })
    emit('created', invite.value)
  } catch (err) {
    error.value = err.message
  }
}

async function exclude() {
  error.value = ''
  excluded.value = false
  try {
    await api(`/dimensions/${route.params.id}/members/${userId.value.trim()}/exclude`, {
      method: 'POST',
      token: session.token,
    })
    excluded.value = true
  } catch (err) {
    error.value = err.message
  }
}
</script>

<template>
  <section class="card">
    <div class="card-head">
      <h2>Приглашение</h2>
      <router-link class="back" :to="`/dimensions/${route.params.id}`">К пространству →</router-link>
    </div>
    <form @submit.prevent="createInvite">
      <label>Срок, секунды
        <span class="with-icon">
          <input v-model="ttl" name="ttl" type="number" min="1" required />
          <svg class="field-icon" viewBox="0 0 24 24" aria-hidden="true">
            <circle cx="12" cy="12" r="8" fill="none" stroke="currentColor" stroke-width="1.6" />
            <path d="M12 8v4.5l3 2" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
          </svg>
        </span>
      </label>
      <button class="btn primary" type="submit">Создать приглашение</button>
    </form>
    <p v-if="invite">
      Ссылка:
      <router-link :to="`/invite/${invite.token}`">{{ invite.token }}</router-link>
    </p>
    <form @submit.prevent="exclude">
      <label>Идентификатор участника
        <span class="with-icon">
          <input v-model="userId" name="user_id" placeholder="Идентификатор участника" required />
          <svg class="field-icon" viewBox="0 0 24 24" aria-hidden="true">
            <circle cx="12" cy="8" r="3" fill="none" stroke="currentColor" stroke-width="1.6" />
            <path d="M6 19c1.2-2.4 3-3.5 6-3.5s4.8 1.1 6 3.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
          </svg>
        </span>
      </label>
      <button class="btn gold" type="submit">Исключить</button>
    </form>
    <p v-if="excluded">Участник исключён</p>
    <p v-if="error" class="error">{{ error }}</p>
  </section>
</template>
