<script setup>
import { reactive, ref, watch } from 'vue'
import BibleSearch from './BibleSearch.vue'
import SongSearch from './SongSearch.vue'
import SongbookPicker from './SongbookPicker.vue'
import AppModal from './AppModal.vue'
import { bibleItem, itemTitle, songItem, textItem } from '../planItems'

const props = defineProps({
  item: { type: Object, default: null },
  token: { type: String, default: '' },
})

const emit = defineEmits(['close', 'add', 'change'])
const kindKey = 'flammebible.planItemKind'
const kinds = [
  { id: 'song', label: 'Песня' },
  { id: 'bible', label: 'Стих из Библии' },
  { id: 'text', label: 'Текст' },
]

const kind = ref('song')
const error = ref('')
const added = ref('')
const songbookId = ref('')
const sectionId = ref('')
const staged = ref(null)
const planTitle = ref('')
const titleTouched = ref(false)
const bible = reactive(blankBible())
const note = reactive({ caption: '', reference: '', quote: '', class: '' })
const editing = ref(false)

function blankBible() {
  return {
    module_short_name: '',
    module_short_name_second: '',
    book: '',
    chapter: '',
    verse: '',
    reference: '',
    quote: '',
    class: '',
  }
}

function reset() {
  error.value = ''
  added.value = ''
  staged.value = null
  planTitle.value = ''
  titleTouched.value = false
  songbookId.value = ''
  sectionId.value = ''
  Object.assign(bible, blankBible())
  note.caption = ''
  note.reference = ''
  note.quote = ''
  note.class = ''
}

function fill(item) {
  reset()
  editing.value = true
  kind.value = item.kind || 'text'
  if (item.kind === 'song') {
    staged.value = {
      fbid: item.song_id,
      songbook_id: item.songbook_id,
      title: item.title,
      songbook_title: item.songbook_title || '',
      section_title: item.section_title || '',
    }
    planTitle.value = item.title || ''
    return
  }
  if (item.kind === 'bible') {
    bible.module_short_name = item.module_short_name || ''
    bible.module_short_name_second = item.module_short_name_second || ''
    bible.book = item.book || ''
    bible.chapter = item.chapter || ''
    bible.verse = item.verse || ''
    bible.reference = item.reference || ''
    bible.quote = item.quote || ''
    bible.class = item.class || ''
    return
  }
  note.caption = item.caption || ''
  note.reference = item.reference || ''
  note.quote = item.quote || ''
  note.class = item.class || ''
}

watch(() => props.item, (item) => {
  if (item) fill(item)
}, { immediate: true })

if (!props.item) {
  editing.value = false
  kind.value = localStorage.getItem(kindKey) || 'song'
  if (!kinds.some((item) => item.id === kind.value)) kind.value = 'song'
}

function setKind(next) {
  if (editing.value) return
  kind.value = next
  error.value = ''
}

function onSong(hit) {
  const previous = staged.value?.title || ''
  staged.value = hit
  if (!titleTouched.value || !planTitle.value.trim() || planTitle.value === previous) {
    planTitle.value = hit?.title || ''
    titleTouched.value = false
  }
  error.value = ''
}

function onPassage(hit) {
  bible.module_short_name = hit?.module_short_name || ''
  bible.book = hit?.book || ''
  bible.chapter = hit?.chapter || ''
  bible.verse = hit?.verse_start || ''
  bible.reference = hit?.reference || ''
  bible.quote = hit?.text || ''
  error.value = ''
}

function build() {
  if (kind.value === 'song') {
    if (!staged.value) throw new Error('Выберите песню.')
    return songItem(staged.value, planTitle.value)
  }
  if (kind.value === 'bible') return bibleItem(bible)
  return textItem(note)
}

function submit() {
  error.value = ''
  try {
    const item = build()
    if (editing.value) {
      emit('change', item)
      emit('close')
      return
    }
    localStorage.setItem(kindKey, kind.value)
    emit('add', item)
    added.value = itemTitle(item)
    if (kind.value === 'song') {
      staged.value = null
      planTitle.value = ''
      titleTouched.value = false
    }
  } catch (err) {
    error.value = err.message
  }
}
</script>

<template>
  <AppModal :title="editing ? 'Изменить пункт' : 'Добавить пункт'" @close="emit('close')">
    <div v-if="!editing" class="tabs" role="tablist" aria-label="Тип пункта">
      <button
        v-for="item in kinds"
        :key="item.id"
        type="button"
        class="tab"
        :class="{ on: kind === item.id }"
        @click="setKind(item.id)"
      >{{ item.label }}</button>
    </div>
    <p v-else class="hint">{{ kinds.find((item) => item.id === kind)?.label }}</p>
    <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
    <p v-if="added" class="banner ok" role="status">Добавлено: «{{ added }}»</p>

    <div v-if="kind === 'song'">
      <SongSearch :token="token" picking :songbook-id="songbookId" :section-id="sectionId" @pick="onSong">
        <template #filter>
          <SongbookPicker
            :token="token"
            :songbook-id="songbookId"
            :section-id="sectionId"
            @select="songbookId = $event.songbookId; sectionId = $event.sectionId"
          />
        </template>
      </SongSearch>
      <label>Название в плане
        <input v-model="planTitle" name="plan_title" @input="titleTouched = true" />
      </label>
    </div>

    <div v-else-if="kind === 'bible'">
      <BibleSearch :token="token" pick-label="Выбрать" @pick="onPassage" />
      <details class="more">
        <summary>Указать место вручную</summary>
        <div class="fields">
          <label>Перевод
            <input v-model="bible.module_short_name" name="module_short_name" placeholder="RST" />
          </label>
          <label>Второй перевод
            <input v-model="bible.module_short_name_second" name="module_short_name_second" placeholder="необязательно" />
          </label>
          <label>Книга
            <input v-model="bible.book" name="book" type="number" min="0" max="200" />
          </label>
          <label>Глава
            <input v-model="bible.chapter" name="chapter" type="number" min="0" max="200" />
          </label>
          <label>Стих
            <input v-model="bible.verse" name="verse" type="number" min="0" max="500" />
          </label>
          <label>Ссылка
            <input v-model="bible.reference" name="reference" placeholder="Ин. 3:16" />
          </label>
        </div>
        <label>Текст стиха
          <textarea v-model="bible.quote" name="quote" rows="3"></textarea>
        </label>
      </details>
    </div>

    <div v-else>
      <label>Заголовок
        <input v-model="note.caption" name="caption" />
      </label>
      <label>Ссылка
        <input v-model="note.reference" name="reference" />
      </label>
      <label>Текст
        <textarea v-model="note.quote" name="quote" rows="3"></textarea>
      </label>
      <details class="more">
        <summary>Ещё</summary>
        <label>Класс
          <input v-model="note.class" name="class" placeholder="можно оставить пустым" />
        </label>
      </details>
    </div>

    <template #footer>
      <button v-if="added" class="btn slim" type="button" @click="emit('close')">Готово</button>
      <button class="btn slim primary" type="button" @click="submit">{{ editing ? 'Сохранить' : 'Добавить' }}</button>
    </template>
  </AppModal>
</template>
