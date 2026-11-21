<script setup>
import { emojiSrc } from '../emoji'

defineProps({
  name: { type: String, required: true },
})
</script>

<template>
  <img v-if="emojiSrc[name]" class="emoji" :src="emojiSrc[name]" alt="" aria-hidden="true" />
</template>
