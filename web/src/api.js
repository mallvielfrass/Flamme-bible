export class ApiError extends Error {
  constructor(status, message, body) {
    super(message)
    this.status = status
    this.body = body || {}
  }
}

const known = {
  запрос: 'Проверьте заполненные поля.',
  запрещено: 'Это действие доступно администратору.',
  'не найдено': 'Не найдено. Проверьте ссылку или код приглашения.',
  'не авторизован': 'Нужно войти снова.',
  'уже участник': 'Вы уже в этой общине.',
  конфликт: 'Данные уже изменились. Обновите страницу и повторите.',
  email: 'Такая почта уже зарегистрирована.',
  сборник: 'Такой сборник уже есть.',
  община: 'Приглашение можно создать только для общины.',
  цикл: 'Сборник не может ссылаться сам на себя.',
  глубина: 'Слишком длинная цепочка сборников.',
  operation_id: 'Эта операция уже отправлена с другими данными.',
  reload: 'Сервер просит начать сверку заново.',
  ошибка: 'Не получилось выполнить действие.',
}

export function explain(error) {
  const msg = typeof error === 'string' ? error : (error?.message || '')
  if (!msg) return 'Не получилось выполнить действие.'
  return known[msg] || msg
}

async function send(path, { method = 'GET', token, headers, body } = {}) {
  let response
  try {
    response = await fetch(path, { method, headers, body })
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
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    payload = JSON.stringify(body)
  }
  const response = await send(path, { method, headers, body: payload })
  const data = await response.json().catch(() => ({}))
  if (!response.ok) throw new ApiError(response.status, data.error || 'ошибка', data)
  return data
}

export async function apiForm(path, { method = 'POST', form, token } = {}) {
  const response = await send(path, { method, headers: authHeaders(token), body: form })
  if (!response.ok) await readError(response)
  return response.json().catch(() => ({}))
}

export async function apiBlob(path, { token } = {}) {
  const response = await send(path, { headers: authHeaders(token) })
  if (!response.ok) await readError(response)
  return {
    blob: await response.blob(),
    type: response.headers.get('content-type') || '',
  }
}
