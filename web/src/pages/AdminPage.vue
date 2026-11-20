<script setup>
import { onMounted, ref, watch } from 'vue'
import { api, apiForm, explain } from '../api'
import { releaseVersionOk, statusText } from '../checks'
import { useSessionStore } from '../stores/session'

const session = useSessionStore()
const status = ref('pending')
const kind = ref('')
const submissions = ref([])
const releases = ref([])
const activeVersion = ref('')
const error = ref('')
const notice = ref('')
const loading = ref(true)
const busy = ref(false)
const rejecting = ref(null)
const note = ref('')
const version = ref('')
const notes = ref('')
const linuxSha = ref('')
const windowsSha = ref('')
const macosSha = ref('')
const files = ref({ linux_file: null, windows_file: null, macos_file: null })

const statuses = [
  { id: 'pending', label: 'На проверке' },
  { id: 'draft', label: 'Черновики' },
  { id: 'approved', label: 'Одобренные' },
  { id: 'rejected', label: 'Отклонённые' },
  { id: 'all', label: 'Все' },
]

function kindLabel(value) {
  if (value === 'songbook') return 'Сборник'
  if (value === 'module') return 'Модуль Библии'
  return value
}

function collection(value) {
  return value === 'module' ? 'modules' : 'songbooks'
}

async function loadSubmissions() {
  const params = new URLSearchParams()
  if (status.value) params.set('status', status.value)
  if (kind.value) params.set('kind', kind.value)
  const data = await api(`/api/admin/submissions?${params}`, { token: session.token })
  submissions.value = data.submissions || []
}

async function loadReleases() {
  const data = await api('/api/admin/releases', { token: session.token })
  activeVersion.value = data.active_version || ''
  releases.value = data.releases || []
}

async function load() {
  error.value = ''
  loading.value = true
  try {
    await Promise.all([loadSubmissions(), loadReleases()])
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
}

function isRejecting(item) {
  return rejecting.value?.id === item.id && rejecting.value?.kind === item.kind
}

async function moderate(item, action, body) {
  error.value = ''
  notice.value = ''
  busy.value = true
  try {
    await api(`/api/admin/${collection(item.kind)}/${item.id}/${action}`, {
      method: 'POST',
      token: session.token,
      body,
    })
    rejecting.value = null
    note.value = ''
    if (action === 'approve') notice.value = 'Одобрено. Теперь это видно в общем каталоге.'
    else if (action === 'reject') notice.value = 'Отклонено. Автор увидит причину.'
    else notice.value = 'Снято с публикации и возвращено в черновик.'
    await loadSubmissions()
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

function approve(item) {
  moderate(item, 'approve')
}

function startReject(item) {
  rejecting.value = { id: item.id, kind: item.kind }
  note.value = ''
}

function reject(item) {
  const text = note.value.trim()
  if (!text) {
    error.value = 'Напишите, что нужно исправить.'
    return
  }
  moderate(item, 'reject', { note: text })
}

function revoke(item) {
  moderate(item, 'revoke', {})
}

function takeFile(field, event) {
  files.value[field] = event.target.files?.[0] || null
}

async function upload(event) {
  event.preventDefault()
  error.value = ''
  notice.value = ''
  const next = version.value.trim()
  if (!releaseVersionOk(next)) {
    error.value = 'Версия: латиница, цифры и символы . _ + -, без «..» и без слэшей.'
    return
  }
  const form = new FormData()
  form.set('version', next)
  let count = 0
  for (const field of ['linux_file', 'windows_file', 'macos_file']) {
    if (files.value[field]) {
      form.set(field, files.value[field])
      count += 1
    }
  }
  if (!count) {
    error.value = 'Приложите хотя бы один файл: Linux, Windows или macOS.'
    return
  }
  if (notes.value.trim()) form.set('notes', notes.value.trim())
  if (linuxSha.value.trim()) form.set('linux_sha256', linuxSha.value.trim())
  if (windowsSha.value.trim()) form.set('windows_sha256', windowsSha.value.trim())
  if (macosSha.value.trim()) form.set('macos_sha256', macosSha.value.trim())
  busy.value = true
  try {
    await apiForm('/api/admin/releases', { form, token: session.token })
    notice.value = 'Сборка загружена. Люди её не увидят, пока вы её не опубликуете.'
    version.value = ''
    notes.value = ''
    linuxSha.value = ''
    windowsSha.value = ''
    macosSha.value = ''
    files.value = { linux_file: null, windows_file: null, macos_file: null }
    event.target.reset()
    await loadReleases()
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function publish(item) {
  error.value = ''
  notice.value = ''
  busy.value = true
  try {
    const data = await api(`/api/admin/releases/${encodeURIComponent(item.version)}/publish`, {
      method: 'POST',
      token: session.token,
    })
    activeVersion.value = data.active_version || item.version
    notice.value = 'Эта версия теперь предлагается для скачивания.'
    await loadReleases()
  } catch (err) {
    error.value = err.status === 409
      ? 'Для публикации нужны файлы Linux и Windows.'
      : explain(err)
  } finally {
    busy.value = false
  }
}

async function removeRelease(item) {
  error.value = ''
  notice.value = ''
  busy.value = true
  try {
    await api(`/api/admin/releases/${encodeURIComponent(item.version)}`, {
      method: 'DELETE',
      token: session.token,
    })
    notice.value = 'Черновик сборки удалён.'
    await loadReleases()
  } catch (err) {
    error.value = err.status === 409
      ? 'Опубликованную сборку удалить нельзя.'
      : explain(err)
  } finally {
    busy.value = false
  }
}

function readyToPublish(item) {
  return item.linux_size > 0 && item.windows_size > 0
}

onMounted(load)
watch([status, kind], async () => {
  error.value = ''
  try {
    await loadSubmissions()
  } catch (err) {
    error.value = explain(err)
  }
})
</script>

<template>
  <h1>Админ-панель</h1>
  <p v-if="notice" class="banner ok" role="status">{{ notice }}</p>
  <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  <p v-if="loading" class="empty" role="status">Загрузка…</p>
  <template v-else>
    <section>
      <h2>Модерация</h2>
      <div class="fields">
        <label>Статус
          <select v-model="status">
            <option v-for="item in statuses" :key="item.id" :value="item.id">{{ item.label }}</option>
          </select>
        </label>
        <label>Что показать
          <select v-model="kind">
            <option value="">Сборники и модули</option>
            <option value="songbook">Только сборники</option>
            <option value="module">Только модули</option>
          </select>
        </label>
      </div>
      <div v-if="!submissions.length" class="empty-state">
        <p>В этой очереди пусто.</p>
      </div>
      <div v-else class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Что</th>
              <th>Название</th>
              <th>Статус</th>
              <th>Причина</th>
              <th>Действие</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="item in submissions" :key="`${item.kind}-${item.id}`">
              <tr>
                <td>{{ kindLabel(item.kind) }}</td>
                <td>
                  <strong class="title-serif">{{ item.title || item.short_name || 'Без названия' }}</strong>
                  <p v-if="item.short_name" class="hint">{{ item.short_name }}</p>
                </td>
                <td><span class="badge" :class="item.status">{{ statusText(item.status) }}</span></td>
                <td>{{ item.moderation_note }}</td>
                <td class="btn-row">
                  <button class="btn slim primary" type="button" :disabled="busy" @click="approve(item)">Одобрить</button>
                  <button class="btn slim" type="button" :disabled="busy" @click="startReject(item)">Отклонить</button>
                  <button class="btn slim" type="button" :disabled="busy" @click="revoke(item)">Снять с публикации</button>
                </td>
              </tr>
              <tr v-if="isRejecting(item)">
                <td colspan="5">
                  <form @submit.prevent="reject(item)">
                    <label>Что исправить
                      <input v-model="note" name="note" required />
                    </label>
                    <button class="btn slim danger" type="submit" :disabled="busy">Отклонить с этой причиной</button>
                  </form>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </section>

    <section>
      <h2>Сборки программы</h2>
      <p v-if="activeVersion" class="hint">Сейчас для скачивания предлагается версия {{ activeVersion }}.</p>
      <p v-else class="hint">Активной версии ещё нет. Загрузите черновик и опубликуйте его.</p>
      <div v-if="!releases.length" class="empty-state">
        <p>Сборок пока нет.</p>
      </div>
      <div v-else class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Версия</th>
              <th>Состояние</th>
              <th>Файлы</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in releases" :key="item.version">
              <td>
                <strong>{{ item.version }}</strong>
                <span v-if="item.version === activeVersion" class="badge approved">скачивают эту</span>
              </td>
              <td>{{ item.status === 'released' ? 'опубликована' : 'черновик' }}</td>
              <td>
                <span v-if="item.linux_size">Linux</span>
                <span v-if="item.windows_size"> Windows</span>
                <span v-if="item.macos_size"> macOS</span>
              </td>
              <td class="btn-row">
                <button class="btn slim primary" type="button" :disabled="busy || !readyToPublish(item)" @click="publish(item)">Опубликовать</button>
                <button v-if="item.status === 'dev'" class="btn slim" type="button" :disabled="busy" @click="removeRelease(item)">Удалить черновик</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="hint">Для публикации нужны Linux и Windows. macOS можно добавить сразу или позже, пока сборка ещё черновик.</p>
      <form class="card" @submit="upload">
        <h3>Загрузить черновик</h3>
        <label>Версия
          <input v-model="version" name="version" placeholder="1.0.0" required />
        </label>
        <div class="fields">
          <label>Linux
            <input name="linux_file" type="file" @change="takeFile('linux_file', $event)" />
          </label>
          <label>Windows
            <input name="windows_file" type="file" @change="takeFile('windows_file', $event)" />
          </label>
          <label>macOS
            <input name="macos_file" type="file" @change="takeFile('macos_file', $event)" />
          </label>
        </div>
        <label>Заметки к версии
          <textarea v-model="notes" name="notes" rows="3"></textarea>
        </label>
        <details>
          <summary>Сверить контрольные суммы</summary>
          <label>SHA-256 Linux
            <input v-model="linuxSha" name="linux_sha256" />
          </label>
          <label>SHA-256 Windows
            <input v-model="windowsSha" name="windows_sha256" />
          </label>
          <label>SHA-256 macOS
            <input v-model="macosSha" name="macos_sha256" />
          </label>
        </details>
        <button class="btn primary" type="submit" :disabled="busy">Загрузить сборку</button>
      </form>
    </section>
  </template>
</template>
