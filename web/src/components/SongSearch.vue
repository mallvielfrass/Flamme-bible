<script setup>
import { ref } from 'vue'
import { api, explain } from '../api'
import { statusText } from '../checks'

const props = defineProps({
  token: { type: String, default: '' },
  picking: { type: Boolean, default: false },
  songbookId: { type: String, default: '' },
  sectionId: { type: String, default: '' },
})
const emit = defineEmits(['pick'])

const query = ref('')
const mode = ref('words')
const songs = ref([])
const offset = ref(0)
const more = ref(false)
const error = ref('')
const loading = ref(false)
const searched = ref(false)

function setMode(next) {
  if (mode.value === next) return
  mode.value = next
  songs.value = []
  more.value = false
  searched.value = false
  error.value = ''
}

async function run(nextOffset) {
  const q = query.value.trim()
  error.value = ''
  if (!props.token) {
    error.value = 'Войдите, чтобы искать песни.'
    return
  }
  if (!q) {
    error.value = mode.value === 'meaning'
      ? 'Опишите, о чём песня.'
      : 'Введите слово из названия или текста песни.'
    return
  }
  loading.value = true
  try {
    const scope = new URLSearchParams()
    if (props.songbookId) scope.set('songbook_id', props.songbookId)
    if (props.sectionId) scope.set('section_id', props.sectionId)
    if (mode.value === 'meaning') {
      const params = new URLSearchParams({ q })
      scope.forEach((value, key) => params.set(key, value))
      const data = await api(`/api/songs/semantic?${params}`, { token: props.token })
      songs.value = data.songs || []
      offset.value = songs.value.length
      more.value = false
      searched.value = true
      return
    }
    const params = new URLSearchParams({ q, limit: '20', offset: String(nextOffset) })
    scope.forEach((value, key) => params.set(key, value))
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
  <div class="tabs" role="tablist" aria-label="Как искать">
    <button type="button" class="tab" :class="{ on: mode === 'words' }" @click="setMode('words')">По словам</button>
    <button type="button" class="tab" :class="{ on: mode === 'meaning' }" @click="setMode('meaning')">По смыслу</button>
  </div>
  <form class="search-bar" @submit.prevent="search">
    <slot name="filter" />
    <label class="grow">Найти песню
      <input
        v-model="query"
        name="q"
        type="search"
        :placeholder="mode === 'meaning' ? 'О чём песня, своими словами' : 'Название или строчка'"
      />
    </label>
    <button class="btn slim primary" type="submit" :disabled="loading">Найти</button>
  </form>
  <p v-if="mode === 'words'" class="hint">Ищем по вашим сборникам и по уже одобренным общим.</p>
  <p v-else class="hint">Ищем близкие по смыслу песни среди ваших и одобренных общих. На сервере нужна локальная модель.</p>
  <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  <p v-else-if="loading && !songs.length" class="empty" role="status">Загрузка…</p>
  <div v-else-if="searched && !songs.length" class="empty-state">
    <p v-if="mode === 'meaning'">Ничего близкого не нашлось. Опишите тему другими словами.</p>
    <p v-else>Ничего не нашлось. Попробуйте другое слово.</p>
  </div>
  <ul v-else class="plain">
    <li v-for="(song, index) in songs" :key="`${song.songbook_id}-${song.fbid}-${index}`">
      <div class="card-head">
        <strong class="title-serif">{{ song.title || 'Без названия' }}</strong>
        <span class="badge" :class="song.status">{{ statusText(song.status) }}</span>
      </div>
      <p class="hint">{{ [song.songbook_title, song.section_title].filter(Boolean).join(' · ') || 'Сборник' }}</p>
      <p v-if="song.fragment" class="quote">{{ song.fragment }}</p>
      <button v-if="picking" class="btn slim" type="button" @click="emit('pick', song)">Добавить</button>
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
