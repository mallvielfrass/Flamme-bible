<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiURL } from '../api'
import { createAiApi } from '../../shared/ai/aiApi.js'
import { useSessionStore } from '../stores/session'
import { useAiStore } from '../../shared/ai/store.js'
import AiWorkspace from '../../shared/ai/AiWorkspace.vue'
import AiSettingsModal from '../../shared/ai/AiSettingsModal.vue'

const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const ai = useAiStore()
const settingsOpen = ref(false)

const client = createAiApi({
  baseUrl: '',
  getAuth: () => session.token,
  fetch: (path, init) => fetch(apiURL(path), init),
})
ai.useApi(client)

const chatId = computed(() => String(route.params.id || ''))

function openChat(id) {
  router.push(id ? `/ai/${id}` : '/ai')
}

async function closeSettings() {
  settingsOpen.value = false
  try {
    await ai.loadProviders()
  } catch {
    /* ошибку показывает сам чат */
  }
}
</script>

<template>
  <AiWorkspace :chat-id="chatId" @open-chat="openChat" @open-settings="settingsOpen = true" />
  <AiSettingsModal v-if="settingsOpen" :client="client" @close="closeSettings" />
</template>
