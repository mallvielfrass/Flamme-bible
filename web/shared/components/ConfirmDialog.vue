<script setup>
import { ref } from 'vue'
import AppModal from './AppModal.vue'

const open = ref(false)
const title = ref('')
const text = ref('')
const action = ref('Удалить')
const danger = ref(true)
const triple = ref(false)
let settle = null

function confirm(options = {}) {
  title.value = options.title || 'Подтвердите'
  text.value = options.text || ''
  action.value = options.action || 'Удалить'
  danger.value = options.danger !== false
  triple.value = Boolean(options.triple)
  open.value = true
  return new Promise((resolve) => {
    settle = resolve
  })
}

function finish(value) {
  open.value = false
  const done = settle
  settle = null
  done?.(value)
}

defineExpose({ confirm })
</script>

<template>
  <AppModal v-if="open" :title="title" @close="finish(false)">
    <p v-if="text" class="confirm-text">{{ text }}</p>
    <template #footer>
      <template v-if="triple">
        <button class="btn slim primary" type="button" @click="finish('save')">Сохранить</button>
        <button class="btn slim" type="button" @click="finish('discard')">Не сохранять</button>
        <button class="btn slim" type="button" data-autofocus @click="finish(false)">Отмена</button>
      </template>
      <template v-else>
        <button class="btn slim" type="button" data-autofocus @click="finish(false)">Отмена</button>
        <button class="btn slim" :class="{ danger }" type="button" @click="finish(true)">{{ action }}</button>
      </template>
    </template>
  </AppModal>
</template>
