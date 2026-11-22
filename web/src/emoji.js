import books from './assets/emoji/books.svg'
import calendar from './assets/emoji/calendar.svg'
import laptop from './assets/emoji/laptop.svg'
import people from './assets/emoji/people.svg'
import shield from './assets/emoji/shield.svg'

export const emojiSrc = { books, calendar, laptop, people, shield }

const twemojiFiles = import.meta.glob('./assets/twemoji/*.svg', { query: '?url', import: 'default' })

function twemojiKey(cps) {
  return `./assets/twemoji/${cps}.svg`
}

/** Есть ли SVG этого знака в локальном наборе. */
export function hasTwemoji(cps) {
  return Boolean(twemojiFiles[twemojiKey(cps)])
}

/** Подгружает один SVG, не таща весь каталог в основной файл. */
export function loadTwemoji(cps) {
  const load = twemojiFiles[twemojiKey(cps)]
  return load ? load() : Promise.resolve('')
}

/**
 * Имя файла Twemoji: кодовые точки через дефис.
 * FE0F (вариант эмодзи) выкидывается, ZWJ (200D) остаётся.
 * @param {string} text
 * @returns {string}
 */
export function emojiToCodepoints(text) {
  const parts = []
  for (const ch of String(text || '')) {
    const cp = ch.codePointAt(0)
    if (cp === 0xfe0f || cp === 0xfe0e) continue
    parts.push(cp.toString(16))
  }
  return parts.join('-')
}

const emojiRE = /(\p{Regional_Indicator}{2}|\p{Extended_Pictographic}(?:\uFE0F|\uFE0E)?(?:\u200D\p{Extended_Pictographic}(?:\uFE0F|\uFE0E)?)*|[#*0-9]\uFE0F?\u20E3)/gu

/**
 * Делит строку на текст и известные знаки. Неизвестного файла нет — остаётся текст.
 * @param {string} text
 * @returns {{type: 'text'|'emoji', text: string, cps?: string}[]}
 */
export function splitEmoji(text) {
  const source = String(text || '')
  const re = new RegExp(emojiRE.source, 'gu')
  const out = []
  let last = 0
  for (const match of source.matchAll(re)) {
    const start = match.index || 0
    if (start > last) out.push({ type: 'text', text: source.slice(last, start) })
    const raw = match[0]
    const cps = emojiToCodepoints(raw)
    if (hasTwemoji(cps)) out.push({ type: 'emoji', text: raw, cps })
    else out.push({ type: 'text', text: raw })
    last = start + raw.length
  }
  if (last < source.length) out.push({ type: 'text', text: source.slice(last) })
  return out
}
