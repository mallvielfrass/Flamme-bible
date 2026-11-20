const MAGIC = 'VisioBibleModule'
const HEADER_FIXED = 72

export const fallbackHeader = {
  format_version: '0x102',
  flags: 0,
  charset: '0x4b0',
  header_short: 'SB',
  header_title: 'Songbook',
}

function readUtf16(view, offset) {
  if (offset + 2 > view.byteLength) throw new Error('Это не файл сборника VisioBible.')
  const count = view.getUint16(offset, true)
  const bytes = count * 2
  const start = offset + 2
  if (start + bytes > view.byteLength) throw new Error('Это не файл сборника VisioBible.')
  const text = new TextDecoder('utf-16le').decode(new Uint8Array(view.buffer, view.byteOffset + start, bytes))
  return { text, next: start + bytes }
}

export function readVbmCard(bytes) {
  const data = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes)
  if (data.length < HEADER_FIXED) throw new Error('Это не файл сборника VisioBible.')
  const magic = String.fromCharCode(...data.subarray(0, 16))
  if (magic !== MAGIC) throw new Error('Это не файл сборника VisioBible.')
  const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
  const version = view.getUint32(16, true)
  const flags = view.getUint32(28, true)
  const charset = view.getUint32(32, true)
  const shortName = readUtf16(view, HEADER_FIXED)
  const title = readUtf16(view, shortName.next)
  return {
    header: {
      format_version: `0x${version.toString(16)}`,
      flags,
      charset: `0x${charset.toString(16)}`,
      header_short: shortName.text,
      header_title: title.text,
    },
  }
}

export function songbookPayload({ title, shortName, publisher, comment, fbid, header }) {
  const name = (title || '').trim()
  const brief = (shortName || '').trim()
  const head = header || {
    ...fallbackHeader,
    header_short: brief || fallbackHeader.header_short,
    header_title: name || fallbackHeader.header_title,
  }
  return {
    fbid,
    title: name,
    short_name: brief,
    publisher: (publisher || '').trim(),
    revision_date: '',
    comment: (comment || '').trim(),
    header: {
      format_version: head.format_version,
      flags: Number(head.flags) || 0,
      charset: head.charset,
      header_short: head.header_short || '',
      header_title: head.header_title || '',
    },
  }
}
