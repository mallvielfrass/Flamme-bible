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