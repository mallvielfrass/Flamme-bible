<script setup>
import { computed, ref } from 'vue'
import AppModal from '../../shared/components/AppModal.vue'
import IconButton from '../../shared/components/IconButton.vue'
import { copyText } from '../../shared/ai/clipboard.js'
import { api, explain } from '../api'
import { useSessionStore } from '../stores/session'

const props = defineProps({
  space: { type: Object, required: true },
})
const emit = defineEmits(['close'])

const session = useSessionStore()
const ttl = ref(86400)
const invite = ref(null)
const error = ref('')
const notice = ref('')
const busy = ref(false)

const title = computed(() => `Пригласить в «${props.space.name || 'Общину'}»`)
const link = computed(() => {
  if (!invite.value?.token || typeof location === 'undefined') return ''
  return `${location.origin}/invite/${invite.value.token}`
})

async function createInvite() {
  error.value = ''
  notice.value = ''
  invite.value = null
  busy.value = true
  try {
    invite.value = await api(`/api/dimensions/${props.space.id}/invites`, {
      method: 'POST',
      token: session.token,
      body: { ttl_seconds: Number(ttl.value) },
    })
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function copyLink() {
  if (!link.value) return
  await copyText(link.value)
  notice.value = 'Ссылка скопирована.'
}
</script>

<template>
  <AppModal :title="title" @close="emit('close')">
    <p class="hint">Ссылка одноразовая. Человек откроет её, войдёт и окажется в общине.</p>
    <form @submit.prevent="createInvite">
      <label>Срок ссылки
        <select v-model="ttl" name="ttl" data-autofocus>
          <option :value="3600">1 час</option>
          <option :value="86400">1 день</option>
          <option :value="604800">7 дней</option>
        </select>
      </label>
    </form>
    <p v-if="link" class="invite-link">
      <a :href="link">{{ link }}</a>
      <IconButton icon="copy" label="Копировать ссылку" small @click="copyLink" />
    </p>
    <p v-if="notice" class="banner ok" role="status">{{ notice }}</p>
    <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
    <template #footer>
      <button class="btn slim" type="button" @click="emit('close')">Закрыть</button>
      <button class="btn slim primary" type="button" :disabled="busy" @click="createInvite">Создать приглашение</button>
    </template>
  </AppModal>
</template>
