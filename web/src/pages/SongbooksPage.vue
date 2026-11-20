<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useSessionStore } from '../stores/session'
import SongSearch from '../components/SongSearch.vue'
import SongbookManager from '../components/SongbookManager.vue'
import ModuleManager from '../components/ModuleManager.vue'
import PublicCatalog from '../components/PublicCatalog.vue'

const route = useRoute()
const session = useSessionStore()
const tab = ref(route.meta.catalog ? 'catalog' : 'search')

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
  <SongbookManager v-else-if="tab === 'books'" :dimension-id="session.currentId" />
  <ModuleManager v-else-if="tab === 'modules'" :dimension-id="session.currentId" />
  <PublicCatalog v-else :guest="!session.token" @switch="tab = $event" />
</template>
