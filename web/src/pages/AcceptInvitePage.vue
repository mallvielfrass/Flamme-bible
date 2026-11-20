<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { useSessionStore } from '../stores/session'

const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const error = ref('')

async function accept() {
  error.value = ''
  try {
    const data = await api(`/invites/${encodeURIComponent(route.params.token)}/accept`, {
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
</script>

<template>
  <section class="join-strip">
    <p>
      <svg class="icon" viewBox="0 0 24 24" aria-hidden="true">
        <path d="M12 3l7 3v6c0 4.2-2.8 7.2-7 9-4.2-1.8-7-4.8-7-9V6l7-3z" fill="none" stroke="currentColor" stroke-width="1.6" />
        <path d="M9 12l2 2 4-4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
      </svg>
      <span><strong>Вступление.</strong> Одноразовая ссылка в это пространство.</span>
    </p>
    <button class="btn primary slim" type="button" @click="accept">Принять приглашение</button>
  </section>
  <p v-if="error" class="error">{{ error }}</p>
</template>
