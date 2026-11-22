<script setup>
import { computed, onMounted, ref } from 'vue'
import { api, explain } from '../api'
import { useSessionStore } from '../stores/session'
import AppModal from './AppModal.vue'
import ConfirmDialog from './ConfirmDialog.vue'

const emit = defineEmits(['close'])
const session = useSessionStore()

const taskFields = [
  { key: 'chat', label: 'Чат' },
  { key: 'vision', label: 'Фото' },
  { key: 'parse_song', label: 'Разбор песни' },
  { key: 'title', label: 'Автоназвание' },
]

const tab = ref('providers')
const loading = ref(true)
const busy = ref(false)
const off = ref(false)
const error = ref('')
const providers = ref([])
const tasks = ref({ chat: '', vision: '', parse_song: '', title: '' })
const results = ref({})
const testing = ref('')
const form = ref(null)
const modelsFor = ref('')
const draft = ref([])
const remote = ref([])
const manual = ref('')
const confirmRef = ref(null)

const modelChoices = computed(() => {
  const rows = []
  for (const provider of providers.value) {
    for (const model of provider.models || []) {
      rows.push({
        id: model.id,
        label: `${provider.name}: ${model.display_name || model.model}`,
      })
    }
  }
  return rows
})

function fresh() {
  return { chat: '', vision: '', parse_song: '', title: '' }
}

async function load(quiet = false) {
  if (!quiet) loading.value = true
  error.value = ''
  try {
    const data = await api('/api/ai/providers', { token: session.token })
    providers.value = data.providers || []
    tasks.value = { ...fresh(), ...(data.tasks || {}) }
    off.value = false
  } catch (err) {
    off.value = err.status === 503
    error.value = explain(err)
  } finally {
    loading.value = false
  }
}

function startAdd() {
  error.value = ''
  form.value = { id: '', name: '', base_url: '', token: '' }
}

function startEdit(provider) {
  error.value = ''
  form.value = { id: provider.id, name: provider.name, base_url: provider.base_url, token: '' }
}

async function saveProvider() {
  const current = form.value
  if (!current) return
  busy.value = true
  error.value = ''
  try {
    if (current.id) {
      await api(`/api/ai/providers/${current.id}`, {
        method: 'PATCH',
        token: session.token,
        body: { name: current.name, base_url: current.base_url },
      })
      if (current.token.trim()) {
        await api(`/api/ai/providers/${current.id}/token`, {
          method: 'PUT',
          token: session.token,
          body: { token: current.token },
        })
      }
    } else {
      await api('/api/ai/providers', {
        method: 'POST',
        token: session.token,
        body: { name: current.name, base_url: current.base_url, token: current.token },
      })
    }
    form.value = null
    await load(true)
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function removeProvider(provider) {
  const ok = await confirmRef.value.confirm({
    title: 'Удалить провайдера',
    text: provider.name,
    action: 'Удалить',
  })
  if (!ok) return
  busy.value = true
  error.value = ''
  try {
    await api(`/api/ai/providers/${provider.id}`, { method: 'DELETE', token: session.token })
    if (modelsFor.value === provider.id) modelsFor.value = ''
    await load(true)
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function testProvider(provider) {
  testing.value = provider.id
  error.value = ''
  try {
    const data = await api(`/api/ai/providers/${provider.id}/test`, { method: 'POST', token: session.token })
    results.value = {
      ...results.value,
      [provider.id]: data.ok
        ? `Связь есть. Моделей: ${data.count}. ${data.ms} мс`
        : (data.error || 'Нет связи'),
    }
    await load(true)
  } catch (err) {
    results.value = { ...results.value, [provider.id]: explain(err) }
  } finally {
    testing.value = ''
  }
}

function line(provider) {
  return results.value[provider.id] || provider.check_error || ''
}

function openModels(provider) {
  modelsFor.value = provider.id
  remote.value = []
  manual.value = ''
  draft.value = (provider.models || []).map((model) => ({
    model: model.model,
    display_name: model.display_name || model.model,
    can_tools: Boolean(model.can_tools),
    can_vision: Boolean(model.can_vision),
  }))
}

function picked(id) {
  return draft.value.some((row) => row.model === id)
}

function toggleRemote(id, on) {
  if (on && !picked(id)) {
    draft.value.push({ model: id, display_name: id, can_tools: true, can_vision: false })
    return
  }
  if (!on) draft.value = draft.value.filter((row) => row.model !== id)
}

function addManual() {
  const id = manual.value.trim()
  if (!id || picked(id)) return
  draft.value.push({ model: id, display_name: id, can_tools: true, can_vision: false })
  manual.value = ''
}

async function loadRemote(provider) {
  busy.value = true
  error.value = ''
  try {
    const data = await api(`/api/ai/providers/${provider.id}/remote-models`, { token: session.token })
    remote.value = data.models || []
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function saveModels(provider) {
  busy.value = true
  error.value = ''
  try {
    await api(`/api/ai/providers/${provider.id}/models`, {
      method: 'PUT',
      token: session.token,
      body: draft.value.map((row) => ({
        model: row.model,
        display_name: row.display_name,
        can_tools: Boolean(row.can_tools),
        can_vision: Boolean(row.can_vision),
      })),
    })
    await load(true)
    const freshProvider = providers.value.find((item) => item.id === provider.id)
    if (freshProvider) openModels(freshProvider)
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function saveTasks() {
  busy.value = true
  error.value = ''
  try {
    const data = await api('/api/ai/tasks', {
      method: 'PUT',
      token: session.token,
      body: { ...fresh(), ...tasks.value },
    })
    tasks.value = { ...fresh(), ...(data.tasks || {}) }
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <AppModal wide title="Настройки ИИ" @close="emit('close')">
    <p v-if="loading" class="hint">Загрузка…</p>
    <p v-else-if="off" class="error">{{ error }}</p>
    <template v-else>
      <div class="tabs">
        <button class="tab" :class="{ on: tab === 'providers' }" type="button" @click="tab = 'providers'">Провайдеры</button>
        <button class="tab" :class="{ on: tab === 'tasks' }" type="button" @click="tab = 'tasks'">Задачи</button>
      </div>
      <p v-if="error" class="error">{{ error }}</p>
      <div v-show="tab === 'providers'" class="ai-settings">
        <p class="hint">Токен хранится на сервере в зашифрованном виде. Здесь видны только последние 4 символа.</p>
        <form v-if="form" class="card" @submit.prevent="saveProvider">
          <h3>{{ form.id ? 'Провайдер' : 'Новый провайдер' }}</h3>
          <label>
            Название
            <input v-model="form.name" required maxlength="80" data-autofocus />
          </label>
          <label>
            Адрес
            <input v-model="form.base_url" required placeholder="https://example.com/v1" inputmode="url" />
          </label>
          <label>
            {{ form.id ? 'Новый токен' : 'Токен' }}
            <input v-model="form.token" :required="!form.id" type="password" autocomplete="off" />
          </label>
          <p v-if="form.id" class="hint">Оставьте токен пустым, если менять его не нужно.</p>
          <div class="ai-actions">
            <button class="btn slim primary" type="submit" :disabled="busy">Сохранить</button>
            <button class="btn slim" type="button" @click="form = null">Отмена</button>
          </div>
        </form>
        <p v-if="!providers.length && !form" class="empty">Провайдеров пока нет.</p>
        <article v-for="provider in providers" :key="provider.id" class="card">
          <h3>{{ provider.name }}</h3>
          <p class="ai-url">{{ provider.base_url }}</p>
          <p class="hint">Токен ••••{{ provider.token_last4 }}</p>
          <p v-if="line(provider)" class="hint">{{ line(provider) }}</p>
          <div class="ai-actions">
            <button class="btn slim" type="button" :disabled="busy || testing === provider.id" @click="testProvider(provider)">
              {{ testing === provider.id ? 'Проверка…' : 'Проверить соединение' }}
            </button>
            <button class="btn slim" type="button" @click="openModels(provider)">Модели</button>
            <button class="btn slim" type="button" @click="startEdit(provider)">Изменить</button>
            <button class="btn slim danger" type="button" :disabled="busy" @click="removeProvider(provider)">Удалить</button>
          </div>
          <div v-if="modelsFor === provider.id" class="ai-model">
            <div class="ai-actions">
              <button class="btn slim" type="button" :disabled="busy" @click="loadRemote(provider)">Список с сервера</button>
            </div>
            <div v-if="remote.length" class="ai-remote">
              <label v-for="id in remote" :key="id" class="inline-check">
                <input type="checkbox" :checked="picked(id)" @change="toggleRemote(id, $event.target.checked)" />
                {{ id }}
              </label>
            </div>
            <div class="ai-actions">
              <input v-model="manual" placeholder="Имя модели" @keydown.enter.prevent="addManual" />
              <button class="btn slim" type="button" @click="addManual">Добавить вручную</button>
            </div>
            <div v-for="(row, index) in draft" :key="row.model" class="ai-model">
              <strong>{{ row.model }}</strong>
              <label>
                Название
                <input v-model="row.display_name" maxlength="200" />
              </label>
              <div class="ai-checks">
                <label class="inline-check">
                  <input v-model="row.can_tools" type="checkbox" />
                  инструменты
                </label>
                <label class="inline-check">
                  <input v-model="row.can_vision" type="checkbox" />
                  фото
                </label>
              </div>
              <button class="btn slim" type="button" @click="draft.splice(index, 1)">Убрать</button>
            </div>
            <div class="ai-actions">
              <button class="btn slim primary" type="button" :disabled="busy" @click="saveModels(provider)">Сохранить модели</button>
            </div>
          </div>
        </article>
        <div v-if="!form" class="ai-actions">
          <button class="btn slim primary" type="button" @click="startAdd">Добавить провайдера</button>
        </div>
      </div>
      <div v-show="tab === 'tasks'">
        <p class="hint">Для каждой задачи выберите модель своего провайдера.</p>
        <label v-for="field in taskFields" :key="field.key">
          {{ field.label }}
          <select v-model="tasks[field.key]">
            <option value="">Не выбрано</option>
            <option v-for="choice in modelChoices" :key="field.key + choice.id" :value="choice.id">{{ choice.label }}</option>
          </select>
        </label>
        <p v-if="!modelChoices.length" class="empty">Сначала сохраните модели у провайдера.</p>
        <div class="ai-actions">
          <button class="btn slim primary" type="button" :disabled="busy" @click="saveTasks">Сохранить</button>
        </div>
      </div>
    </template>
    <template #footer>
      <button class="btn slim" type="button" @click="emit('close')">Закрыть</button>
    </template>
  </AppModal>
  <ConfirmDialog ref="confirmRef" />
</template>
