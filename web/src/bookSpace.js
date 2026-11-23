import { computed, ref } from 'vue'

/**
 * Куда сохранять свои сборники и модули Библии: личное пространство или община.
 * Общая логика переключателя SpaceFilter для страниц «Сборники» и «Библия».
 * bookSpace: 'personal', id общины или 'community', если общин ещё нет.
 */
export function useBookSpace(session) {
  const wantCommunity = ref(false)
  const communities = computed(() => session.spaces.filter((space) => space.kind === 'community'))
  const bookSpace = computed(() => {
    if (wantCommunity.value && !communities.value.length) return 'community'
    return session.current?.kind === 'community' ? session.currentId : 'personal'
  })

  function onBookSpace(value) {
    if (value === 'community') {
      wantCommunity.value = true
      return
    }
    wantCommunity.value = false
    if (value === 'personal') {
      const personal = session.spaces.find((space) => space.kind === 'personal')
      if (personal) session.select(personal.id)
      return
    }
    session.select(value)
  }

  return { communities, bookSpace, onBookSpace }
}
