<script setup>
import { computed, ref, watch } from 'vue'
import { api, explain } from '../api'
import { inRange, passageItem, rangeLabel, selectVerse } from '../bibleReader.js'
import AddToPlanModal from '../../shared/ai/AddToPlanModal.vue'

// Чтение главы: книга и глава по основному переводу, стихи всех выбранных переводов рядом.
const props = defineProps({
  token: { type: String, required: true },
  translations: { type: Array, required: true },
})

const books = ref([])
const book = ref(0)
const chapter = ref(0)
const columns = ref([])
const range = ref(null)
const error = ref('')
const loading = ref(false)
const adding = ref(null)
const notice = ref('')
let seq = 0

const primary = computed(() => props.translations[0] || '')
const current = computed(() => books.value.find((item) => item.book === book.value) || null)
const chapters = computed(() => Array.from({ length: current.value?.chapters || 0 }, (_, i) => i + 1))
const bookName = computed(() => columns.value.find((col) => col.bookName)?.bookName || current.value?.book_name || '')
// Строки по номерам стихов: в каждой строке текст каждого перевода.
const rows = computed(() => {
  const numbers = new Set()
  for (const col of columns.value) for (const v of col.verses) numbers.add(v.verse)
  return [...numbers].sort((a, b) => a - b).map((verse) => ({
    verse,
    cells: columns.value.map((col) => col.verses.find((v) => v.verse === verse)?.text || ''),
  }))
})
const primaryVerses = computed(() => columns.value[0]?.verses || [])
const label = computed(() => rangeLabel(bookName.value, chapter.value, range.value))

async function loadBooks() {
  books.value = []
  error.value = ''
  if (!primary.value) return
  try {
    const data = await api(`/api/bible/books?module=${encodeURIComponent(primary.value)}`, { token: props.token })
    books.value = data.books || []
    if (!books.value.some((item) => item.book === book.value)) {
      book.value = books.value[0]?.book || 0
      chapter.value = book.value ? 1 : 0
    }
  } catch (err) {
    error.value = explain(err)
  }
}

async function loadChapter() {
  columns.value = []
  if (!book.value || !chapter.value || !props.translations.length) return
  loading.value = true
  error.value = ''
  const mine = ++seq
  const params = `book=${book.value}&chapter=${chapter.value}`
  const loaded = await Promise.all(props.translations.map(async (name) => {
    try {
      const data = await api(`/api/bible/chapter?module=${encodeURIComponent(name)}&${params}`, { token: props.token })
      return { name, title: data.module_title || name, bookName: data.book_name, verses: data.verses || [], missing: false }
    } catch {
      return { name, title: name, bookName: '', verses: [], missing: true }
    }
  }))
  if (mine !== seq) return
  columns.value = loaded
  loading.value = false
}

function pickBook(value) {
  book.value = Number(value) || 0
  chapter.value = 1
  range.value = null
}

function pickChapter(value) {
  chapter.value = value
  range.value = null
}

function step(delta) {
  const next = chapter.value + delta
  if (next >= 1 && next <= chapters.value.length) pickChapter(next)
}

function clickVerse(verse) {
  range.value = selectVerse(range.value, verse)
  notice.value = ''
}

/** Открыть место из поиска: книга, глава и выделенные стихи. */
function open(place) {
  book.value = place.book
  chapter.value = place.chapter
  range.value = place.range
  notice.value = ''
}

function addToPlan() {
  try {
    adding.value = passageItem({
      translations: props.translations,
      book: book.value,
      bookName: bookName.value,
      chapter: chapter.value,
      range: range.value,
      verses: primaryVerses.value,
    })
  } catch (err) {
    error.value = err.message
  }
}

function onAdded(title) {
  notice.value = title ? `Добавлено в план «${title}».` : 'Добавлено в план.'
  adding.value = null
}

watch(primary, loadBooks, { immediate: true })
watch([book, chapter, () => props.translations.join(',')], loadChapter, { immediate: true })

defineExpose({ open })
</script>

<template>
  <section class="bible-reader">
    <div class="bible-place">
      <label>Книга
        <select :value="book" name="book" @change="pickBook($event.target.value)">
          <option v-for="item in books" :key="item.book" :value="item.book">{{ item.book_name || `Книга ${item.book}` }}</option>
        </select>
      </label>
      <div v-if="chapters.length" class="bible-chapters" role="group" aria-label="Глава">
        <button
          v-for="n in chapters"
          :key="n"
          type="button"
          class="chip"
          :class="{ on: n === chapter }"
          :aria-pressed="n === chapter"
          @click="pickChapter(n)"
        >{{ n }}</button>
      </div>
    </div>
    <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
    <p v-if="loading" class="empty" role="status">Загрузка…</p>
    <template v-else-if="rows.length">
      <div class="card-head">
        <h2 class="title-serif">{{ bookName }} {{ chapter }}</h2>
        <div class="bible-step">
          <button class="btn slim" type="button" :disabled="chapter <= 1" @click="step(-1)">← Глава</button>
          <button class="btn slim" type="button" :disabled="chapter >= chapters.length" @click="step(1)">Глава →</button>
        </div>
      </div>
      <p class="hint">Нажмите на стих, затем на последний стих отрывка. Ещё одно нажатие начнёт выделение заново.</p>
      <div class="bible-grid" :style="{ '--cols': columns.length }">
        <div class="bible-row bible-head" aria-hidden="true">
          <span />
          <span v-for="col in columns" :key="col.name" :title="col.title">{{ col.name }}</span>
        </div>
        <div v-for="col in columns.filter((c) => c.missing)" :key="`miss-${col.name}`" class="hint">
          В переводе {{ col.name }} нет этой главы.
        </div>
        <button
          v-for="row in rows"
          :key="row.verse"
          type="button"
          class="bible-row"
          :class="{ on: inRange(range, row.verse) }"
          :aria-pressed="inRange(range, row.verse)"
          @click="clickVerse(row.verse)"
        >
          <span class="bible-num">{{ row.verse }}</span>
          <span v-for="(text, i) in row.cells" :key="columns[i].name" class="bible-cell">
            <small class="bible-tag">{{ columns[i].name }}</small>{{ text }}
          </span>
        </button>
      </div>
    </template>
    <div v-else-if="book && !error" class="empty-state"><p>В этой главе нет стихов.</p></div>
    <div v-if="range || notice" class="bible-bar" role="status">
      <template v-if="range">
        <strong>{{ label }}</strong>
        <button class="btn slim primary" type="button" @click="addToPlan">В план</button>
        <button class="btn slim" type="button" @click="range = null">Снять выделение</button>
      </template>
      <span v-if="notice" class="hint">{{ notice }}</span>
    </div>
    <AddToPlanModal v-if="adding" :item="adding" @close="adding = null" @added="onAdded" />
  </section>
</template>
