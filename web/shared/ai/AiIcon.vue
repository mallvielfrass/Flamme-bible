<script setup>
import { icons } from '../icons.js'

defineProps({
  name: { type: String, required: true },
})
</script>

<template>
  <svg class="ai-ic" viewBox="0 0 24 24" aria-hidden="true">
    <path v-for="(d, index) in icons[name] || []" :key="index" :d="d" />
  </svg>
</template>
