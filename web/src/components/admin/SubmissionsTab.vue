<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { api, explain } from '../../api'
import { canApprove, canReject, canRevoke, formatWhen, moderationHint } from '../../checks'
import { useSessionStore } from '../../stores/session'
import AppModal from '../AppModal.vue'
import ConfirmDialog from '../ConfirmDialog.vue'
import DataTable from '../DataTable.vue'
import IconButton from '../IconButton.vue'
import StatusBadge from '../StatusBadge.vue'
import EmbedQueue from './EmbedQueue.vue'

const session = useSessionStore()
const status = ref('pending')
const kind = ref('')
const q = ref('')
const offset = ref(0)
const limit = 50
const rows = ref([])
const total = ref(0)
const counts = ref(null)
const loading = ref(true)
const error = ref('')
const selected = ref([])
const expanded = ref('')
const busyKey = ref('')
const actionError = ref('')
const toastText = ref('')
const detail = ref(null)
const rejectOpen = ref(false)
const rejectNote = ref('')
const rejectTargets = ref([])
const revokeOpen = ref(false)
const revokeNote = ref('')
const revokeTarget = ref(null)
const confirms = ref(null)
let toastTimer = 0

const columns = [
  { key: 'kind', label: 'Тип' },
  { key: 'title', label: 'Название' },
  { key: 'who', label: 'Община' },
  { key: 'submitted', label: 'Отправлено' },
  { key: 'status', label: 'Статус' },
  { key: 'actions', label: '' },
]

const kindFilters = computed(() => {
  const bag = counts.value?.kind || {}
  return [
    { id: '', label: 'Всё', n: bag.all },
    { id: 'songbook', label: 'Сборники', n: bag.songbook },
    { id: 'module', label: 'Библия', n: bag.module },
  ]
})

const statusFilters = computed(() => {
  const bag = counts.value?.status || {}
  return [
    { id: 'pending', label: 'На проверке', n: bag.pending },
    { id: 'approved', label: 'Одобренные', n: bag.approved },
    { id: 'rejected', label: 'Отклонённые', n: bag.rejected },
    { id: 'draft', label: 'Черновики', n: bag.draft },
    { id: 'hidden', label: 'Скрытые', n: bag.hidden },
    { id: 'all', label: 'Все', n: bag.all },
  ]
})

const presets = ['Дубликат', 'Нет авторов', 'Битая кодировка', 'Нарушение авторских прав']

const tableRows = computed(() => rows.value.map((row) => ({ ...row, key: keyOf(row) })))

function keyOf(row) {
  return `${row.kind}:${row.id}`
}

function collection(value) {
  return value === 'module' ? 'modules' : 'songbooks'
}

function showToast(text) {
  toastText.value = text
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastText.value = '' }, 3500)
}

async function load() {
  error.value = ''
  loading.value = true
  const params = new URLSearchParams()
  params.set('status', status.value)
  if (kind.value) params.set('kind', kind.value)
  const needle = q.value.trim()
  if (needle) params.set('q', needle)
  params.set('sort', 'submitted')
  params.set('limit', String(limit))
  params.set('offset', String(offset.value))
  try {
    const data = await api(`/api/admin/submissions?${params}`, { token: session.token })
    let list = data.submissions || []
    if (data.total == null && needle) {
      const low = needle.toLowerCase()
      list = list.filter((item) => [item.title, item.short_name, item.author_email]
        .some((part) => String(part || '').toLowerCase().includes(low)))
    }
    rows.value = list
    total.value = data.total ?? list.length
    counts.value = data.counts || null
  } catch (err) {
    rows.value = []
    error.value = err.status === 400 && status.value === 'hidden'
      ? 'Скрытые появятся после обновления сервера.'
      : explain(err)
  } finally {
    loading.value = false
  }
}

function reload() {
  offset.value = 0
  selected.value = []
  load()
}

async function loadDetail(row) {
  detail.value = null
  actionError.value = ''
  const path = row.kind === 'module' ? `/api/admin/modules/${row.id}` : `/api/admin/songbooks/${row.id}`
  try {
    detail.value = await api(path, { token: session.token })
  } catch (err) {
    if (err.status === 404) {
      detail.value = { fallback: true, note: row.moderation_note || '' }
      return
    }
    actionError.value = explain(err)
  }
}

function onExpanded(value) {
  expanded.value = value
  const row = rows.value.find((item) => keyOf(item) === value)
  if (row) loadDetail(row)
}

async function moderate(row, action, body) {
  actionError.value = ''
  busyKey.value = keyOf(row)
  try {
    await api(`/api/admin/${collection(row.kind)}/${row.id}/${action}`, {
      method: 'POST',
      token: session.token,
      body: body || {},
    })
    const text = action === 'approve' ? 'Одобрено.' : action === 'reject' ? 'Отклонено.' : 'Снято с публикации.'
    showToast(text)
    await load()
    session.loadAdminSummary()
  } catch (err) {
    actionError.value = explain(err)
    throw err
  } finally {
    busyKey.value = ''
  }
}

function approve(row) {
  moderate(row, 'approve', {}).catch(() => {})
}

function startReject(list) {
  rejectTargets.value = list
  rejectNote.value = ''
  rejectOpen.value = true
}

async function sendReject() {
  const note = rejectNote.value.trim()
  if (!note) {
    actionError.value = 'Напишите причину.'
    return
  }
  rejectOpen.value = false
  await runMany(rejectTargets.value, 'reject', { note })
}

function startRevoke(row) {
  revokeTarget.value = row
  revokeNote.value = ''
  revokeOpen.value = true
}

async function sendRevoke() {
  const row = revokeTarget.value
  revokeOpen.value = false
  if (!row) return
  const note = revokeNote.value.trim()
  moderate(row, 'revoke', note ? { note } : {}).catch(() => {})
}

async function remove(row) {
  const patches = Number(row.patch_count || 0)
  const text = row.kind === 'module'
    ? 'Модуль и его стихи будут удалены безвозвратно.'
    : patches
      ? `У сборника есть ${patches} патчей — он и патчи будут скрыты.`
      : 'Сборник будет удалён безвозвратно. Если патчи есть, сборник будет скрыт вместе с ними.'
  const ok = await confirms.value.confirm({ title: 'Удалить', text, action: 'Удалить' })
  if (!ok) return
  busyKey.value = keyOf(row)
  actionError.value = ''
  try {
    await api(`/api/admin/${collection(row.kind)}/${row.id}`, { method: 'DELETE', token: session.token })
    showToast(patches ? 'Скрыто.' : 'Удалено.')
    await load()
  } catch (err) {
    actionError.value = err.status === 404 ? 'Удаление появится после обновления сервера.' : explain(err)
  } finally {
    busyKey.value = ''
  }
}

async function unhide(row) {
  busyKey.value = keyOf(row)
  try {
    await api(`/api/admin/songbooks/${row.id}/unhide`, { method: 'POST', token: session.token })
    showToast('Сборник снова виден.')
    await load()
  } catch (err) {
    actionError.value = err.status === 404 ? 'Возврат появится после обновления сервера.' : explain(err)
  } finally {
    busyKey.value = ''
  }
}

async function approveSelected() {
  const items = picked()
  if (!items.length) return
  const ok = await confirms.value.confirm({
    title: 'Одобрить выбранные',
    text: items.map((item) => item.title || item.short_name || 'Без названия').join('\n'),
    action: 'Одобрить',
    danger: false,
  })
  if (!ok) return
  await runMany(items, 'approve', {})
}

function picked() {
  const have = new Set(selected.value)
  return rows.value.filter((row) => have.has(keyOf(row)))
}

async function runMany(items, action, body) {
  let done = 0
  let failed = 0
  for (const item of items) {
    try {
      await api(`/api/admin/${collection(item.kind)}/${item.id}/${action}`, {
        method: 'POST',
        token: session.token,
        body,
      })
      done += 1
    } catch {
      failed += 1
    }
  }
  const verb = action === 'approve' ? 'одобрено' : 'отклонено'
  showToast(failed ? `${verb} ${done} из ${items.length}, ${failed} ошибка` : `${verb} ${done} из ${items.length}`)
  selected.value = []
  await load()
  session.loadAdminSummary()
}

function subline(row) {
  const count = row.kind === 'module' ? row.books_count : row.songs_count
  const unit = row.kind === 'module' ? 'книг' : 'песен'
  const parts = []
  if (count != null) parts.push(`${count} ${unit}`)
  if (row.short_name) parts.push(row.short_name)
  return parts.join(' · ')
}

onMounted(load)
watch([status, kind], reload)
</script>

<template>
  <EmbedQueue v-if="session.role === 'admin'" />
  <div class="admin-tools">
    <label class="search">Поиск
      <input v-model="q" type="search" placeholder="Название, короткое имя, почта" @change="reload" />
    </label>
    <div class="seg" role="group" aria-label="Тип">
      <button v-for="item in kindFilters" :key="item.label" type="button" :class="{ on: kind === item.id }" @click="kind = item.id">
        {{ item.label }}<template v-if="item.n != null"> {{ item.n }}</template>
      </button>
    </div>
    <div class="seg" role="group" aria-label="Статус">
      <button v-for="item in statusFilters" :key="item.id" type="button" :class="{ on: status === item.id }" @click="status = item.id">
        {{ item.label }}<template v-if="item.n != null"> {{ item.n }}</template>
      </button>
    </div>
  </div>
  <div v-if="selected.length" class="bulk-bar">
    <span>Выбрано: {{ selected.length }}</span>
    <button class="btn slim" type="button" @click="approveSelected">Одобрить выбранные</button>
    <button class="btn slim" type="button" @click="startReject(picked())">Отклонить…</button>
    <button class="btn slim" type="button" @click="selected = []">Снять выбор</button>
  </div>
  <p v-if="actionError" class="error">{{ actionError }}</p>
  <DataTable
    :columns="columns"
    :rows="tableRows"
    row-key="key"
    selectable
    expandable
    :loading="loading"
    :error="error"
    empty="В этой очереди пусто."
    :selected="selected"
    :expanded="expanded"
    @update:selected="selected = $event"
    @update:expanded="onExpanded"
    @retry="load"
  >
    <template #cell-kind="{ row }">
      <StatusBadge :status="row.kind === 'module' ? 'module' : 'songbook'" />
    </template>
    <template #cell-title="{ row }">
      <strong class="title-serif">{{ row.title || row.short_name || 'Без названия' }}</strong>
      <p v-if="subline(row)" class="subline">{{ subline(row) }}</p>
      <p v-if="row.kind === 'songbook' && row.index_pending" class="subline">Индексируется: {{ row.index_pending }} куплетов</p>
      <p v-if="row.kind === 'songbook' && row.index_failed" class="subline">Ошибки индексации: {{ row.index_failed }}</p>
    </template>
    <template #cell-who="{ row }">
      <span>{{ row.community_name || '—' }}</span>
      <p v-if="row.author_email" class="subline">{{ row.author_email }}</p>
    </template>
    <template #cell-submitted="{ row }">{{ formatWhen(row.submitted_at) }}</template>
    <template #cell-status="{ row }">
      <StatusBadge v-if="row.hidden_at" status="hidden" />
      <StatusBadge v-else :status="row.status" />
    </template>
    <template #cell-actions="{ row }">
      <div class="row-actions">
        <IconButton icon="eye" label="Смотреть" :disabled="busyKey === row.key" @click="onExpanded(row.key)" />
        <IconButton
          v-if="row.hidden_at"
          icon="undo"
          tone="green"
          label="Вернуть"
          :busy="busyKey === row.key"
          @click="unhide(row)"
        />
        <template v-else>
          <IconButton
            icon="check"
            tone="green"
            :label="canApprove(row.status) ? 'Одобрить' : moderationHint('approve', row.status)"
            :disabled="!canApprove(row.status) || busyKey === row.key"
            :busy="busyKey === row.key"
            @click="approve(row)"
          />
          <IconButton
            icon="cross"
            tone="red"
            :label="canReject(row.status) ? 'Отклонить' : moderationHint('reject', row.status)"
            :disabled="!canReject(row.status) || busyKey === row.key"
            @click="startReject([row])"
          />
          <IconButton
            icon="undo"
            :label="canRevoke(row.status) ? 'Снять с публикации' : moderationHint('revoke', row.status)"
            :disabled="!canRevoke(row.status) || busyKey === row.key"
            @click="startRevoke(row)"
          />
        </template>
        <IconButton icon="trash" tone="red" label="Удалить" :disabled="busyKey === row.key" @click="remove(row)" />
      </div>
    </template>
    <template #expand="{ row }">
      <div v-if="detail" class="expand-grid">
        <p v-if="detail.fallback" class="hint">Подробности появятся после обновления сервера. {{ detail.note }}</p>
        <template v-else>
          <p v-if="detail.publisher">Издатель: {{ detail.publisher }}</p>
          <p v-if="detail.short_name">Короткое имя: {{ detail.short_name }}</p>
          <p v-if="detail.moderation_note">Прошлая причина: {{ detail.moderation_note }}</p>
          <ul v-if="detail.songs?.length" class="plain">
            <li v-for="song in detail.songs" :key="song.fbid">{{ song.title || song.fbid }}</li>
          </ul>
          <ul v-if="detail.books?.length" class="plain">
            <li v-for="book in detail.books" :key="book.book">{{ book.book_name || book.book }} · {{ book.verses }} стихов</li>
          </ul>
          <p v-if="detail.songs_count != null">Всего песен: {{ detail.songs_count }}</p>
          <p v-if="detail.verses_count != null">Стихов: {{ detail.verses_count }}</p>
          <ul v-if="detail.similar?.length" class="plain">
            <li v-for="(match, index) in detail.similar" :key="index">Похоже в каталоге: {{ match.title }} ({{ match.match }})</li>
          </ul>
          <ul v-if="detail.audit?.length" class="plain">
            <li v-for="entry in detail.audit" :key="entry.id">{{ formatWhen(entry.at) }} · {{ entry.summary || entry.action }}</li>
          </ul>
        </template>
      </div>
    </template>
  </DataTable>
  <div v-if="total" class="pager">
    <button class="btn slim" type="button" :disabled="offset === 0" @click="offset -= limit; load()">Назад</button>
    <span>{{ offset + 1 }}–{{ Math.min(offset + rows.length, total) }} из {{ total }}</span>
    <button class="btn slim" type="button" :disabled="offset + limit >= total" @click="offset += limit; load()">Дальше</button>
  </div>
  <p v-if="toastText" class="toast" role="status">{{ toastText }}</p>
  <AppModal v-if="rejectOpen" title="Отклонить" @close="rejectOpen = false">
    <p>Одна причина для всех выбранных.</p>
    <div class="chips">
      <button v-for="preset in presets" :key="preset" type="button" @click="rejectNote = preset">{{ preset }}</button>
    </div>
    <label>Причина
      <textarea v-model="rejectNote" rows="3" required></textarea>
    </label>
    <template #footer>
      <button class="btn slim" type="button" @click="rejectOpen = false">Отмена</button>
      <button class="btn slim danger" type="button" @click="sendReject">Отклонить</button>
    </template>
  </AppModal>
  <AppModal v-if="revokeOpen" title="Снять с публикации" @close="revokeOpen = false">
    <p>Материал пропадёт из общего каталога и вернётся в черновик.</p>
    <label>Заметка, если нужна
      <textarea v-model="revokeNote" rows="3"></textarea>
    </label>
    <template #footer>
      <button class="btn slim" type="button" @click="revokeOpen = false">Отмена</button>
      <button class="btn slim danger" type="button" @click="sendRevoke">Снять</button>
    </template>
  </AppModal>
  <ConfirmDialog ref="confirms" />
</template>
