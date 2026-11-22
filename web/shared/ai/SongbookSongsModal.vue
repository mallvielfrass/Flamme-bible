<script setup>
import { computed, onMounted, ref } from 'vue'
import AppModal from '../components/AppModal.vue'
import { explain } from '../http.js'
import ResultList from './ResultList.vue'
import { filterSongs } from './songbooks.js'
import { useAiStore } from './store.js'

const props = defineProps({
  book: { type: Object, required: true },
})
const emit = defineEmits(['close'])

const ai = useAiStore()
const songs = ref([])
const query = ref('')
const loading = ref(true)
const error = ref('')

const shown = computed(() => filterSongs(songs.value, query.value))

onMounted(async () => {
  try {
    songs.value = await ai.listSongbookSongs(props.book)
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <Teleport to="body">
  <AppModal wide :title="book.title || 'Сборник'" @close="emit('close')">
    <p v-if="book.note" class="hint">{{ book.note }}</p>
    <label class="ai-book-search">Поиск
      <input v-model="query" type="search" data-autofocus placeholder="Название или автор" />
    </label>
    <p v-if="loading" class="hint">Загрузка…</p>
    <p v-else-if="error" class="error">{{ error }}</p>
    <p v-else-if="!songs.length" class="hint">В сборнике пока нет песен.</p>
    <p v-else-if="!shown.length" class="hint">Ничего не нашлось.</p>
    <ResultList v-else :items="shown" />
    <template #footer>
      <button class="btn slim" type="button" @click="emit('close')">Закрыть</button>
    </template>
  </AppModal>
  </Teleport>
</template>
