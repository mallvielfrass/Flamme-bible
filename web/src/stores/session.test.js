import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useSessionStore } from './session'

function memory() {
  const mem = new Map()
  return {
    getItem: (key) => (mem.has(key) ? mem.get(key) : null),
    setItem: (key, value) => mem.set(key, String(value)),
    removeItem: (key) => mem.delete(key),
  }
}

function json(data, status = 200) {
  return { ok: status < 400, status, json: async () => data }
}

const auth = {
  token: 't',
  user_id: 'u',
  device_id: 'd',
  dimension_id: 'dim',
  role: 'admin',
}

beforeEach(() => {
  vi.stubGlobal('sessionStorage', memory())
  vi.stubGlobal('localStorage', memory())
  setActivePinia(createPinia())
})

describe('session', () => {
  it('keeps the role and the personal space after login', async () => {
    vi.stubGlobal('fetch', async (path) => {
      if (path === '/login') return json(auth)
      if (path === '/api/me') return json({ user_id: 'u', email: 'a@example.com', role: 'admin' })
      if (path === '/api/dimensions') {
        return json([{ id: 'dim', kind: 'personal', name: '', address: '', email: '', confession: '', description: '' }])
      }
      throw new Error(path)
    })
    const session = useSessionStore()
    await session.login('a@example.com', 'secret')
    expect(session.token).toBe('t')
    expect(session.role).toBe('admin')
    expect(session.email).toBe('a@example.com')
    expect(session.currentId).toBe('dim')
    expect(session.current.kind).toBe('personal')
    expect(session.current.name).toBe('Личное')
    const saved = JSON.parse(sessionStorage.getItem('flammebible.session'))
    expect(saved.role).toBe('admin')
    expect(localStorage.getItem('flammebible.device.a@example.com')).toBe('d')
  })

  it('clears the session when the profile is rejected', async () => {
    vi.stubGlobal('fetch', async (path) => {
      if (path === '/login') return json(auth)
      if (path === '/api/me') return json({ error: 'не авторизован' }, 401)
      if (path === '/logout') return json({})
      throw new Error(path)
    })
    const session = useSessionStore()
    await session.login('a@example.com', 'secret')
    expect(session.token).toBe('')
    expect(session.role).toBe('')
  })

  it('switches only to a known space and logs out locally if the server is down', async () => {
    vi.stubGlobal('fetch', async (path) => {
      if (path === '/login') return json({ ...auth, role: 'user' })
      if (path === '/api/me') return json({ user_id: 'u', email: 'a@example.com', role: 'user' })
      if (path === '/api/dimensions') {
        return json([
          { id: 'dim', kind: 'personal', name: '', address: '', email: '', confession: '', description: '', role: 'admin', member_count: 1 },
          { id: 'com', kind: 'community', name: 'Дом молитвы', address: '', email: '', confession: '', description: '', role: 'member', member_count: 3 },
        ])
      }
      throw new Error('down')
    })
    const session = useSessionStore()
    await session.login('a@example.com', 'secret')
    session.select('missing')
    expect(session.currentId).toBe('dim')
    session.select('com')
    expect(session.current.name).toBe('Дом молитвы')
    expect(session.current.role).toBe('member')
    expect(session.current.memberCount).toBe(3)
    await session.logout()
    expect(session.token).toBe('')
    expect(session.spaces).toEqual([])
    expect(sessionStorage.getItem('flammebible.session')).toBeNull()
  })
})
