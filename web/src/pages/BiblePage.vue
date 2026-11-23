<script setup>
import { onMounted, provide, ref } from 'vue'
import { useSessionStore } from '../stores/session'
import { api, explain } from '../api'
import { availableTranslations, loadTranslations, maxTranslations, saveTranslations, toggleTranslation } from '../bibleReader.js'
import { aiPlanKey } from '../../shared/ai/results.js'
import { sitePlanTarget } from '../sitePlan.js'
import { useBookSpace } from '../bookSpace.js'
import BibleFinder from '../components/BibleFinder.vue'
import BibleReader from '../components/BibleReader.vue'
import BookSpaceFilter from '../components/BookSpaceFilter.vue'
import ModuleManager from '../components/ModuleManager.vue'
import PublicCatalog from '../components/PublicCatalog.vue'

// Страница «Библия»: чтение и поиск по одобренным переводам, свои модули, каталог.
const session = useSessionStore()
provide(aiPlanKey, sitePlanTarget(session))
const space = useBookSpace(session)

const tab = ref('read')
const tabs = [
  { id: 'read', label: 'Чтение и поиск' },
  { id: 'modules', label: 'Мои модули' },
  { id: 'catalog', label: 'Общий каталог' },
]
const known = ref([])
const chosen = ref(loadTranslations(localStorage))
const error = ref('')
const loading = ref(true)
const reader = ref(null)

onMounted(async () => {
  try {
    const data = await api('/api/bible/translations', { token: session.token })
    known.value = data.translations || []
    chosen.value = saveTranslations(localStorage, availableTranslations(chosen.value, known.value))
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
})

function toggle(name) {
  const next = toggleTranslation(chosen.value, name)
  if (!next.length) return
  chosen.value = saveTranslations(localStorage, next)
}

function openPlace(place) {
  reader.value?.open(place)
  reader.value?.$el?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
}
</script>

<template>
  <h1>Библия</h1>
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
  <template v-if="tab === 'read'">
    <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
    <p v-else-if="loading" class="empty" role="status">Загрузка…</p>
    <div v-else-if="!known.length" class="empty-state">
      <p>На сервере пока нет одобренных переводов.</p>
      <button class="btn slim primary" type="button" @click="tab = 'modules'">Загрузить модуль</button>
    </div>
    <template v-else>
      <div class="bible-translations" role="group" aria-label="Переводы">
        <span class="hint">Переводы:</span>
        <button
          v-for="item in known"
          :key="item.short_name"
          type="button"
          class="chip"
          :class="{ on: chosen.includes(item.short_name) }"
          :aria-pressed="chosen.includes(item.short_name)"
          :title="item.title"
          :disabled="!chosen.includes(item.short_name) && chosen.length >= maxTranslations"
          @click="toggle(item.short_name)"
        >{{ item.short_name }}<small v-if="chosen[0] === item.short_name"> · основной</small></button>
      </div>
      <p class="hint">До {{ maxTranslations }} переводов рядом. Книги и поиск — по основному, это первый выбранный.</p>
      <BibleFinder :token="session.token" :translations="chosen" @open="openPlace" />
      <BibleReader ref="reader" :token="session.token" :translations="chosen" />
    </template>
  </template>
  <template v-else-if="tab === 'modules'">
    <BookSpaceFilter :space="space" :preferred="session.communityId" />
    <ModuleManager v-if="space.bookSpace.value !== 'community'" :dimension-id="session.currentId" />
  </template>
  <PublicCatalog v-else show="modules" @switch="tab = $event" />
</template>
