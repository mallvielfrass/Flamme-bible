<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import SubmissionsTab from '../components/admin/SubmissionsTab.vue'

const route = useRoute()
const tab = computed(() => String(route.params.tab || 'songbooks'))
</script>

<template>
  <SubmissionsTab v-if="tab === 'songbooks'" />
  <p v-else class="empty">Раздел «{{ tab }}».</p>
</template>
