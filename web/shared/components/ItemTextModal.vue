<script setup>
import { onMounted, ref } from 'vue'
import AppModal from './AppModal.vue'

// Просмотр полного текста пункта плана. load() → { title, source, text }.
const props = defineProps({
  title: { type: String, default: '' },
  load: { type: Function, required: true },
})
const emit = defineEmits(['close'])

const view = ref(null)
const error = ref('')

onMounted(async () => {
  try {
    view.value = await props.load()
  } catch (err) {
    error.value = err instanceof Error && err.message ? err.message : 'Текст не найден.'
  }
})
</script>

<template>
  <AppModal wide :title="view?.title || title" @close="emit('close')">
    <p v-if="!view && !error" class="hint">Загрузка…</p>
    <p v-else-if="error" class="error">{{ error }}</p>
    <template v-else>
      <p v-if="view.source" class="hint item-text-source">{{ view.source }}</p>
      <p class="item-text-body">{{ view.text || 'Текста нет.' }}</p>
    </template>
    <template #footer>
      <button class="btn slim" type="button" @click="emit('close')">Закрыть</button>
    </template>
  </AppModal>
</template>

<style scoped>
.item-text-source { margin: 0 0 0.6rem; }
.item-text-body { margin: 0; white-space: pre-wrap; max-height: 60vh; overflow: auto; }
</style>
