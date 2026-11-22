<script setup>
import { ref } from 'vue'
import IconButton from './IconButton.vue'

// Поле пароля с кнопкой «показать». Остальные атрибуты (name, autocomplete)
// уходят во внутренний input.
defineOptions({ inheritAttrs: false })
const value = defineModel({ type: String, default: '' })
const shown = ref(false)
</script>

<template>
  <span class="password-input">
    <input v-model="value" v-bind="$attrs" :type="shown ? 'text' : 'password'" />
    <IconButton
      class="password-toggle"
      :icon="shown ? 'eyeOff' : 'eye'"
      :label="shown ? 'Скрыть пароль' : 'Показать пароль'"
      small
      :aria-pressed="shown"
      @click="shown = !shown"
    />
  </span>
</template>

<style scoped>
.password-input {
  position: relative;
  display: block;
}
.password-input input {
  width: 100%;
  box-sizing: border-box;
  padding-right: 2.5rem;
}
.password-toggle {
  position: absolute;
  top: 50%;
  right: 0.35rem;
  transform: translateY(-50%);
}
</style>
