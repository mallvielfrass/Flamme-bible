import { ApiError } from './api'

/**
 * @typedef {Object} AiEvent
 * @property {string} event
 * @property {Record<string, any>} data
 */

/**
 * Клиент чата. fetch передаёт вызывающий код: так браузер и тесты
 * не зашиты внутрь, а заголовок Authorization можно поставить самим.
 * EventSource не подходит: он не умеет Bearer.
 * @param {{ baseUrl?: string, getAuth?: () => string, fetch: typeof fetch }} options
 */
export function createAiApi({ baseUrl = '', getAuth, fetch: doFetch } = {}) {
  if (typeof doFetch !== 'function') throw new Error('нет fetch')
  const root = String(baseUrl || '').replace(/\/$/, '')

  function url(path) {
    return root + path
  }

  function headers(extra) {
    const result = { ...extra }
    const token = typeof getAuth === 'function' ? getAuth() : ''
    if (token) result.Authorization = `Bearer ${token}`
    return result
  }

  async function request(path, { method = 'GET', body } = {}) {
    const hdrs = headers()
    let payload
    if (body !== undefined) {
      hdrs['Content-Type'] = 'application/json'
      payload = JSON.stringify(body)
    }
    let response
    try {
      response = await doFetch(url(path), { method, headers: hdrs, body: payload })
    } catch {
      throw new ApiError(0, 'Нет связи с сервером.')
    }
    const data = await response.json().catch(() => ({}))
    if (!response.ok) throw new ApiError(response.status, data.error || 'ошибка', data)
    return data
  }

  return {
    listChats(q = '') {
      const query = q ? `?q=${encodeURIComponent(q)}` : ''
      return request(`/api/ai/chats${query}`)
    },
    createChat(body = {}) {
      return request('/api/ai/chats', { method: 'POST', body })
    },
    getChat(id) {
      return request(`/api/ai/chats/${id}`)
    },
    patchChat(id, body) {
      return request(`/api/ai/chats/${id}`, { method: 'PATCH', body })
    },
    deleteChat(id) {
      return request(`/api/ai/chats/${id}`, { method: 'DELETE' })
    },
    stop(id) {
      return request(`/api/ai/chats/${id}/stop`, { method: 'POST', body: {} })
    },
    providers() {
      return request('/api/ai/providers')
    },
    /**
     * @param {string} id
     * @param {Record<string, any>} body
     * @param {(event: AiEvent) => void} onEvent
     * @param {AbortSignal} [signal]
     */
    async send(id, body, onEvent, signal) {
      let response
      try {
        response = await doFetch(url(`/api/ai/chats/${id}/messages`), {
          method: 'POST',
          headers: headers({ 'Content-Type': 'application/json', Accept: 'text/event-stream' }),
          body: JSON.stringify(body || {}),
          signal,
        })
      } catch (err) {
        if (err && err.name === 'AbortError') throw err
        throw new ApiError(0, 'Нет связи с сервером.')
      }
      if (!response.ok) {
        const data = await response.json().catch(() => ({}))
        throw new ApiError(response.status, data.error || 'ошибка', data)
      }
      if (!response.body) throw new ApiError(0, 'Нет связи с сервером.')
      await readSSE(response.body, onEvent)
    },
  }
}

/**
 * Читает text/event-stream из тела fetch.
 * @param {ReadableStream<Uint8Array>} body
 * @param {(event: AiEvent) => void} onEvent
 */
export async function readSSE(body, onEvent) {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    buf = takeEvents(buf, onEvent)
  }
  buf += decoder.decode()
  takeEvents(`${buf}\n\n`, onEvent)
}

function takeEvents(buf, onEvent) {
  let rest = buf.replace(/\r\n/g, '\n')
  for (;;) {
    const idx = rest.indexOf('\n\n')
    if (idx < 0) return rest
    const raw = rest.slice(0, idx)
    rest = rest.slice(idx + 2)
    const event = parseBlock(raw)
    if (event && onEvent) onEvent(event)
  }
}

function parseBlock(raw) {
  let event = 'message'
  const data = []
  for (const line of raw.split('\n')) {
    if (!line || line.startsWith(':')) continue
    if (line.startsWith('event:')) event = line.slice(6).trim()
    else if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''))
  }
  if (!data.length) return null
  const text = data.join('\n')
  try {
    return { event, data: JSON.parse(text) }
  } catch {
    return { event, data: { text } }
  }
}
