<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const tab = computed(() => String(route.params.tab || 'songbooks'))
</script>

<template>
  <p class="empty">Раздел «{{ tab }}».</p>
</template>
