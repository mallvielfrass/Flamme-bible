import { describe, expect, it } from 'vitest'
import { readVbmCard, songbookPayload } from './vbmMeta'

function utf16(text) {
  const units = []
  for (let i = 0; i < text.length; i += 1) units.push(text.charCodeAt(i))
  const bytes = new Uint8Array(2 + units.length * 2)
  const view = new DataView(bytes.buffer)
  view.setUint16(0, units.length, true)
  units.forEach((unit, index) => view.setUint16(2 + index * 2, unit, true))
  return bytes
}

function cardBytes({ version = 0x102, flags = 3, charset = 0x4b0, shortName = 'SB', title = 'Гимны' } = {}) {
  const left = utf16(shortName)
  const right = utf16(title)
  const bytes = new Uint8Array(72 + left.length + right.length)
  bytes.set(new TextEncoder().encode('VisioBibleModule'), 0)
  const view = new DataView(bytes.buffer)
  view.setUint32(16, version, true)
  view.setUint32(28, flags, true)
  view.setUint32(32, charset, true)
  bytes.set(left, 72)
  bytes.set(right, 72 + left.length)
  return bytes
}

describe('vbm card', () => {
  it('reads the header the server stores with a songbook', () => {
    const card = readVbmCard(cardBytes())
    expect(card.header).toEqual({
      format_version: '0x102',
      flags: 3,
      charset: '0x4b0',
      header_short: 'SB',
      header_title: 'Гимны',
    })
  })

  it('rejects a file that is not a songbook', () => {
    expect(() => readVbmCard(new Uint8Array(8))).toThrow(/сборника/)
    const bytes = cardBytes()
    bytes[0] = 0
    expect(() => readVbmCard(bytes)).toThrow(/сборника/)
  })

  it('builds the create payload with a header', () => {
    const manual = songbookPayload({ title: 'Молодые', shortName: 'Мол', fbid: 'id-1' })
    expect(manual.fbid).toBe('id-1')
    expect(manual.title).toBe('Молодые')
    expect(manual.header.format_version).toBe('0x102')
    expect(manual.header.charset).toBe('0x4b0')
    expect(manual.header.header_title).toBe('Молодые')
    const fromFile = songbookPayload({
      title: 'Другое имя',
      shortName: 'SB',
      fbid: 'id-2',
      header: readVbmCard(cardBytes()).header,
    })
    expect(fromFile.title).toBe('Другое имя')
    expect(fromFile.header.header_title).toBe('Гимны')
  })
})
