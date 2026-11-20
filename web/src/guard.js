export function homePath(session) {
  return session?.token ? '/songbooks' : '/catalog'
}

export function redirectFor(to, session) {
  const loggedIn = Boolean(session?.token)
  if (to.meta?.admin) {
    if (!loggedIn) return { path: '/login', query: { next: to.fullPath } }
    if (session.role !== 'admin') return { path: '/songbooks' }
  }
  if (!to.meta?.public && !loggedIn) {
    return { path: '/login', query: { next: to.fullPath } }
  }
  if (loggedIn && (to.path === '/login' || to.path === '/register')) {
    return { path: '/songbooks' }
  }
}
