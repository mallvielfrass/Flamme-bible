import { moduleNameOk } from './checks'
import { bibleItem } from './planItems'

// Логика страницы «Библия» без Vue: выбор переводов, выделение стихов, пункт плана.

export const translationsKey = 'flamme.bible.translations'
export const maxTranslations = 4

/** Список коротких имён: без повторов и мусора, не больше maxTranslations. */
export function cleanTranslations(list) {
  const out = []
  for (const raw of Array.isArray(list) ? list : []) {
    const name = String(raw || '').trim()
    if (moduleNameOk(name) && !out.includes(name)) out.push(name)
    if (out.length === maxTranslations) break
  }
  return out
}

export function loadTranslations(storage) {
  try {
    return cleanTranslations(JSON.parse(storage?.getItem(translationsKey) || '[]'))
  } catch {
    return []
  }
}

export function saveTranslations(storage, list) {
  const clean = cleanTranslations(list)
  try {
    storage?.setItem(translationsKey, JSON.stringify(clean))
  } catch {
    // Хранилище недоступно (приватный режим): выбор живёт до перезагрузки.
  }
  return clean
}

/** Включить или выключить перевод. Порядок выбора сохраняется: первый — основной. */
export function toggleTranslation(list, name) {
  if (list.includes(name)) return list.filter((item) => item !== name)
  if (list.length >= maxTranslations) return list
  return list.concat(name)
}

/** Оставить выбранными только переводы, которые есть на сервере; пусто — первый доступный. */
export function availableTranslations(list, known) {
  const names = new Set((known || []).map((item) => item.short_name))
  const kept = list.filter((name) => names.has(name))
  if (kept.length || !known?.length) return kept
  return [known[0].short_name]
}

/**
 * Клик по стиху. Первый клик выделяет стих, второй растягивает выделение до него,
 * следующий начинает заново. Клик по единственному выделенному стиху снимает выделение.
 * @returns {{start: number, end: number} | null}
 */
export function selectVerse(range, verse) {
  if (!range) return { start: verse, end: verse }
  if (range.start === range.end) {
    if (range.start === verse) return null
    return { start: Math.min(range.start, verse), end: Math.max(range.start, verse) }
  }
  return { start: verse, end: verse }
}

export function inRange(range, verse) {
  return Boolean(range) && verse >= range.start && verse <= range.end
}

export function rangeLabel(bookName, chapter, range) {
  const name = bookName || 'Книга'
  if (!range) return `${name} ${chapter}`
  const verses = range.start === range.end ? `${range.start}` : `${range.start}-${range.end}`
  return `${name} ${chapter}:${verses}`
}

/** Текст выделенных стихов: «16 Ибо так…» построчно, как у отрывков поиска по смыслу. */
export function rangeText(verses, range) {
  return (verses || [])
    .filter((item) => inRange(range, item.verse))
    .map((item) => `${item.verse} ${String(item.text || '').trim()}`)
    .join('\n')
}

/**
 * Пункт плана «Стих» из выделения. Основной перевод — первый выбранный,
 * второй выбранный уходит в module_short_name_second (параллельный показ в VisioBible).
 */
export function passageItem({ translations, book, bookName, chapter, range, verses }) {
  if (!range) throw new Error('Выделите стихи.')
  return bibleItem({
    module_short_name: translations[0] || '',
    module_short_name_second: translations[1] || '',
    book,
    chapter,
    verse: range.start,
    reference: rangeLabel(bookName, chapter, range),
    quote: rangeText(verses, range),
  })
}

/** Место из поиска (стих по словам или отрывок по смыслу) как выделение главы. */
export function hitRange(hit) {
  const start = Number(hit?.verse || hit?.verse_start) || 0
  const end = Number(hit?.verse_end) || start
  if (!start) return null
  return { start, end: Math.max(start, end) }
}

/** Пункт плана прямо из результата поиска, с текстом из найденного перевода. */
export function hitItem(translations, hit) {
  const range = hitRange(hit)
  const own = hit?.module_short_name || ''
  const rest = translations.filter((name) => name !== own)
  return bibleItem({
    module_short_name: own || translations[0] || '',
    module_short_name_second: own ? rest[0] || '' : translations[1] || '',
    book: hit?.book,
    chapter: hit?.chapter,
    verse: range?.start || 0,
    reference: hit?.reference || rangeLabel(hit?.book_name, hit?.chapter, range),
    quote: hit?.text || '',
  })
}
