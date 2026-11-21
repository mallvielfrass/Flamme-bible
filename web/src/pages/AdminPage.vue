<script setup>
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSessionStore } from '../stores/session'

const session = useSessionStore()
const route = useRoute()
const router = useRouter()

const tabs = [
  { id: 'songbooks', label: 'Сборники', staff: true },
  { id: 'releases', label: 'Релизы', admin: true },
  { id: 'users', label: 'Пользователи', admin: true },
  { id: 'communities', label: 'Общины', admin: true },
  { id: 'audit', label: 'Журнал', staff: true },
]

const tab = computed(() => String(route.params.tab || ''))
const isAdmin = computed(() => session.role === 'admin')
const visibleTabs = computed(() => tabs.filter((item) => item.staff || isAdmin.value))

const summaryLine = computed(() => {
  const summary = session.adminSummary
  if (!summary) return ''
  const version = summary.active_version || 'нет'
  const pending = Number(summary.pending_songbooks || 0) + Number(summary.pending_modules || 0)
  return `Клиент у людей: ${version} · на проверке: ${pending}`
})

function countFor(item) {
  const summary = session.adminSummary
  if (!summary) return null
  if (item.id === 'songbooks') {
    return Number(summary.pending_songbooks || 0) + Number(summary.pending_modules || 0)
  }
  if (item.id === 'releases') return Number(summary.draft_releases || 0)
  return null
}

function toneFor(item) {
  return item.id === 'songbooks' ? 'gold' : 'muted'
}

watch(tab, (value) => {
  const known = visibleTabs.value.some((item) => item.id === value)
  if (!known) router.replace('/admin/songbooks')
}, { immediate: true })
</script>

<template>
  <div class="admin-head">
    <h1>Админ-панель</h1>
    <p v-if="summaryLine" class="hint admin-summary">{{ summaryLine }}</p>
  </div>
  <nav class="admin-tabs" aria-label="Разделы админки">
    <router-link
      v-for="item in visibleTabs"
      :key="item.id"
      class="admin-tab"
      :class="{ on: tab === item.id }"
      :to="`/admin/${item.id}`"
    >
      {{ item.label }}
      <span v-if="countFor(item)" class="tab-count" :class="toneFor(item)">{{ countFor(item) }}</span>
    </router-link>
  </nav>
  <router-view />
</template>
