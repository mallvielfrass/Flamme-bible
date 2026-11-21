<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AuditTab from '../components/admin/AuditTab.vue'
import CommunitiesTab from '../components/admin/CommunitiesTab.vue'
import ReleasesTab from '../components/admin/ReleasesTab.vue'
import SubmissionsTab from '../components/admin/SubmissionsTab.vue'
import UsersTab from '../components/admin/UsersTab.vue'

const route = useRoute()
const tab = computed(() => String(route.params.tab || 'songbooks'))
</script>

<template>
  <SubmissionsTab v-if="tab === 'songbooks'" />
  <ReleasesTab v-else-if="tab === 'releases'" />
  <UsersTab v-else-if="tab === 'users'" />
  <CommunitiesTab v-else-if="tab === 'communities'" />
  <AuditTab v-else-if="tab === 'audit'" />
  <p v-else class="empty">Неизвестный раздел.</p>
</template>
