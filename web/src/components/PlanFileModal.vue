<script setup>
import { computed, ref, watch } from 'vue'
import { api, explain } from '../api'
import { readPlanFile } from '../checks'
import {
  appendItems,
  countByKind,
  importFormatFromName,
  maxPlanItems,
  planEncodingLabel,
  planFormatLabel,
} from '../planItems'
import AppModal from './AppModal.vue'

const props = defineProps({
  mode: { type: String, default: 'import' },
  token: { type: String, default: '' },
  plans: { type: Array, default: () => [] },
  currentItems: { type: Array, default: () => [] },
  destinations: { type: Array, default: () => [] },
  destination: { type: String, default: '' },
  errorText: { type: String, default: '' },
})

const emit = defineEmits(['close', 'import', 'append'])

const tab = ref('file')
const format = ref('auto')
const encoding = ref('auto')
const title = ref('')
const where = ref(props.destination || '')
const fileName = ref('')
const raw = ref(null)
const payload = ref('')
const usedEncoding = ref('')
const parsed = ref(null)
const notice = ref('')
const error = ref('')
const busy = ref(false)
const over = ref(false)
const query = ref('')
const chosenPlan = ref('')
const kinds = ref([])
const skipSongs = ref(true)

const kindOptions = [
  { id: 'song', label: 'Песни' },
  { id: 'bible', label: 'Стихи из Библии' },
  { id: 'text', label: 'Текст и заметки' },
]

const shownPlans = computed(() => {
  if (props.plans.length <= 8) return props.plans
  const q = query.value.trim().toLowerCase()
  if (!q) return props.plans
  return props.plans.filter((plan) => String(plan.title || '').toLowerCase().includes(q))
})

const sourceItems = computed(() => parsed.value?.items || [])
const counts = computed(() => countByKind(sourceItems.value))
const added = computed(() => {
  const next = appendItems(props.currentItems, sourceItems.value, { kinds: kinds.value, skipSongs: skipSongs.value })
  return next.length - props.currentItems.length
})
const tooMany = computed(() => props.currentItems.length + added.value > maxPlanItems)
const summary = computed(() => {
  if (tooMany.value) {
    const room = Math.max(0, maxPlanItems - props.currentItems.length)
    return `В плане может быть не больше 500 пунктов. Сейчас ${props.currentItems.length}, добавить можно ещё ${room}.`
  }
  return `Будет добавлено ${added.value} пунктов`
})

watch(() => props.destination, (value) => {
  where.value = value || ''
})

watch([format, encoding], () => {
  if (!raw.value) return
  decodeRaw()
  if (props.mode !== 'append') return
  parsed.value = null
  parseFile()
})

function resolvedFormat() {
  if (format.value !== 'auto') return format.value
  return importFormatFromName(fileName.value)
}

function decodeRaw() {
  if (!raw.value) return
  const read = readPlanFile(raw.value, encoding.value)
  payload.value = read.text
  usedEncoding.value = read.encoding
}

function takeFile(file) {
  if (!file) return
  fileName.value = file.name
  error.value = ''
  parsed.value = null
  notice.value = ''
  file.arrayBuffer().then((buffer) => {
    raw.value = new Uint8Array(buffer)
    decodeRaw()
    const guessed = importFormatFromName(file.name)
    if (guessed && format.value !== guessed) {
      format.value = guessed
      return
    }
    if (props.mode === 'append') parseFile()
  }).catch(() => {
    error.value = 'Не удалось прочитать файл.'
  })
}

function onFile(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  takeFile(file)
}

function onDrop(event) {
  over.value = false
  takeFile(event.dataTransfer?.files?.[0])
}

async function parseFile() {
  const chosen = resolvedFormat()
  if (!payload.value) return
  if (!chosen) {
    error.value = 'Не удалось определить формат. Выберите его в списке.'
    return
  }
  busy.value = true
  error.value = ''
  try {
    const result = await api('/api/plans/parse', {
      method: 'POST',
      token: props.token,
      body: { format: chosen, payload: payload.value },
    })
    parsed.value = result
    kinds.value = kindOptions.map((item) => item.id).filter((id) => countByKind(result.items)[id] > 0)
    const warning = result.encoding_notice?.encoding_warning
    notice.value = result.encoding_notice?.mojibake ? warning : ''
  } catch (err) {
    parsed.value = null
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function choosePlan(plan) {
  chosenPlan.value = plan.id
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const full = await api(`/api/plans/${plan.id}`, { token: props.token })
    parsed.value = { title: full.title, items: full.items || [] }
    kinds.value = kindOptions.map((item) => item.id).filter((id) => countByKind(full.items)[id] > 0)
    fileName.value = ''
    usedEncoding.value = ''
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

function toggleKind(id, checked) {
  if (checked) kinds.value = [...kinds.value, id]
  else kinds.value = kinds.value.filter((item) => item !== id)
}

function submitImport() {
  const chosen = resolvedFormat()
  if (!payload.value) {
    error.value = 'Выберите файл.'
    return
  }
  if (!chosen) {
    error.value = 'Не удалось определить формат. Выберите его в списке.'
    return
  }
  emit('import', {
    format: chosen,
    payload: payload.value,
    title: title.value.trim(),
    dimension_id: where.value,
  })
}

function submitAppend() {
  if (!added.value || tooMany.value) return
  emit('append', {
    items: sourceItems.value,
    kinds: kinds.value.slice(),
    skipSongs: skipSongs.value,
  })
}

function statusLine() {
  const chosen = resolvedFormat()
  if (!chosen || !parsed.value) return ''
  const encoding = usedEncoding.value ? ` (${planEncodingLabel(usedEncoding.value)})` : ''
  return `Выбран ${planFormatLabel(chosen)}${encoding} · найдено ${sourceItems.value.length} пунктов`
}
</script>

<template>
  <AppModal :title="mode === 'import' ? 'Импортировать план' : 'Дополнить пунктами из другого плана'" @close="emit('close')">
    <div v-if="mode === 'append'" class="tabs" role="tablist">
      <button type="button" class="tab" :class="{ on: tab === 'file' }" @click="tab = 'file'">Из файла</button>
      <button type="button" class="tab" :class="{ on: tab === 'plan' }" @click="tab = 'plan'">Из другого плана</button>
    </div>
    <p v-if="error || errorText" class="banner bad" role="alert">{{ error || errorText }}</p>
    <p v-if="notice" class="banner bad" role="status">{{ notice }}</p>

    <template v-if="mode === 'import' || tab === 'file'">
      <label class="drop" :class="{ over }" @dragover.prevent="over = true" @dragleave.prevent="over = false" @drop.prevent="onDrop">
        {{ fileName || 'Перетащите файл или выберите' }}
        <input type="file" accept=".json,.vplt,.ini,.txt,text/plain" @change="onFile" />
      </label>
      <div class="fields">
        <label>Формат
          <select v-model="format">
            <option value="auto">Определить по имени файла</option>
            <option value="vplt">plan.vplt</option>
            <option value="history">History.ini</option>
            <option value="plandef">PlanDef.ini</option>
            <option value="json">JSON</option>
          </select>
        </label>
        <label>Кодировка
          <select v-model="encoding">
            <option value="auto">Определить автоматически</option>
            <option value="utf-8">UTF-8</option>
            <option value="windows-1251">Windows-1251</option>
          </select>
        </label>
      </div>
      <template v-if="mode === 'import'">
        <label>Название
          <input v-model="title" name="import_title" placeholder="Возьмём из файла" />
        </label>
        <label>Куда
          <select v-model="where">
            <option v-for="item in destinations" :key="item.id || 'personal'" :value="item.id">{{ item.name }}</option>
          </select>
        </label>
      </template>
      <p v-if="mode === 'append' && statusLine()" class="hint">{{ statusLine() }}</p>
      <button v-if="mode === 'append' && payload && !parsed" class="btn slim" type="button" :disabled="busy" @click="parseFile">Показать пункты</button>
    </template>

    <template v-if="mode === 'append' && tab === 'plan'">
      <label v-if="plans.length > 8">Название
        <input v-model="query" name="plan_query" type="search" placeholder="Найти план" />
      </label>
      <ul class="plain">
        <li v-for="plan in shownPlans" :key="plan.id">
          <button class="text-button" type="button" @click="choosePlan(plan)">
            <strong class="title-serif">{{ plan.title }}</strong>
          </button>
          <p class="hint"><span class="badge">{{ plan.badge }}</span></p>
        </li>
      </ul>
      <p v-if="!shownPlans.length" class="empty">Других планов нет.</p>
    </template>

    <template v-if="mode === 'append' && parsed">
      <label v-for="item in kindOptions" :key="item.id" class="check-line">
        <input
          type="checkbox"
          :disabled="!counts[item.id]"
          :checked="kinds.includes(item.id)"
          @change="toggleKind(item.id, $event.target.checked)"
        />
        {{ item.label }} ({{ counts[item.id] || 0 }})
      </label>
      <label class="check-line">
        <input v-model="skipSongs" type="checkbox" />
        Пропускать песни, которые уже есть в плане
      </label>
      <p class="hint">{{ summary }}</p>
    </template>

    <template #footer>
      <button class="btn slim" type="button" @click="emit('close')">Отмена</button>
      <button v-if="mode === 'import'" class="btn slim primary" type="button" :disabled="busy" @click="submitImport">Импортировать</button>
      <button v-else class="btn slim primary" type="button" :disabled="busy || !parsed || !added || tooMany" @click="submitAppend">Дополнить</button>
    </template>
  </AppModal>
</template>
