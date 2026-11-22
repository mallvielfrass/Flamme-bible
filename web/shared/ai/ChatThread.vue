<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import music from '../../src/assets/twemoji/1f3b5.svg'
import book from '../../src/assets/twemoji/1f4d6.svg'
import camera from '../../src/assets/twemoji/1f4f7.svg'
import pray from '../../src/assets/twemoji/1f64f.svg'
import done from '../../src/assets/twemoji/2705.svg'
import { copyText } from './clipboard.js'
import { MAX_IMAGES, imageFiles, prepareImage } from './images.js'
import AiIcon from './AiIcon.vue'
import MessageView from './MessageView.vue'
import SongbookChips from './SongbookChips.vue'
import { songbookCards } from './songbooks.js'

const props = defineProps({
  messages: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] },
  modelId: { type: String, default: '' },
  visionModelId: { type: String, default: '' },
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
const fileInput = ref(null)
const attached = ref(/** @type {{ url: string, name: string }[]} */ ([]))
const preparing = ref(false)
const attachNote = ref('')

const ideas = [
  { icon: music, title: 'Подбери песни о благодати', note: 'по смыслу, из моих сборников' },
  { icon: book, title: 'Найди места о прощении', note: 'синодальный перевод' },
  { icon: done, title: 'Собери план на воскресенье', note: '3 песни и чтение' },
  { icon: camera, title: 'Найди песни с фото', note: 'лист с текстом, список или скриншот', photo: true },
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

// Фото читает сама модель чата с отметкой «фото» или модель задачи «Фото» из настроек.
const canImages = computed(() => {
  if (picked.value?.model.can_vision) return true
  if (!props.visionModelId) return false
  return props.providers.some((provider) => (provider.models || []).some((model) => model.id === props.visionModelId))
})

const noVision = 'Выбранная модель не читает изображения. В «Настройках ИИ» отметьте у неё «фото» или выберите модель для задачи «Фото».'

function pickPhotos() {
  attachNote.value = ''
  if (blocked.value || props.streaming) return
  if (!canImages.value) {
    attachNote.value = noVision
    return
  }
  if (attached.value.length >= MAX_IMAGES) {
    attachNote.value = `Не больше ${MAX_IMAGES} изображений за раз.`
    return
  }
  fileInput.value?.click()
}

async function addFiles(files) {
  attachNote.value = ''
  const all = Array.from(files || []).filter((file) => String(file?.type).startsWith('image/'))
  if (!all.length || blocked.value || props.streaming) return
  if (!canImages.value) {
    attachNote.value = noVision
    return
  }
  const fresh = imageFiles(all, attached.value.length)
  if (fresh.length < all.length) attachNote.value = `Не больше ${MAX_IMAGES} изображений за раз.`
  preparing.value = true
  try {
    for (const file of fresh) {
      try {
        attached.value = [...attached.value, await prepareImage(file)]
      } catch (err) {
        attachNote.value = err.message
      }
    }
  } finally {
    preparing.value = false
  }
}

function onPicked(event) {
  addFiles(event.target.files)
  event.target.value = ''
}

function onPaste(event) {
  const files = event.clipboardData?.files
  if (files && Array.from(files).some((file) => String(file.type).startsWith('image/'))) {
    event.preventDefault()
    addFiles(files)
  }
}

function onDrop(event) {
  addFiles(event.dataTransfer?.files)
}

function unattach(index) {
  attached.value = attached.value.filter((_, i) => i !== index)
  attachNote.value = ''
}

const placeholder = computed(() => {
  if (!props.hasProvider) return 'Сначала подключите провайдера ИИ'
  if (props.messages.length) return 'Сообщение…'
  return 'Спросите о песнях, Библии или планах…'
})

// Лента держится у низа, пока пользователь сам не прокрутил вверх.
const stick = ref(true)

function onScroll() {
  const el = box.value
  if (el) stick.value = el.scrollHeight - el.scrollTop - el.clientHeight < 48
}

watch(() => props.messages.length, () => {
  stick.value = true
})

watch(() => [props.messages, props.streaming], async () => {
  await nextTick()
  if (box.value && stick.value) box.value.scrollTop = box.value.scrollHeight
}, { deep: true })

function submit(text) {
  const value = (text ?? draft.value).trim()
  const images = text === undefined ? attached.value.map((item) => item.url) : []
  if ((!value && !images.length) || blocked.value || props.streaming || preparing.value) return
  draft.value = ''
  menu.value = false
  if (images.length) {
    attached.value = []
    attachNote.value = ''
  }
  emit('send', value, images)
}

function onIdea(idea) {
  if (idea.photo) pickPhotos()
  else submit(idea.title)
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

// Пока ответ пишется, сборники не показываем: иначе текст растёт над ними и уходит вверх.
function booksOf(msg) {
  if (props.streaming && lastAssistant.value && msg.id === lastAssistant.value.id) return []
  return songbookCards(msg.cards)
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
        <button v-for="idea in ideas" :key="idea.title" class="ai-chip" type="button" :disabled="streaming || chatOff" @click="onIdea(idea)">
          <img class="twemoji" :src="idea.icon" alt="" />
          <span><b>{{ idea.title }}</b><span>{{ idea.note }}</span></span>
        </button>
      </div>
    </div>
  </div>
  <div v-else ref="box" class="ai-msgs" @scroll.passive="onScroll">
    <div class="ai-col">
      <article v-for="msg in messages" :key="msg.id || msg.seq" :class="msg.role === 'user' ? 'ai-user' : 'ai-ai'">
        <template v-if="msg.role === 'user'">
          <textarea v-if="editing === msg.seq" v-model="editText" rows="3" @keydown.enter.exact.prevent="saveEdit" />
          <template v-else>
            <div v-if="msg.images?.previews?.length" class="ai-shots">
              <img v-for="(url, i) in msg.images.previews" :key="i" :src="url" alt="Приложенное изображение" />
            </div>
            <div v-else-if="msg.images?.count" class="ai-shot-count">Изображений: {{ msg.images.count }}</div>
            <div v-if="msg.content" class="ai-user-text">{{ msg.content }}</div>
            <details v-if="msg.images?.text" class="ai-shot-text">
              <summary>Текст с изображений</summary>
              <pre>{{ msg.images.text }}</pre>
            </details>
          </template>
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
            <SongbookChips v-if="booksOf(msg).length" :items="booksOf(msg)" />
            <div v-if="msg.content || booksOf(msg).length" class="ai-acts">
              <button v-if="msg.content" class="ai-ib" type="button" aria-label="Копировать" @click="copyText(msg.content)">
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

  <form class="ai-composer" @submit.prevent="submit()" @dragover.prevent @drop.prevent="onDrop">
    <div class="ai-cbox">
      <div v-if="attached.length || preparing" class="ai-attach">
        <div v-for="(item, index) in attached" :key="index" class="ai-thumb">
          <img :src="item.url" :alt="item.name" />
          <button type="button" class="ai-thumb-x" :aria-label="`Убрать ${item.name}`" @click="unattach(index)">×</button>
        </div>
        <span v-if="preparing" class="ai-attach-wait">Готовлю фото…</span>
      </div>
      <textarea v-model="draft" rows="1" :disabled="blocked || streaming" :placeholder="placeholder" @keydown="onKey" @paste="onPaste" />
      <div class="ai-cbar">
        <input ref="fileInput" class="ai-file" type="file" accept="image/jpeg,image/png,image/webp,image/*" multiple @change="onPicked" />
        <button
          class="ai-ib ai-clip"
          type="button"
          :disabled="blocked || streaming"
          :class="{ off: !canImages }"
          :title="canImages ? `Приложить фото (до ${MAX_IMAGES})` : 'Модель не читает фото'"
          :aria-label="canImages ? 'Приложить фото' : 'Модель не читает фото'"
          @click="pickPhotos"
        >
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
                <span v-if="model.can_vision" class="ai-badge">фото</span>
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
        <button v-else class="ai-send" type="submit" aria-label="Отправить" :disabled="blocked || preparing">
          <AiIcon name="up" />
        </button>
      </div>
    </div>
    <p v-if="attachNote" class="ai-attach-note" role="status">{{ attachNote }}</p>
    <p v-else-if="attached.length && !canImages" class="ai-attach-note" role="status">{{ noVision }}</p>
    <div class="ai-under"><span>Enter — отправить, Shift+Enter — новая строка. Фото можно вставить или перетащить</span></div>
  </form>
</template>
