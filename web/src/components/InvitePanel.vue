<script setup>
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api, explain } from '../api'
import { useSessionStore } from '../stores/session'

const props = defineProps({
  dimensionId: { type: String, default: '' },
})

const emit = defineEmits(['created'])
const route = useRoute()
const session = useSessionStore()
const ttl = ref(86400)
const invite = ref(null)
const userId = ref('')
const excluded = ref(false)
const error = ref('')
const busy = ref(false)

const spaceId = computed(() => props.dimensionId || route.params.id || '')

const link = computed(() => {
  if (!invite.value?.token || typeof location === 'undefined') return ''
  return `${location.origin}/invite/${invite.value.token}`
})

async function createInvite() {
  error.value = ''
  invite.value = null
  busy.value = true
  try {
    invite.value = await api(`/api/dimensions/${spaceId.value}/invites`, {
      method: 'POST',
      token: session.token,
      body: { ttl_seconds: Number(ttl.value) },
    })
    emit('created', invite.value)
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function exclude() {
  error.value = ''
  excluded.value = false
  busy.value = true
  try {
    await api(`/dimensions/${spaceId.value}/members/${encodeURIComponent(userId.value.trim())}/exclude`, {
      method: 'POST',
      token: session.token,
    })
    excluded.value = true
    userId.value = ''
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section id="invite" class="card">
    <h2>Пригласить в общину</h2>
    <p class="hint">Ссылка одноразовая. Человек откроет её, войдёт и окажется в общине.</p>
    <form @submit.prevent="createInvite">
      <label>Срок ссылки
        <select v-model="ttl" name="ttl">
          <option :value="3600">1 час</option>
          <option :value="86400">1 день</option>
          <option :value="604800">7 дней</option>
        </select>
      </label>
      <button class="btn primary" type="submit" :disabled="busy || !spaceId">Создать приглашение</button>
    </form>
    <p v-if="link" class="answer">
      Ссылка: <a :href="link">{{ link }}</a>
    </p>
    <details>
      <summary>Исключить участника</summary>
      <form @submit.prevent="exclude">
        <label>Идентификатор участника
          <input v-model="userId" name="user_id" required />
        </label>
        <button class="btn slim" type="submit" :disabled="busy">Исключить</button>
      </form>
      <p v-if="excluded" class="banner ok">Участник исключён.</p>
    </details>
    <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  </section>
</template>
