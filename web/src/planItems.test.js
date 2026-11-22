import { describe, expect, it } from 'vitest'
import {
  appendItems,
  bibleItem,
  bibleItemFromPassage,
  cloneItems,
  exportFileName,
  filterByKinds,
  importFormatFromName,
  itemStamp,
  moveItem,
  planItemFromResult,
  planTarget,
  planUpdatedLabel,
  songItem,
  textItem,
} from './planItems'

describe('plan items', () => {
  it('builds a song from a search hit', () => {
    const item = songItem({ fbid: 'song-1', songbook_id: 'book-1', title: 'Тихая ночь' })
    expect(item.kind).toBe('song')
    expect(item.song_id).toBe('song-1')
    expect(item.songbook_id).toBe('book-1')
    expect(item.title).toBe('Тихая ночь')
  })

  it('requires a translation and a place for a verse', () => {
    expect(() => bibleItem({ module_short_name: 'RST' })).toThrow(/книги или ссылку/)
    const item = bibleItem({
      module_short_name: 'RST',
      module_short_name_second: 'BTI',
      book: '43',
      chapter: '3',
      verse: '16',
      reference: 'Ин. 3:16',
      quote: 'Ибо так возлюбил Бог мир',
    })
    expect(item.kind).toBe('bible')
    expect(item.book).toBe(43)
    expect(item.module_short_name_second).toBe('BTI')
    expect(() => bibleItem({ module_short_name: 'Синод', reference: 'Ин. 3:16' })).toThrow(/латиница/)
  })

  it('adds a semantic bible passage as one plan item', () => {
    const item = bibleItemFromPassage({
      module_short_name: 'EX',
      book: 1,
      chapter: 2,
      verse_start: 1,
      verse_end: 2,
      reference: 'Истории 2:1-2',
      text: 'Царь признал вину.',
    })
    expect(item.kind).toBe('bible')
    expect(item.module_short_name).toBe('EX')
    expect(item.verse).toBe(1)
    expect(item.reference).toBe('Истории 2:1-2')
    expect(item.quote).toBe('Царь признал вину.')
  })

  it('requires some text for a note and moves items', () => {
    expect(() => textItem({ caption: ' ', quote: '' })).toThrow(/заголовок/)
    const note = textItem({ caption: 'Приветствие', quote: 'Мир вам' })
    expect(note.kind).toBe('text')
    const items = [{ id: 'a' }, { id: 'b' }, { id: 'c' }]
    expect(moveItem(items, 0, -1)).toEqual(items)
    expect(moveItem(items, 1, -1).map((item) => item.id)).toEqual(['b', 'a', 'c'])
  })

  it('recognizes plan files by name', () => {
    expect(importFormatFromName('plan.vplt')).toBe('vplt')
    expect(importFormatFromName('History.ini')).toBe('history')
    expect(importFormatFromName('PlanDef.ini')).toBe('plandef')
    expect(importFormatFromName('plan.json')).toBe('json')
    expect(importFormatFromName('notes.txt')).toBe('')
    expect(exportFileName('vplt')).toBe('plan.vplt')
    expect(exportFileName('history')).toBe('History.ini')
    expect(exportFileName('plandef')).toBe('PlanDef.ini')
    expect(exportFileName('json')).toBe('plan.json')
  })

  it('filters, copies and appends items without touching the source', () => {
    const source = [
      { id: 's', kind: 'song', song_id: 'a', title: 'А' },
      { id: 'b', kind: 'bible', reference: 'Ин. 1:1' },
      { id: 't', kind: 'text', caption: 'Заметка' },
    ]
    expect(filterByKinds(source, ['song', 'text']).map((item) => item.id)).toEqual(['s', 't'])
    const copy = cloneItems(source)
    expect(copy).toHaveLength(3)
    expect(copy[0].id).not.toBe('s')
    expect(copy[0].title).toBe('А')
    expect(source[0].id).toBe('s')
    const target = [{ id: 'have', kind: 'song', song_id: 'a', title: 'А' }]
    const next = appendItems(target, source, { kinds: ['song', 'bible', 'text'] })
    expect(next.map((item) => item.kind)).toEqual(['song', 'bible', 'text'])
    expect(next[0].id).toBe('have')
    expect(next[1].kind).toBe('bible')
    expect(next[1].id).not.toBe('b')
    expect(source[1].id).toBe('b')
    const kept = appendItems(target, source, { kinds: ['song'], skipSongs: false })
    expect(kept).toHaveLength(2)
    expect(kept[1].song_id).toBe('a')
  })

  it('formats plan and item times in the local zone', () => {
    const now = new Date(2026, 10, 21, 12, 0, 0)
    const created = new Date(2026, 10, 18, 19, 2, 0)
    const updated = new Date(2026, 10, 20, 21, 40, 0)
    const older = new Date(2025, 10, 18, 19, 2, 0)
    expect(planUpdatedLabel(updated.toISOString(), now)).toBe('изменён 20.11, 21:40')
    expect(itemStamp({ created_at: created.toISOString() }, now)).toEqual({
      text: 'создан 18.11, 19:02',
      title: '18.11.2026, 19:02',
    })
    expect(itemStamp({ created_at: created.toISOString(), updated_at: updated.toISOString() }, now).text).toBe('изменён 20.11, 21:40')
    expect(itemStamp({ created_at: older.toISOString() }, now).text).toBe('создан 18.11.2025, 19:02')
    expect(itemStamp({}, now).text).toBe('не сохранён')
  })
})

describe('plan items from chat results', () => {
  it('turns a song and a verse into plan items and appends to a plan once', async () => {
    const song = planItemFromResult({ kind: 'song', id: 'dim/book-1/fb-1', title: 'Песня', note: 'Сборник', body: '' })
    expect(song).toMatchObject({ kind: 'song', song_id: 'fb-1', songbook_id: 'book-1', title: 'Песня', songbook_title: 'Сборник' })
    const verse = planItemFromResult({ kind: 'verse', id: 'v', title: 'Ин. 3:16', note: '', body: 'Ибо так возлюбил', module_short_name: 'RST', book: 43, chapter: 3, verse: 16 })
    expect(verse).toMatchObject({ kind: 'bible', module_short_name: 'RST', book: 43, chapter: 3, verse: 16, reference: 'Ин. 3:16', quote: 'Ибо так возлюбил' })
    expect(planItemFromResult({ kind: 'verse', id: 'v', title: 'Ин. 3:16', body: 'текст' })).toMatchObject({ kind: 'text', caption: 'Ин. 3:16', quote: 'текст' })

    let saved = null
    const created = []
    const target = planTarget({
      list: async () => [{ id: 'p1', title: 'Воскресенье' }],
      read: async () => ({ id: 'p1', title: 'Воскресенье', version: 2, items: [] }),
      save: async (plan) => { saved = plan },
      create: async (title, items) => { created.push({ title, items }) },
    })
    await target.add('p1', { kind: 'song', id: 'dim/book-1/fb-1', title: 'Песня', note: '' })
    expect(saved.version).toBe(2)
    expect(saved.items.map((item) => item.song_id)).toEqual(['fb-1'])
    await target.create('Новый', { kind: 'song', id: 'dim/book-1/fb-1', title: 'Песня', note: '' })
    expect(created[0].title).toBe('Новый')
    expect(created[0].items).toHaveLength(1)

    const full = planTarget({ read: async () => ({ id: 'p1', items: [song] }), save: async () => {} })
    await expect(full.add('p1', { kind: 'song', id: 'dim/book-1/fb-1', title: 'Песня' })).rejects.toThrow('уже есть')
  })
})
