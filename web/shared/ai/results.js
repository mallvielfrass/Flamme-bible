/** Ключ provide: загрузка текста песни или стиха, если в пункте его нет. */
export const aiResultKey = Symbol('aiResult')

/**
 * Ключ provide: куда добавить пункт ответа. Без него кнопки «В план» нет.
 * @typedef {Object} AiPlanTarget
 * @property {() => Promise<{ id: string, title: string }[]>} list
 * @property {(planId: string, item: AiResult) => Promise<void>} add
 * @property {() => Promise<{ id: string, name: string }[]>} places общины, куда можно сохранить новый план
 * @property {() => string} defaultPlace id общины по умолчанию, '' — личное
 * @property {(title: string, item: AiResult, dimensionId: string) => Promise<void>} create
 */
export const aiPlanKey = Symbol('aiPlan')

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

/**
 * @typedef {Object} AiResult
 * @property {'song'|'verse'} kind
 * @property {string} id
 * @property {string} title
 * @property {string} note
 * @property {string} body
 * @property {string} [module_short_name] только стих
 * @property {number} [book] только стих
 * @property {number} [chapter] только стих
 * @property {number} [verse] только стих
 */

/**
 * Блок ```results в ответе. Пустой или битый JSON — не результаты.
 * @param {string} raw
 * @returns {AiResult[]}
 */
export function parseResults(raw) {
  let data
  try {
    data = JSON.parse(String(raw || ''))
  } catch {
    return []
  }
  if (!Array.isArray(data)) return []
  return data.map(normalizeResult).filter(Boolean)
}

/**
 * @param {any} raw
 * @returns {AiResult|null}
 */
export function normalizeResult(raw) {
  if (!raw || typeof raw !== 'object') return null
  const kind = raw.kind === 'verse' || raw.kind === 'bible' ? 'verse' : raw.kind === 'song' ? 'song' : ''
  if (!kind) return null
  if (kind === 'song') {
    const id = songId(raw)
    const title = text(raw.title) || 'Песня'
    const body = songBody(raw)
    if (!id && !body) return null
    return { kind, id, title, note: text(raw.songbook_title || raw.note), body }
  }
  const title = text(raw.reference || raw.title || raw.id) || 'Стих'
  const id = text(raw.id) || title
  return {
    kind,
    id,
    title,
    note: text(raw.module_title || raw.module_short_name || raw.note),
    body: text(raw.text || raw.body),
    module_short_name: text(raw.module_short_name),
    book: Number(raw.book) || 0,
    chapter: Number(raw.chapter) || 0,
    verse: Number(raw.verse_start || raw.verse) || 0,
  }
}

function text(value) {
  return String(value || '').trim()
}

function songId(raw) {
  const explicit = text(raw.id)
  if (explicit.includes('/')) return explicit
  const dim = text(raw.dimension_id)
  const book = text(raw.songbook_id)
  const fbid = text(raw.fbid || raw.id)
  if (dim && book && fbid) return `${dim}/${book}/${fbid}`
  return explicit
}

function songBody(raw) {
  const ready = text(raw.lyrics || raw.body)
  if (ready) return ready
  if (!Array.isArray(raw.parts)) return ''
  return raw.parts.map((part) => text(part?.text)).filter(Boolean).join('\n\n')
}

const books = new Map()

/**
 * Текст пункта, которого нет в самом ответе.
 * Песня: id = dimension/songbook/fbid, снимок сборника.
 * Стих: id или reference уходит в поиск по смыслу.
 * @param {{ snapshot: Function, bibleSemantic: Function }} client
 * @param {AiResult} item
 */
export async function loadResult(client, item) {
  if (item.kind === 'song') return loadSong(client, item.id)
  return loadVerse(client, item.title || item.id)
}

function parseSongRef(id) {
  const parts = String(id || '').split('/')
  if (parts.length >= 3 && uuid.test(parts[0]) && uuid.test(parts[1])) {
    const fbid = parts.slice(2).join('/')
    if (!fbid || fbid.includes('..')) return null
    return { dim: parts[0], book: parts[1], fbid }
  }
  if (parts.length >= 2 && uuid.test(parts[0])) {
    const fbid = parts.slice(1).join('/')
    if (!fbid || fbid.includes('..')) return null
    return { book: parts[0], fbid }
  }
  return null
}

async function loadSong(client, id) {
  const ref = parseSongRef(id)
  if (!ref) throw new Error('Текст не найден.')
  // /api/songs/text работает и для одобренных общих сборников, снимок — только для своих.
  if (typeof client.songText === 'function') {
    const data = await client.songText(ref.book, ref.fbid)
    const body = text(data?.text)
    if (body) return body
  }
  const dim = ref.dim
  const book = ref.book
  const fbid = ref.fbid
  if (!dim) throw new Error('Текст не найден.')
  const key = `${dim}/${book}`
  if (!books.has(key)) {
    books.set(key, client.snapshot(dim, book).catch((err) => {
      books.delete(key)
      throw err
    }))
  }
  const snap = await books.get(key)
  const song = (snap?.songs || []).find((row) => row.fbid === fbid && !row.deleted)
  const body = songBody(song || {})
  if (!body) throw new Error('Текст не найден.')
  return body
}

async function loadVerse(client, q) {
  const query = text(q).slice(0, 200)
  if (!query) throw new Error('Текст не найден.')
  const data = await client.bibleSemantic(query)
  const list = data?.passages || []
  const hit = list.find((row) => text(row.reference) === query) || list[0]
  const body = text(hit?.text)
  if (!body) throw new Error('Текст не найден.')
  return body
}
