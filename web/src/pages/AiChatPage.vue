<script setup>
import { computed, provide, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, apiURL } from '../api'
import { planTarget } from '../planItems.js'
import { aiPlanKey } from '../../shared/ai/results.js'
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

provide(aiPlanKey, planTarget({
  list: async () => {
    const list = await api('/api/plans?scope=all', { token: session.token })
    return Array.isArray(list) ? list : []
  },
  read: (id) => api(`/api/plans/${id}`, { token: session.token }),
  save: (plan) => api(`/api/plans/${plan.id}`, {
    method: 'PUT',
    token: session.token,
    body: { title: plan.title, version: plan.version, updated_at: plan.updated_at || undefined, items: plan.items },
  }),
  places: async () => session.spaces
    .filter((space) => space.kind === 'community')
    .map((space) => ({ id: space.id, name: space.name || 'Община' })),
  defaultPlace: () => (session.current?.kind === 'community' ? session.current.id : ''),
  create: (title, items, dimensionId) => api('/api/plans', {
    method: 'POST',
    token: session.token,
    body: { title, dimension_id: dimensionId, items },
  }),
}))

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
