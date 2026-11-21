<script setup>
import { icons } from './icons'

defineProps({
  icon: { type: String, required: true },
  label: { type: String, required: true },
  tone: { type: String, default: '' },
  round: { type: Boolean, default: false },
  small: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  dirty: { type: Boolean, default: false },
  busy: { type: Boolean, default: false },
})
</script>

<template>
  <button
    class="icon-btn"
    :class="[tone, { round, small, dirty }]"
    type="button"
    :aria-label="label"
    :data-tip="label"
    :disabled="disabled || busy"
  >
    <span v-if="busy" class="spin" aria-hidden="true"></span>
    <svg v-else viewBox="0 0 24 24" aria-hidden="true">
      <path v-for="(d, index) in icons[icon] || []" :key="index" :d="d" />
    </svg>
    <span v-if="dirty" class="dirty-dot" aria-hidden="true"></span>
  </button>
</template>
