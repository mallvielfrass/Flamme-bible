import { computed, reactive, ref } from 'vue'
import { explain } from '../http.js'
import { loadResult } from './results.js'
import { mergeCards, songFromHit } from './songbooks.js'

/**
 * @typedef {Object} AiChat
 * @property {string} id
 * @property {string} title
 * @property {string} model_id
 * @property {boolean} pinned
 * @property {string} updated_at
 */

/**
 * @typedef {Object} AiMessage
 * @property {string} id
 * @property {number} seq
 * @property {string} role
 * @property {string} content
 * @property {string} [model]
 * @property {string} [status]
 * @property {number} [prompt_tokens]
 * @property {number} [completion_tokens]
 * @property {{ count: number, text?: string, previews?: string[] }} [images]
 */

/** @type {ReturnType<typeof createAi> | null} */
let singleton = null

/** Общее состояние чата для сайта и настольного клиента. */
export function useAiStore() {
  if (!singleton) singleton = createAi()
  return singleton
}

function createAi() {
  /** @type {ReturnType<import('./aiApi.js').createAiApi> | null} */
  let client = null
  let pending = null

  const chats = ref(/** @type {AiChat[]} */ ([]))
  const messages = ref(/** @type {AiMessage[]} */ ([]))
  const current = ref(/** @type {AiChat | null} */ (null))
  const providers = ref(/** @type {any[]} */ ([]))
  const tasks = ref({ chat: '', vision: '', parse_song: '', title: '' })
  const modelId = ref('')
  const query = ref('')
  const streaming = ref(false)
  const activeId = ref('')
  const error = ref('')
  const chatOff = ref(false)

  const hasProvider = computed(() => providers.value.length > 0)

  function useApi(next) {
    client = next
  }

  function replaceMessage(msg) {
    messages.value = messages.value.map((item) => (item.id === msg.id ? msg : item))
  }

  async function loadProviders() {
    const data = await client.providers()
    providers.value = data.providers || []
    tasks.value = { chat: '', vision: '', parse_song: '', title: '', ...(data.tasks || {}) }
    if (!modelId.value && tasks.value.chat) modelId.value = tasks.value.chat
  }

  async function loadChats(q = query.value) {
    query.value = q || ''
    try {
      const data = await client.listChats(query.value)
      chats.value = data.chats || []
      chatOff.value = false
    } catch (err) {
      chatOff.value = err.status === 503
      error.value = explain(err)
    }
  }

  async function open(id) {
    activeId.value = id || ''
    if (!id) {
      current.value = null
      messages.value = []
      return
    }
    const data = await client.getChat(id)
    current.value = data.chat
    messages.value = data.messages || []
    if (data.chat?.model_id) modelId.value = data.chat.model_id
  }

  async function create() {
    const body = {}
    if (modelId.value) body.model_id = modelId.value
    const chat = await client.createChat(body)
    chats.value = [chat, ...chats.value.filter((item) => item.id !== chat.id)]
    return chat
  }

  async function patch(id, body) {
    const chat = await client.patchChat(id, body)
    chats.value = chats.value.map((item) => (item.id === id ? { ...item, ...chat } : item))
    if (body.pinned !== undefined) {
      chats.value = [...chats.value].sort((a, b) => {
        if (a.pinned !== b.pinned) return a.pinned ? -1 : 1
        return String(b.updated_at).localeCompare(String(a.updated_at))
      })
    }
    if (current.value?.id === id) current.value = chat
    if (body.model_id !== undefined) modelId.value = chat.model_id || ''
    return chat
  }

  async function remove(id) {
    await client.deleteChat(id)
    chats.value = chats.value.filter((item) => item.id !== id)
    if (current.value?.id === id) {
      current.value = null
      messages.value = []
      activeId.value = ''
    }
  }

  async function pickModel(id) {
    modelId.value = id
    if (!current.value?.id) return
    await patch(current.value.id, { model_id: id })
  }

  /**
   * @param {string} id
   * @param {string} text
   * @param {{ model_id?: string, from_seq?: number, images?: string[] }} [extra]
   */
  async function send(id, text, extra = {}) {
    streaming.value = true
    activeId.value = id
    error.value = ''
    const controller = new AbortController()
    pending = controller
    const payload = { text: text || '' }
    if (extra.model_id) payload.model_id = extra.model_id
    if (extra.from_seq) payload.from_seq = extra.from_seq
    const images = extra.images || []
    if (images.length) payload.images = images
    if (extra.from_seq) messages.value = messages.value.filter((item) => item.seq < extra.from_seq)
    if (text || images.length) {
      const seq = extra.from_seq || ((messages.value.at(-1)?.seq || 0) + 1)
      const local = { id: 'local-user', seq, role: 'user', content: text, status: 'done' }
      // Превью живут только до перезагрузки чата: сервер картинки не хранит.
      if (images.length) local.images = { count: images.length, previews: images }
      messages.value = [...messages.value, local]
    }
    let assistant = null
    try {
      await client.send(id, payload, (ev) => {
        const data = ev.data || {}
        if (ev.event === 'message_start') {
          assistant = {
            id: data.message_id,
            seq: (messages.value.at(-1)?.seq || 0) + 1,
            role: 'assistant',
            content: '',
            model: data.model || '',
            status: 'done',
          }
          messages.value = [...messages.value, assistant]
        } else if (ev.event === 'delta' && assistant) {
          assistant = { ...assistant, content: assistant.content + (data.text || '') }
          replaceMessage(assistant)
        } else if (ev.event === 'tool_result' && assistant) {
          const cards = mergeCards(assistant.cards, data.cards)
          if (cards.length) {
            assistant = { ...assistant, cards }
            replaceMessage(assistant)
          }
        } else if (ev.event === 'usage' && assistant) {
          assistant = {
            ...assistant,
            prompt_tokens: Number(data.prompt_tokens || 0),
            completion_tokens: Number(data.completion_tokens || 0),
          }
          replaceMessage(assistant)
        } else if (ev.event === 'title') {
          const title = data.title || ''
          chats.value = chats.value.map((item) => (item.id === id ? { ...item, title } : item))
          if (current.value?.id === id) current.value = { ...current.value, title }
        } else if (ev.event === 'error') {
          error.value = data.error || 'Не получилось получить ответ'
          if (assistant) {
            assistant = { ...assistant, status: 'error' }
            replaceMessage(assistant)
          }
        } else if (ev.event === 'done' && assistant) {
          assistant = { ...assistant, status: data.stopped ? 'stopped' : assistant.status }
          replaceMessage(assistant)
        }
      }, controller.signal)
    } catch (err) {
      if (!err || err.name !== 'AbortError') error.value = explain(err)
    } finally {
      streaming.value = false
      pending = null
      try {
        await open(id)
        await loadChats()
      } catch (err) {
        if (!error.value) error.value = explain(err)
      }
    }
  }

  function fetchResult(item) {
    if (!client) throw new Error('Нет связи с сервером.')
    return loadResult(client, item)
  }

  async function listSongbookSongs(book) {
    if (!client) throw new Error('Нет связи с сервером.')
    const data = await client.searchSongs('', { songbookId: book.id, limit: 1000 })
    return (data.songs || []).map(songFromHit).filter(Boolean)
  }

  async function stop(id) {
    try {
      await client.stop(id)
    } catch (err) {
      error.value = explain(err)
    }
    pending?.abort()
  }

  return reactive({
    chats, messages, current, providers, tasks, modelId, query, streaming, activeId,
    error, chatOff, hasProvider,
    useApi, loadProviders, loadChats, open, create, patch, remove, pickModel, send, stop, fetchResult, listSongbookSongs,
  })
}
