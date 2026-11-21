<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, explain } from '../api'

const props = defineProps({
  token: { type: String, default: '' },
  songbookId: { type: String, default: '' },
  sectionId: { type: String, default: '' },
})

const emit = defineEmits(['select'])
const root = ref(null)
const open = ref(false)
const groups = ref([])
const error = ref('')
const loading = ref(false)
const loaded = ref(false)

const buttonLabel = computed(() => {
  if (!props.songbookId) return 'Все сборники'
  for (const group of groups.value) {
    for (const book of group.songbooks || []) {
      if (book.id !== props.songbookId) continue
      const name = book.title || book.short_name || 'Сборник'
      const section = (book.sections || []).find((item) => item.id === props.sectionId)
      return section ? `${name} › ${section.title}` : name
    }
  }
  return 'Сборник'
})

function groupLabel(group) {
  if (group.group === 'personal') return 'Личные'
  if (group.group === 'approved') return 'Одобренные общие'
  return `Сборники общины «${group.name || 'Община'}»`
}

async function ensureTree() {
  if (loaded.value || loading.value) return
  loading.value = true
  error.value = ''
  try {
    const data = await api('/api/songbooks/tree', { token: props.token })
    groups.value = Array.isArray(data) ? data : []
    loaded.value = true
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
}

async function toggle() {
  open.value = !open.value
  if (open.value) await ensureTree()
}

function choose(songbookId, sectionId) {
  open.value = false
  emit('select', { songbookId: songbookId || '', sectionId: sectionId || '' })
}

function onDoc(event) {
  if (!root.value?.contains(event.target)) open.value = false
}

function onKey(event) {
  if (event.key === 'Escape') open.value = false
}

onMounted(() => document.addEventListener('pointerdown', onDoc))
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDoc))
</script>

<template>
  <div ref="root" class="picker">
    <button
      class="btn slim picker-btn"
      type="button"
      :title="buttonLabel"
      aria-haspopup="listbox"
      :aria-expanded="open"
      @click="toggle"
      @keydown="onKey"
    >
      <span class="space-label">{{ buttonLabel }}</span>
    </button>
    <div v-if="open" class="menu picker-menu" role="listbox" aria-label="Сборник и раздел" @keydown="onKey">
      <p v-if="loading" class="hint">Загрузка…</p>
      <p v-else-if="error" class="banner bad" role="alert">{{ error }}</p>
      <button type="button" role="option" :aria-selected="!songbookId" @click="choose('', '')">Все сборники</button>
      <template v-for="group in groups" :key="`${group.group}-${group.dimension_id || group.name}`">
        <p class="group-label">{{ groupLabel(group) }}</p>
        <template v-for="book in group.songbooks || []" :key="book.id">
          <button type="button" role="option" :aria-selected="songbookId === book.id && !sectionId" @click="choose(book.id, '')">
            {{ book.title || book.short_name || 'Сборник' }}
          </button>
          <p v-if="!(book.sections || []).length" class="group-label indent">без разделов</p>
          <button
            v-for="section in book.sections || []"
            :key="section.id"
            type="button"
            class="indent"
            role="option"
            :aria-selected="sectionId === section.id"
            @click="choose(book.id, section.id)"
          >{{ section.title }}</button>
        </template>
      </template>
    </div>
  </div>
</template>
