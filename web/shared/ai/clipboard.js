/**
 * Копирование текста. Сначала Clipboard API, затем запасной путь
 * для окна, где буфер браузера закрыт.
 * @param {string} text
 */
export async function copyText(text) {
  const value = String(text || '')
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value)
      return
    } catch {
      /* ниже запасной путь */
    }
  }
  if (typeof document === 'undefined') return
  const area = document.createElement('textarea')
  area.value = value
  area.setAttribute('readonly', '')
  area.style.position = 'fixed'
  area.style.left = '-9999px'
  document.body.appendChild(area)
  area.select()
  try {
    document.execCommand('copy')
  } catch {
    /* буфер может быть закрыт */
  }
  area.remove()
}
