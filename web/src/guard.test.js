import { describe, expect, it } from 'vitest'
import { homePath, redirectFor } from './guard'

const guest = {}
const user = { token: 't', role: 'user' }
const admin = { token: 't', role: 'admin' }

function to(path, meta = {}) {
  return { path, fullPath: path, meta }
}

describe('navigation guard', () => {
  it('sends a guest home to the public catalog', () => {
    expect(homePath(guest)).toBe('/catalog')
    expect(homePath(user)).toBe('/songbooks')
  })

  it('leaves the catalog and the download page open', () => {
    expect(redirectFor(to('/catalog', { public: true }), guest)).toBeUndefined()
    expect(redirectFor(to('/download', { public: true }), guest)).toBeUndefined()
  })

  it('asks a guest to log in before plans', () => {
    expect(redirectFor(to('/plans'), guest)).toEqual({ path: '/login', query: { next: '/plans' } })
  })

  it('hides the admin panel from a regular user', () => {
    expect(redirectFor(to('/admin', { admin: true }), guest)).toEqual({
      path: '/login',
      query: { next: '/admin' },
    })
    expect(redirectFor(to('/admin', { admin: true }), user)).toEqual({ path: '/songbooks' })
    expect(redirectFor(to('/admin', { admin: true }), admin)).toBeUndefined()
  })

  it('does not show the login form to someone who is already in', () => {
    expect(redirectFor(to('/login', { public: true }), user)).toEqual({ path: '/songbooks' })
  })
})
