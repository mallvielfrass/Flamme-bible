<script setup>
import { ref } from 'vue'
import IconButton from '../IconButton.vue'

defineProps({
  chats: { type: Array, default: () => [] },
  active: { type: String, default: '' },
})

const emit = defineEmits(['open-chat', 'new-chat', 'search', 'pin', 'rename', 'remove'])

const q = ref('')
const renaming = ref('')
const draft = ref('')
let timer = 0

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
  <div class="ai-side-head">
    <button class="btn slim primary" type="button" @click="emit('new-chat')">Новый чат</button>
    <label class="ai-search">Поиск
      <input v-model="q" type="search" placeholder="Название или текст" @input="onSearch" />
    </label>
  </div>
  <div class="ai-chats">
    <p v-if="!chats.length" class="hint ai-empty-list">Чатов пока нет.</p>
    <div v-for="chat in chats" :key="chat.id" class="ai-chat" :class="{ on: chat.id === active }">
      <input
        v-if="renaming === chat.id"
        v-model="draft"
        class="ai-rename"
        @keydown.enter.prevent="saveRename(chat)"
        @keydown.esc.prevent="renaming = ''"
        @blur="saveRename(chat)"
      />
      <button v-else class="ai-chat-open" type="button" @click="emit('open-chat', chat.id)">
        <span class="ai-chat-title">{{ chat.title || 'Без названия' }}</span>
      </button>
      <span class="ai-chat-tools">
        <IconButton icon="pin" :label="chat.pinned ? 'Открепить' : 'Закрепить'" small :class="{ pinned: chat.pinned }" @click="emit('pin', chat)" />
        <IconButton icon="pencil" label="Переименовать" small @click="startRename(chat)" />
        <IconButton icon="trash" label="Удалить" small tone="red" @click="emit('remove', chat)" />
      </span>
    </div>
  </div>
</template>
