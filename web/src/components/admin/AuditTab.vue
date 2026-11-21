<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { api, explain } from '../../api'
import { formatWhen } from '../../checks'
import { useSessionStore } from '../../stores/session'
import DataTable from '../DataTable.vue'

const session = useSessionStore()
const actor = ref('')
const action = ref('')
const offset = ref(0)
const limit = 50
const rows = ref([])
const total = ref(0)
const loading = ref(true)
const error = ref('')
const missing = ref(false)

const actions = [
  { id: '', label: 'Все' },
  { id: 'approve', label: 'Одобрения' },
  { id: 'reject', label: 'Отказы' },
  { id: 'revoke', label: 'Снятия' },
  { id: 'hide', label: 'Скрытия' },
  { id: 'delete', label: 'Удаления' },
  { id: 'role', label: 'Роли' },
  { id: 'block', label: 'Блокировки' },
  { id: 'release_publish', label: 'Публикации' },
]

const columns = [
  { key: 'at', label: 'Время' },
  { key: 'who', label: 'Кто' },
  { key: 'summary', label: 'Действие' },
  { key: 'note', label: 'Заметка' },
]

const tableRows = computed(() => rows.value.map((row) => ({ ...row, key: row.id })))

async function load() {
  loading.value = true
  error.value = ''
  missing.value = false
  const params = new URLSearchParams()
  if (actor.value.trim()) params.set('actor', actor.value.trim())
  if (action.value) params.set('action', action.value)
  params.set('limit', String(limit))
  params.set('offset', String(offset.value))
  try {
    const data = await api(`/api/admin/audit?${params}`, { token: session.token })
    rows.value = data.entries || []
    total.value = data.total || 0
  } catch (err) {
    rows.value = []
    if (err.status === 404) missing.value = true
    else error.value = explain(err)
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(action, () => { offset.value = 0; load() })
</script>

<template>
  <p v-if="missing" class="empty-state">Журнал появится после обновления сервера.</p>
  <template v-else>
    <div class="admin-tools">
      <label class="search">Кто
        <input v-model="actor" type="search" placeholder="Почта" @change="offset = 0; load()" />
      </label>
      <div class="seg">
        <button v-for="item in actions" :key="item.id" type="button" :class="{ on: action === item.id }" @click="action = item.id">{{ item.label }}</button>
      </div>
    </div>
    <DataTable
      :columns="columns"
      :rows="tableRows"
      row-key="key"
      :loading="loading"
      :error="error"
      empty="Записей нет."
      @retry="load"
    >
      <template #cell-at="{ row }">{{ formatWhen(row.at) }}</template>
      <template #cell-who="{ row }">{{ row.actor_email || '—' }}</template>
      <template #cell-summary="{ row }">{{ row.summary }}</template>
      <template #cell-note="{ row }">{{ row.note }}</template>
    </DataTable>
    <div v-if="total" class="pager">
      <button class="btn slim" type="button" :disabled="offset === 0" @click="offset -= limit; load()">Назад</button>
      <span>{{ offset + 1 }}–{{ Math.min(offset + rows.length, total) }} из {{ total }}</span>
      <button class="btn slim" type="button" :disabled="offset + limit >= total" @click="offset += limit; load()">Дальше</button>
    </div>
  </template>
</template>
