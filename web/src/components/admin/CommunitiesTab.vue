<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { api, explain } from '../../api'
import { formatWhen } from '../../checks'
import { communityStats } from '../../stats'
import { useSessionStore } from '../../stores/session'
import AppModal from '../AppModal.vue'
import ConfirmDialog from '../ConfirmDialog.vue'
import DataTable from '../DataTable.vue'
import IconButton from '../IconButton.vue'
import StatChips from '../StatChips.vue'

const session = useSessionStore()
const q = ref('')
const rows = ref([])
const total = ref(0)
const loading = ref(true)
const error = ref('')
const limited = ref(false)
const actionError = ref('')
const toastText = ref('')
const expanded = ref('')
const detail = ref(null)
const createOpen = ref(false)
const form = ref({ name: '', address: '', confession: '' })
const inviteFor = ref(null)
const ttl = ref(86400)
const inviteLink = ref('')
const confirms = ref(null)
let toastTimer = 0

const columns = [
  { key: 'name', label: 'Название', class: 'col-grow' },
  { key: 'address', label: 'Адрес', class: 'col-address' },
  { key: 'composition', label: 'Состав', class: 'col-stats' },
  { key: 'actions', label: '', class: 'col-actions' },
]

const tableRows = computed(() => rows.value.map((row) => ({ ...row, key: row.id })))

function showToast(text) {
  toastText.value = text
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastText.value = '' }, 3500)
}

async function load() {
  loading.value = true
  error.value = ''
  limited.value = false
  const params = new URLSearchParams()
  if (q.value.trim()) params.set('q', q.value.trim())
  try {
    const data = await api(`/api/admin/communities?${params}`, { token: session.token })
    rows.value = data.communities || []
    total.value = data.total || rows.value.length
  } catch (err) {
    if (err.status === 404) {
      limited.value = true
      try {
        const list = await api('/api/communities', { token: session.token })
        rows.value = (Array.isArray(list) ? list : []).map((item) => ({
          ...item,
          members: null,
          admins: null,
          songbooks: null,
          plans: null,
        }))
        total.value = rows.value.length
      } catch (inner) {
        error.value = explain(inner)
      }
    } else error.value = explain(err)
  } finally {
    loading.value = false
  }
}

async function loadDetail(id) {
  detail.value = null
  actionError.value = ''
  try {
    const [members, invites] = await Promise.all([
      api(`/api/admin/communities/${id}/members`, { token: session.token }).catch((err) => {
        if (err.status === 404) return api(`/api/communities/${id}/members`, { token: session.token })
        throw err
      }),
      api(`/api/communities/${id}/invites`, { token: session.token }).catch((err) => {
        if (err.status === 404) return { invites: [] }
        throw err
      }),
    ])
    detail.value = { members: members.members || [], invites: invites.invites || [] }
  } catch (err) {
    if (err.status === 404) detail.value = { members: [], invites: [], fallback: true }
    else actionError.value = explain(err)
  }
}

function onExpanded(value) {
  expanded.value = value
  if (value) loadDetail(value)
}

async function createCommunity() {
  actionError.value = ''
  try {
    await api('/api/communities', { method: 'POST', token: session.token, body: form.value })
    createOpen.value = false
    form.value = { name: '', address: '', confession: '' }
    showToast('Община создана.')
    await load()
    session.bootstrap()
  } catch (err) {
    actionError.value = explain(err)
  }
}

async function makeInvite() {
  const row = inviteFor.value
  if (!row) return
  try {
    const data = await api(`/api/communities/${row.id}/invites`, {
      method: 'POST',
      token: session.token,
      body: { ttl_seconds: Number(ttl.value) },
    })
    inviteLink.value = `${location.origin}/invite/${data.token}`
    showToast('Ссылка создана.')
    if (expanded.value === row.id) loadDetail(row.id)
  } catch (err) {
    actionError.value = explain(err)
  }
}

async function memberAct(row, member, path, title, text) {
  const ok = await confirms.value.confirm({ title, text, action: 'Да' })
  if (!ok) return
  try {
    await api(`/api/communities/${row.id}/members/${member.user_id}/${path}`, {
      method: 'POST',
      token: session.token,
      body: path === 'role' ? { role: member.role === 'admin' ? 'member' : 'admin' } : undefined,
    })
    showToast('Готово.')
    await loadDetail(row.id)
    await load()
  } catch (err) {
    actionError.value = err.status === 404 ? 'Действие появится после обновления сервера.' : explain(err)
  }
}

async function revoke(row, invite) {
  try {
    await api(`/api/communities/${row.id}/invites/${invite.digest}`, { method: 'DELETE', token: session.token })
    showToast('Приглашение отозвано.')
    await loadDetail(row.id)
  } catch (err) {
    actionError.value = explain(err)
  }
}

async function remove(row) {
  const typed = window.prompt(`Введите название «${row.name}», чтобы удалить общину`)
  if (typed !== row.name) return
  const ok = await confirms.value.confirm({
    title: 'Удалить общину',
    text: `Участников: ${row.members ?? '?'}. Сборников: ${row.songbooks ?? '?'}. Планов: ${row.plans ?? '?'}.`,
    action: 'Удалить',
  })
  if (!ok) return
  try {
    await api(`/api/admin/communities/${row.id}`, { method: 'DELETE', token: session.token })
    showToast('Община удалена.')
    await load()
  } catch (err) {
    actionError.value = err.status === 404 ? 'Удаление появится после обновления сервера.' : explain(err)
  }
}

onMounted(load)
watch(q, () => {})
</script>

<template>
  <p v-if="limited" class="hint">Показаны только общины, в которых вы состоите. Полный список появится после обновления сервера.</p>
  <div class="admin-tools">
    <label class="search">Поиск
      <input v-model="q" type="search" placeholder="Название или адрес" @change="load" />
    </label>
    <button class="btn slim primary" type="button" @click="createOpen = true">Создать общину</button>
  </div>
  <p v-if="actionError" class="error">{{ actionError }}</p>
  <DataTable
    :columns="columns"
    :rows="tableRows"
    row-key="key"
    expandable
    :loading="loading"
    :error="error"
    empty="Общин нет."
    :expanded="expanded"
    @update:expanded="onExpanded"
    @retry="load"
  >
    <template #cell-name="{ row }">
      <strong>{{ row.name }}</strong>
      <p v-if="row.confession" class="subline">{{ row.confession }}</p>
      <StatChips class="stats-under" :items="communityStats(row)" />
    </template>
    <template #cell-address="{ row }">{{ row.address }}</template>
    <template #cell-composition="{ row }">
      <StatChips :items="communityStats(row)" />
    </template>
    <template #cell-actions="{ row }">
      <div class="row-actions">
        <IconButton icon="link" tone="green" label="Создать приглашение" @click="inviteFor = row; inviteLink = ''" />
        <IconButton icon="eye" label="Открыть общину" @click="$router.push(`/dimensions/${row.id}`)" />
        <IconButton icon="trash" tone="red" label="Удалить" @click="remove(row)" />
      </div>
    </template>
    <template #expand="{ row }">
      <div v-if="detail" class="expand-grid">
        <p v-if="detail.fallback" class="hint">Список участников появится после обновления сервера.</p>
        <p v-for="member in detail.members" :key="member.user_id">
          {{ member.email }} · {{ member.role }}
          <span v-if="member.excluded"> · исключён</span>
          <button class="text-button" type="button" @click="memberAct(row, member, member.excluded ? 'include' : 'exclude', member.excluded ? 'Вернуть' : 'Исключить', member.email)">
            {{ member.excluded ? 'вернуть' : 'исключить' }}
          </button>
          <button class="text-button" type="button" @click="memberAct(row, member, 'role', 'Сменить роль', member.email)">
            {{ member.role === 'admin' ? 'сделать участником' : 'сделать админом' }}
          </button>
        </p>
        <p v-for="invite in detail.invites" :key="invite.digest">
          Приглашение до {{ formatWhen(invite.expires_at) }}
          <button class="text-button" type="button" @click="revoke(row, invite)">отозвать</button>
        </p>
      </div>
    </template>
  </DataTable>
  <p v-if="total" class="hint">{{ total }} общин</p>
  <p v-if="toastText" class="toast" role="status">{{ toastText }}</p>
  <AppModal v-if="createOpen" title="Создать общину" @close="createOpen = false">
    <label>Название <input v-model="form.name" required /></label>
    <label>Адрес <input v-model="form.address" /></label>
    <label>Конфессия <input v-model="form.confession" /></label>
    <template #footer>
      <button class="btn slim" type="button" @click="createOpen = false">Отмена</button>
      <button class="btn slim primary" type="button" :disabled="!form.name.trim()" @click="createCommunity">Создать</button>
    </template>
  </AppModal>
  <AppModal v-if="inviteFor" title="Приглашение" @close="inviteFor = null">
    <label>Срок
      <select v-model="ttl">
        <option :value="86400">1 день</option>
        <option :value="604800">7 дней</option>
        <option :value="2592000">30 дней</option>
      </select>
    </label>
    <p v-if="inviteLink" class="answer">{{ inviteLink }}</p>
    <template #footer>
      <button class="btn slim" type="button" @click="inviteFor = null">Закрыть</button>
      <button class="btn slim primary" type="button" @click="makeInvite">Создать ссылку</button>
    </template>
  </AppModal>
  <ConfirmDialog ref="confirms" />
</template>
