<script setup>
import { onMounted, ref } from 'vue'
import { api, explain } from '../api'
import { formatSize } from '../checks'

const release = ref(null)
const error = ref('')
const missing = ref(false)
const loading = ref(true)

const platforms = [
  { id: 'linux', label: 'Linux', hash: 'linux_sha256', size: 'linux_size' },
  { id: 'windows', label: 'Windows', hash: 'windows_sha256', size: 'windows_size' },
  { id: 'macos', label: 'macOS', hash: 'macos_sha256', size: 'macos_size' },
]

onMounted(async () => {
  try {
    release.value = await api('/api/releases/latest')
  } catch (err) {
    if (err.status === 404) missing.value = true
    else error.value = explain(err)
  } finally {
    loading.value = false
  }
})

function href(platform) {
  return `/api/releases/${encodeURIComponent(release.value.version)}/download/${platform}`
}
</script>

<template>
  <h1>Скачать программу</h1>
  <p class="hint">Программа на компьютере синхронизирует папку VisioBible. Сайт для этого не нужен: здесь только сборники, планы и общины.</p>
  <p v-if="loading" class="empty" role="status">Загрузка…</p>
  <p v-else-if="error" class="banner bad" role="alert">{{ error }}</p>
  <div v-else-if="missing" class="empty-state">
    <p>Программа ещё не опубликована.</p>
    <p class="hint">Зайдите позже или попросите того, кто ведёт сайт.</p>
  </div>
  <section v-else class="card">
    <p class="badge approved">Актуальная версия</p>
    <h2>{{ release.version }}</h2>
    <p v-if="release.notes" class="quote">{{ release.notes }}</p>
    <div class="stack">
      <article v-for="platform in platforms" :key="platform.id" class="download-row">
        <div>
          <strong>{{ platform.label }}</strong>
          <p v-if="release[platform.size] > 0" class="hint">{{ formatSize(release[platform.size]) }}</p>
          <p v-else class="hint">Файл для этой системы ещё не выложен.</p>
          <p v-if="release[platform.hash]" class="mono">SHA-256 {{ release[platform.hash] }}</p>
        </div>
        <a v-if="release[platform.size] > 0" class="btn slim primary" :href="href(platform.id)">Скачать</a>
      </article>
    </div>
    <p class="hint">После загрузки можно сверить SHA-256, чтобы убедиться, что файл не повреждён.</p>
  </section>
</template>
