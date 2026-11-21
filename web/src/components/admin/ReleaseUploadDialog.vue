<script setup>
import { ref } from 'vue'
import { apiForm, explain } from '../../api'
import { releaseVersionOk } from '../../checks'
import { useSessionStore } from '../../stores/session'
import AppModal from '../AppModal.vue'

const props = defineProps({
  version: { type: String, default: '' },
})
const emit = defineEmits(['close', 'done'])
const session = useSessionStore()

const version = ref(props.version)
const notes = ref('')
const files = ref({ linux_file: null, windows_file: null, macos_file: null })
const sums = ref({ linux_sha256: '', windows_sha256: '', macos_sha256: '' })
const progress = ref(0)
const error = ref('')
const busy = ref(false)

const zones = [
  { field: 'windows_file', sum: 'windows_sha256', label: 'Windows', hint: 'нужен для публикации' },
  { field: 'linux_file', sum: 'linux_sha256', label: 'Linux', hint: 'нужен для публикации' },
  { field: 'macos_file', sum: 'macos_sha256', label: 'macOS', hint: 'по желанию' },
]

function take(field, event) {
  files.value[field] = event.target.files?.[0] || null
}

async function upload() {
  error.value = ''
  const next = version.value.trim()
  if (!releaseVersionOk(next)) {
    error.value = 'Версия: латиница, цифры и символы . _ + -, без «..» и без слэшей.'
    return
  }
  const form = new FormData()
  form.set('version', next)
  let count = 0
  for (const zone of zones) {
    if (files.value[zone.field]) {
      form.set(zone.field, files.value[zone.field])
      count += 1
    }
  }
  if (!count) {
    error.value = 'Приложите хотя бы один файл.'
    return
  }
  if (notes.value.trim()) form.set('notes', notes.value.trim())
  for (const zone of zones) {
    if (sums.value[zone.sum].trim()) form.set(zone.sum, sums.value[zone.sum].trim())
  }
  busy.value = true
  progress.value = 0
  try {
    await apiForm('/api/admin/releases', {
      form,
      token: session.token,
      onProgress: (loaded, total) => {
        progress.value = total ? Math.round((loaded / total) * 100) : 0
      },
    })
    emit('done')
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AppModal title="Загрузить сборку" @close="emit('close')">
    <div class="fields">
      <label>Версия
        <input v-model="version" :readonly="Boolean(props.version)" name="version" placeholder="1.0.0" required />
      </label>
      <label>Заметки
        <input v-model="notes" name="notes" />
      </label>
    </div>
    <div class="fields">
      <label v-for="zone in zones" :key="zone.field" class="drop">
        <input type="file" @change="take(zone.field, $event)" />
        <strong>{{ zone.label }}</strong>
        <span class="hint">{{ zone.hint }}</span>
        <span v-if="files[zone.field]">{{ files[zone.field].name }}</span>
      </label>
    </div>
    <details>
      <summary>Сверить SHA-256</summary>
      <label v-for="zone in zones" :key="zone.sum">{{ zone.label }}
        <input v-model="sums[zone.sum]" />
      </label>
    </details>
    <div v-if="busy || progress" class="progress" aria-hidden="true"><span :style="{ width: `${progress}%` }"></span></div>
    <p class="hint">Один файл не больше 200 МБ.</p>
    <p v-if="error" class="error">{{ error }}</p>
    <template #footer>
      <button class="btn slim" type="button" @click="emit('close')">Отмена</button>
      <button class="btn slim primary" type="button" :disabled="busy" @click="upload">Загрузить</button>
    </template>
  </AppModal>
</template>
