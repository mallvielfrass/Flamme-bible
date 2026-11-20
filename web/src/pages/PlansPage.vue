<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { api, apiBlob, explain } from '../api'
import { clipPlanText, decodePlanText, importEncodingNotice } from '../checks'
import {
  bibleItem,
  exportFileName,
  importFormatFromName,
  itemKindLabel,
  itemTitle,
  moveItem,
  songItem,
  textItem,
} from '../planItems'
import { useSessionStore } from '../stores/session'
import SongSearch from '../components/SongSearch.vue'

const session = useSessionStore()
const plans = ref([])
const scope = ref('all')
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const editor = ref(null)
const addTab = ref('song')
const confirmDelete = ref(false)
const importFormat = ref('json')
const importTitle = ref('')
const exportEncoding = ref('utf-8')
const repair = ref(null)
const bible = reactive({
  module_short_name: '',
  module_short_name_second: '',
  book: '',
  chapter: '',
  verse: '',
  reference: '',
  quote: '',
})
const note = reactive({ caption: '', reference: '', quote: '', class: '' })

const scopes = [
  { id: 'all', label: 'Все' },
  { id: 'personal', label: 'Личные' },
  { id: 'community', label: 'Общинные' },
]

const placeLabel = computed(() => {
  const space = session.current
  if (space?.kind === 'community') return `План будет в общине «${space.name}»`
  return 'План будет личным'
})

function points(count) {
  const value = count || 0
  const mod10 = value % 10
  const mod100 = value % 100
  if (mod10 === 1 && mod100 !== 11) return `${value} пункт`
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return `${value} пункта`
  return `${value} пунктов`
}

function applyEditor(view) {
  editor.value = {
    id: view.id,
    title: view.title || '',
    version: view.version,
    updated_at: view.updated_at || '',
    items: (view.items || []).map((item) => ({ ...item })),
  }
  confirmDelete.value = false
  repair.value = null
}

async function load() {
  error.value = ''
  loading.value = true
  try {
    const params = new URLSearchParams({ scope: scope.value })
    const list = await api(`/api/plans?${params}`, { token: session.token })
    plans.value = Array.isArray(list) ? list : []
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
}

function blank() {
  editor.value = { id: '', title: '', version: 0, updated_at: '', items: [] }
  confirmDelete.value = false
  repair.value = null
  error.value = ''
  notice.value = ''
}

async function openPlan(plan) {
  error.value = ''
  notice.value = ''
  repair.value = null
  busy.value = true
  try {
    applyEditor(await api(`/api/plans/${plan.id}`, { token: session.token }))
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

function addSong(hit) {
  try {
    editor.value.items.push(songItem(hit))
    error.value = ''
  } catch (err) {
    error.value = err.message
  }
}

function addBible() {
  try {
    editor.value.items.push(bibleItem(bible))
    bible.module_short_name = ''
    bible.module_short_name_second = ''
    bible.book = ''
    bible.chapter = ''
    bible.verse = ''
    bible.reference = ''
    bible.quote = ''
    error.value = ''
  } catch (err) {
    error.value = err.message
  }
}

function addText() {
  try {
    editor.value.items.push(textItem(note))
    note.caption = ''
    note.reference = ''
    note.quote = ''
    note.class = ''
    error.value = ''
  } catch (err) {
    error.value = err.message
  }
}

function move(index, delta) {
  editor.value.items = moveItem(editor.value.items, index, delta)
}

function remove(index) {
  editor.value.items.splice(index, 1)
}

async function save() {
  if (!editor.value) return
  const title = editor.value.title.trim()
  error.value = ''
  notice.value = ''
  if (!title) {
    error.value = 'Введите название плана.'
    return
  }
  busy.value = true
  try {
    if (!editor.value.id) {
      const space = session.current
      const created = await api('/api/plans', {
        method: 'POST',
        token: session.token,
        body: {
          title,
          dimension_id: space?.kind === 'community' ? space.id : '',
          items: editor.value.items,
        },
      })
      applyEditor(created)
      notice.value = 'План создан.'
    } else {
      const saved = await api(`/api/plans/${editor.value.id}`, {
        method: 'PUT',
        token: session.token,
        body: {
          title,
          version: editor.value.version,
          updated_at: editor.value.updated_at || undefined,
          items: editor.value.items,
        },
      })
      applyEditor(saved)
      notice.value = 'План сохранён.'
    }
    await load()
  } catch (err) {
    if (err.status === 409 && err.body?.plan) {
      applyEditor(err.body.plan)
      error.value = 'План уже изменили. Показана последняя версия — проверьте пункты и сохраните ещё раз.'
    } else {
      error.value = explain(err)
    }
  } finally {
    busy.value = false
  }
}

async function removePlan() {
  if (!editor.value?.id) return
  if (!confirmDelete.value) {
    confirmDelete.value = true
    return
  }
  busy.value = true
  error.value = ''
  try {
    const params = new URLSearchParams({ version: String(editor.value.version) })
    if (editor.value.updated_at) params.set('updated_at', editor.value.updated_at)
    await api(`/api/plans/${editor.value.id}?${params}`, { method: 'DELETE', token: session.token })
    editor.value = null
    notice.value = 'План удалён.'
    await load()
  } catch (err) {
    if (err.status === 409 && err.body?.plan) {
      applyEditor(err.body.plan)
      error.value = 'План уже изменили. Удаление не выполнено.'
    } else {
      error.value = explain(err)
    }
  } finally {
    busy.value = false
  }
}

async function importPlan(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  const guessed = importFormatFromName(file.name)
  if (guessed) importFormat.value = guessed
  error.value = ''
  notice.value = ''
  busy.value = true
  try {
    const payload = decodePlanText(new Uint8Array(await file.arrayBuffer()))
    const space = session.current
    const title = importTitle.value.trim()
    const created = await api('/api/plans/import', {
      method: 'POST',
      token: session.token,
      body: {
        format: importFormat.value,
        payload,
        title,
        dimension_id: space?.kind === 'community' ? space.id : '',
      },
    })
    applyEditor(created)
    repair.value = importEncodingNotice(payload, created)
    if (repair.value && title) repair.value.title = ''
    importTitle.value = ''
    notice.value = repair.value ? 'План импортирован. Проверьте кодировку.' : 'План импортирован.'
    await load()
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function applyRepair() {
  if (!editor.value?.id || !repair.value) return
  error.value = ''
  notice.value = ''
  busy.value = true
  try {
    const saved = await api(`/api/plans/${editor.value.id}`, {
      method: 'PUT',
      token: session.token,
      body: {
        title: repair.value.title || editor.value.title,
        version: editor.value.version,
        updated_at: editor.value.updated_at || undefined,
        items: repair.value.items,
      },
    })
    applyEditor(saved)
    repair.value = null
    notice.value = 'Кодировка исправлена.'
    await load()
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function download(format) {
  if (!editor.value?.id) return
  error.value = ''
  try {
    const encoding = format === 'json' ? '' : `&encoding=${encodeURIComponent(exportEncoding.value)}`
    const { blob } = await apiBlob(`/api/plans/${editor.value.id}/export?format=${format}${encoding}`, { token: session.token })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = exportFileName(format)
    link.click()
    URL.revokeObjectURL(url)
  } catch (err) {
    error.value = explain(err)
  }
}

watch(scope, load, { immediate: true })
</script>

<template>
  <div class="page-head">
    <h1>Планы</h1>
    <button class="btn slim primary" type="button" @click="blank">Создать план</button>
  </div>
  <div class="tabs" role="tablist" aria-label="Какие планы показать">
    <button
      v-for="item in scopes"
      :key="item.id"
      type="button"
      class="tab"
      :class="{ on: scope === item.id }"
      @click="scope = item.id"
    >{{ item.label }}</button>
  </div>
  <p v-if="error && !editor" class="banner bad" role="alert">{{ error }}</p>
  <p v-if="notice && !editor" class="banner ok" role="status">{{ notice }}</p>
  <div class="plan-layout">
    <section>
      <p v-if="loading" class="empty" role="status">Загрузка…</p>
      <div v-else-if="!plans.length" class="empty-state">
        <p>Планов пока нет.</p>
        <button class="btn slim primary" type="button" @click="blank">Создать первый план</button>
      </div>
      <ul v-else class="plain">
        <li v-for="plan in plans" :key="plan.id">
          <button class="text-button" type="button" @click="openPlan(plan)">
            <strong class="title-serif">{{ plan.title }}</strong>
          </button>
          <p class="hint">
            <span class="badge">{{ plan.dimension_id ? 'Общинный' : 'Личный' }}</span>
            {{ points(plan.items?.length) }}
          </p>
        </li>
      </ul>
    </section>

    <section v-if="editor" class="card">
      <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
      <p v-if="notice" class="banner ok" role="status">{{ notice }}</p>
      <div v-if="repair" class="banner bad" role="alert">
        <p>{{ repair.warning }}</p>
        <p class="hint">До</p>
        <p class="quote">{{ clipPlanText(repair.before) }}</p>
        <p class="hint">После</p>
        <p class="quote">{{ clipPlanText(repair.after) }}</p>
        <button class="btn slim" type="button" :disabled="busy" @click="applyRepair">Починить кодировку</button>
      </div>
      <p class="hint">{{ placeLabel }}</p>
      <label>Название
        <input v-model="editor.title" name="title" />
      </label>

      <h2>Пункты</h2>
      <div v-if="!editor.items.length" class="empty-state">
        <p>В плане пока пусто. Добавьте песню, стих или заметку.</p>
      </div>
      <ol v-else class="plain order">
        <li v-for="(item, index) in editor.items" :key="item.id || index">
          <div class="card-head">
            <span class="badge">{{ itemKindLabel(item.kind) }}</span>
            <strong class="title-serif">{{ itemTitle(item) }}</strong>
          </div>
          <p v-if="item.kind === 'bible' && item.module_short_name_second" class="hint">Второй перевод: {{ item.module_short_name_second }}</p>
          <p v-if="item.quote" class="quote">{{ item.quote }}</p>
          <div class="btn-row">
            <button class="btn slim" type="button" :disabled="index === 0" @click="move(index, -1)">Выше</button>
            <button class="btn slim" type="button" :disabled="index === editor.items.length - 1" @click="move(index, 1)">Ниже</button>
            <button class="btn slim" type="button" @click="remove(index)">Убрать</button>
          </div>
        </li>
      </ol>

      <div class="tabs" role="tablist" aria-label="Что добавить">
        <button type="button" class="tab" :class="{ on: addTab === 'song' }" @click="addTab = 'song'">Песня</button>
        <button type="button" class="tab" :class="{ on: addTab === 'bible' }" @click="addTab = 'bible'">Стих из Библии</button>
        <button type="button" class="tab" :class="{ on: addTab === 'text' }" @click="addTab = 'text'">Текст</button>
      </div>
      <SongSearch v-if="addTab === 'song'" :token="session.token" picking @pick="addSong" />
      <form v-else-if="addTab === 'bible'" @submit.prevent="addBible">
        <div class="fields">
          <label>Перевод
            <input v-model="bible.module_short_name" name="module_short_name" placeholder="RST" required />
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
        <button class="btn slim primary" type="submit">Добавить стих</button>
      </form>
      <form v-else @submit.prevent="addText">
        <label>Заголовок
          <input v-model="note.caption" name="caption" />
        </label>
        <label>Ссылка
          <input v-model="note.reference" name="reference" />
        </label>
        <label>Текст
          <textarea v-model="note.quote" name="quote" rows="3"></textarea>
        </label>
        <label>Класс
          <input v-model="note.class" name="class" placeholder="можно оставить пустым" />
        </label>
        <button class="btn slim primary" type="submit">Добавить текст</button>
      </form>

      <div class="btn-row">
        <button class="btn slim primary" type="button" :disabled="busy" @click="save">Сохранить план</button>
        <button v-if="editor.id" class="btn slim" type="button" :disabled="busy" @click="removePlan">
          {{ confirmDelete ? 'Подтвердить удаление' : 'Удалить план' }}
        </button>
      </div>

      <h3>Файл плана</h3>
      <p class="hint">Можно принести plan.vplt, History.ini, PlanDef.ini или JSON и скачать план в том же виде.</p>
      <div class="fields">
        <label>Формат
          <select v-model="importFormat">
            <option value="json">JSON</option>
            <option value="vplt">plan.vplt</option>
            <option value="history">History.ini</option>
            <option value="plandef">PlanDef.ini</option>
          </select>
        </label>
        <label>Название при импорте
          <input v-model="importTitle" name="import_title" placeholder="необязательно" />
        </label>
        <label>Кодировка файла
          <select v-model="exportEncoding">
            <option value="utf-8">UTF-8</option>
            <option value="windows-1251">Windows-1251</option>
          </select>
        </label>
      </div>
      <label>Импортировать файл
        <input type="file" accept=".json,.vplt,.ini,.txt,text/plain" @change="importPlan" />
      </label>
      <div v-if="editor.id" class="btn-row">
        <button class="btn slim" type="button" @click="download('vplt')">Скачать plan.vplt</button>
        <button class="btn slim" type="button" @click="download('history')">Скачать History.ini</button>
        <button class="btn slim" type="button" @click="download('plandef')">Скачать PlanDef.ini</button>
        <button class="btn slim" type="button" @click="download('json')">Скачать JSON</button>
      </div>
    </section>
    <section v-else class="empty-state">
      <p>Выберите план слева или создайте новый.</p>
    </section>
  </div>
</template>
