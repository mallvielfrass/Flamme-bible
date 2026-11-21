<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { api, explain } from '../../api'
import { formatWhen } from '../../checks'
import { userStats } from '../../stats'
import { useSessionStore } from '../../stores/session'
import ConfirmDialog from '../ConfirmDialog.vue'
import DataTable from '../DataTable.vue'
import IconButton from '../IconButton.vue'
import StatChips from '../StatChips.vue'
import StatusBadge from '../StatusBadge.vue'

const session = useSessionStore()
const q = ref('')
const filter = ref('all')
const offset = ref(0)
const limit = 50
const rows = ref([])
const total = ref(0)
const counts = ref(null)
const loading = ref(true)
const error = ref('')
const missing = ref(false)
const actionError = ref('')
const toastText = ref('')
const selected = ref([])
const menuFor = ref('')
const confirms = ref(null)
let toastTimer = 0

const columns = [
  { key: 'email', label: 'Почта', class: 'col-grow' },
  { key: 'role', label: 'Роль' },
  { key: 'composition', label: 'Состав', class: 'col-stats' },
  { key: 'created', label: 'Регистрация', class: 'col-when' },
  { key: 'seen', label: 'Вход', class: 'col-when' },
  { key: 'actions', label: '', class: 'col-actions' },
]

const filters = computed(() => {
  const bag = counts.value || {}
  return [
    { id: 'all', label: 'Все', n: bag.all },
    { id: 'admin', label: 'Админы', n: bag.admin },
    { id: 'moderator', label: 'Модераторы', n: bag.moderator },
    { id: 'blocked', label: 'Заблокированные', n: bag.blocked },
  ]
})

const tableRows = computed(() => rows.value.map((row) => ({ ...row, key: row.id })))

function showToast(text) {
  toastText.value = text
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastText.value = '' }, 3500)
}

async function load() {
  loading.value = true
  error.value = ''
  missing.value = false
  const params = new URLSearchParams()
  if (q.value.trim()) params.set('q', q.value.trim())
  if (filter.value === 'blocked') params.set('blocked', '1')
  else if (filter.value !== 'all') params.set('role', filter.value)
  params.set('limit', String(limit))
  params.set('offset', String(offset.value))
  try {
    const data = await api(`/api/admin/users?${params}`, { token: session.token })
    rows.value = data.users || []
    total.value = data.total || 0
    counts.value = data.counts || null
  } catch (err) {
    rows.value = []
    if (err.status === 404) missing.value = true
    else error.value = explain(err)
  } finally {
    loading.value = false
  }
}

function whyBlock(row) {
  if (row.id === session.userId) return 'Себя заблокировать нельзя.'
  if (row.role === 'admin') return 'Администратора блокировать нельзя.'
  return row.blocked ? 'Разблокировать' : 'Заблокировать'
}

async function act(row, path, title, text) {
  const ok = await confirms.value.confirm({ title, text, action: 'Да' })
  if (!ok) return
  actionError.value = ''
  try {
    await api(path, { method: 'POST', token: session.token })
    showToast('Готово.')
    menuFor.value = ''
    await load()
  } catch (err) {
    actionError.value = err.status === 404 ? 'Действие появится после обновления сервера.' : explain(err)
  }
}

async function setRole(row, role) {
  const ok = await confirms.value.confirm({
    title: 'Сменить роль',
    text: `${row.email} станет: ${role}`,
    action: 'Сменить',
    danger: false,
  })
  if (!ok) return
  menuFor.value = ''
  try {
    await api(`/api/admin/users/${row.id}/role`, { method: 'POST', token: session.token, body: { role } })
    showToast('Роль обновлена.')
    await load()
  } catch (err) {
    actionError.value = explain(err)
  }
}

async function remove(row) {
  const ok = await confirms.value.confirm({
    title: 'Удалить пользователя',
    text: 'Личное пространство и его сборники, планы и модули будут удалены. Участие в общинах снимется, сами общины останутся.',
    action: 'Удалить',
  })
  if (!ok) return
  try {
    await api(`/api/admin/users/${row.id}`, { method: 'DELETE', token: session.token })
    showToast('Пользователь удалён.')
    await load()
  } catch (err) {
    actionError.value = err.status === 404 ? 'Удаление появится после обновления сервера.' : explain(err)
  }
}

async function bulk(path, title) {
  const items = rows.value.filter((row) => selected.value.includes(row.id))
  const ok = await confirms.value.confirm({ title, text: items.map((row) => row.email).join('\n'), action: 'Да' })
  if (!ok) return
  let done = 0
  let failed = 0
  for (const row of items) {
    try {
      await api(`/api/admin/users/${row.id}/${path}`, { method: 'POST', token: session.token })
      done += 1
    } catch {
      failed += 1
    }
  }
  showToast(failed ? `готово ${done} из ${items.length}, ${failed} ошибка` : `готово ${done} из ${items.length}`)
  selected.value = []
  await load()
}

onMounted(load)
watch(filter, () => { offset.value = 0; selected.value = []; load() })
</script>

<template>
  <p v-if="missing" class="empty-state">Скоро: список пользователей появится после обновления сервера.</p>
  <template v-else>
    <div class="admin-tools">
      <label class="search">Поиск
        <input v-model="q" type="search" placeholder="Почта" @change="offset = 0; load()" />
      </label>
      <div class="seg">
        <button v-for="item in filters" :key="item.id" type="button" :class="{ on: filter === item.id }" @click="filter = item.id">
          {{ item.label }}<template v-if="item.n != null"> {{ item.n }}</template>
        </button>
      </div>
    </div>
    <div v-if="selected.length" class="bulk-bar">
      <span>Выбрано: {{ selected.length }}</span>
      <button class="btn slim" type="button" @click="bulk('sessions/revoke', 'Завершить сессии')">Завершить сессии</button>
      <button class="btn slim" type="button" @click="bulk('block', 'Заблокировать')">Заблокировать</button>
      <button class="btn slim" type="button" @click="selected = []">Снять выбор</button>
    </div>
    <p v-if="actionError" class="error">{{ actionError }}</p>
    <DataTable
      :columns="columns"
      :rows="tableRows"
      row-key="key"
      selectable
      :loading="loading"
      :error="error"
      empty="Пользователей нет."
      :selected="selected"
      @update:selected="selected = $event"
      @retry="load"
    >
      <template #cell-email="{ row }">
        <span>{{ row.email }}</span>
        <StatChips class="stats-under" :items="userStats(row)" />
      </template>
      <template #cell-role="{ row }">
        <StatusBadge :status="row.role" />
        <StatusBadge v-if="row.blocked" status="blocked" />
      </template>
      <template #cell-composition="{ row }">
        <StatChips :items="userStats(row)" />
      </template>
      <template #cell-created="{ row }">{{ formatWhen(row.created_at) }}</template>
      <template #cell-seen="{ row }">{{ formatWhen(row.last_seen_at) }}</template>
      <template #cell-actions="{ row }">
        <div class="row-actions">
          <details class="menu" :open="menuFor === row.id" @toggle="menuFor = $event.target.open ? row.id : ''">
            <summary class="icon-btn" aria-label="Сменить роль" data-tip="Сменить роль">роль</summary>
            <div class="menu-pop">
              <button type="button" @click="setRole(row, 'user')">Пользователь</button>
              <button type="button" @click="setRole(row, 'moderator')">Модератор</button>
              <button type="button" @click="setRole(row, 'admin')">Админ</button>
            </div>
          </details>
          <IconButton icon="refresh" label="Завершить все сессии" @click="act(row, `/api/admin/users/${row.id}/sessions/revoke`, 'Завершить сессии', row.email)" />
          <IconButton
            icon="ban"
            tone="red"
            :label="whyBlock(row)"
            :disabled="row.role === 'admin' || row.id === session.userId"
            @click="act(row, `/api/admin/users/${row.id}/${row.blocked ? 'unblock' : 'block'}`, row.blocked ? 'Разблокировать' : 'Заблокировать', 'Вход будет закрыт, материалы останутся.')"
          />
          <IconButton icon="trash" tone="red" label="Удалить" :disabled="row.id === session.userId" @click="remove(row)" />
        </div>
      </template>
    </DataTable>
    <div v-if="total" class="pager">
      <button class="btn slim" type="button" :disabled="offset === 0" @click="offset -= limit; load()">Назад</button>
      <span>{{ offset + 1 }}–{{ Math.min(offset + rows.length, total) }} из {{ total }}</span>
      <button class="btn slim" type="button" :disabled="offset + limit >= total" @click="offset += limit; load()">Дальше</button>
    </div>
  </template>
  <p v-if="toastText" class="toast" role="status">{{ toastText }}</p>
  <ConfirmDialog ref="confirms" />
</template>
