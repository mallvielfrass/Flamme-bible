<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useSessionStore } from '../stores/session'
import SongSearch from '../components/SongSearch.vue'
import SongbookManager from '../components/SongbookManager.vue'
import ModuleManager from '../components/ModuleManager.vue'
import PublicCatalog from '../components/PublicCatalog.vue'
import SpaceFilter from '../components/SpaceFilter.vue'

const route = useRoute()
const session = useSessionStore()
const tab = ref(route.meta.catalog ? 'catalog' : 'search')
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

const tabs = computed(() => {
  const all = [
    { id: 'search', label: 'Поиск' },
    { id: 'books', label: 'Мои сборники' },
    { id: 'modules', label: 'Модули Библии' },
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
  <h1>Сборники и поиск</h1>
  <SpaceFilter
    v-if="session.token"
    :model-value="bookSpace"
    :communities="communities"
    :allow-all="false"
    :allow-all-communities="false"
    :preferred="session.communityId"
    @update:model-value="onBookSpace"
  />
  <div v-if="bookSpace === 'community'" class="empty-state">
    <p>Вы пока не состоите в общине.</p>
    <router-link to="/communities">Создать или вступить в общину</router-link>
  </div>
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
  <ModuleManager v-else-if="tab === 'modules' && bookSpace !== 'community'" :dimension-id="session.currentId" />
  <PublicCatalog v-else-if="tab === 'catalog'" :guest="!session.token" @switch="tab = $event" />
</template>
