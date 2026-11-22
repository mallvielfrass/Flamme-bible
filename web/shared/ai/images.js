// Картинки к вопросу в чате ИИ. Перед отправкой фото уменьшается до 1600 px
// по длинной стороне и пересжимается в JPEG: так текст ещё читается моделью,
// а запрос остаётся в пределах сервера (до 4 картинок по 2 МБ).

export const MAX_IMAGES = 4
export const MAX_IMAGE_BYTES = 2 * 1024 * 1024
export const MAX_SIDE = 1600

/**
 * Размер после уменьшения: длинная сторона не больше max, пропорции те же.
 * @param {number} width
 * @param {number} height
 * @param {number} [max]
 */
export function fitSize(width, height, max = MAX_SIDE) {
  const w = Math.max(1, Math.round(width))
  const h = Math.max(1, Math.round(height))
  const scale = Math.min(1, max / Math.max(w, h))
  return { width: Math.max(1, Math.round(w * scale)), height: Math.max(1, Math.round(h * scale)) }
}

/** Байтов в base64 из data: URL. */
export function dataUrlBytes(url) {
  const comma = String(url).indexOf(',')
  if (comma < 0) return 0
  const body = String(url).slice(comma + 1)
  const pad = body.endsWith('==') ? 2 : body.endsWith('=') ? 1 : 0
  return Math.floor((body.length * 3) / 4) - pad
}

/**
 * Файлы-картинки из выбора, вставки или перетаскивания, не больше свободных мест.
 * @param {Iterable<File> | ArrayLike<File> | null | undefined} files
 * @param {number} taken сколько уже приложено
 */
export function imageFiles(files, taken = 0) {
  const list = Array.from(files || []).filter((file) => file && String(file.type).startsWith('image/'))
  return list.slice(0, Math.max(0, MAX_IMAGES - taken))
}

async function decode(file) {
  if (typeof createImageBitmap === 'function') {
    try {
      return await createImageBitmap(file, { imageOrientation: 'from-image' })
    } catch {
      // Старый WebKit не знает опций: пробуем через <img>.
    }
  }
  const url = await new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result)
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('не картинка'))
    img.src = url
  })
}

/**
 * Уменьшает и пересжимает картинку в JPEG data: URL. Если всё равно больше
 * MAX_IMAGE_BYTES, ужимает сильнее.
 * @param {File} file
 * @returns {Promise<{ url: string, name: string }>}
 */
export async function prepareImage(file) {
  let source
  try {
    source = await decode(file)
  } catch {
    throw new Error(`Не получилось открыть «${file.name || 'изображение'}»`)
  }
  let side = MAX_SIDE
  for (const quality of [0.85, 0.75, 0.6]) {
    const size = fitSize(source.width, source.height, side)
    const canvas = document.createElement('canvas')
    canvas.width = size.width
    canvas.height = size.height
    const ctx = canvas.getContext('2d')
    ctx.fillStyle = '#fff'
    ctx.fillRect(0, 0, size.width, size.height)
    ctx.drawImage(source, 0, 0, size.width, size.height)
    const url = canvas.toDataURL('image/jpeg', quality)
    if (dataUrlBytes(url) <= MAX_IMAGE_BYTES) {
      if (typeof source.close === 'function') source.close()
      return { url, name: file.name || 'изображение' }
    }
    side = Math.round(side * 0.8)
  }
  if (typeof source.close === 'function') source.close()
  throw new Error(`«${file.name || 'изображение'}» слишком большое`)
}
