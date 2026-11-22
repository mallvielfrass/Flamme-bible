/** Общины из списка пространств, без личного кабинета. */
export function communityCards(spaces) {
  return (spaces || []).filter((space) => space?.kind === 'community')
}

/** Приглашать и смотреть состав может админ общины или администратор сайта. */
export function canManageCommunity(space, siteRole) {
  return space?.kind === 'community' && (space.role === 'admin' || siteRole === 'admin')
}

export function roleLabel(role) {
  if (role === 'admin') return 'Администратор'
  if (role === 'member') return 'Участник'
  return role || ''
}

export function memberCountLabel(count) {
  const n = Number(count)
  const value = Number.isFinite(n) && n > 0 ? Math.trunc(n) : 0
  const mod100 = value % 100
  const mod10 = value % 10
  let word = 'участников'
  if (mod100 < 11 || mod100 > 14) {
    if (mod10 === 1) word = 'участник'
    else if (mod10 >= 2 && mod10 <= 4) word = 'участника'
  }
  return `${value} ${word}`
}
