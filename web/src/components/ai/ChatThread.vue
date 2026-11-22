<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import Emoji from '../Emoji.vue'
import StatusBadge from '../StatusBadge.vue'
import MessageView from './MessageView.vue'

const props = defineProps({
  messages: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] },
  modelId: { type: String, default: '' },
  streaming: { type: Boolean, default: false },
  hasProvider: { type: Boolean, default: false },
  chatOff: { type: Boolean, default: false },
  error: { type: String, default: '' },
  title: { type: String, default: '' },
})

const emit = defineEmits(['send', 'stop', 'edit', 'regenerate', 'model', 'open-settings'])

const draft = ref('')
const editing = ref(0)
const editText = ref('')
const box = ref(null)

const ideas = [
  'Подбери песни к воскресному служению',
  'Объясни Римлянам 8 коротко',
  'Собери порядок вечера хвалы',
  'Найди стих о надежде',
]

const lastUser = computed(() => {
  for (let i = props.messages.length - 1; i >= 0; i -= 1) {
    if (props.messages[i].role === 'user') return props.messages[i]
  }
  return null
})

const lastAssistant = computed(() => {
  const last = props.messages.at(-1)
  return last && last.role === 'assistant' ? last : null
})

const usage = computed(() => {
  const msg = [...props.messages].reverse().find((item) => item.role === 'assistant' && (item.prompt_tokens || item.completion_tokens))
  if (!msg) return null
  return { prompt: msg.prompt_tokens || 0, completion: msg.completion_tokens || 0 }
})

const blocked = computed(() => props.chatOff || !props.hasProvider)

watch(() => props.messages, async () => {
  await nextTick()
  if (box.value) box.value.scrollTop = box.value.scrollHeight
}, { deep: true })

function submit(text) {
  const value = (text ?? draft.value).trim()
  if (!value || blocked.value || props.streaming) return
  draft.value = ''
  emit('send', value)
}

function saveEdit() {
  const text = editText.value.trim()
  const seq = editing.value
  editing.value = 0
  if (!text || !seq) return
  emit('edit', { text, from_seq: seq })
}
</script>

<template>
  <header class="ai-thread-head">
    <button class="btn slim ai-drawer-btn" type="button" @click="emit('open-settings', 'drawer')">Чаты</button>
    <h1>{{ title || 'AI Чат' }}</h1>
  </header>
  <div ref="box" class="ai-thread">
    <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
    <section v-if="!hasProvider" class="card ai-setup">
      <h2>Подключите модель</h2>
      <ol>
        <li>Откройте «Настройки ИИ».</li>
        <li>Добавьте адрес провайдера и проверьте связь.</li>
        <li>Выберите модель для задачи «Чат».</li>
      </ol>
      <button class="btn slim primary" type="button" @click="emit('open-settings')">Настройки ИИ</button>
    </section>
    <section v-else-if="!messages.length" class="ai-hello">
      <h2><Emoji name="books" /> Чем помочь к служению?</h2>
      <p>Подберу песни, объясню место Писания и соберу порядок.</p>
      <div class="ai-ideas">
        <button v-for="idea in ideas" :key="idea" type="button" :disabled="streaming || chatOff" @click="submit(idea)">{{ idea }}</button>
      </div>
    </section>
    <article v-for="msg in messages" :key="msg.id || msg.seq" class="ai-msg" :class="msg.role">
      <div class="ai-bubble">
        <template v-if="editing === msg.seq">
          <textarea v-model="editText" rows="3" />
          <div class="ai-row">
            <button class="btn slim primary" type="button" @click="saveEdit">Сохранить</button>
            <button class="btn slim" type="button" @click="editing = 0">Отмена</button>
          </div>
        </template>
        <MessageView v-else :text="msg.content" />
        <p v-if="msg.role === 'assistant' && !msg.content && streaming" class="hint">Пишет…</p>
      </div>
      <div class="ai-msg-meta">
        <StatusBadge v-if="msg.status === 'stopped'" status="draft" label="остановлено" />
        <StatusBadge v-if="msg.status === 'error'" status="rejected" label="ошибка" />
        <button
          v-if="lastUser && msg.id === lastUser.id && !streaming && editing !== msg.seq"
          class="btn slim"
          type="button"
          @click="editing = msg.seq; editText = msg.content"
        >Изменить</button>
        <button
          v-if="lastAssistant && msg.id === lastAssistant.id && !streaming && msg.status !== 'error'"
          class="btn slim"
          type="button"
          @click="emit('regenerate', msg.seq)"
        >Заново</button>
      </div>
    </article>
  </div>
  <form class="ai-compose" @submit.prevent="submit()">
    <div class="ai-row">
      <label class="ai-model-pick">Модель
        <select :value="modelId" :disabled="blocked || streaming" @change="emit('model', $event.target.value)">
          <option value="">Не выбрана</option>
          <optgroup v-for="provider in providers" :key="provider.id" :label="provider.name">
            <option v-for="model in provider.models || []" :key="model.id" :value="model.id">
              {{ model.display_name || model.model }}
            </option>
          </optgroup>
        </select>
      </label>
      <span v-if="usage" class="hint">{{ usage.prompt }} + {{ usage.completion }} ток.</span>
      <button v-if="!hasProvider" class="btn slim" type="button" @click="emit('open-settings')">Настройки ИИ</button>
    </div>
    <textarea
      v-model="draft"
      rows="3"
      :disabled="blocked || streaming"
      placeholder="Напишите вопрос"
      @keydown.enter.exact.prevent="submit()"
    />
    <div class="ai-row">
      <button v-if="streaming" class="btn slim danger" type="button" @click="emit('stop')">Стоп</button>
      <button v-else class="btn slim primary" type="submit" :disabled="blocked || !draft.trim()">Отправить</button>
    </div>
  </form>
</template>
