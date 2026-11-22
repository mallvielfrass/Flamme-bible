import { describe, expect, it } from 'vitest'
import { MAX_IMAGES, dataUrlBytes, fitSize, imageFiles } from '../shared/ai/images.js'

describe('ai images', () => {
  it('fits the long side into 1600 px', () => {
    expect(fitSize(4000, 3000)).toEqual({ width: 1600, height: 1200 })
    expect(fitSize(1000, 3000)).toEqual({ width: 533, height: 1600 })
    expect(fitSize(800, 600)).toEqual({ width: 800, height: 600 })
  })

  it('counts base64 bytes', () => {
    expect(dataUrlBytes('data:image/jpeg;base64,AAAA')).toBe(3)
    expect(dataUrlBytes('data:image/jpeg;base64,AAA=')).toBe(2)
    expect(dataUrlBytes('data:image/jpeg;base64,AA==')).toBe(1)
    expect(dataUrlBytes('нет запятой')).toBe(0)
  })

  it('keeps only images and free slots', () => {
    const files = [
      { name: 'a.jpg', type: 'image/jpeg' },
      { name: 'b.pdf', type: 'application/pdf' },
      { name: 'c.png', type: 'image/png' },
      { name: 'd.webp', type: 'image/webp' },
    ]
    expect(imageFiles(files).map((f) => f.name)).toEqual(['a.jpg', 'c.png', 'd.webp'])
    expect(imageFiles(files, MAX_IMAGES - 1).map((f) => f.name)).toEqual(['a.jpg'])
    expect(imageFiles(files, MAX_IMAGES)).toEqual([])
    expect(imageFiles(null)).toEqual([])
  })
})
