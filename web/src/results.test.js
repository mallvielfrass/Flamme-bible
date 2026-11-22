import { describe, expect, it, vi } from 'vitest'
import { loadResult, normalizeResult, parseResults } from '../shared/ai/results.js'

const dim = '11111111-1111-4111-8111-111111111111'
const book = '22222222-2222-4222-8222-222222222222'

describe('parseResults', () => {
  it('keeps a song that already has lyrics and a verse that only has an id', () => {
    const items = parseResults(JSON.stringify([
      { kind: 'song', title: 'Благодать', lyrics: 'Куплет', songbook_title: 'Гимны' },
      { kind: 'verse', id: 'Ин. 3:16' },
      { kind: 'note' },
    ]))
    expect(items).toEqual([
      { kind: 'song', id: '', title: 'Благодать', note: 'Гимны', body: 'Куплет' },
      { kind: 'verse', id: 'Ин. 3:16', title: 'Ин. 3:16', note: '', body: '' },
    ])
  })

  it('builds a song id from the three parts', () => {
    const item = normalizeResult({
      kind: 'song',
      dimension_id: dim,
      songbook_id: book,
      fbid: 'grace',
      title: 'Благодать',
    })
    expect(item.id).toBe(`${dim}/${book}/grace`)
    expect(item.body).toBe('')
  })
})

describe('loadResult', () => {
  it('reads lyrics from the songbook snapshot and reuses it', async () => {
    const snapshot = vi.fn(async () => ({
      songs: [{ fbid: 'grace', parts: [{ text: 'Первый куплет' }, { text: 'Припев' }] }],
    }))
    const client = { snapshot, bibleSemantic: vi.fn() }
    const item = { kind: 'song', id: `${dim}/${book}/grace`, title: 'Благодать', note: '', body: '' }
    await expect(loadResult(client, item)).resolves.toBe('Первый куплет\n\nПрипев')
    await loadResult(client, { ...item, id: `${dim}/${book}/grace` })
    expect(snapshot).toHaveBeenCalledTimes(1)
  })

  it('reads a verse by its reference', async () => {
    const client = {
      snapshot: vi.fn(),
      bibleSemantic: vi.fn(async () => ({ passages: [{ reference: 'Ин. 3:16', text: 'Ибо так' }] })),
    }
    const item = { kind: 'verse', id: 'Ин. 3:16', title: 'Ин. 3:16', note: '', body: '' }
    await expect(loadResult(client, item)).resolves.toBe('Ибо так')
    expect(client.bibleSemantic).toHaveBeenCalledWith('Ин. 3:16')
  })
})
