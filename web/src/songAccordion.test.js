import { describe, expect, it, vi } from 'vitest'
import { songKey, songPanelId, useSongAccordion } from './songAccordion.js'

const book = '22222222-2222-4222-8222-222222222222'
const a = { songbook_id: book, fbid: 'grace', title: 'Благодать' }
const b = { songbook_id: book, fbid: 'night', title: 'Тихая ночь' }

describe('songKey', () => {
  it('joins songbook and fbid and ignores a song without them', () => {
    expect(songKey(a)).toBe(`${book}/grace`)
    expect(songKey({})).toBe('')
    expect(songKey({ songbook_id: book })).toBe('')
    expect(songPanelId(a)).toBe(`song-text-${book}-grace`)
  })
})

describe('useSongAccordion', () => {
  it('opens one song, loads text once, and closes it on a second click', async () => {
    const load = vi.fn(async (song) => `текст ${song.fbid}`)
    const acc = useSongAccordion(load)

    await acc.toggle(a)
    expect(acc.isOpen(a)).toBe(true)
    expect(acc.isOpen(b)).toBe(false)
    expect(acc.textOf(a)).toBe('текст grace')
    expect(acc.isLoading(a)).toBe(false)

    await acc.toggle(b)
    expect(acc.isOpen(a)).toBe(false)
    expect(acc.isOpen(b)).toBe(true)
    expect(acc.textOf(b)).toBe('текст night')

    await acc.toggle(b)
    expect(acc.isOpen(b)).toBe(false)

    await acc.toggle(a)
    expect(acc.isOpen(a)).toBe(true)
    expect(load).toHaveBeenCalledTimes(2)
  })

  it('keeps the panel open and shows an error when loading fails, then retries', async () => {
    const load = vi.fn()
      .mockRejectedValueOnce(new Error('Нет связи с сервером.'))
      .mockResolvedValueOnce('Куплет\n\nПрипев')
    const acc = useSongAccordion(load)

    await acc.toggle(a)
    expect(acc.isOpen(a)).toBe(true)
    expect(acc.errorOf(a)).toBe('Нет связи с сервером.')
    expect(acc.textOf(a)).toBe('')

    acc.collapse()
    await acc.toggle(a)
    expect(acc.errorOf(a)).toBe('')
    expect(acc.textOf(a)).toBe('Куплет\n\nПрипев')
    expect(load).toHaveBeenCalledTimes(2)
  })

  it('treats an empty response as an error and does not cache it', async () => {
    const load = vi.fn()
      .mockResolvedValueOnce('  ')
      .mockResolvedValueOnce('Куплет')
    const acc = useSongAccordion(load)

    await acc.toggle(a)
    expect(acc.errorOf(a)).toBe('Текст не найден.')
    acc.collapse()
    await acc.toggle(a)
    expect(acc.textOf(a)).toBe('Куплет')
  })

  it('does not start a second request while the first is in flight', async () => {
    let release
    const load = vi.fn(() => new Promise((resolve) => { release = resolve }))
    const acc = useSongAccordion(load)

    const first = acc.toggle(a)
    expect(acc.isLoading(a)).toBe(true)
    const second = acc.toggle(a)
    release('Куплет')
    await Promise.all([first, second])
    expect(load).toHaveBeenCalledTimes(1)
    expect(acc.isOpen(a)).toBe(false)
    expect(acc.textOf(a)).toBe('Куплет')
  })
})
