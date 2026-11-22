import { describe, expect, it } from 'vitest'
import { useAiStore } from '../shared/ai/store.js'

describe('useAiStore', () => {
  it('shares one state and unwraps lists', () => {
    const first = useAiStore()
    const second = useAiStore()
    expect(second).toBe(first)
    expect(Array.isArray(first.chats)).toBe(true)
    first.error = 'проверка'
    expect(second.error).toBe('проверка')
    first.error = ''
  })
})
describe('useAiStore.send', () => {
  it('sends images and shows them on the local question', async () => {
    const ai = useAiStore()
    let payload = null
    let shown = null
    ai.useApi({
      async send(id, body, onEvent) {
        payload = body
        shown = ai.messages.at(-1)
        onEvent({ event: 'done', data: {} })
      },
      async getChat() {
        return { chat: { id: 'c1', title: '', model_id: '' }, messages: [] }
      },
      async listChats() {
        return { chats: [] }
      },
    })
    await ai.send('c1', '', { model_id: 'm1', images: ['data:image/jpeg;base64,AAAA'] })
    expect(payload).toEqual({ text: '', model_id: 'm1', images: ['data:image/jpeg;base64,AAAA'] })
    expect(shown.role).toBe('user')
    expect(shown.images).toEqual({ count: 1, previews: ['data:image/jpeg;base64,AAAA'] })
  })

  it('sends no images field without pictures', async () => {
    const ai = useAiStore()
    let payload = null
    ai.useApi({
      async send(id, body) {
        payload = body
      },
      async getChat() {
        return { chat: { id: 'c1' }, messages: [] }
      },
      async listChats() {
        return { chats: [] }
      },
    })
    await ai.send('c1', 'привет')
    expect(payload).toEqual({ text: 'привет' })
  })
})
