const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

/**
 * @typedef {Object} SongbookCard
 * @property {'songbook'} kind
 * @property {string} id
 * @property {string} title
 * @property {string} note
 * @property {string} [dimension_id]
 */

/**
 * @param {any} raw
 * @returns {SongbookCard|null}
 */
export function normalizeSongbook(raw) {
  if (!raw || typeof raw !== 'object') return null
  if (raw.kind && raw.kind !== 'songbook') return null
  const id = String(raw.id || '').trim()
  if (!uuid.test(id)) return null
  const dim = String(raw.dimension_id || '').trim()
  return {
    kind: 'songbook',
    id,
    title: String(raw.title || '').trim() || 'Сборник',
    note: String(raw.note || '').trim(),
    dimension_id: uuid.test(dim) ? dim : '',
  }
}

/**
 * @param {any} raw
 * @returns {SongbookCard[]}
 */
export function songbookCards(raw) {
  if (!Array.isArray(raw)) return []
  return raw.map(normalizeSongbook).filter(Boolean)
}

/**
 * @param {SongbookCard[]|undefined} current
 * @param {any} extra
 * @returns {SongbookCard[]}
 */
export function mergeCards(current, extra) {
  const out = [...(current || [])]
  const seen = new Set(out.map((card) => `${card.kind}:${card.id}`))
  for (const card of songbookCards(extra)) {
    const key = `${card.kind}:${card.id}`
    if (seen.has(key)) continue
    seen.add(key)
    out.push(card)
  }
  return out
}

/**
 * @param {import('./results.js').AiResult[]} songs
 * @param {string} q
 * @returns {import('./results.js').AiResult[]}
 */
export function filterSongs(songs, q) {
  const query = String(q || '').trim().toLowerCase()
  if (!query) return songs
  return songs.filter((song) => {
    const hay = [song.title, song.note, song.alt_title, song.author]
      .map((value) => String(value || '').toLowerCase())
    return hay.some((value) => value.includes(query))
  })
}

/**
 * Песня из поиска по сборнику — тот же формат, что у блока ```results.
 * @param {any} hit
 * @returns {import('./results.js').AiResult|null}
 */
export function songFromHit(hit) {
  if (!hit || typeof hit !== 'object') return null
  const book = String(hit.songbook_id || '').trim()
  const fbid = String(hit.fbid || '').trim()
  if (!uuid.test(book) || !fbid || fbid.includes('..')) return null
  const dim = String(hit.dimension_id || '').trim()
  const id = uuid.test(dim) ? `${dim}/${book}/${fbid}` : `${book}/${fbid}`
  return {
    kind: 'song',
    id,
    title: String(hit.title || '').trim() || 'Песня',
    note: String(hit.section_title || hit.songbook_title || '').trim(),
    body: '',
  }
}
