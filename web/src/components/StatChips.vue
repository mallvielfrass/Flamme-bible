<script setup>
import { isZeroStat, statShown, statText } from '../stats'
import Emoji from './Emoji.vue'

defineProps({
  items: { type: Array, default: () => [] },
})
</script>

<template>
  <span class="stat-chips">
    <span
      v-for="item in items"
      :key="item.label"
      class="stat-chip"
      :class="{ zero: isZeroStat(item.value) }"
      role="img"
      :title="statText(item.label, item.value)"
      :aria-label="statText(item.label, item.value)"
    >
      <Emoji :name="item.icon" />
      <span class="stat-num">{{ statShown(item.value) }}</span>
    </span>
  </span>
</template>
