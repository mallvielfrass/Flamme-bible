import { moduleNameOk } from './checks'

function id() {
  return crypto.randomUUID()
}

export function songItem(hit, titleOverride) {
  const title = (titleOverride != null ? titleOverride : hit?.title || '').trim()
  const songId = (hit?.fbid || hit?.song_id || '').trim()
  if (!songId && !title) throw new Error('У песни нет названия.')
  return {
    id: id(),
    kind: 'song',
    song_id: songId,
    songbook_id: hit?.songbook_id || '',
    title: title || 'Песня',
    songbook_title: hit?.songbook_title || '',
    section_title: hit?.section_title || '',
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

// Пункт ответа чата (песня или стих) как пункт плана.
// Стих без короткого имени перевода становится заметкой со ссылкой и текстом.
export function planItemFromResult(result) {
  if (result.kind === 'song') {
    const [, songbookId = '', ...rest] = String(result.id || '').split('/')
    return songItem({
      fbid: rest.join('/') || result.id,
      songbook_id: songbookId,
      title: result.title,
      songbook_title: result.note,
    })
  }
  const quote = result.body || ''
  if (moduleNameOk(result.module_short_name || '')) {
    return bibleItem({ ...result, reference: result.title, quote })
  }
  return textItem({ caption: result.title, quote })
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

export const maxPlanItems = 500

export function filterByKinds(items, kinds) {
  const allow = new Set(kinds || [])
  return (items || []).filter((item) => allow.has(item.kind))
}

export function cloneItems(items) {
  return (items || []).map((item) => {
    const copy = { ...item, id: id() }
    delete copy.position
    delete copy.created_at
    delete copy.updated_at
    return copy
  })
}

function songIdentity(item) {
  const songId = String(item?.song_id || '').trim()
  if (songId) return `id:${songId}`
  return `title:${String(item?.title || '').trim().toLowerCase()}`
}

// Песни, которые уже есть, пропускаются, если skipSongs не выключен.
// Новые пункты получают новые id и встают в конец в исходном порядке.
export function appendItems(items, extra, options = {}) {
  const skipSongs = options.skipSongs !== false
  const have = new Set((items || []).filter((item) => item.kind === 'song').map(songIdentity))
  const picked = []
  for (const item of filterByKinds(extra, options.kinds)) {
    if (skipSongs && item.kind === 'song') {
      const key = songIdentity(item)
      if (have.has(key)) continue
      have.add(key)
    }
    picked.push(item)
  }
  return (items || []).concat(cloneItems(picked))
}

export function countByKind(items) {
  const counts = { song: 0, bible: 0, text: 0 }
  for (const item of items || []) {
    if (Object.hasOwn(counts, item.kind)) counts[item.kind] += 1
  }
  return counts
}

export function pointsLabel(count) {
  const value = count || 0
  const mod10 = value % 10
  const mod100 = value % 100
  if (mod10 === 1 && mod100 !== 11) return `${value} пункт`
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return `${value} пункта`
  return `${value} пунктов`
}

export function planFormatLabel(format) {
  if (format === 'vplt') return 'plan.vplt'
  if (format === 'history') return 'History.ini'
  if (format === 'plandef') return 'PlanDef.ini'
  if (format === 'json') return 'JSON'
  return 'файл'
}

export function planEncodingLabel(encoding) {
  return encoding === 'windows-1251' ? 'Windows-1251' : 'UTF-8'
}

function pad(value) {
  return String(value).padStart(2, '0')
}

function clock(date, now, withYear) {
  const day = `${pad(date.getDate())}.${pad(date.getMonth() + 1)}`
  const year = withYear || date.getFullYear() !== now.getFullYear() ? `.${date.getFullYear()}` : ''
  return `${day}${year}, ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function planUpdatedLabel(value, now = new Date()) {
  const date = new Date(value || '')
  if (Number.isNaN(date.getTime())) return ''
  return `изменён ${clock(date, now, false)}`
}

export function itemStamp(item, now = new Date()) {
  if (!item?.created_at) return { text: 'не сохранён', title: 'Ещё не сохранён' }
  const created = new Date(item.created_at)
  const updated = new Date(item.updated_at || item.created_at)
  if (Number.isNaN(created.getTime())) return { text: 'не сохранён', title: 'Ещё не сохранён' }
  const changed = !Number.isNaN(updated.getTime()) && updated.getTime() > created.getTime()
  const when = changed ? updated : created
  return {
    text: `${changed ? 'изменён' : 'создан'} ${clock(when, now, false)}`,
    title: clock(when, now, true),
  }
}

// Куда чат кладёт пункт «В план». io — запросы сайта или клиента:
// list() → планы, read(id) → план с items, save(plan) → сохранить items, create(title, items).
export function planTarget(io) {
  return {
    list: io.list,
    async add(planId, result) {
      const plan = await io.read(planId)
      const before = plan.items || []
      const items = appendItems(before, [planItemFromResult(result)], { kinds: ['song', 'bible', 'text'] })
      if (items.length === before.length) throw new Error('Эта песня уже есть в плане.')
      await io.save({ ...plan, items })
    },
    async create(title, result) {
      await io.create(title, [planItemFromResult(result)])
    },
  }
}

// Полный текст пункта плана для просмотра «глазом».
// io — запросы сайта или клиента: song(songbookId, fbid), verse(module, book, chapter, verse).
// Если сервер текста не дал, показывается то, что сохранено в самом пункте.
export async function planItemText(item, io) {
  if (item.kind === 'song') {
    const fallback = {
      title: item.title || 'Песня',
      source: [item.songbook_title, item.section_title].filter(Boolean).join(' · '),
      text: '',
    }
    if (!item.songbook_id || !item.song_id) return fallback
    const song = await io.song(item.songbook_id, item.song_id)
    return {
      title: song.title || fallback.title,
      source: [song.songbook_title || item.songbook_title, song.section_title || item.section_title].filter(Boolean).join(' · '),
      text: song.text || '',
    }
  }
  if (item.kind === 'bible') {
    const reference = item.reference || [item.book, item.chapter, item.verse].filter(Boolean).join(':')
    const view = { title: reference || 'Стих', source: [item.module_short_name, reference].filter(Boolean).join(' · '), text: item.quote || '' }
    if (!item.module_short_name || !item.book || !item.chapter || !item.verse) return view
    try {
      const verse = await io.verse(item.module_short_name, item.book, item.chapter, item.verse)
      const module = verse.module_title ? `${verse.module_short_name} — ${verse.module_title}` : verse.module_short_name
      return { ...view, source: [module, reference].filter(Boolean).join(' · '), text: verse.text || view.text }
    } catch (err) {
      if (view.text) return view
      throw err
    }
  }
  return { title: item.caption || 'Заметка', source: 'Заметка', text: item.quote || '' }
}
