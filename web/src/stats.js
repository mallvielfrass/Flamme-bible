export function statShown(value) {
  if (value == null || value === '') return '—'
  return String(value)
}

export function statText(label, value) {
  return `${label}: ${statShown(value)}`
}

export function isZeroStat(value) {
  return value === 0 || value === '0'
}

export function communityStats(row) {
  return [
    { icon: 'people', label: 'Участники', value: row?.members },
    { icon: 'shield', label: 'Админы', value: row?.admins },
    { icon: 'books', label: 'Сборники', value: row?.songbooks },
    { icon: 'calendar', label: 'Планы', value: row?.plans },
  ]
}

export function userStats(row) {
  return [
    { icon: 'people', label: 'Общины', value: row?.communities },
    { icon: 'laptop', label: 'Устройства', value: row?.devices },
  ]
}
