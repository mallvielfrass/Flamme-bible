import { describe, expect, it } from 'vitest'
import {
  bookStatus,
  canApprove,
  canReject,
  canRevoke,
  canSubmit,
  formatWhen,
  moderationHint,
  clipPlanText,
  decodePlanText,
  importEncodingNotice,
  readPlanFile,
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

  it('allows approve from pending or rejected and revoke only from approved', () => {
    expect(canApprove('pending')).toBe(true)
    expect(canApprove('rejected')).toBe(true)
    expect(canApprove('approved')).toBe(false)
    expect(canApprove('draft')).toBe(false)
    expect(canReject('pending')).toBe(true)
    expect(canReject('rejected')).toBe(false)
    expect(canRevoke('approved')).toBe(true)
    expect(canRevoke('pending')).toBe(false)
    expect(moderationHint('revoke', 'pending')).toMatch(/одобрен/)
    expect(formatWhen('')).toBe('—')
    expect(formatWhen('2026-01-02T03:04:00Z')).toContain('2026')
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

  it('keeps a mojibake warning until the user asks to repair it', () => {
    expect(importEncodingNotice('ÐŸ', { mojibake: false })).toBe(null)
    const notice = importEncodingNotice('ÐŸÑ€Ð¸Ð²ÐµÑ‚', {
      mojibake: true,
      encoding_warning: 'проверьте кодировку',
      suggested_text: 'Привет',
      suggested_items: [{ kind: 'text', caption: 'Привет' }],
      suggested_title: '',
    })
    expect(notice.warning).toBe('проверьте кодировку')
    expect(notice.before).toBe('ÐŸÑ€Ð¸Ð²ÐµÑ‚')
    expect(notice.after).toBe('Привет')
    expect(notice.items[0].caption).toBe('Привет')
    expect(clipPlanText(`  ${'а'.repeat(300)}  `).endsWith('…')).toBe(true)
    expect(clipPlanText('Привет')).toBe('Привет')
  })

  it('decodes a plan file from utf-8 or windows-1251', () => {
    expect(decodePlanText(new TextEncoder().encode('План'))).toBe('План')
    expect(decodePlanText(new Uint8Array([0xef, 0xbb, 0xbf, 0x41]))).toBe('A')
    expect(decodePlanText(new Uint8Array([0xc0]))).toBe('А')
    expect(readPlanFile(new Uint8Array([0xc0]), 'auto')).toEqual({ text: 'А', encoding: 'windows-1251' })
    expect(readPlanFile(new TextEncoder().encode('План'), 'utf-8').encoding).toBe('utf-8')
  })
})
