<script setup>
import SpaceFilter from '../../shared/components/SpaceFilter.vue'

// Переключатель «Личное / община» для своих сборников и модулей (см. bookSpace.js).
defineProps({
  space: { type: Object, required: true },
  preferred: { type: String, default: '' },
})
</script>

<template>
  <SpaceFilter
    :model-value="space.bookSpace.value"
    :communities="space.communities.value"
    :allow-all="false"
    :allow-all-communities="false"
    :preferred="preferred"
    @update:model-value="space.onBookSpace"
  />
  <div v-if="space.bookSpace.value === 'community'" class="empty-state">
    <p>Вы пока не состоите в общине.</p>
    <router-link to="/communities">Создать или вступить в общину</router-link>
  </div>
</template>
