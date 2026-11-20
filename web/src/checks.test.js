import { describe, expect, it } from 'vitest'
import {
  bookStatus,
  canSubmit,
  decodePlanText,
  inviteToken,
  moduleNameOk,
  releaseVersionOk,
  statusText,
} from './checks'

describe('checks', () => {
  it('names moderation states in plain russian', () => {
    expect(statusText('draft')).toBe('черновик')
    expect(statusText('pending')).toBe('на проверке')
    expect(statusText('approved')).toBe('одобрено')
    expect(statusText('rejected')).toBe('отклонено')
    expect(bookStatus({ public: true })).toBe('одобрено')
    expect(bookStatus({ publish_requested: true })).toBe('на проверке')
    expect(bookStatus({})).toBe('черновик')
    expect(bookStatus({ status: 'rejected', public: true })).toBe('отклонено')
  })

  it('allows submit only from a draft or a rejection', () => {
    expect(canSubmit('draft')).toBe(true)
    expect(canSubmit('rejected')).toBe(true)
    expect(canSubmit('')).toBe(true)
    expect(canSubmit('pending')).toBe(false)
    expect(canSubmit('approved')).toBe(false)
  })

  it('accepts module names and release versions the server accepts', () => {
    expect(moduleNameOk('RST')).toBe(true)
    expect(moduleNameOk('RST+')).toBe(true)
    expect(moduleNameOk('кир')).toBe(false)
    expect(releaseVersionOk('1.2.3')).toBe(true)
    expect(releaseVersionOk('1..2')).toBe(false)
    expect(releaseVersionOk('../x')).toBe(false)
  })

  it('reads an invite from a code or a link', () => {
    expect(inviteToken('  one-time  ')).toBe('one-time')
    expect(inviteToken('https://church.test/invite/one-time')).toBe('one-time')
    expect(inviteToken('invite/abc')).toBe('abc')
  })

  it('decodes a plan file from utf-8 or windows-1251', () => {
    expect(decodePlanText(new TextEncoder().encode('План'))).toBe('План')
    expect(decodePlanText(new Uint8Array([0xef, 0xbb, 0xbf, 0x41]))).toBe('A')
    expect(decodePlanText(new Uint8Array([0xc0]))).toBe('А')
  })
})
