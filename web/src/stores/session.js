import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
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

export function toSpace(view) {
  const personal = view.kind === 'personal'
  return {
    id: view.id,
    kind: view.kind || 'community',
    name: personal ? 'Личное' : (view.name || 'Община'),
    address: view.address || '',
    email: view.email || '',
    confession: view.confession || '',
    description: view.description || '',
  }
}

export const useSessionStore = defineStore('session', () => {
  const saved = readSession()
  const token = ref(saved?.token || '')
  const userId = ref(saved?.userId || '')
  const email = ref(saved?.email || '')
  const role = ref(saved?.role || '')
  const deviceIdValue = ref(saved?.deviceId || '')
  const dimensionId = ref(saved?.dimensionId || '')
  const currentId = ref(saved?.currentId || saved?.dimensionId || '')
  const communityId = ref(saved?.communityId || '')
  const spaces = ref(Array.isArray(saved?.spaces) ? saved.spaces : [])

  const current = computed(() => spaces.value.find((space) => space.id === currentId.value) || null)

  function persist() {
    sessionStorage.setItem(sessionKey, JSON.stringify({
      token: token.value,
      userId: userId.value,
      email: email.value,
      role: role.value,
      deviceId: deviceIdValue.value,
      dimensionId: dimensionId.value,
      currentId: currentId.value,
      communityId: communityId.value,
      spaces: spaces.value,
    }))
  }

  function ensureCurrent() {
    if (!spaces.value.some((space) => space.id === currentId.value)) {
      const personal = spaces.value.find((space) => space.kind === 'personal')
      currentId.value = personal?.id || spaces.value[0]?.id || dimensionId.value || ''
    }
    const chosen = spaces.value.find((space) => space.id === communityId.value && space.kind === 'community')
    if (chosen) return
    const current = spaces.value.find((space) => space.id === currentId.value)
    communityId.value = current?.kind === 'community'
      ? current.id
      : (spaces.value.find((space) => space.kind === 'community')?.id || '')
  }

  function remember(view, options = {}) {
    const item = toSpace(view)
    const index = spaces.value.findIndex((space) => space.id === item.id)
    if (index >= 0) spaces.value[index] = { ...spaces.value[index], ...item }
    else spaces.value.push(item)
    if (options.select !== false) {
      currentId.value = item.id
      if (item.kind === 'community') communityId.value = item.id
    }
    persist()
  }

  function select(id) {
    if (!spaces.value.some((space) => space.id === id)) return
    currentId.value = id
    const space = spaces.value.find((item) => item.id === id)
    if (space?.kind === 'community') communityId.value = id
    persist()
  }

  function applyAuth(data, mail) {
    token.value = data.token
    userId.value = data.user_id
    deviceIdValue.value = data.device_id
    dimensionId.value = data.dimension_id
    role.value = data.role || role.value || 'user'
    if (mail) email.value = mail
    currentId.value = data.dimension_id
    remember({ id: data.dimension_id, kind: 'personal', name: 'Личное' })
  }

  async function bootstrap() {
    if (!token.value) return
    try {
      const me = await api('/api/me', { token: token.value })
      userId.value = me.user_id || userId.value
      email.value = me.email || email.value
      role.value = me.role || role.value || 'user'
    } catch (err) {
      if (err.status === 401) {
        await logout()
        return
      }
    }
    try {
      const list = await api('/api/dimensions', { token: token.value })
      if (Array.isArray(list)) {
        spaces.value = list.map(toSpace)
        ensureCurrent()
      }
    } catch (err) {
      if (err.status === 401) {
        await logout()
        return
      }
    }
    persist()
  }

  async function register(mail, password) {
    const known = localStorage.getItem(deviceKey(mail)) || ''
    const data = await api('/register', {
      method: 'POST',
      body: { email: mail, password, device_id: known },
    })
    localStorage.setItem(deviceKey(mail), data.device_id)
    applyAuth(data, mail)
    await bootstrap()
  }

  async function login(mail, password) {
    const known = localStorage.getItem(deviceKey(mail)) || ''
    const data = await api('/login', {
      method: 'POST',
      body: { email: mail, password, device_id: known },
    })
    localStorage.setItem(deviceKey(mail), data.device_id)
    applyAuth(data, mail)
    await bootstrap()
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
    email.value = ''
    role.value = ''
    dimensionId.value = ''
    currentId.value = ''
    communityId.value = ''
    spaces.value = []
    sessionStorage.removeItem(sessionKey)
  }

  async function acceptInvite(inviteToken) {
    const data = await api(`/api/invites/${encodeURIComponent(inviteToken)}/accept`, {
      method: 'POST',
      token: token.value,
    })
    const view = await api(`/api/dimensions/${data.dimension_id}`, { token: token.value })
    remember(view)
    return view
  }

  return {
    token, userId, email, role, deviceId: deviceIdValue, dimensionId, currentId, communityId, spaces, current,
    remember, select, register, login, logout, bootstrap, acceptInvite,
  }
})
