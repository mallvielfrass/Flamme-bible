import { describe, expect, it } from 'vitest'
import {
  bibleItem,
  bibleItemFromPassage,
  exportFileName,
  importFormatFromName,
  moveItem,
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
})
