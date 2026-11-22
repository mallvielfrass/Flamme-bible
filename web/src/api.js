import { ApiError, explain } from '../shared/http.js'

export { ApiError, explain }

let apiBase = ''

export function setApiBase(url) {
  apiBase = String(url || '').replace(/\/$/, '')
}

export function apiURL(path) {
  if (!apiBase || typeof path !== 'string' || !path.startsWith('/')) return path
  return apiBase + path
}

async function send(path, { method = 'GET', token, headers, body } = {}) {
  let response
  try {
    response = await fetch(apiURL(path), { method, headers, body })
  } catch {
    throw new ApiError(0, 'Нет связи с сервером.')
  }
  return response
}

async function readError(response) {
  const data = await response.json().catch(() => ({}))
  throw new ApiError(response.status, data.error || 'ошибка', data)
}

function authHeaders(token, extra) {
  const headers = { ...extra }
  if (token) headers.Authorization = `Bearer ${token}`
  return headers
}

export async function api(path, { method = 'GET', body, token } = {}) {
  const headers = authHeaders(token)
  let payload
  if (body instanceof Blob) {
    payload = body
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    payload = JSON.stringify(body)
  }
  const response = await send(path, { method, headers, body: payload })
  const data = await response.json().catch(() => ({}))
  if (!response.ok) throw new ApiError(response.status, data.error || 'ошибка', data)
  return data
}

export function apiForm(path, { method = 'POST', form, token, onProgress } = {}) {
  if (typeof onProgress === 'function') return sendForm(path, { method, form, token, onProgress })
  return send(path, { method, headers: authHeaders(token), body: form }).then(async (response) => {
    if (!response.ok) await readError(response)
    return response.json().catch(() => ({}))
  })
}

function sendForm(path, { method, form, token, onProgress }) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open(method, apiURL(path))
    if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) onProgress(event.loaded, event.total)
    }
    xhr.onerror = () => reject(new ApiError(0, 'Нет связи с сервером.'))
    xhr.onload = () => {
      let data = {}
      try {
        data = JSON.parse(xhr.responseText || '{}')
      } catch {
        data = {}
      }
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(new ApiError(xhr.status, data.error || 'ошибка', data))
        return
      }
      resolve(data)
    }
    xhr.send(form)
  })
}

export async function apiBlob(path, { token } = {}) {
  const response = await send(path, { headers: authHeaders(token) })
  if (!response.ok) await readError(response)
  return {
    blob: await response.blob(),
    type: response.headers.get('content-type') || '',
  }
}
