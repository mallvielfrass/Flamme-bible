import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api'

const sessionKey = 'flammebible.session'

function deviceKey(email) {
  return `flammebible.device.${email}`
}

function readSession() {
  try {
    return JSON.parse(sessionStorage.getItem(sessionKey)) || null
  } catch {
    return null
  }
}

export const useSessionStore = defineStore('session', () => {
  const saved = readSession()
  const token = ref(saved?.token || '')
  const userId = ref(saved?.userId || '')
  const deviceIdValue = ref(saved?.deviceId || '')
  const dimensionId = ref(saved?.dimensionId || '')
  const spaces = ref(saved?.spaces || [])

  function persist() {
    sessionStorage.setItem(sessionKey, JSON.stringify({
      token: token.value,
      userId: userId.value,
      deviceId: deviceIdValue.value,
      dimensionId: dimensionId.value,
      spaces: spaces.value,
    }))
  }

  function applyAuth(data) {
    token.value = data.token
    userId.value = data.user_id
    deviceIdValue.value = data.device_id
    dimensionId.value = data.dimension_id
    remember({ id: data.dimension_id, kind: 'personal', name: 'личное' })
  }

  function remember(view) {
    const item = {
      id: view.id,
      kind: view.kind,
      name: view.name || (view.kind === 'personal' ? 'личное' : view.id),
    }
    const index = spaces.value.findIndex((space) => space.id === item.id)
    if (index >= 0) spaces.value[index] = item
    else spaces.value.push(item)
    persist()
  }

  async function register(email, password) {
    const known = localStorage.getItem(deviceKey(email)) || ''
    const data = await api('/register', {
      method: 'POST',
      body: { email, password, device_id: known },
    })
    localStorage.setItem(deviceKey(email), data.device_id)
    applyAuth(data)
  }

  async function login(email, password) {
    const known = localStorage.getItem(deviceKey(email)) || ''
    const data = await api('/login', {
      method: 'POST',
      body: { email, password, device_id: known },
    })
    localStorage.setItem(deviceKey(email), data.device_id)
    applyAuth(data)
  }

  async function logout() {
    if (token.value) {
      try {
        await api('/logout', { method: 'POST', token: token.value })
      } catch {
        // Локальная сессия сбрасывается и при недоступном сервере.
      }
    }
    token.value = ''
    userId.value = ''
    dimensionId.value = ''
    spaces.value = []
    sessionStorage.removeItem(sessionKey)
  }

  return {
    token, userId, deviceId: deviceIdValue, dimensionId, spaces,
    remember, register, login, logout,
  }
})
