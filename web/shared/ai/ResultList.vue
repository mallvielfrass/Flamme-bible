<script setup>
import { inject, ref } from 'vue'
import { explain } from '../http.js'
import { aiResultKey } from './results.js'

defineProps({
  items: { type: Array, default: () => [] },
})

const load = inject(aiResultKey, null)
const extra = ref({})
const error = ref({})
const loading = ref({})

function shown(item, index) {
  return item.body || extra.value[index] || ''
}

async function onToggle(item, index, event) {
  if (!event.target.open || shown(item, index) || loading.value[index]) return
  loading.value = { ...loading.value, [index]: true }
  error.value = { ...error.value, [index]: '' }
  try {
    const text = await load?.(item)
    extra.value = { ...extra.value, [index]: text || '' }
    if (!text) error.value = { ...error.value, [index]: 'Текст не найден.' }
  } catch (err) {
    error.value = { ...error.value, [index]: explain(err) }
  } finally {
    loading.value = { ...loading.value, [index]: false }
  }
}
</script>

<template>
  <div class="ai-hits">
    <details
      v-for="(item, index) in items"
      :key="`${item.kind}-${item.id}-${index}`"
      class="ai-hit"
      @toggle="onToggle(item, index, $event)"
    >
      <summary>
        <span class="ai-hit-k">{{ item.kind === 'song' ? 'Песня' : 'Стих' }}</span>
        <span class="ai-hit-t">{{ item.title }}</span>
        <span v-if="item.note" class="ai-hit-n">{{ item.note }}</span>
      </summary>
      <div class="ai-hit-body">
        <p v-if="loading[index]" class="hint">Загрузка…</p>
        <p v-else-if="error[index]" class="error">{{ error[index] }}</p>
        <p v-else class="ai-hit-text">{{ shown(item, index) }}</p>
      </div>
    </details>
  </div>
</template>
