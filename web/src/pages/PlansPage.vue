<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { api, apiBlob, explain } from '../api'
import { importEncodingNotice } from '../checks'
import {
  appendItems,
  cloneItems,
  exportFileName,
  itemTitle,
  maxPlanItems,
  moveItem,
  pointsLabel,
} from '../planItems'
import { useSessionStore } from '../stores/session'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import PlanCard from '../components/PlanCard.vue'
import PlanDownloadModal from '../components/PlanDownloadModal.vue'
import PlanFileModal from '../components/PlanFileModal.vue'
import PlanItemModal from '../components/PlanItemModal.vue'
import SpaceFilter from '../components/SpaceFilter.vue'

const session = useSessionStore()
const route = useRoute()
const router = useRouter()
const plans = ref([])
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const editor = ref(null)
const savedSnap = ref('')
const openId = ref('')
const repair = ref(null)
const confirmRef = ref(null)
const itemOpen = ref(false)
const itemIndex = ref(-1)
const fileMode = ref('')
const filePlan = ref(null)
const downloadOpen = ref(false)
const fileError = ref('')
const wantCommunity = ref(false)
let noticeTimer = 0

const communities = computed(() => session.spaces.filter((space) => space.kind === 'community'))

const space = computed(() => {
  const value = String(route.query.space || (wantCommunity.value ? 'community' : 'all'))
  if (value === 'all' || value === 'personal' || value === 'community') return value
  if (communities.value.some((item) => item.id === value)) return value
  return 'all'
})

const counts = computed(() => {
  const map = { all: plans.value.length, personal: 0, community: 0 }
  for (const plan of plans.value) {
    if (plan.dimension_id) {
      map.community += 1
      map[plan.dimension_id] = (map[plan.dimension_id] || 0) + 1
    } else map.personal += 1
  }
  return map
})

const visiblePlans = computed(() => plans.value.filter((plan) => {
  if (space.value === 'all') return true
  if (space.value === 'personal') return !plan.dimension_id
  if (space.value === 'community') return Boolean(plan.dimension_id)
  return plan.dimension_id === space.value
}))

const communityMissing = computed(() => !communities.value.length && (space.value === 'community' || (space.value !== 'all' && space.value !== 'personal')))
const dirty = computed(() => Boolean(editor.value) && snap(editor.value) !== savedSnap.value)
const showDestination = computed(() => Boolean(editor.value) && !editor.value.id && (space.value === 'all' || space.value === 'community'))
const editingItem = computed(() => (itemIndex.value >= 0 ? editor.value?.items?.[itemIndex.value] || null : null))
const otherPlans = computed(() => plans.value
  .filter((plan) => plan.id !== filePlan.value?.id)
  .map((plan) => ({ id: plan.id, title: plan.title, badge: badgeFor(plan.dimension_id) })))
const destinations = computed(() => [
  { id: '', name: 'Личный' },
  ...communities.value.map((item) => ({ id: item.id, name: item.name || 'Община' })),
])

function snap(value) {
  return JSON.stringify({
    title: value.title || '',
    dimension_id: value.dimension_id || '',
    items: (value.items || []).map((item) => ({
      id: item.id,
      kind: item.kind,
      song_id: item.song_id || '',
      songbook_id: item.songbook_id || '',
      title: item.title || '',
      module_short_name: item.module_short_name || '',
      module_short_name_second: item.module_short_name_second || '',
      book: item.book || 0,
      chapter: item.chapter || 0,
      verse: item.verse || 0,
      caption: item.caption || '',
      reference: item.reference || '',
      quote: item.quote || '',
      class: item.class || '',
    })),
  })
}

function badgeFor(dimensionId) {
  if (!dimensionId) return 'Личный'
  return session.spaces.find((item) => item.id === dimensionId)?.name || 'Община'
}

function placeText(dimensionId) {
  if (!dimensionId) return 'План будет личным'
  return `План будет в общине «${badgeFor(dimensionId)}»`
}

function initialDimension() {
  if (space.value === 'personal') return ''
  if (communities.value.some((item) => item.id === space.value)) return space.value
  return ''
}

function showNotice(text) {
  notice.value = text
  clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => {
    if (notice.value === text) notice.value = ''
  }, 3000)
}

function replaceQuery(patch) {
  const query = { ...route.query, ...patch }
  for (const key of Object.keys(query)) {
    if (query[key] === '' || query[key] == null) delete query[key]
  }
  return router.replace({ query })
}

function setSpace(value) {
  wantCommunity.value = value === 'community' && !communities.value.length
  if (value !== 'all' && value !== 'personal' && value !== 'community') session.select(value)
  const query = { ...route.query }
  if (!value || value === 'all') delete query.space
  else query.space = value
  router.replace({ query })
}

function applyEditor(view) {
  editor.value = {
    id: view.id,
    title: view.title || '',
    version: view.version,
    updated_at: view.updated_at || '',
    dimension_id: view.dimension_id || '',
    items: (view.items || []).map((item) => ({ ...item })),
  }
  savedSnap.value = snap(editor.value)
  repair.value = null
}

async function reload() {
  const list = await api('/api/plans?scope=all', { token: session.token })
  plans.value = Array.isArray(list) ? list : []
}

async function load() {
  error.value = ''
  loading.value = true
  try {
    await reload()
    const id = String(route.query.plan || '')
    if (id && id !== editor.value?.id) {
      const found = plans.value.find((plan) => plan.id === id)
      if (found) await openPlan(found, true)
    }
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
}

async function leaveEditor() {
  if (!dirty.value) return true
  const name = editor.value.title.trim() || 'Новый план'
  const choice = await confirmRef.value.confirm({
    title: `Сохранить изменения в «${name}»?`,
    triple: true,
  })
  if (choice === false) return false
  if (choice === 'discard') return true
  return save()
}

async function openPlan(plan, skipGuard = false) {
  if (editor.value?.id === plan.id && openId.value === plan.id) return true
  if (!skipGuard && !(await leaveEditor())) return false
  error.value = ''
  notice.value = ''
  busy.value = true
  try {
    applyEditor(await api(`/api/plans/${plan.id}`, { token: session.token }))
    openId.value = plan.id
    await replaceQuery({ plan: plan.id })
    return true
  } catch (err) {
    error.value = explain(err)
    return false
  } finally {
    busy.value = false
  }
}

async function toggle(plan) {
  if (openId.value === plan.id) {
    if (!(await leaveEditor())) return
    editor.value = null
    openId.value = ''
    savedSnap.value = ''
    repair.value = null
    await replaceQuery({ plan: '' })
    return
  }
  await openPlan(plan)
}

async function blank() {
  if (!(await leaveEditor())) return
  const dimensionId = initialDimension()
  editor.value = { id: '', title: '', version: 0, updated_at: '', dimension_id: dimensionId, items: [] }
  savedSnap.value = snap(editor.value)
  openId.value = 'new'
  repair.value = null
  error.value = ''
  notice.value = ''
  await replaceQuery({ plan: '' })
}

async function cancelDraft() {
  const empty = !editor.value.title.trim()
    && !editor.value.items.length
    && (editor.value.dimension_id || '') === (initialDimension() || '')
  if (!empty) {
    const ok = await confirmRef.value.confirm({
      title: 'Отменить новый план?',
      text: 'Название и пункты этого плана пропадут.',
      action: 'Отменить',
    })
    if (!ok) return
  }
  editor.value = null
  openId.value = ''
  savedSnap.value = ''
}

async function save() {
  if (!editor.value) return false
  const title = editor.value.title.trim()
  error.value = ''
  if (!title) {
    error.value = 'Введите название плана.'
    return false
  }
  busy.value = true
  try {
    const items = editor.value.items
    let saved
    if (!editor.value.id) {
      saved = await api('/api/plans', {
        method: 'POST',
        token: session.token,
        body: { title, dimension_id: editor.value.dimension_id || '', items },
      })
    } else {
      saved = await api(`/api/plans/${editor.value.id}`, {
        method: 'PUT',
        token: session.token,
        body: {
          title,
          version: editor.value.version,
          updated_at: editor.value.updated_at || undefined,
          items,
        },
      })
    }
    applyEditor(saved)
    openId.value = saved.id
    if (saved.dimension_id) session.select(saved.dimension_id)
    await replaceQuery({ plan: saved.id })
    showNotice('План сохранён')
    await reload()
    return true
  } catch (err) {
    if (err.status === 409 && err.body?.plan) {
      applyEditor(err.body.plan)
      error.value = 'План уже изменили. Показана последняя версия — проверьте пункты и сохраните ещё раз.'
    } else {
      error.value = explain(err)
    }
    return false
  } finally {
    busy.value = false
  }
}

async function removePlan(plan) {
  const source = editor.value?.id === plan.id ? editor.value : plan
  const name = (source.title || '').trim() || 'План'
  const count = source.items?.length || 0
  const text = source.dimension_id
    ? `Удалить план «${name}» из общины «${badgeFor(source.dimension_id)}»? В нём ${pointsLabel(count)}. План пропадёт у всех участников общины и на компьютерах после следующей синхронизации.`
    : `Удалить план «${name}»? В нём ${pointsLabel(count)}. План пропадёт на ваших компьютерах после следующей синхронизации.`
  const ok = await confirmRef.value.confirm({ title: 'Удалить план?', text, action: 'Удалить' })
  if (!ok) return
  busy.value = true
  error.value = ''
  try {
    const params = new URLSearchParams({ version: String(source.version) })
    if (source.updated_at) params.set('updated_at', source.updated_at)
    await api(`/api/plans/${source.id}?${params}`, { method: 'DELETE', token: session.token })
    if (editor.value?.id === source.id) {
      editor.value = null
      openId.value = ''
      await replaceQuery({ plan: '' })
    }
    showNotice('План удалён.')
    await reload()
  } catch (err) {
    if (err.status === 409 && err.body?.plan) {
      applyEditor(err.body.plan)
      openId.value = err.body.plan.id
      error.value = 'План уже изменили. Удаление не выполнено.'
    } else error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function duplicate(plan) {
  if (!(await leaveEditor())) return
  busy.value = true
  error.value = ''
  try {
    const full = await api(`/api/plans/${plan.id}`, { token: session.token })
    const created = await api('/api/plans', {
      method: 'POST',
      token: session.token,
      body: {
        title: `Копия — ${full.title}`,
        dimension_id: full.dimension_id || '',
        items: cloneItems(full.items),
      },
    })
    applyEditor(created)
    openId.value = created.id
    await replaceQuery({ plan: created.id })
    showNotice('План сохранён')
    await reload()
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

function openDownload(plan) {
  filePlan.value = plan
  downloadOpen.value = true
}

function openAppend(plan) {
  filePlan.value = plan
  fileError.value = ''
  fileMode.value = 'append'
}

function openImport() {
  fileError.value = ''
  fileMode.value = 'import'
}

async function onDownload({ format, encoding, saveFirst }) {
  const plan = filePlan.value
  if (!plan) return
  if (saveFirst && editor.value?.id === plan.id) {
    const ok = await save()
    if (!ok) return
  }
  error.value = ''
  try {
    const id = editor.value?.id === plan.id ? editor.value.id : plan.id
    const query = format === 'json' || !encoding ? '' : `&encoding=${encodeURIComponent(encoding)}`
    const { blob } = await apiBlob(`/api/plans/${id}/export?format=${format}${query}`, { token: session.token })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = exportFileName(format)
    link.click()
    URL.revokeObjectURL(url)
    downloadOpen.value = false
  } catch (err) {
    error.value = explain(err)
  }
}

async function onImport(body) {
  if (!(await leaveEditor())) return
  busy.value = true
  fileError.value = ''
  try {
    const created = await api('/api/plans/import', { method: 'POST', token: session.token, body })
    applyEditor(created)
    repair.value = importEncodingNotice(body.payload, created)
    if (repair.value && body.title) repair.value.title = ''
    openId.value = created.id
    fileMode.value = ''
    await replaceQuery({ plan: created.id })
    showNotice(repair.value ? 'План импортирован. Проверьте кодировку.' : 'План импортирован.')
    await reload()
  } catch (err) {
    fileError.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function onAppend(payload) {
  const plan = filePlan.value
  fileMode.value = ''
  if (!plan) return
  if (editor.value?.id !== plan.id) {
    const opened = await openPlan(plan)
    if (!opened) return
  }
  const next = appendItems(editor.value.items, payload.items, payload)
  if (next.length > maxPlanItems) {
    error.value = 'В плане может быть не больше 500 пунктов.'
    return
  }
  editor.value.items = next
}

async function applyRepair() {
  if (!editor.value?.id || !repair.value) return
  busy.value = true
  error.value = ''
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
    showNotice('Кодировка исправлена.')
    await reload()
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

function addItem(item) {
  editor.value.items.push(item)
}

function changeItem(item) {
  const index = itemIndex.value
  const prev = editor.value.items[index]
  if (!prev) return
  editor.value.items.splice(index, 1, {
    ...item,
    id: prev.id,
    created_at: prev.created_at,
    updated_at: prev.updated_at,
  })
}

async function removeItem(index) {
  const item = editor.value.items[index]
  const ok = await confirmRef.value.confirm({
    title: 'Убрать пункт?',
    text: `Убрать пункт №${index + 1} «${itemTitle(item)}»? Изменение сохранится, когда вы нажмёте дискету.`,
    action: 'Убрать',
  })
  if (!ok) return
  editor.value.items.splice(index, 1)
}

function onKey(event) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
    event.preventDefault()
    if (editor.value && !busy.value) save()
  }
}

function onBeforeUnload(event) {
  if (!dirty.value) return
  event.preventDefault()
  event.returnValue = ''
}

onBeforeRouteLeave(async () => {
  if (!dirty.value) return true
  const name = editor.value.title.trim() || 'Новый план'
  const choice = await confirmRef.value.confirm({
    title: `Сохранить изменения в «${name}»?`,
    triple: true,
  })
  if (choice === false) return false
  if (choice === 'discard') return true
  return save()
})

onMounted(() => {
  window.addEventListener('keydown', onKey)
  window.addEventListener('beforeunload', onBeforeUnload)
  const value = String(route.query.space || '')
  if (value && value !== 'all' && value !== 'personal' && value !== 'community') session.select(value)
  load()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('beforeunload', onBeforeUnload)
  clearTimeout(noticeTimer)
})
</script>

<template>
  <div class="page-head">
    <h1>Планы</h1>
    <div class="plan-actions">
      <button class="btn slim" type="button" @click="openImport">Импортировать план</button>
      <button class="btn slim primary" type="button" @click="blank">Создать план</button>
    </div>
  </div>
  <SpaceFilter
    :model-value="space"
    :communities="communities"
    :counts="counts"
    :preferred="session.communityId"
    show-counts
    @update:model-value="setSpace"
  />
  <p v-if="error && !editor" class="banner bad" role="alert">{{ error }}</p>
  <p v-if="notice && !editor" class="banner ok" role="status">{{ notice }}</p>
  <p v-if="loading" class="empty" role="status">Загрузка…</p>
  <div v-else-if="communityMissing" class="empty-state">
    <p>Вы пока не состоите в общине.</p>
    <router-link to="/communities">Создать или вступить в общину</router-link>
  </div>
  <div v-else class="plan-list">
    <PlanCard
      v-if="openId === 'new' && editor"
      open
      :title="editor.title"
      :badge="badgeFor(editor.dimension_id)"
      :count="editor.items.length"
      :dirty="dirty"
      :busy="busy"
      :error="error"
      :notice="notice"
      :place="placeText(editor.dimension_id)"
      :show-destination="showDestination"
      :destination="editor.dimension_id"
      :communities="communities"
      :items="editor.items"
      @toggle="cancelDraft"
      @save="save"
      @cancel="cancelDraft"
      @title="editor.title = $event"
      @destination="editor.dimension_id = $event"
      @add="itemIndex = -1; itemOpen = true"
      @edit="itemIndex = $event; itemOpen = true"
      @move="(index, delta) => { editor.items = moveItem(editor.items, index, delta) }"
      @remove-item="removeItem"
    />
    <p v-if="!visiblePlans.length && openId !== 'new'" class="empty-state">Планов пока нет.</p>
    <PlanCard
      v-for="plan in visiblePlans"
      :key="plan.id"
      :open="openId === plan.id"
      :saved="true"
      :title="openId === plan.id && editor ? editor.title : plan.title"
      :badge="badgeFor(openId === plan.id && editor ? editor.dimension_id : plan.dimension_id)"
      :count="openId === plan.id && editor ? editor.items.length : (plan.items?.length || 0)"
      :updated-at="openId === plan.id && editor ? editor.updated_at : plan.updated_at"
      :dirty="openId === plan.id && dirty"
      :busy="openId === plan.id && busy"
      :error="openId === plan.id ? error : ''"
      :notice="openId === plan.id ? notice : ''"
      :repair="openId === plan.id ? repair : null"
      :place="placeText(plan.dimension_id)"
      :items="openId === plan.id && editor ? editor.items : []"
      @toggle="toggle(plan)"
      @save="save"
      @download="openDownload(plan)"
      @append="openAppend(plan)"
      @duplicate="duplicate(plan)"
      @remove="removePlan(plan)"
      @title="editor.title = $event"
      @add="itemIndex = -1; itemOpen = true"
      @edit="itemIndex = $event; itemOpen = true"
      @move="(index, delta) => { editor.items = moveItem(editor.items, index, delta) }"
      @remove-item="removeItem"
      @repair="applyRepair"
    />
  </div>
  <PlanItemModal
    v-if="itemOpen && editor"
    :item="editingItem"
    :token="session.token"
    @close="itemOpen = false"
    @add="addItem"
    @change="changeItem"
  />
  <PlanFileModal
    v-if="fileMode"
    :mode="fileMode"
    :token="session.token"
    :plans="otherPlans"
    :current-items="editor?.id === filePlan?.id ? editor.items : (filePlan?.items || [])"
    :destinations="destinations"
    :destination="initialDimension()"
    :error-text="fileError"
    @close="fileMode = ''"
    @import="onImport"
    @append="onAppend"
  />
  <PlanDownloadModal
    v-if="downloadOpen"
    :dirty="Boolean(filePlan && editor?.id === filePlan.id && dirty)"
    @close="downloadOpen = false"
    @download="onDownload"
  />
  <ConfirmDialog ref="confirmRef" />
</template>
