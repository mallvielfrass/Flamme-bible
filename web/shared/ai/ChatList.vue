<script setup>
import { computed, ref } from 'vue'
import AiIcon from './AiIcon.vue'

const props = defineProps({
  chats: { type: Array, default: () => [] },
  active: { type: String, default: '' },
})

const emit = defineEmits(['open-chat', 'new-chat', 'search', 'pin', 'rename', 'remove'])

const q = ref('')
const renaming = ref('')
const draft = ref('')
let timer = 0

const groups = computed(() => {
  const pinned = []
  const today = []
  const yesterday = []
  const earlier = []
  const start = new Date()
  start.setHours(0, 0, 0, 0)
  const prev = new Date(start)
  prev.setDate(prev.getDate() - 1)
  for (const chat of props.chats) {
    if (chat.pinned) {
      pinned.push(chat)
      continue
    }
    const at = new Date(chat.updated_at)
    if (at >= start) today.push(chat)
    else if (at >= prev) yesterday.push(chat)
    else earlier.push(chat)
  }
  return [
    { id: 'pinned', label: 'Закреплённые', items: pinned },
    { id: 'today', label: 'Сегодня', items: today },
    { id: 'yesterday', label: 'Вчера', items: yesterday },
    { id: 'earlier', label: 'Ранее', items: earlier },
  ].filter((group) => group.items.length)
})

function onSearch() {
  clearTimeout(timer)
  timer = setTimeout(() => emit('search', q.value.trim()), 200)
}

function startRename(chat) {
  renaming.value = chat.id
  draft.value = chat.title || ''
}

function saveRename(chat) {
  if (renaming.value !== chat.id) return
  const title = draft.value.trim()
  renaming.value = ''
  if (title !== (chat.title || '')) emit('rename', { id: chat.id, title })
}
</script>

<template>
  <button class="ai-new" type="button" @click="emit('new-chat')">
    <AiIcon name="plus" /> Новый чат
  </button>
  <div class="ai-search">
    <AiIcon name="search" />
    <input v-model="q" type="search" placeholder="Поиск по чатам" @input="onSearch" />
  </div>
  <div class="ai-chats">
    <p v-if="!chats.length" class="hint">Чатов пока нет.</p>
    <template v-for="group in groups" :key="group.id">
      <div class="ai-grp">{{ group.label }}</div>
      <div
        v-for="chat in group.items"
        :key="chat.id"
        class="ai-dlg"
        :class="{ on: chat.id === active, edit: renaming === chat.id }"
      >
        <template v-if="renaming === chat.id">
          <input
            v-model="draft"
            @keydown.enter.prevent="saveRename(chat)"
            @keydown.esc.prevent="renaming = ''"
            @blur="saveRename(chat)"
          />
          <button class="ai-ib" type="button" aria-label="Сохранить" @mousedown.prevent @click="saveRename(chat)">
            <AiIcon name="check" />
          </button>
        </template>
        <template v-else>
          <AiIcon v-if="chat.pinned" name="pin" class="ai-pin" />
          <button class="ai-dlg-open" type="button" @click="emit('open-chat', chat.id)">
            <span>{{ chat.title || 'Новый чат' }}</span>
          </button>
          <span class="acts">
            <button class="ai-ib" type="button" aria-label="Закрепить" @click="emit('pin', chat)">
              <AiIcon name="pin" />
            </button>
            <button class="ai-ib" type="button" aria-label="Переименовать" @click="startRename(chat)">
              <AiIcon name="pencil" />
            </button>
            <button class="ai-ib red" type="button" aria-label="Удалить" @click="emit('remove', chat)">
              <AiIcon name="trash" />
            </button>
          </span>
        </template>
      </div>
    </template>
  </div>
</template>
