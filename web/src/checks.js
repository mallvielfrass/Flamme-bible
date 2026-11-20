const moduleNameRe = /^[0-9A-Za-z][0-9A-Za-z_+-]{0,31}$/
const releaseVersionRe = /^[0-9A-Za-z][0-9A-Za-z._+-]{0,31}$/

const statusNames = {
  draft: 'черновик',
  pending: 'на проверке',
  approved: 'одобрено',
  rejected: 'отклонено',
}

export function moduleNameOk(name) {
  return moduleNameRe.test(name)
}

export function releaseVersionOk(version) {
  if (!version || version.includes('..') || version.includes('/') || version.includes('\\')) return false
  return releaseVersionRe.test(version)
}

export function statusText(status) {
  return statusNames[status] || 'черновик'
}

export function bookStatus(book) {
  if (book?.status) return statusText(book.status)
  if (book?.public) return statusText('approved')
  if (book?.publish_requested) return statusText('pending')
  return statusText('draft')
}

export function canSubmit(status) {
  return !status || status === 'draft' || status === 'rejected'
}

export function inviteToken(raw) {
  const text = String(raw || '').trim()
  if (!text) return ''
  try {
    const url = new URL(text)
    const parts = url.pathname.split('/').filter(Boolean)
    const index = parts.indexOf('invite')
    if (index >= 0 && parts[index + 1]) return decodeURIComponent(parts[index + 1])
  } catch {
    // Строка без схемы — код или кусок пути.
  }
  const match = text.match(/invite\/([^/?#\s]+)/)
  if (match) return decodeURIComponent(match[1])
  return text
}

export function decodePlanText(bytes) {
  const data = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes)
  let start = 0
  if (data.length >= 3 && data[0] === 0xef && data[1] === 0xbb && data[2] === 0xbf) start = 3
  const slice = data.subarray(start)
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(slice)
  } catch {
    return new TextDecoder('windows-1251').decode(slice)
  }
}

export function formatSize(size) {
  const n = Number(size) || 0
  if (n <= 0) return ''
  if (n < 1024) return `${n} Б`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} КБ`
  return `${(n / (1024 * 1024)).toFixed(1)} МБ`
}
