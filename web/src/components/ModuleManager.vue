<script setup>
import { computed, ref, watch } from 'vue'
import { api, explain } from '../api'
import { canSubmit, moduleNameOk, statusText } from '../checks'
import { useSessionStore } from '../stores/session'

const props = defineProps({
  dimensionId: { type: String, default: '' },
})

const session = useSessionStore()
const modules = ref([])
const shortName = ref('')
const title = ref('')
const error = ref('')
const notice = ref('')
const loading = ref(false)
const busy = ref(false)

const space = computed(() => session.spaces.find((item) => item.id === props.dimensionId) || null)

function place(item) {
  if (item.dimension_id) {
    const found = session.spaces.find((entry) => entry.id === item.dimension_id)
    return found?.name || 'Община'
  }
  return 'Личное'
}

async function load() {
  error.value = ''
  loading.value = true
  try {
    const list = await api('/api/modules', { token: session.token })
    modules.value = Array.isArray(list) ? list : []
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
}

async function createModule() {
  error.value = ''
  notice.value = ''
  const name = shortName.value.trim()
  if (!moduleNameOk(name)) {
    error.value = 'Короткое имя: латиница, цифры и символы + _ -, например RST.'
    return
  }
  busy.value = true
  try {
    const body = { short_name: name, title: title.value.trim() }
    if (space.value?.kind === 'community') body.dimension_id = space.value.id
    await api('/api/modules', { method: 'POST', token: session.token, body })
    shortName.value = ''
    title.value = ''
    notice.value = 'Модуль создан как черновик.'
    await load()
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function submit(item) {
  error.value = ''
  notice.value = ''
  busy.value = true
  try {
    await api(`/api/modules/${item.id}/submit`, { method: 'POST', token: session.token })
    notice.value = 'Модуль отправлен на публикацию.'
    await load()
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

watch(() => props.dimensionId, load, { immediate: true })
</script>

<template>
  <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  <p v-if="notice" class="banner ok" role="status">{{ notice }}</p>
  <p class="hint">Новый модуль попадёт в {{ space?.kind === 'community' ? `общину «${space.name}»` : 'личное пространство' }}.</p>
  <p v-if="loading" class="empty" role="status">Загрузка…</p>
  <div v-else-if="!modules.length" class="empty-state">
    <p>Модулей Библии пока нет.</p>
    <p class="hint">Добавьте перевод ниже и отправьте его на проверку.</p>
  </div>
  <ul v-else class="plain">
    <li v-for="item in modules" :key="item.id">
      <div class="card-head">
        <strong class="title-serif">{{ item.title || item.short_name }}</strong>
        <span class="badge" :class="item.status">{{ statusText(item.status) }}</span>
      </div>
      <p class="hint">{{ item.short_name }} · {{ place(item) }}</p>
      <p v-if="item.status === 'rejected' && item.moderation_note" class="hint">Причина: {{ item.moderation_note }}</p>
      <button v-if="canSubmit(item.status)" class="btn slim primary" type="button" :disabled="busy" @click="submit(item)">Отправить на публикацию</button>
    </li>
  </ul>
  <section class="card">
    <h2>Новый модуль</h2>
    <form @submit.prevent="createModule">
      <div class="fields">
        <label>Короткое имя
          <input v-model="shortName" name="short_name" placeholder="RST" required />
        </label>
        <label>Название
          <input v-model="title" name="title" placeholder="Синодальный перевод" />
        </label>
      </div>
      <button class="btn primary" type="submit" :disabled="busy">Добавить модуль</button>
    </form>
  </section>
</template>
