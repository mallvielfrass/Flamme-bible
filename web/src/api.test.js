import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, apiForm, ApiError, explain } from './api'

function json(data, status = 200) {
  return {
    ok: status < 400,
    status,
    json: async () => data,
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('api', () => {
  it('sends json and the bearer token', async () => {
    let seen
    vi.stubGlobal('fetch', async (path, init) => {
      seen = { path, init }
      return json({ ok: true })
    })
    const data = await api('/api/plans', {
      method: 'POST',
      token: 'abc',
      body: { title: 'Воскресенье', items: [] },
    })
    expect(data.ok).toBe(true)
    expect(seen.path).toBe('/api/plans')
    expect(seen.init.headers.Authorization).toBe('Bearer abc')
    expect(seen.init.headers['Content-Type']).toBe('application/json')
    expect(JSON.parse(seen.init.body)).toEqual({ title: 'Воскресенье', items: [] })
  })

  it('keeps the conflict body', async () => {
    vi.stubGlobal('fetch', async () => json({ error: 'конфликт', plan: { id: '1', version: 2 } }, 409))
    const err = await api('/api/plans/1', { method: 'PUT', body: { version: 1, items: [] } }).catch((error) => error)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(409)
    expect(err.body.plan.version).toBe(2)
    expect(explain(err)).toBe('Данные уже изменились. Обновите страницу и повторите.')
  })

  it('does not force a content type on a file form', async () => {
    let headers
    vi.stubGlobal('fetch', async (_path, init) => {
      headers = init.headers
      return json({ release: { version: '1.0.0' } }, 201)
    })
    await apiForm('/api/admin/releases', { form: new FormData(), token: 't' })
    expect(headers.Authorization).toBe('Bearer t')
    expect(headers['Content-Type']).toBeUndefined()
  })

  it('reports a lost connection', async () => {
    vi.stubGlobal('fetch', async () => {
      throw new TypeError('network')
    })
    await expect(api('/api/me')).rejects.toMatchObject({ status: 0, message: 'Нет связи с сервером.' })
  })
})
