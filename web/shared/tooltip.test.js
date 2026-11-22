import { describe, expect, it } from 'vitest'
import { placeTooltip } from './tooltip.js'

const gap = 6
const tip = { width: 80, height: 24 }
const viewport = { width: 800, height: 600 }

function box(left, top, width = 32, height = 32) {
  return { left, top, width, height, right: left + width, bottom: top + height }
}

describe('placeTooltip', () => {
  it('centers the tip above the trigger when there is room', () => {
    expect(placeTooltip(box(100, 80), tip, viewport, gap)).toEqual({ top: 50, left: 76 })
  })

  it('flips below when there is no room above', () => {
    expect(placeTooltip(box(100, 20), tip, viewport, gap)).toEqual({ top: 58, left: 76 })
  })

  it('keeps the tip inside the viewport when it does not fit above or below', () => {
    const tall = { width: 80, height: 500 }
    expect(placeTooltip(box(100, 80), tall, viewport, gap)).toEqual({ top: 94, left: 76 })
  })

  it('clamps horizontally at the viewport edges', () => {
    expect(placeTooltip(box(0, 80), tip, viewport, gap).left).toBe(6)
    expect(placeTooltip(box(780, 80), tip, viewport, gap).left).toBe(714)
  })
})
