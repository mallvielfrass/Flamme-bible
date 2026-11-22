<script setup>
import { onBeforeUnmount, onMounted, useId, ref } from 'vue'

defineProps({
  title: { type: String, default: '' },
  wide: { type: Boolean, default: false },
})

const emit = defineEmits(['close'])
const root = ref(null)
const titleId = useId()

function focusable() {
  if (!root.value) return []
  return [...root.value.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])')]
    .filter((el) => !el.disabled && el.getAttribute('aria-hidden') !== 'true')
}

function onKey(event) {
  if (event.key === 'Escape') {
    event.preventDefault()
    emit('close')
    return
  }
  if (event.key !== 'Tab') return
  const nodes = focusable()
  if (!nodes.length) {
    event.preventDefault()
    return
  }
  const first = nodes[0]
  const last = nodes[nodes.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

onMounted(() => {
  const preferred = root.value?.querySelector('[data-autofocus]')
  ;(preferred || focusable()[0] || root.value)?.focus()
  document.addEventListener('keydown', onKey)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKey)
})
</script>

<template>
  <div class="modal-back" @click.self="emit('close')">
    <div
      ref="root"
      class="modal"
      :class="{ wide }"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      tabindex="-1"
    >
      <header class="modal-head">
        <h2 :id="titleId">{{ title }}</h2>
      </header>
      <div class="modal-body">
        <slot />
      </div>
      <footer v-if="$slots.footer" class="modal-foot">
        <slot name="footer" />
      </footer>
    </div>
  </div>
</template>
