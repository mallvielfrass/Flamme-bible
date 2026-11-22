import { describe, expect, it } from 'vitest'
import { filterSongs, mergeCards, normalizeSongbook, songFromHit, songbookCards } from '../shared/ai/songbooks.js'

const book = '22222222-2222-4222-8222-222222222222'
const dim = '11111111-1111-4111-8111-111111111111'

describe('songbookCards', () => {
  it('keeps a songbook with an id and skips a broken row', () => {
    expect(songbookCards([
      { kind: 'songbook', id: book, title: 'Домашний', note: 'Личное', dimension_id: dim },
      { kind: 'songbook', id: 'нет' },
      { kind: 'song' },
    ])).toEqual([
      { kind: 'songbook', id: book, title: 'Домашний', note: 'Личное', dimension_id: dim },
    ])
  })

  it('merges cards by id', () => {
    const first = songbookCards([{ id: book, title: 'Домашний' }])
    const next = mergeCards(first, [{ id: book, title: 'Другое' }, { id: '33333333-3333-4333-8333-333333333333', title: 'Общий' }])
    expect(next.map((card) => card.title)).toEqual(['Домашний', 'Общий'])
  })

  it('fills an empty title', () => {
    expect(normalizeSongbook({ id: book }).title).toBe('Сборник')
  })
})

describe('filterSongs', () => {
  const songs = [
    { title: 'Благодать', note: 'Гимны' },
    { title: 'Тихая ночь', note: 'Рождество' },
  ]

  it('filters by title or note and keeps all when the query is empty', () => {
    expect(filterSongs(songs, '  благо  ').map((song) => song.title)).toEqual(['Благодать'])
    expect(filterSongs(songs, 'рождество')).toHaveLength(1)
    expect(filterSongs(songs, '')).toHaveLength(2)
  })
})

describe('songFromHit', () => {
  it('builds a song id with a dimension when it is present', () => {
    const item = songFromHit({ songbook_id: book, fbid: 'grace', title: 'Благодать', dimension_id: dim, section_title: '1' })
    expect(item).toEqual({ kind: 'song', id: `${dim}/${book}/grace`, title: 'Благодать', note: '1', body: '' })
  })

  it('works without a dimension', () => {
    const item = songFromHit({ songbook_id: book, fbid: 'grace', title: 'Благодать', songbook_title: 'Гимны' })
    expect(item.id).toBe(`${book}/grace`)
    expect(item.note).toBe('Гимны')
  })
})
