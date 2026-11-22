import { api } from './api.js'
import { planTarget } from './planItems.js'

/** Куда сайт кладёт пункт «В план»: те же запросы, что у страницы планов. */
export function sitePlanTarget(session) {
  return planTarget({
    list: async () => {
      const list = await api('/api/plans?scope=all', { token: session.token })
      return Array.isArray(list) ? list : []
    },
    read: (id) => api(`/api/plans/${id}`, { token: session.token }),
    save: (plan) => api(`/api/plans/${plan.id}`, {
      method: 'PUT',
      token: session.token,
      body: {
        title: plan.title,
        version: plan.version,
        updated_at: plan.updated_at || undefined,
        items: plan.items,
      },
    }),
    places: async () => session.spaces
      .filter((space) => space.kind === 'community')
      .map((space) => ({ id: space.id, name: space.name || 'Община' })),
    defaultPlace: () => (session.current?.kind === 'community' ? session.current.id : ''),
    create: (title, items, dimensionId) => api('/api/plans', {
      method: 'POST',
      token: session.token,
      body: { title, dimension_id: dimensionId, items },
    }),
  })
}
