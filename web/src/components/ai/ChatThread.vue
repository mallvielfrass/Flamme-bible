<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import music from '../../assets/twemoji/1f3b5.svg'
import book from '../../assets/twemoji/1f4d6.svg'
import camera from '../../assets/twemoji/1f4f7.svg'
import pray from '../../assets/twemoji/1f64f.svg'
import done from '../../assets/twemoji/2705.svg'
import AiIcon from './AiIcon.vue'
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

const emit = defineEmits(['send', 'stop', 'edit', 'regenerate', 'model', 'open-settings', 'drawer'])

const draft = ref('')
const editing = ref(0)
const editText = ref('')
const menu = ref(false)
const box = ref(null)

const ideas = [
  { icon: music, title: 'Подбери песни о благодати', note: 'по смыслу, из моих сборников' },
  { icon: book, title: 'Найди места о прощении', note: 'синодальный перевод' },
  { icon: done, title: 'Собери план на воскресенье', note: '3 песни и чтение' },
  { icon: camera, title: 'Распознай песню с фото', note: 'черновик в личный сборник' },
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

const picked = computed(() => {
  for (const provider of props.providers) {
    for (const model of provider.models || []) {
      if (model.id === props.modelId) return { provider, model }
    }
  }
  return null
})

const blocked = computed(() => props.chatOff || !props.hasProvider)

const placeholder = computed(() => {
  if (!props.hasProvider) return 'Сначала подключите провайдера ИИ'
  if (props.messages.length) return 'Сообщение…'
  return 'Спросите о песнях, Библии или планах…'
})

watch(() => props.messages, async () => {
  await nextTick()
  if (box.value) box.value.scrollTop = box.value.scrollHeight
}, { deep: true })

function submit(text) {
  const value = (text ?? draft.value).trim()
  if (!value || blocked.value || props.streaming) return
  draft.value = ''
  menu.value = false
  emit('send', value)
}

function onKey(event) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    submit()
  }
}

function saveEdit() {
  const text = editText.value.trim()
  const seq = editing.value
  editing.value = 0
  if (!text || !seq) return
  emit('edit', { text, from_seq: seq })
}

function choose(id) {
  menu.value = false
  emit('model', id)
}

function tokenLine(msg) {
  const parts = []
  if (msg.model) parts.push(msg.model)
  if (msg.prompt_tokens != null || msg.completion_tokens != null) {
    const n = Number(msg.prompt_tokens || 0) + Number(msg.completion_tokens || 0)
    parts.push(`${n.toLocaleString('ru-RU')} токенов`)
  }
  return parts.join(' · ')
}

async function copy(text) {
  try {
    await navigator.clipboard.writeText(text || '')
  } catch {
    /* буфер может быть закрыт */
  }
}
</script>

<template>
  <header class="ai-head">
    <button class="ai-ib ai-burger" type="button" aria-label="Чаты" @click="emit('drawer')">
      <AiIcon name="menu" />
    </button>
    <h1>{{ title || 'Новый чат' }}</h1>
    <button class="ai-head-btn" type="button" @click="emit('open-settings')">
      <AiIcon name="gear" /> Настройки ИИ
    </button>
  </header>

  <p v-if="error" class="banner bad ai-banner" role="alert">{{ error }}</p>
  <div v-if="!hasProvider" class="ai-stage">
    <div class="ai-empty">
      <div class="ai-setup">
        <h2>Подключите свой ИИ</h2>
        <p class="hint">FlammeBible не запускает языковые модели на сервере. Чат работает через ваш ключ OpenAI-совместимого провайдера.</p>
        <ol>
          <li>Получите API-ключ у провайдера.</li>
          <li>Откройте «Настройки ИИ» и добавьте адрес, ключ и модель.</li>
          <li>Нажмите «Проверить соединение».</li>
        </ol>
        <button class="ai-new ai-setup-btn" type="button" @click="emit('open-settings')">
          <AiIcon name="gear" /> Открыть настройки ИИ
        </button>
      </div>
    </div>
  </div>
  <div v-else-if="!messages.length" class="ai-stage">
    <div class="ai-empty">
      <div class="ai-ava big">F</div>
      <h2>Чем помочь в подготовке служения? <img class="twemoji" :src="pray" alt="" /></h2>
      <p class="hint ai-empty-note">ИИ ищет в ваших сборниках и Библии, собирает планы и разбирает песни. Удаление — только с вашим подтверждением.</p>
      <div class="ai-chips">
        <button v-for="idea in ideas" :key="idea.title" class="ai-chip" type="button" :disabled="streaming || chatOff" @click="submit(idea.title)">
          <img class="twemoji" :src="idea.icon" alt="" />
          <span><b>{{ idea.title }}</b><span>{{ idea.note }}</span></span>
        </button>
      </div>
    </div>
  </div>
  <div v-else ref="box" class="ai-msgs">
    <div class="ai-col">
      <article v-for="msg in messages" :key="msg.id || msg.seq" :class="msg.role === 'user' ? 'ai-user' : 'ai-ai'">
        <template v-if="msg.role === 'user'">
          <textarea v-if="editing === msg.seq" v-model="editText" rows="3" @keydown.enter.exact.prevent="saveEdit" />
          <div v-else class="ai-user-text">{{ msg.content }}</div>
          <div class="ai-user-acts">
            <button v-if="editing === msg.seq" class="ai-ib" type="button" aria-label="Сохранить" @click="saveEdit">
              <AiIcon name="check" />
            </button>
            <button
              v-else-if="lastUser && msg.id === lastUser.id && !streaming"
              class="ai-ib"
              type="button"
              aria-label="Изменить"
              @click="editing = msg.seq; editText = msg.content"
            >
              <AiIcon name="pencil" />
            </button>
          </div>
        </template>
        <template v-else>
          <div class="ai-ava">F</div>
          <div class="ai-body">
            <MessageView v-if="msg.content" :text="msg.content" />
            <p v-else-if="streaming" class="ai-wait">Пишет…</p>
            <div v-if="msg.content" class="ai-acts">
              <button class="ai-ib" type="button" aria-label="Копировать" @click="copy(msg.content)">
                <AiIcon name="copy" />
              </button>
              <button
                v-if="lastAssistant && msg.id === lastAssistant.id && !streaming"
                class="ai-ib"
                type="button"
                aria-label="Ответить заново"
                @click="emit('regenerate', msg.seq)"
              >
                <AiIcon name="refresh" />
              </button>
              <span v-if="tokenLine(msg)" class="ai-tok">{{ tokenLine(msg) }}</span>
            </div>
          </div>
        </template>
      </article>
    </div>
  </div>

  <form class="ai-composer" @submit.prevent="submit()">
    <div class="ai-cbox">
      <textarea v-model="draft" rows="1" :disabled="blocked || streaming" :placeholder="placeholder" @keydown="onKey" />
      <div class="ai-cbar">
        <button class="ai-ib ai-clip" type="button" disabled title="Фото — скоро" aria-label="Фото — скоро">
          <AiIcon name="clip" />
        </button>
        <div class="ai-mp">
          <button class="ai-mp-btn" type="button" :disabled="blocked" @click="menu = !menu">
            <AiIcon name="spark" />
            <span class="ai-mp-name">{{ picked ? (picked.model.display_name || picked.model.model) : 'Модель не выбрана' }}<span v-if="picked" class="ai-pv"> · {{ picked.provider.name }}</span></span>
            <AiIcon name="chevron" />
          </button>
          <button v-if="menu" class="ai-menu-back" type="button" aria-label="Закрыть список моделей" @click="menu = false" />
          <div v-if="menu" class="ai-menu">
            <template v-for="provider in providers" :key="provider.id">
              <div class="ai-menu-grp">{{ provider.name }}</div>
              <button
                v-for="model in provider.models || []"
                :key="model.id"
                class="ai-opt"
                :class="{ sel: model.id === modelId }"
                type="button"
                @click="choose(model.id)"
              >
                <AiIcon v-if="model.id === modelId" name="check" />
                <span>{{ model.display_name || model.model }}</span>
                <span v-if="!model.can_tools" class="ai-badge">без tools</span>
                <span class="id">{{ model.model }}</span>
              </button>
            </template>
            <div class="ai-menu-sep"></div>
            <button class="ai-opt" type="button" @click="menu = false; emit('open-settings')">
              <AiIcon name="gear" /> Настроить модели…
            </button>
          </div>
        </div>
        <button v-if="streaming" class="ai-send stop" type="button" aria-label="Остановить" @click="emit('stop')">
          <span class="sq"></span>
        </button>
        <button v-else class="ai-send" type="submit" aria-label="Отправить" :disabled="blocked">
          <AiIcon name="up" />
        </button>
      </div>
    </div>
    <div class="ai-under"><span>Enter — отправить, Shift+Enter — новая строка</span></div>
  </form>
</template>
