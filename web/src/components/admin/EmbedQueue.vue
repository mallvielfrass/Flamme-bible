<script setup>
import { onMounted, ref } from 'vue'
import { api, explain } from '../../api'
import { useSessionStore } from '../../stores/session'

const session = useSessionStore()
const stats = ref(null)
const error = ref('')
const busy = ref(false)
const mode = ref('auto')
const rate = ref(32)
const percent = ref(50)
const batch = ref(4)
const paused = ref(false)
const desiredModel = ref('cointegrated/rubert-tiny2')
const ggufName = ref('')

function apply(data) {
  stats.value = data
  const settings = data.settings || {}
  mode.value = settings.mode || 'auto'
  rate.value = Number(settings.embed_tokens_per_second || 0)
  percent.value = Number(settings.embed_capacity_percent || 50)
  batch.value = Number(settings.embed_batch_size || 4)
  paused.value = !!settings.paused
  const model = data.model || {}
  if (model.desired_model) desiredModel.value = model.desired_model
  ggufName.value = model.desired_file || ''
}

async function load() {
  error.value = ''
  try {
    apply(await api('/api/admin/embed', { token: session.token }))
  } catch (err) {
    error.value = explain(err)
  }
}

async function save() {
  busy.value = true
  error.value = ''
  try {
    apply(await api('/api/admin/embed', {
      method: 'PUT',
      token: session.token,
      body: {
        mode: mode.value,
        embed_tokens_per_second: Number(rate.value),
        embed_capacity_percent: Number(percent.value),
        embed_batch_size: Number(batch.value),
        paused: paused.value,
      },
    }))
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function retry() {
  busy.value = true
  error.value = ''
  try {
    apply(await api('/api/admin/embed/retry', { method: 'POST', token: session.token }))
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function saveModel() {
  busy.value = true
  error.value = ''
  try {
    apply(await api('/api/admin/embed/model', {
      method: 'PUT',
      token: session.token,
      body: { source: 'tei', model_id: desiredModel.value.trim() },
    }))
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function resetModel() {
  busy.value = true
  error.value = ''
  try {
    apply(await api('/api/admin/embed/model/reset', { method: 'POST', token: session.token }))
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function useGGUF() {
  busy.value = true
  error.value = ''
  try {
    apply(await api('/api/admin/embed/model', {
      method: 'PUT',
      token: session.token,
      body: { source: 'llama', file: ggufName.value.trim() },
    }))
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function uploadGGUF(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  busy.value = true
  error.value = ''
  try {
    const name = encodeURIComponent(file.name)
    apply(await api(`/api/admin/embed/gguf?name=${name}`, {
      method: 'POST',
      token: session.token,
      body: file,
    }))
    ggufName.value = file.name
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function reindex() {
  busy.value = true
  error.value = ''
  try {
    apply(await api('/api/admin/embed/reindex', { method: 'POST', token: session.token }))
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

function eta(seconds) {
  const n = Number(seconds || 0)
  if (!n) return '—'
  if (n < 90) return `${n} с`
  return `${Math.round(n / 60)} мин`
}

onMounted(load)
</script>

<template>
  <section class="embed-queue">
    <h2>Индексация</h2>
    <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
    <p v-if="stats?.model_state === 'model_mismatch'" class="banner bad" role="alert">
      Модель эмбеддинга не совпадает с уже посчитанными векторами. Поиск по смыслу для песен идёт по словам, старые векторы не перезаписываются.
      <button class="btn slim" type="button" :disabled="busy" @click="reindex">Начать полный пересчёт</button>
    </p>
    <p v-else-if="stats?.model_state === 'shadow'" class="banner ok" role="status">
      Идёт пересчёт в теневой индекс. Поиск пока на старых векторах.
    </p>
    <p v-if="stats" class="hint">
      Очередь: {{ stats.pending + stats.processing }} · ошибки: {{ stats.failed }} ·
      измеренная скорость: {{ Math.round(stats.measured_tokens_per_sec || 0) }} ток/с ·
      рабочая: {{ Math.round(stats.effective_tokens_per_sec || 0) }} ток/с ·
      оценка: {{ eta(stats.eta_seconds) }}
    </p>
    <p v-if="stats?.model?.id || stats?.live_model" class="hint">
      Сейчас: {{ stats.model?.source || 'tei' }} · {{ stats.model?.id || stats.live_model }} ·
      {{ stats.model?.dimension || stats.live_dim || '—' }} чисел ·
      окно {{ stats.model?.max_tokens || '—' }} ток. ·
      файл {{ stats.model?.file_size ? Math.round(stats.model.file_size / 1048576) + ' МиБ' : '—' }} ·
      замер {{ Math.round(stats.model?.measured_tokens_per_sec || stats.measured_tokens_per_sec || 0) }} ток/с
    </p>
    <form class="embed-form" @submit.prevent="saveModel">
      <label>Модель Hugging Face
        <input v-model="desiredModel" name="model_id" placeholder="org/name" />
      </label>
      <button class="btn slim" type="submit" :disabled="busy">Запомнить</button>
      <button class="btn slim" type="button" :disabled="busy" @click="resetModel">rubert-tiny2</button>
    </form>
    <p v-if="stats?.model?.restart" class="hint">Сервер сам контейнер не перезапускает. На хосте: {{ stats.model.restart }}</p>
    <form class="embed-form" @submit.prevent="useGGUF">
      <label>Файл GGUF
        <input v-model="ggufName" name="gguf" placeholder="model.gguf" />
      </label>
      <button class="btn slim" type="submit" :disabled="busy">Выбрать llama.cpp</button>
      <label class="btn slim">Загрузить
        <input type="file" accept=".gguf" hidden @change="uploadGGUF" />
      </label>
    </form>
    <form class="embed-form" @submit.prevent="save">
      <label>Режим
        <select v-model="mode">
          <option value="auto">Авто, доля мощности</option>
          <option value="manual">Вручную, токены в секунду</option>
        </select>
      </label>
      <label v-if="mode === 'manual'">Токенов в секунду
        <input v-model.number="rate" type="number" min="0" max="10000" />
      </label>
      <label v-else>Процент мощности
        <input v-model.number="percent" type="number" min="1" max="100" />
      </label>
      <label>Пачка
        <input v-model.number="batch" type="number" min="1" max="8" />
      </label>
      <label class="check-line">
        <input v-model="paused" type="checkbox" />
        Пауза
      </label>
      <button class="btn slim primary" type="submit" :disabled="busy">Сохранить</button>
      <button v-if="stats?.failed" class="btn slim" type="button" :disabled="busy" @click="retry">Повторить ошибки</button>
    </form>
  </section>
</template>
