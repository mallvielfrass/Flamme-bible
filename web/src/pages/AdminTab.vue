<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import ReleasesTab from '../components/admin/ReleasesTab.vue'
import SubmissionsTab from '../components/admin/SubmissionsTab.vue'

const route = useRoute()
const tab = computed(() => String(route.params.tab || 'songbooks'))
</script>

<template>
  <SubmissionsTab v-if="tab === 'songbooks'" />
  <ReleasesTab v-else-if="tab === 'releases'" />
  <p v-else class="empty">Раздел «{{ tab }}».</p>
</template>
