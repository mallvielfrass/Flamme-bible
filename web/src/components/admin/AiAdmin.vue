<script setup>
import { onMounted, ref } from 'vue'
import { api, explain } from '../../api'
import { useSessionStore } from '../../stores/session'

const session = useSessionStore()
const error = ref('')
const busy = ref(false)
const enabled = ref(true)
const per10 = ref(0)
const perDay = ref(0)
const tools = ref(8)
const active = ref(0)
const messages = ref(0)

function apply(data) {
  const settings = data.settings || {}
  enabled.value = settings.enabled !== false
  per10.value = Number(settings.msg_per_10min || 0)
  perDay.value = Number(settings.msg_per_day || 0)
  tools.value = Number(settings.max_tool_calls || 8)
  active.value = Number(data.active_answers || 0)
  messages.value = Number(data.messages_24h || 0)
}

async function load() {
  error.value = ''
  try {
    apply(await api('/api/admin/ai', { token: session.token }))
  } catch (err) {
    error.value = explain(err)
  }
}

async function save() {
  busy.value = true
  error.value = ''
  try {
    apply(await api('/api/admin/ai', {
      method: 'PUT',
      token: session.token,
      body: {
        enabled: enabled.value,
        msg_per_10min: Number(per10.value),
        msg_per_day: Number(perDay.value),
        max_tool_calls: Number(tools.value),
      },
    }))
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="embed-queue">
    <h2>ИИ</h2>
    <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
    <p class="hint">Сейчас отвечают: {{ active }} · сообщений за сутки: {{ messages }}</p>
    <form class="embed-form" @submit.prevent="save">
      <label class="check-line">
        <input v-model="enabled" type="checkbox" />
        Чат включён
      </label>
      <label>Сообщений за 10 минут, 0 — без лимита
        <input v-model.number="per10" type="number" min="0" />
      </label>
      <label>Сообщений за сутки, 0 — без лимита
        <input v-model.number="perDay" type="number" min="0" />
      </label>
      <label>Вызовов инструментов
        <input v-model.number="tools" type="number" min="1" max="20" />
      </label>
      <button class="btn slim" type="submit" :disabled="busy">Сохранить</button>
    </form>
  </section>
</template>
