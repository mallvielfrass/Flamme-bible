<script setup>
import { ref } from 'vue'
import { api, explain } from '../api'

const props = defineProps({
  token: { type: String, default: '' },
  pickLabel: { type: String, default: 'Добавить в план' },
})
const emit = defineEmits(['pick'])

const query = ref('')
const moduleName = ref('')
const passages = ref([])
const error = ref('')
const loading = ref(false)
const searched = ref(false)

async function search() {
  const q = query.value.trim()
  error.value = ''
  if (!props.token) {
    error.value = 'Войдите, чтобы искать место в Библии.'
    return
  }
  if (!q) {
    error.value = 'Опишите ситуацию или человека.'
    return
  }
  loading.value = true
  try {
    const params = new URLSearchParams({ q })
    const name = moduleName.value.trim()
    if (name) params.set('module', name)
    const data = await api(`/api/bible/semantic?${params}`, { token: props.token })
    passages.value = data.passages || []
    searched.value = true
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <form class="search-bar" @submit.prevent="search">
    <label class="grow">Найти место по описанию
      <input v-model="query" name="q" type="search" placeholder="Утешение в горе или Давид и Вирсавия" />
    </label>
    <label>Перевод
      <input v-model="moduleName" name="module" placeholder="RST, можно пустым" />
    </label>
    <button class="btn slim primary" type="submit" :disabled="loading">Найти</button>
  </form>
  <p class="hint">Ищем в одобренных переводах, которые уже загружены на сервер. Отрывок — несколько стихов подряд.</p>
  <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  <p v-else-if="loading" class="empty" role="status">Загрузка…</p>
  <div v-else-if="searched && !passages.length" class="empty-state">
    <p>Ничего близкого не нашлось. Опишите ситуацию другими словами.</p>
  </div>
  <ul v-else class="plain">
    <li v-for="(hit, index) in passages" :key="`${hit.module_id}-${hit.reference}-${index}`">
      <strong class="title-serif">{{ hit.reference || 'Место' }}</strong>
      <p class="hint">{{ hit.module_title || hit.module_short_name }}</p>
      <p v-if="hit.text" class="quote">{{ hit.text }}</p>
      <button class="btn slim" type="button" @click="emit('pick', hit)">{{ pickLabel }}</button>
    </li>
  </ul>
</template>
