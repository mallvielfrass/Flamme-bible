import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { api } from '../api'
import { useSessionStore } from './session'

function cursorKey(id) {
  return `flammebible.cursor.${id}`
}

function outgoingKey(id) {
  return `flammebible.outgoing.${id}`
}

function conflictKey(id) {
  return `flammebible.conflicts.${id}`
}

function readJSON(key, fallback) {
  try {
    const value = JSON.parse(localStorage.getItem(key))
    return value ?? fallback
  } catch {
    return fallback
  }
}

export const useSyncStore = defineStore('sync', () => {
  const dimensionId = ref('')
  const songbooks = ref([])
  const bookId = ref('')
  const appliedCursor = ref(0)
  const serverCursor = ref(null)
  const events = ref([])
  const outgoing = ref([])
  const conflicts = ref([])
  const readError = ref('')
  const pushError = ref('')
  const ready = ref(false)

  const done = computed(() =>
    ready.value
    && !readError.value
    && !pushError.value
    && outgoing.value.length === 0
    && events.value.length === 0
    && conflicts.value.every((item) => item.answer === 'server' || item.answer === 'new'),
  )

  function saveOutgoing() {
    const all = readJSON(outgoingKey(dimensionId.value), {})
    all[bookId.value] = outgoing.value
    localStorage.setItem(outgoingKey(dimensionId.value), JSON.stringify(all))
  }

  function saveConflicts() {
    localStorage.setItem(conflictKey(dimensionId.value), JSON.stringify(conflicts.value))
  }

  function saveCursor() {
    localStorage.setItem(cursorKey(dimensionId.value), String(appliedCursor.value))
  }

  async function load(id) {
    const session = useSessionStore()
    if (dimensionId.value !== id) {
      songbooks.value = []
      events.value = []
      outgoing.value = []
      conflicts.value = []
      serverCursor.value = null
      bookId.value = ''
    }
    dimensionId.value = id
    ready.value = false
    readError.value = ''
    appliedCursor.value = Number(localStorage.getItem(cursorKey(id)) || 0)
    conflicts.value = readJSON(conflictKey(id), [])
    try {
      songbooks.value = await api(`/dimensions/${id}/songbooks`, { token: session.token })
      if (!songbooks.value.some((book) => book.id === bookId.value)) {
        bookId.value = songbooks.value[0]?.id || ''
      }
      const stored = readJSON(outgoingKey(id), {})
      outgoing.value = bookId.value ? (stored[bookId.value] || []) : []
      const page = await api(`/dimensions/${id}/changes?cursor=${appliedCursor.value}`, { token: session.token })
      events.value = page.events || []
      if (bookId.value) {
        const snapshot = await api(`/dimensions/${id}/songbooks/${bookId.value}/snapshot`, { token: session.token })
        serverCursor.value = snapshot.cursor
      } else {
        serverCursor.value = null
      }
      ready.value = true
    } catch (err) {
      events.value = []
      readError.value = err.message
    }
  }

  async function selectBook(id) {
    bookId.value = id
    await load(dimensionId.value)
  }

  async function send() {
    const session = useSessionStore()
    if (!bookId.value || outgoing.value.length === 0) return
    try {
      const data = await api(`/dimensions/${dimensionId.value}/songbooks/${bookId.value}/push`, {
        method: 'POST',
        token: session.token,
        // base_revision — уже применённый курсор. Расхождение с сервером даёт конфликт и не двигает счётчик.
        body: { base_revision: appliedCursor.value, operations: outgoing.value },
      })
      pushError.value = ''
      const left = []
      for (const op of outgoing.value) {
        const result = (data.results || []).find((item) => item.operation_id === op.operation_id)
        if (!result) {
          left.push(op)
          continue
        }
        if (result.status === 'applied') {
          conflicts.value = conflicts.value.filter((item) => item.operation_id !== op.operation_id)
          continue
        }
        if (result.status === 'conflict') {
          const previous = conflicts.value.find((item) => item.operation_id === op.operation_id)
          const next = {
            operation_id: op.operation_id,
            book_id: bookId.value,
            server: result.server ?? null,
            client: result.client ?? null,
            answer: previous?.answer || '',
          }
          conflicts.value = conflicts.value.filter((item) => item.operation_id !== op.operation_id)
          conflicts.value.push(next)
          continue
        }
        left.push(op)
        pushError.value = result.status
      }
      outgoing.value = left
      saveOutgoing()
      saveConflicts()
    } catch (err) {
      pushError.value = err.message
    }
  }

  async function applyPage() {
    if (events.value.length === 0) return
    appliedCursor.value = events.value[events.value.length - 1].seq
    saveCursor()
    await load(dimensionId.value)
  }

  async function reloadSnapshot() {
    appliedCursor.value = 0
    saveCursor()
    await load(dimensionId.value)
  }

  function answer(operationId, choice) {
    if (choice !== 'server' && choice !== 'new') return
    conflicts.value = conflicts.value.map((item) =>
      item.operation_id === operationId ? { ...item, answer: choice } : item,
    )
    saveConflicts()
  }

  return {
    dimensionId, songbooks, bookId, appliedCursor, serverCursor, events, outgoing, conflicts,
    readError, pushError, ready, done,
    load, selectBook, send, applyPage, reloadSnapshot, answer,
  }
})
