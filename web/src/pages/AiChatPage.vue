<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiURL } from '../api'
import { createAiApi } from '../aiApi'
import { useSessionStore } from '../stores/session'
import { useAiStore } from '../stores/ai'
import AiWorkspace from '../components/ai/AiWorkspace.vue'

const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const ai = useAiStore()

ai.useApi(createAiApi({
  baseUrl: '',
  getAuth: () => session.token,
  fetch: (path, init) => fetch(apiURL(path), init),
}))

const chatId = computed(() => String(route.params.id || ''))
const emit = defineEmits(['open-ai-settings'])

function openChat(id) {
  router.push(id ? `/ai/${id}` : '/ai')
}
</script>

<template>
  <AiWorkspace :chat-id="chatId" @open-chat="openChat" @open-settings="emit('open-ai-settings')" />
</template>
