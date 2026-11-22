<script setup>
import { onMounted, ref } from 'vue'
import { api, apiBlob, explain } from '../../api'
import { formatSize } from '../../checks'
import { useSessionStore } from '../../stores/session'
import AppModal from '../../../shared/components/AppModal.vue'
import ConfirmDialog from '../../../shared/components/ConfirmDialog.vue'
import DataTable from '../DataTable.vue'
import IconButton from '../../../shared/components/IconButton.vue'
import StatusBadge from '../StatusBadge.vue'
import ReleaseUploadDialog from './ReleaseUploadDialog.vue'

const session = useSessionStore()
const rows = ref([])
const active = ref('')
const loading = ref(true)
const error = ref('')
const actionError = ref('')
const toastText = ref('')
const uploadOpen = ref(false)
const lockedVersion = ref('')
const notesEdit = ref(null)
const notesText = ref('')
const confirms = ref(null)
let toastTimer = 0

const columns = [
  { key: 'version', label: 'Версия' },
  { key: 'state', label: 'Состояние' },
  { key: 'files', label: 'Файлы' },
  { key: 'notes', label: 'Заметки' },
  { key: 'dates', label: 'Даты' },
  { key: 'actions', label: '' },
]

const platforms = [
  { id: 'linux', label: 'Linux' },
  { id: 'windows', label: 'Windows' },
  { id: 'macos', label: 'macOS' },
]

function showToast(text) {
  toastText.value = text
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastText.value = '' }, 3500)
}

function ready(item) {
  return item.linux_size > 0 && item.windows_size > 0
}

function stateOf(item) {
  if (item.version === active.value && item.status === 'released') return 'live'
  if (item.status === 'released') return 'released'
  return 'draft'
}

async function load() {
  error.value = ''
  loading.value = true
  try {
    const data = await api('/api/admin/releases', { token: session.token })
    active.value = data.active_version || ''
    rows.value = (data.releases || []).map((item) => ({ ...item, key: item.version }))
  } catch (err) {
    rows.value = []
    error.value = err.status === 404
      ? 'Скоро: релизы появятся после обновления сервера.'
      : explain(err)
  } finally {
    loading.value = false
  }
}

function openUpload(version = '') {
  lockedVersion.value = version
  uploadOpen.value = true
}

async function publish(item) {
  const rollback = item.status === 'released' && item.version !== active.value
  const text = rollback
    ? `Люди снова будут получать v${item.version} вместо v${active.value}.`
    : `Версия ${item.version} станет доступна всем для скачивания и автообновления.`
  const ok = await confirms.value.confirm({
    title: rollback ? 'Вернуть эту версию людям (откат)' : 'Опубликовать',
    text,
    action: rollback ? 'Откатить' : 'Опубликовать',
    danger: false,
  })
  if (!ok) return
  actionError.value = ''
  try {
    const data = await api(`/api/admin/releases/${encodeURIComponent(item.version)}/publish`, {
      method: 'POST',
      token: session.token,
    })
    active.value = data.active_version || item.version
    showToast(rollback ? 'Версия снова у людей.' : 'Версия опубликована.')
    await load()
    session.loadAdminSummary()
  } catch (err) {
    actionError.value = err.status === 409 ? 'Для публикации нужны файлы Linux и Windows.' : explain(err)
  }
}

async function remove(item) {
  const ok = await confirms.value.confirm({
    title: 'Удалить черновик',
    text: 'Файлы будут удалены с сервера, это нельзя отменить.',
    action: 'Удалить',
  })
  if (!ok) return
  actionError.value = ''
  try {
    await api(`/api/admin/releases/${encodeURIComponent(item.version)}`, {
      method: 'DELETE',
      token: session.token,
    })
    showToast('Черновик удалён.')
    await load()
    session.loadAdminSummary()
  } catch (err) {
    actionError.value = err.status === 409 ? 'Опубликованную сборку удалить нельзя.' : explain(err)
  }
}

async function copySha(item) {
  const lines = platforms
    .filter((platform) => item[`${platform.id}_sha256`])
    .map((platform) => `${platform.label} ${item[`${platform.id}_sha256`]}`)
  try {
    await navigator.clipboard.writeText(lines.join('\n'))
    showToast('Суммы скопированы.')
  } catch {
    actionError.value = 'Не удалось скопировать.'
  }
}

async function download(item, platform) {
  actionError.value = ''
  const released = item.status === 'released'
  const path = released
    ? `/api/releases/${encodeURIComponent(item.version)}/download/${platform}`
    : `/api/admin/releases/${encodeURIComponent(item.version)}/download/${platform}`
  try {
    const file = await apiBlob(path, { token: released ? '' : session.token })
    const url = URL.createObjectURL(file.blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `${item.version}-${platform}`
    link.click()
    URL.revokeObjectURL(url)
  } catch (err) {
    actionError.value = err.status === 404 ? 'Скачивание черновика появится после обновления сервера.' : explain(err)
  }
}

function startNotes(item) {
  notesEdit.value = item.version
  notesText.value = item.notes || ''
}

async function saveNotes(item) {
  actionError.value = ''
  try {
    await api(`/api/admin/releases/${encodeURIComponent(item.version)}`, {
      method: 'PATCH',
      token: session.token,
      body: { notes: notesText.value },
    })
    notesEdit.value = null
    showToast('Заметки сохранены.')
    await load()
  } catch (err) {
    actionError.value = err.status === 404 ? 'Правка заметок появится после обновления сервера.' : explain(err)
  }
}

function shortHash(value) {
  return value ? value.slice(0, 8) : ''
}

onMounted(load)
</script>

<template>
  <p class="hint">
    Люди скачивают {{ active ? `v${active}` : 'ещё ни одну версию' }}.
    Черновик видят только админы. Для публикации нужны Linux и Windows.
  </p>
  <div class="admin-tools">
    <button class="btn slim primary" type="button" @click="openUpload()">Загрузить сборку</button>
  </div>
  <p v-if="actionError" class="error">{{ actionError }}</p>
  <DataTable
    :columns="columns"
    :rows="rows"
    row-key="key"
    :loading="loading"
    :error="error"
    empty="Сборок пока нет."
    @retry="load"
  >
    <template #cell-version="{ row }"><strong>{{ row.version }}</strong></template>
    <template #cell-state="{ row }"><StatusBadge :status="stateOf(row)" /></template>
    <template #cell-files="{ row }">
      <span
        v-for="platform in platforms"
        :key="platform.id"
        class="pill"
        :class="{ missing: !row[`${platform.id}_size`] }"
        :title="row[`${platform.id}_sha256`] || 'файла нет'"
      >
        {{ platform.label }}
        <template v-if="row[`${platform.id}_size`]"> {{ shortHash(row[`${platform.id}_sha256`]) }} · {{ formatSize(row[`${platform.id}_size`]) }}</template>
      </span>
    </template>
    <template #cell-notes="{ row }">
      <span>{{ row.notes }}</span>
      <IconButton icon="pencil" label="Править заметки" small @click="startNotes(row)" />
    </template>
    <template #cell-dates="{ row }">
      <span class="subline">загружена {{ row.created_at ? new Date(row.created_at).toLocaleDateString('ru-RU') : '—' }}</span>
      <p v-if="row.published_at" class="subline">опубликована {{ new Date(row.published_at).toLocaleDateString('ru-RU') }}</p>
    </template>
    <template #cell-actions="{ row }">
      <div class="row-actions">
        <IconButton v-if="row.status === 'dev'" icon="plus" tone="green" label="Добавить файлы" @click="openUpload(row.version)" />
        <IconButton
          v-for="platform in platforms.filter((item) => row[`${item.id}_size`])"
          :key="platform.id"
          icon="download"
          :label="`Скачать ${platform.label}`"
          @click="download(row, platform.id)"
        />
        <IconButton
          v-if="row.status === 'released'"
          icon="copy"
          label="Копировать SHA-256"
          @click="copySha(row)"
        />
        <IconButton
          v-if="row.status === 'dev' || (row.status === 'released' && row.version !== active)"
          icon="rocket"
          tone="green"
          :label="row.status === 'dev'
            ? (ready(row) ? 'Опубликовать' : 'Для публикации нужны Linux и Windows.')
            : 'Вернуть эту версию людям (откат)'"
          :disabled="row.status === 'dev' && !ready(row)"
          @click="publish(row)"
        />
        <IconButton v-if="row.status === 'dev'" icon="trash" tone="red" label="Удалить черновик" @click="remove(row)" />
      </div>
    </template>
  </DataTable>
  <p class="hint mono">go run ./cmd/upload -version … -windows … -linux …</p>
  <p v-if="toastText" class="toast" role="status">{{ toastText }}</p>
  <ReleaseUploadDialog v-if="uploadOpen" :version="lockedVersion" @close="uploadOpen = false" @done="uploadOpen = false; load()" />
  <AppModal v-if="notesEdit" title="Заметки к версии" @close="notesEdit = null">
    <label>Текст
      <textarea v-model="notesText" rows="4"></textarea>
    </label>
    <template #footer>
      <button class="btn slim" type="button" @click="notesEdit = null">Отмена</button>
      <button class="btn slim primary" type="button" @click="saveNotes(rows.find((item) => item.version === notesEdit))">Сохранить</button>
    </template>
  </AppModal>
  <ConfirmDialog ref="confirms" />
</template>
