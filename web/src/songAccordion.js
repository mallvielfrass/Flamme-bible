import { ref } from 'vue'

/** Ключ песни в списке поиска: сборник + fbid. */
export function songKey(song) {
  const book = String(song?.songbook_id || '').trim()
  const fbid = String(song?.fbid || '').trim()
  if (!book || !fbid) return ''
  return `${book}/${fbid}`
}

export function songPanelId(song) {
  const key = songKey(song)
  if (!key) return ''
  return `song-text-${key.replace(/[^a-zA-Z0-9_-]/g, '-')}`
}

/**
 * Гармошка: открыта одна песня, текст подгружается при первом открытии и кэшируется.
 * @param {(song: object) => Promise<string>} loadText
 */
export function useSongAccordion(loadText) {
  const openKey = ref('')
  const cache = ref({})
  const loading = ref({})
  const errors = ref({})

  function isOpen(song) {
    const key = songKey(song)
    return Boolean(key) && openKey.value === key
  }

  function textOf(song) {
    return cache.value[songKey(song)] || ''
  }

  function errorOf(song) {
    return errors.value[songKey(song)] || ''
  }

  function isLoading(song) {
    return Boolean(loading.value[songKey(song)])
  }

  function collapse() {
    openKey.value = ''
  }

  async function toggle(song) {
    const key = songKey(song)
    if (!key) return
    if (openKey.value === key) {
      openKey.value = ''
      return
    }
    openKey.value = key
    if (cache.value[key] || loading.value[key]) return
    loading.value = { ...loading.value, [key]: true }
    errors.value = { ...errors.value, [key]: '' }
    try {
      const body = String(await loadText(song) || '')
      if (!body.trim()) throw new Error('Текст не найден.')
      cache.value = { ...cache.value, [key]: body }
    } catch (err) {
      errors.value = { ...errors.value, [key]: err?.message || 'Текст не найден.' }
    } finally {
      loading.value = { ...loading.value, [key]: false }
    }
  }

  return { openKey, cache, toggle, isOpen, textOf, errorOf, isLoading, collapse }
}
