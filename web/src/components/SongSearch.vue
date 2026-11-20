<script setup>
import { ref } from 'vue'
import { api, explain } from '../api'
import { statusText } from '../checks'

const props = defineProps({
  token: { type: String, default: '' },
  picking: { type: Boolean, default: false },
})
const emit = defineEmits(['pick'])

const query = ref('')
const songs = ref([])
const offset = ref(0)
const more = ref(false)
const error = ref('')
const loading = ref(false)
const searched = ref(false)

async function run(nextOffset) {
  const q = query.value.trim()
  error.value = ''
  if (!props.token) {
    error.value = 'Войдите, чтобы искать песни.'
    return
  }
  if (!q) {
    error.value = 'Введите слово из названия или текста песни.'
    return
  }
  loading.value = true
  try {
    const params = new URLSearchParams({ q, limit: '20', offset: String(nextOffset) })
    const data = await api(`/api/songs/search?${params}`, { token: props.token })
    const list = data.songs || []
    songs.value = nextOffset === 0 ? list : songs.value.concat(list)
    offset.value = nextOffset + list.length
    more.value = list.length === 20
    searched.value = true
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
}

function search() {
  songs.value = []
  more.value = false
  run(0)
}
</script>

<template>
  <form class="search-bar" @submit.prevent="search">
    <label class="grow">Найти песню
      <input v-model="query" name="q" type="search" placeholder="Название или строчка" />
    </label>
    <button class="btn slim primary" type="submit" :disabled="loading">Найти</button>
  </form>
  <p class="hint">Ищем по вашим сборникам и по уже одобренным общим.</p>
  <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  <p v-else-if="loading && !songs.length" class="empty" role="status">Загрузка…</p>
  <div v-else-if="searched && !songs.length" class="empty-state">
    <p>Ничего не нашлось. Попробуйте другое слово.</p>
  </div>
  <ul v-else class="plain">
    <li v-for="(song, index) in songs" :key="`${song.songbook_id}-${song.fbid}-${index}`">
      <div class="card-head">
        <strong class="title-serif">{{ song.title || 'Без названия' }}</strong>
        <span class="badge" :class="song.status">{{ statusText(song.status) }}</span>
      </div>
      <p class="hint">{{ song.songbook_title || 'Сборник' }}</p>
      <button v-if="picking" class="btn slim" type="button" @click="emit('pick', song)">Добавить в план</button>
    </li>
  </ul>
  <button
    v-if="more"
    class="btn slim"
    type="button"
    :disabled="loading"
    @click="run(offset)"
  >Показать ещё</button>
</template>
