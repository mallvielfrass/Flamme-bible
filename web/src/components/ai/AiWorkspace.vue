<script setup>
import { onMounted, ref, watch } from 'vue'
import { explain } from '../../api'
import { useAiStore } from '../../stores/ai'
import ConfirmDialog from '../ConfirmDialog.vue'
import ChatList from './ChatList.vue'
import ChatThread from './ChatThread.vue'

const props = defineProps({
  chatId: { type: String, default: '' },
})

const emit = defineEmits(['open-chat', 'open-settings'])

const ai = useAiStore()
const confirmRef = ref(null)
const drawer = ref(false)
let skip = ''

onMounted(async () => {
  try {
    await ai.loadProviders()
  } catch (err) {
    ai.error = explain(err)
    ai.chatOff = err.status === 503
  }
  await ai.loadChats('')
})

watch(() => props.chatId, (id) => {
  if (id && id === skip) {
    skip = ''
    return
  }
  ai.open(id).catch((err) => {
    ai.error = explain(err)
  })
}, { immediate: true })

function openChat(id) {
  drawer.value = false
  emit('open-chat', id)
}

async function newChat() {
  ai.error = ''
  try {
    const chat = await ai.create()
    openChat(chat.id)
  } catch (err) {
    ai.error = explain(err)
  }
}

async function submit(text) {
  ai.error = ''
  let id = props.chatId
  try {
    if (!id) {
      const chat = await ai.create()
      id = chat.id
      skip = id
      emit('open-chat', id)
    }
    await ai.send(id, text, { model_id: ai.modelId })
  } catch (err) {
    ai.error = explain(err)
  }
}

async function edit(payload) {
  if (!props.chatId) return
  await ai.send(props.chatId, payload.text, { model_id: ai.modelId, from_seq: payload.from_seq })
}

async function regenerate(seq) {
  if (!props.chatId) return
  await ai.send(props.chatId, '', { model_id: ai.modelId, from_seq: seq })
}

async function onSearch(q) {
  await ai.loadChats(q)
}

async function onPin(chat) {
  try {
    await ai.patch(chat.id, { pinned: !chat.pinned })
  } catch (err) {
    ai.error = explain(err)
  }
}

async function onRename(payload) {
  try {
    await ai.patch(payload.id, { title: payload.title })
  } catch (err) {
    ai.error = explain(err)
  }
}

async function onRemove(chat) {
  const ok = await confirmRef.value.confirm({
    title: 'Удалить чат',
    text: chat.title || 'Без названия',
    action: 'Удалить',
  })
  if (!ok) return
  try {
    await ai.remove(chat.id)
    if (props.chatId === chat.id) emit('open-chat', '')
  } catch (err) {
    ai.error = explain(err)
  }
}

async function onModel(id) {
  try {
    await ai.pickModel(id)
  } catch (err) {
    ai.error = explain(err)
  }
}

function onOpenSettings() {
  emit('open-settings')
}
</script>

<template>
  <div class="ai-page">
    <button v-if="drawer" class="ai-shade" type="button" aria-label="Закрыть список" @click="drawer = false" />
    <aside class="ai-side" :class="{ open: drawer }">
      <ChatList
        :chats="ai.chats"
        :active="chatId"
        @open-chat="openChat"
        @new-chat="newChat"
        @search="onSearch"
        @pin="onPin"
        @rename="onRename"
        @remove="onRemove"
      />
    </aside>
    <section class="ai-main">
      <ChatThread
        :messages="ai.messages"
        :providers="ai.providers"
        :model-id="ai.modelId"
        :streaming="ai.streaming"
        :has-provider="ai.hasProvider"
        :chat-off="ai.chatOff"
        :error="ai.error"
        :title="ai.current?.title || ''"
        @send="submit"
        @stop="ai.stop(chatId)"
        @edit="edit"
        @regenerate="regenerate"
        @model="onModel"
        @open-settings="onOpenSettings"
        @drawer="drawer = true"
      />
    </section>
    <ConfirmDialog ref="confirmRef" />
  </div>
</template>
