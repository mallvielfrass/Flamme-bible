import { describe, expect, it } from 'vitest'
import { createAiApi, readSSE } from '../shared/ai/aiApi.js'

function streamFrom(chunks) {
  const encoder = new TextEncoder()
  return {
    getReader() {
      let i = 0
      return {
        async read() {
          if (i >= chunks.length) return { done: true, value: undefined }
          const value = encoder.encode(chunks[i])
          i += 1
          return { done: false, value }
        },
      }
    },
  }
}

describe('readSSE', () => {
  it('joins chunks that split an event', async () => {
    const events = []
    await readSSE(streamFrom([
      'event: message_start\ndata: {"message_id":"m1","model":"chat-a"}\n\n',
      'event: delta\ndata: {"text":"При',
      'вет"}\n\n',
      'event: usage\ndata: {"prompt_tokens":2,"completion_tokens":1}\n\n',
      'event: done\ndata: {"message_id":"m1","stopped":false}\n\n',
    ]), (event) => events.push(event))
    expect(events.map((event) => event.event)).toEqual(['message_start', 'delta', 'usage', 'done'])
    expect(events[1].data.text).toBe('Привет')
    expect(events[2].data.prompt_tokens).toBe(2)
    expect(events[3].data.stopped).toBe(false)
  })
})

describe('createAiApi', () => {
  it('calls the fetch passed by the host', async () => {
    let seen = ''
    const client = createAiApi({
      baseUrl: 'https://example.com',
      getAuth: () => 'token',
      fetch: async (path, init) => {
        seen = path
        expect(init.headers.Authorization).toBe('Bearer token')
        return { ok: true, status: 200, json: async () => ({ chats: [] }) }
      },
    })
    const data = await client.listChats('мир')
    expect(seen).toBe('https://example.com/api/ai/chats?q=%D0%BC%D0%B8%D1%80')
    expect(data.chats).toEqual([])
  })
})
