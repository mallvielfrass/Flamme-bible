<script setup>
import { computed, provide, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useSessionStore } from '../stores/session'
import SongSearch from '../components/SongSearch.vue'
import SongbookManager from '../components/SongbookManager.vue'
import PublicCatalog from '../components/PublicCatalog.vue'
import BookSpaceFilter from '../components/BookSpaceFilter.vue'
import { aiPlanKey } from '../../shared/ai/results.js'
import { sitePlanTarget } from '../sitePlan.js'
import { useBookSpace } from '../bookSpace.js'

const route = useRoute()
const session = useSessionStore()
provide(aiPlanKey, sitePlanTarget(session))
const tab = ref(route.meta.catalog ? 'catalog' : 'search')
const space = useBookSpace(session)
const bookSpace = space.bookSpace

const tabs = computed(() => {
  const all = [
    { id: 'search', label: 'Поиск' },
    { id: 'books', label: 'Мои сборники' },
    { id: 'catalog', label: 'Общий каталог' },
  ]
  return session.token ? all : all.filter((item) => item.id === 'catalog')
})

watch(() => route.fullPath, () => {
  if (!session.token) tab.value = 'catalog'
  else if (route.meta.catalog) tab.value = 'catalog'
})
</script>

<template>
  <h1>Сборники</h1>
  <BookSpaceFilter v-if="session.token" :space="space" :preferred="session.communityId" />
  <div class="tabs" role="tablist">
    <button
      v-for="item in tabs"
      :key="item.id"
      type="button"
      role="tab"
      class="tab"
      :class="{ on: tab === item.id }"
      :aria-selected="tab === item.id"
      @click="tab = item.id"
    >{{ item.label }}</button>
  </div>
  <SongSearch v-if="tab === 'search'" :token="session.token" />
  <SongbookManager v-else-if="tab === 'books' && bookSpace !== 'community'" :dimension-id="session.currentId" />
  <PublicCatalog v-else-if="tab === 'catalog'" :guest="!session.token" :show="session.token ? 'songbooks' : 'all'" @switch="tab = $event" />
</template>
