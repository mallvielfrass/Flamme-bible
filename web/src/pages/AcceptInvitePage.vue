<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { explain } from '../api'
import { useSessionStore } from '../stores/session'

const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const error = ref('')
const busy = ref(false)

async function accept() {
  error.value = ''
  busy.value = true
  try {
    const view = await session.acceptInvite(route.params.token)
    router.push(`/dimensions/${view.id}`)
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="card">
    <h1>Приглашение в общину</h1>
    <p class="hint">Нажмите кнопку, чтобы вступить. Ссылка срабатывает один раз.</p>
    <button class="btn primary" type="button" :disabled="busy" @click="accept">Вступить</button>
    <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  </section>
</template>
