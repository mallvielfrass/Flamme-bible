import { describe, expect, it } from 'vitest'
import { emojiToCodepoints, splitEmoji } from './emoji'

describe('emojiToCodepoints', () => {
  it('keeps ZWJ and drops FE0F', () => {
    expect(emojiToCodepoints('\u{1F468}\u200D\u{1F469}\u200D\u{1F467}')).toBe('1f468-200d-1f469-200d-1f467')
    expect(emojiToCodepoints('\u{1F468}\uFE0F\u200D\u{1F469}\uFE0F')).toBe('1f468-200d-1f469')
    expect(emojiToCodepoints('\u2764\uFE0F')).toBe('2764')
    expect(emojiToCodepoints('1\uFE0F\u20E3')).toBe('31-20e3')
    expect(emojiToCodepoints('\u{1F44D}')).toBe('1f44d')
  })
})

describe('splitEmoji', () => {
  it('uses a bundled glyph and leaves an unknown one as text', () => {
    const known = splitEmoji('привет \u2764\uFE0F')
    expect(known.some((part) => part.type === 'emoji' && part.cps === '2764')).toBe(true)
    const unknown = splitEmoji('мозг \u{1F9E0}')
    expect(unknown.every((part) => part.type === 'text')).toBe(true)
    expect(unknown.map((part) => part.text).join('')).toBe('мозг \u{1F9E0}')
  })
})
