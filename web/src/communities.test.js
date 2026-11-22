import { describe, expect, it } from 'vitest'
import { canManageCommunity, communityCards, memberCountLabel, roleLabel } from './communities'

const personal = { id: 'dim', kind: 'personal', name: 'Личное', role: 'admin', memberCount: 1 }
const house = { id: 'com', kind: 'community', name: 'Преображение', role: 'admin', memberCount: 2 }
const second = { id: 'other', kind: 'community', name: '345432', role: 'member', memberCount: 4 }

describe('communityCards', () => {
  it('keeps only communities and ignores an empty list', () => {
    expect(communityCards([personal, house, second])).toEqual([house, second])
    expect(communityCards(null)).toEqual([])
  })
})

describe('canManageCommunity', () => {
  it('allows a community admin or a site admin, but not a regular member', () => {
    expect(canManageCommunity(house, 'user')).toBe(true)
    expect(canManageCommunity(second, 'admin')).toBe(true)
    expect(canManageCommunity(second, 'user')).toBe(false)
    expect(canManageCommunity(personal, 'admin')).toBe(false)
    expect(canManageCommunity(null, 'admin')).toBe(false)
  })
})

describe('labels', () => {
  it('translates roles and declines the member count', () => {
    expect(roleLabel('admin')).toBe('Администратор')
    expect(roleLabel('member')).toBe('Участник')
    expect(roleLabel('')).toBe('')
    expect(memberCountLabel(1)).toBe('1 участник')
    expect(memberCountLabel(2)).toBe('2 участника')
    expect(memberCountLabel(5)).toBe('5 участников')
    expect(memberCountLabel(21)).toBe('21 участник')
    expect(memberCountLabel(14)).toBe('14 участников')
    expect(memberCountLabel(0)).toBe('0 участников')
  })
})
