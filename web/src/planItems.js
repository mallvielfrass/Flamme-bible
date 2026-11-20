import { moduleNameOk } from './checks'

function id() {
  return crypto.randomUUID()
}

export function songItem(hit) {
  const title = (hit?.title || '').trim()
  const songId = (hit?.fbid || '').trim()
  if (!songId && !title) throw new Error('У песни нет названия.')
  return {
    id: id(),
    kind: 'song',
    song_id: songId,
    songbook_id: hit.songbook_id || '',
    title: title || 'Песня',
  }
}

export function bibleItem(fields) {
  const name = (fields.module_short_name || '').trim()
  const second = (fields.module_short_name_second || '').trim()
  if (!moduleNameOk(name)) {
    throw new Error('Короткое имя перевода: латиница, цифры и символы + _ -.')
  }
  if (second && !moduleNameOk(second)) {
    throw new Error('Второй перевод указан неверно.')
  }
  const book = Number(fields.book) || 0
  const chapter = Number(fields.chapter) || 0
  const verse = Number(fields.verse) || 0
  const reference = (fields.reference || '').trim()
  if (book === 0 && !reference) {
    throw new Error('Укажите номер книги или ссылку, например Ин. 3:16.')
  }
  return {
    id: id(),
    kind: 'bible',
    module_short_name: name,
    module_short_name_second: second,
    book,
    chapter,
    verse,
    reference,
    quote: (fields.quote || '').trim(),
    class: (fields.class || '').trim(),
  }
}

export function bibleItemFromPassage(hit) {
  return bibleItem({
    module_short_name: hit?.module_short_name || '',
    book: hit?.book || 0,
    chapter: hit?.chapter || 0,
    verse: hit?.verse_start || 0,
    reference: hit?.reference || '',
    quote: hit?.text || '',
  })
}

export function textItem(fields) {
  const caption = (fields.caption || '').trim()
  const quote = (fields.quote || '').trim()
  if (!caption && !quote) throw new Error('Напишите заголовок или текст заметки.')
  return {
    id: id(),
    kind: 'text',
    caption,
    reference: (fields.reference || '').trim(),
    quote,
    class: (fields.class || '').trim(),
  }
}

export function moveItem(items, index, delta) {
  const next = index + delta
  if (next < 0 || next >= items.length) return items
  const copy = items.slice()
  const [item] = copy.splice(index, 1)
  copy.splice(next, 0, item)
  return copy
}

export function importFormatFromName(name) {
  const lower = String(name || '').toLowerCase()
  if (lower.endsWith('.vplt')) return 'vplt'
  if (lower.includes('history')) return 'history'
  if (lower.includes('plandef')) return 'plandef'
  if (lower.endsWith('.json')) return 'json'
  return ''
}

export function exportFileName(format) {
  if (format === 'vplt') return 'plan.vplt'
  if (format === 'history') return 'History.ini'
  if (format === 'plandef') return 'PlanDef.ini'
  return 'plan.json'
}

export function itemTitle(item) {
  if (item.kind === 'song') return item.title || 'Песня'
  if (item.kind === 'bible') {
    const place = item.reference || [item.book, item.chapter, item.verse].filter(Boolean).join(':')
    return [item.module_short_name, place].filter(Boolean).join(' · ') || 'Стих'
  }
  return item.caption || 'Заметка'
}

export function itemKindLabel(kind) {
  if (kind === 'song') return 'Песня'
  if (kind === 'bible') return 'Стих'
  return 'Заметка'
}
