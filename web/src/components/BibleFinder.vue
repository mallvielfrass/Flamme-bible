<script setup>
import { ref } from 'vue'
import { api, explain } from '../api'
import { hitResult, hitRange } from '../bibleReader.js'
import AddToPlanModal from '../../shared/ai/AddToPlanModal.vue'

// Поиск по Библии в основном переводе: по словам (/api/bible/search) или по смыслу
// (/api/bible/semantic). Найденное открывается в читалке или сразу идёт в план.
const props = defineProps({
  token: { type: String, required: true },
  translations: { type: Array, required: true },
})
const emit = defineEmits(['open'])

const mode = ref('words')
const query = ref('')
const hits = ref([])
const more = ref(false)
const error = ref('')
const loading = ref(false)
const searched = ref(false)
const adding = ref(null)
const added = ref('')

function setMode(next) {
  if (mode.value === next) return
  mode.value = next
  hits.value = []
  more.value = false
  searched.value = false
  error.value = ''
}

async function run(offset) {
  const q = query.value.trim()
  const module = props.translations[0] || ''
  error.value = ''
  added.value = ''
  if (!module) {
    error.value = 'Выберите перевод.'
    return
  }
  if (q.length < 2) {
    error.value = mode.value === 'meaning' ? 'Опишите ситуацию или человека.' : 'Введите слово или фразу из стиха.'
    return
  }
  loading.value = true
  try {
    if (mode.value === 'meaning') {
      const data = await api(`/api/bible/semantic?${new URLSearchParams({ q, module })}`, { token: props.token })
      hits.value = data.passages || []
      more.value = false
    } else {
      const params = new URLSearchParams({ q, module, offset: String(offset) })
      const data = await api(`/api/bible/search?${params}`, { token: props.token })
      const list = data.verses || []
      hits.value = offset ? hits.value.concat(list) : list
      more.value = Boolean(data.more)
    }
    searched.value = true
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
}

function where(hit) {
  return hit.reference || `${hit.book_name || 'Книга'} ${hit.chapter}:${hit.verse}`
}

function open(hit) {
  emit('open', { book: hit.book, chapter: hit.chapter, range: hitRange(hit) })
}

function addToPlan(hit) {
  try {
    adding.value = hitResult(props.translations, hit)
  } catch (err) {
    error.value = err.message
  }
}

function onAdded(title) {
  added.value = title ? `Добавлено в план «${title}».` : 'Добавлено в план.'
  adding.value = null
}
</script>

<template>
  <div class="tabs" role="tablist" aria-label="Как искать">
    <button type="button" class="tab" :class="{ on: mode === 'words' }" @click="setMode('words')">По словам</button>
    <button type="button" class="tab" :class="{ on: mode === 'meaning' }" @click="setMode('meaning')">По смыслу</button>
  </div>
  <form class="search-bar" @submit.prevent="run(0)">
    <label class="grow">Найти в Библии
      <input
        v-model="query"
        name="q"
        type="search"
        :placeholder="mode === 'meaning' ? 'Утешение в горе или Давид и Вирсавия' : 'Слово или фраза из стиха'"
      />
    </label>
    <button class="btn slim primary" type="submit" :disabled="loading">Найти</button>
  </form>
  <p class="hint">
    Ищем в переводе {{ translations[0] || '—' }}.
    <template v-if="mode === 'meaning'">На сервере нужна локальная модель.</template>
  </p>
  <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  <p v-if="added" class="banner ok" role="status">{{ added }}</p>
  <p v-if="loading && !hits.length" class="empty" role="status">Загрузка…</p>
  <div v-else-if="searched && !hits.length" class="empty-state">
    <p>Ничего не нашлось. Попробуйте другие слова.</p>
  </div>
  <ul v-else-if="hits.length" class="plain bible-hits">
    <li v-for="(hit, index) in hits" :key="`${hit.book}-${hit.chapter}-${hit.verse || hit.verse_start}-${index}`">
      <div class="card-head">
        <strong class="title-serif">{{ where(hit) }}</strong>
        <span class="hint">{{ hit.module_short_name }}</span>
      </div>
      <p class="quote">{{ hit.text }}</p>
      <div class="bible-step">
        <button class="btn slim" type="button" @click="open(hit)">Открыть главу</button>
        <button class="btn slim" type="button" @click="addToPlan(hit)">В план</button>
      </div>
    </li>
  </ul>
  <button v-if="more" class="btn slim" type="button" :disabled="loading" @click="run(hits.length)">Показать ещё</button>
  <AddToPlanModal v-if="adding" :item="adding" @close="adding = null" @added="onAdded" />
</template>
