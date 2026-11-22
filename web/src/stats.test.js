import { describe, expect, it } from 'vitest'
import { emojiSrc } from './emoji'
import { communityStats, isZeroStat, statShown, statText, userStats } from './stats'

describe('stat chips', () => {
  it('labels a count and keeps an unknown count readable', () => {
    expect(statText('Участники', 1)).toBe('Участники: 1')
    expect(statText('Планы', 0)).toBe('Планы: 0')
    expect(statShown(null)).toBe('—')
    expect(statText('Админы', null)).toBe('Админы: —')
    expect(isZeroStat(0)).toBe(true)
    expect(isZeroStat(null)).toBe(false)
    expect(isZeroStat(1)).toBe(false)
  })

  it('builds the same counters the admin tables used to split into columns', () => {
    expect(communityStats({ members: 1, admins: 1, songbooks: 0, plans: 1 }).map((item) => item.icon))
      .toEqual(['people', 'shield', 'books', 'calendar'])
    expect(userStats({ communities: 2, devices: 0 }).map((item) => statText(item.label, item.value)))
      .toEqual(['Общины: 2', 'Устройства: 0'])
  })

  it('points every icon at a vendored svg', () => {
    expect(Object.keys(emojiSrc).sort()).toEqual(['books', 'calendar', 'laptop', 'people', 'shield'])
    for (const src of Object.values(emojiSrc)) {
      const value = String(src)
      expect(value.startsWith('data:image/svg+xml') || value.includes('.svg')).toBe(true)
    }
  })
})
