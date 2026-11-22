import { describe, expect, it } from 'vitest'
import {
  availableTranslations,
  cleanTranslations,
  hitItem,
  hitRange,
  loadTranslations,
  passageItem,
  rangeLabel,
  rangeText,
  saveTranslations,
  selectVerse,
  toggleTranslation,
  translationsKey,
} from './bibleReader.js'

function memoryStorage(initial = {}) {
  const data = { ...initial }
  return {
    getItem: (key) => (key in data ? data[key] : null),
    setItem: (key, value) => { data[key] = String(value) },
    data,
  }
}

describe('selectVerse', () => {
  it('selects, extends in either direction, restarts and clears', () => {
    let range = selectVerse(null, 16)
    expect(range).toEqual({ start: 16, end: 16 })
    range = selectVerse(range, 18)
    expect(range).toEqual({ start: 16, end: 18 })
    range = selectVerse(range, 3)
    expect(range).toEqual({ start: 3, end: 3 })
    expect(selectVerse(range, 1)).toEqual({ start: 1, end: 3 })
    expect(selectVerse(range, 3)).toBeNull()
  })
})

describe('translations', () => {
  it('cleans junk, duplicates and keeps at most four', () => {
    expect(cleanTranslations(['RST', 'RST', 'bad name', '', 'KJV', 'NRT', 'UBIO', 'ESV'])).toEqual(['RST', 'KJV', 'NRT', 'UBIO'])
    expect(cleanTranslations('RST')).toEqual([])
  })

  it('persists the choice in storage and survives broken JSON', () => {
    const storage = memoryStorage()
    expect(saveTranslations(storage, ['RST', 'KJV'])).toEqual(['RST', 'KJV'])
    expect(loadTranslations(storage)).toEqual(['RST', 'KJV'])
    expect(loadTranslations(memoryStorage({ [translationsKey]: '{oops' }))).toEqual([])
    expect(loadTranslations(null)).toEqual([])
  })

  it('toggles in pick order up to the limit', () => {
    expect(toggleTranslation(['RST'], 'KJV')).toEqual(['RST', 'KJV'])
    expect(toggleTranslation(['RST', 'KJV'], 'RST')).toEqual(['KJV'])
    expect(toggleTranslation(['A', 'B', 'C', 'D'], 'E')).toEqual(['A', 'B', 'C', 'D'])
  })

  it('drops unknown translations and falls back to the first one on the server', () => {
    const known = [{ short_name: 'KJV' }, { short_name: 'RST' }]
    expect(availableTranslations(['OLD', 'RST'], known)).toEqual(['RST'])
    expect(availableTranslations(['OLD'], known)).toEqual(['KJV'])
    expect(availableTranslations(['OLD'], [])).toEqual([])
  })
})

describe('passage', () => {
  const verses = [
    { verse: 15, text: 'дабы всякий' },
    { verse: 16, text: 'Ибо так возлюбил ' },
    { verse: 17, text: 'Ибо не послал' },
  ]

  it('labels a single verse, a range and a whole chapter', () => {
    expect(rangeLabel('Иоанн', 3, { start: 16, end: 16 })).toBe('Иоанн 3:16')
    expect(rangeLabel('Иоанн', 3, { start: 16, end: 17 })).toBe('Иоанн 3:16-17')
    expect(rangeLabel('', 3, null)).toBe('Книга 3')
  })

  it('joins only the selected verses with numbers', () => {
    expect(rangeText(verses, { start: 16, end: 17 })).toBe('16 Ибо так возлюбил\n17 Ибо не послал')
  })

  it('builds a bible plan item with the second translation', () => {
    const item = passageItem({
      translations: ['RST', 'KJV'], book: 43, bookName: 'Иоанн', chapter: 3, range: { start: 16, end: 17 }, verses,
    })
    expect(item).toMatchObject({
      kind: 'bible', module_short_name: 'RST', module_short_name_second: 'KJV',
      book: 43, chapter: 3, verse: 16, reference: 'Иоанн 3:16-17',
      quote: '16 Ибо так возлюбил\n17 Ибо не послал',
    })
    expect(() => passageItem({ translations: ['RST'], book: 43, chapter: 3, range: null, verses })).toThrow()
  })
})

describe('search hits', () => {
  it('turns a word hit and a meaning passage into a range', () => {
    expect(hitRange({ verse: 16 })).toEqual({ start: 16, end: 16 })
    expect(hitRange({ verse_start: 16, verse_end: 19 })).toEqual({ start: 16, end: 19 })
    expect(hitRange({})).toBeNull()
  })

  it('keeps the translation of the hit and adds another chosen one as second', () => {
    const word = hitItem(['KJV', 'RST'], { module_short_name: 'RST', book: 43, book_name: 'Иоанн', chapter: 3, verse: 16, text: 'Ибо так' })
    expect(word).toMatchObject({ module_short_name: 'RST', module_short_name_second: 'KJV', verse: 16, reference: 'Иоанн 3:16', quote: 'Ибо так' })
    const meaning = hitItem(['RST'], { module_short_name: 'RST', book: 19, chapter: 22, verse_start: 1, verse_end: 4, reference: 'Псалтирь 22:1-4', text: '1 Господь' })
    expect(meaning).toMatchObject({ module_short_name: 'RST', module_short_name_second: '', verse: 1, reference: 'Псалтирь 22:1-4' })
  })
})
