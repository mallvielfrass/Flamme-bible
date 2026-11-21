<script setup>
import { computed, ref, watch } from 'vue'
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
const members = ref(null)
const error = ref('')
const busy = ref(false)

const spaceId = computed(() => props.dimensionId || route.params.id || '')
watch(spaceId, loadMembers, { immediate: true })

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

async function loadMembers() {
  if (!spaceId.value) return
  try {
    const data = await api(`/api/communities/${spaceId.value}/members`, { token: session.token })
    members.value = data.members || []
  } catch (err) {
    if (err.status === 404 || err.status === 403) members.value = null
  }
}

async function exclude(member) {
  error.value = ''
  busy.value = true
  try {
    await api(`/api/communities/${spaceId.value}/members/${member.user_id}/exclude`, {
      method: 'POST',
      token: session.token,
    })
    await loadMembers()
  } catch (err) {
    if (err.status === 404) {
      try {
        await api(`/dimensions/${spaceId.value}/members/${member.user_id}/exclude`, {
          method: 'POST',
          token: session.token,
        })
        await loadMembers()
        return
      } catch (inner) {
        error.value = explain(inner)
        return
      }
    }
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
    <div v-if="members">
      <h3>Участники</h3>
      <p v-for="member in members" :key="member.user_id">
        {{ member.email }} · {{ member.role }}
        <span v-if="member.excluded"> · исключён</span>
        <button v-else class="btn slim" type="button" :disabled="busy" @click="exclude(member)">Исключить</button>
      </p>
    </div>
    <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  </section>
</template>
