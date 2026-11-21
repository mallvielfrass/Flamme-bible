<script setup>
import { ref, watch } from 'vue'
import { api, explain } from '../api'
import { bookStatus, canSubmit } from '../checks'
import { readVbmCard, songbookPayload } from '../vbmMeta'
import { useSessionStore } from '../stores/session'

const props = defineProps({
  dimensionId: { type: String, default: '' },
})

const session = useSessionStore()
const books = ref([])
const title = ref('')
const shortName = ref('')
const publisher = ref('')
const comment = ref('')
const header = ref(null)
const error = ref('')
const notice = ref('')
const loading = ref(false)
const busy = ref(false)
const hideId = ref('')
const sections = ref({})
const bookSongs = ref({})
const sectionDraft = ref({})

async function load() {
  error.value = ''
  books.value = []
  if (!props.dimensionId) {
    loading.value = false
    return
  }
  loading.value = true
  try {
    const list = await api(`/dimensions/${props.dimensionId}/songbooks`, { token: session.token })
    books.value = (Array.isArray(list) ? list : []).filter((book) => !book.hidden)
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
}

async function onFile(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  error.value = ''
  notice.value = ''
  header.value = null
  if (!file) return
  if (!file.name.toLowerCase().endsWith('.vbm')) {
    error.value = 'Нужен файл сборника с расширением .vbm.'
    return
  }
  if (file.size > 32 * 1024 * 1024) {
    error.value = 'Файл больше 32 МБ. Название можно вписать вручную, а песни подтянет программа.'
    return
  }
  try {
    const card = readVbmCard(new Uint8Array(await file.arrayBuffer()))
    header.value = card.header
    if (card.header.header_title) title.value = card.header.header_title
    if (card.header.header_short) shortName.value = card.header.header_short
    notice.value = 'Из файла прочитаны название и шапка. Тексты песен сюда не загружаются: их синхронизирует программа на компьютере.'
  } catch (err) {
    error.value = err.message
  }
}

async function createBook() {
  error.value = ''
  notice.value = ''
  if (!props.dimensionId) {
    error.value = 'Сначала выберите пространство вверху страницы.'
    return
  }
  if (!title.value.trim()) {
    error.value = 'Введите название сборника.'
    return
  }
  busy.value = true
  try {
    await api(`/dimensions/${props.dimensionId}/songbooks`, {
      method: 'POST',
      token: session.token,
      body: songbookPayload({
        title: title.value,
        shortName: shortName.value,
        publisher: publisher.value,
        comment: comment.value,
        fbid: crypto.randomUUID(),
        header: header.value,
      }),
    })
    title.value = ''
    shortName.value = ''
    publisher.value = ''
    comment.value = ''
    header.value = null
    notice.value = 'Сборник добавлен как черновик.'
    await load()
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function submit(book) {
  error.value = ''
  notice.value = ''
  busy.value = true
  try {
    await api(`/api/songbooks/${book.id}/submit`, { method: 'POST', token: session.token })
    notice.value = 'Сборник отправлен на публикацию.'
    await load()
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function hide(book) {
  if (hideId.value !== book.id) {
    hideId.value = book.id
    return
  }
  error.value = ''
  busy.value = true
  try {
    await api(`/songbooks/${book.id}/hide`, { method: 'POST', token: session.token })
    hideId.value = ''
    notice.value = 'Сборник скрыт.'
    await load()
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function loadSections(book) {
  const [list, snap] = await Promise.all([
    api(`/api/songbooks/${book.id}/sections`, { token: session.token }),
    api(`/dimensions/${props.dimensionId}/songbooks/${book.id}/snapshot`, { token: session.token }),
  ])
  sections.value = { ...sections.value, [book.id]: Array.isArray(list) ? list : [] }
  bookSongs.value = {
    ...bookSongs.value,
    [book.id]: (snap.songs || []).filter((song) => !song.deleted),
  }
}

async function onSections(event, book) {
  if (!event.target.open) return
  error.value = ''
  try {
    await loadSections(book)
  } catch (err) {
    error.value = explain(err)
  }
}

async function createSection(book) {
  const title = (sectionDraft.value[book.id] || '').trim()
  if (!title) {
    error.value = 'Введите название раздела.'
    return
  }
  error.value = ''
  busy.value = true
  try {
    await api(`/api/songbooks/${book.id}/sections`, {
      method: 'POST',
      token: session.token,
      body: { title },
    })
    sectionDraft.value = { ...sectionDraft.value, [book.id]: '' }
    await loadSections(book)
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function renameSection(book, section) {
  const title = section.title.trim()
  if (!title) return
  error.value = ''
  try {
    await api(`/api/songbooks/${book.id}/sections/${section.id}`, {
      method: 'PUT',
      token: session.token,
      body: { title },
    })
  } catch (err) {
    error.value = explain(err)
    await loadSections(book)
  }
}

async function removeSection(book, section) {
  error.value = ''
  busy.value = true
  try {
    await api(`/api/songbooks/${book.id}/sections/${section.id}`, {
      method: 'DELETE',
      token: session.token,
    })
    await loadSections(book)
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function toggleSong(book, section, fbid, checked) {
  const current = Array.isArray(section.songs) ? section.songs : []
  const next = current.filter((id) => id !== fbid)
  if (checked) next.push(fbid)
  error.value = ''
  busy.value = true
  try {
    const saved = await api(`/api/sections/${section.id}/songs`, {
      method: 'PUT',
      token: session.token,
      body: { fbids: next },
    })
    section.songs = saved.songs || []
    section.count = saved.count || section.songs.length
  } catch (err) {
    error.value = explain(err)
    await loadSections(book)
  } finally {
    busy.value = false
  }
}

watch(() => props.dimensionId, load, { immediate: true })
</script>

<template>
  <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  <p v-if="notice" class="banner ok" role="status">{{ notice }}</p>
  <p v-if="!dimensionId" class="empty-state">Выберите личное пространство или общину.</p>
  <p v-else-if="loading" class="empty" role="status">Загрузка…</p>
  <div v-else-if="!books.length" class="empty-state">
    <p>В этом пространстве пока нет сборников.</p>
    <p class="hint">Добавьте карточку ниже. Песни из файла .vbm подтянет программа на компьютере.</p>
  </div>
  <ul v-else class="plain">
    <li v-for="book in books" :key="book.id">
      <div class="card-head">
        <strong class="title-serif">{{ book.title || book.short_name || 'Сборник' }}</strong>
        <span class="badge" :class="book.status || 'draft'">{{ bookStatus(book) }}</span>
      </div>
      <p v-if="book.index_pending" class="hint">Индексируется: {{ book.index_pending }} куплетов</p>
      <p v-if="book.index_failed" class="hint">Ошибки индексации: {{ book.index_failed }}</p>
      <p v-if="book.short_name" class="hint">{{ book.short_name }}</p>
      <p v-if="book.status === 'rejected' && book.moderation_note" class="hint">Причина: {{ book.moderation_note }}</p>
      <div class="btn-row">
        <button v-if="canSubmit(book.status)" class="btn slim primary" type="button" :disabled="busy" @click="submit(book)">Отправить на публикацию</button>
        <button class="btn slim" type="button" :disabled="busy" @click="hide(book)">
          {{ hideId === book.id ? 'Подтвердить скрытие' : 'Скрыть' }}
        </button>
      </div>
      <details @toggle="onSections($event, book)">
        <summary>Разделы</summary>
        <ul class="plain">
          <li v-for="section in sections[book.id] || []" :key="section.id">
            <label>Название
              <input v-model="section.title" @change="renameSection(book, section)" />
            </label>
            <p v-if="!(bookSongs[book.id] || []).length" class="hint">В сборнике пока нет песен. Их добавит программа на компьютере.</p>
            <label v-for="song in bookSongs[book.id] || []" :key="song.fbid" class="check-line">
              <input
                type="checkbox"
                :checked="(section.songs || []).includes(song.fbid)"
                :disabled="busy"
                @change="toggleSong(book, section, song.fbid, $event.target.checked)"
              />
              {{ song.title || 'Без названия' }}
            </label>
            <button class="btn slim danger" type="button" :disabled="busy" @click="removeSection(book, section)">Удалить раздел</button>
          </li>
        </ul>
        <form @submit.prevent="createSection(book)">
          <label>Новый раздел
            <input v-model="sectionDraft[book.id]" name="section_title" />
          </label>
          <button class="btn slim" type="submit" :disabled="busy">Добавить раздел</button>
        </form>
      </details>
    </li>
  </ul>

  <section class="card">
    <h2>Новый сборник</h2>
    <form @submit.prevent="createBook">
      <label>Название
        <input v-model="title" name="title" required />
      </label>
      <label>Короткое имя
        <input v-model="shortName" name="short_name" />
      </label>
      <div class="fields">
        <label>Издатель
          <input v-model="publisher" name="publisher" />
        </label>
        <label>Комментарий
          <input v-model="comment" name="comment" />
        </label>
      </div>
      <label>Файл .vbm
        <input name="vbm" type="file" accept=".vbm" @change="onFile" />
      </label>
      <p class="hint">Файл заполняет название и шапку. Сами песни остаются в программе VisioBible.</p>
      <button class="btn primary" type="submit" :disabled="busy">Добавить сборник</button>
    </form>
  </section>
</template>
