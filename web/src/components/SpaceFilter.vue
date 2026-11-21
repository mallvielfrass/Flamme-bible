<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps({
  modelValue: { type: String, default: 'all' },
  communities: { type: Array, default: () => [] },
  counts: { type: Object, default: () => ({}) },
  allowAll: { type: Boolean, default: true },
  allowAllCommunities: { type: Boolean, default: true },
  showCounts: { type: Boolean, default: false },
  preferred: { type: String, default: '' },
})

const emit = defineEmits(['update:modelValue'])
const root = ref(null)
const menu = ref(false)
const active = ref(0)

const selected = computed(() => props.communities.find((item) => item.id === props.modelValue) || null)
const communityOn = computed(() => props.modelValue === 'community' || Boolean(selected.value))
const label = computed(() => (selected.value ? `Общинные · ${selected.value.name || 'Община'}` : 'Общинные'))
const tip = computed(() => {
  if (!props.communities.length) return 'Вы пока не состоите в общине'
  return selected.value ? (selected.value.name || 'Община') : 'Общинные'
})

const options = computed(() => {
  const list = []
  if (props.allowAllCommunities) list.push({ id: 'community', name: 'Все общины' })
  for (const item of props.communities) list.push({ id: item.id, name: item.name || 'Община' })
  return list
})

function emitValue(id) {
  menu.value = false
  emit('update:modelValue', id)
}

function preferredCommunity() {
  if (props.communities.some((item) => item.id === props.preferred)) return props.preferred
  if (props.allowAllCommunities) return 'community'
  return props.communities[0]?.id || 'community'
}

function pickCommunity() {
  if (!props.communities.length) {
    emitValue('community')
    return
  }
  if (communityOn.value) return
  emitValue(preferredCommunity())
}

const list = ref(null)

function focusActive() {
  const buttons = list.value?.querySelectorAll('[role="option"]')
  buttons?.[active.value]?.focus()
}

async function toggleMenu() {
  menu.value = !menu.value
  if (!menu.value) return
  const index = options.value.findIndex((item) => item.id === props.modelValue)
  active.value = index >= 0 ? index : 0
  await nextTick()
  focusActive()
}

function onKey(event) {
  if (!menu.value) return
  if (event.key === 'Escape') {
    event.preventDefault()
    menu.value = false
    return
  }
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    const step = event.key === 'ArrowDown' ? 1 : -1
    const size = options.value.length || 1
    active.value = (active.value + step + size) % size
    focusActive()
    return
  }
  if (event.key === 'Enter' && options.value[active.value]) {
    event.preventDefault()
    emitValue(options.value[active.value].id)
  }
}

function onDoc(event) {
  if (!root.value?.contains(event.target)) menu.value = false
}

function countOf(id) {
  return props.counts[id] || 0
}

onMounted(() => document.addEventListener('pointerdown', onDoc))
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDoc))
</script>

<template>
  <div class="space-filter">
    <button
      v-if="allowAll"
      type="button"
      class="tab"
      :class="{ on: modelValue === 'all' }"
      @click="emitValue('all')"
    >Все</button>
    <button
      type="button"
      class="tab"
      :class="{ on: modelValue === 'personal' }"
      @click="emitValue('personal')"
    >Личные</button>
    <div ref="root" class="split-tab">
      <button
        type="button"
        class="tab split-main"
        :class="{ on: communityOn, lone: !communities.length }"
        :title="tip"
        @click="pickCommunity"
      >
        <span class="space-label">{{ label }}</span>
      </button>
      <button
        v-if="communities.length"
        type="button"
        class="tab split-caret"
        :class="{ on: communityOn }"
        aria-label="Выбрать общину"
        aria-haspopup="listbox"
        :aria-expanded="menu"
        @click="toggleMenu"
        @keydown="onKey"
      >▾</button>
      <ul v-if="menu" ref="list" class="menu" role="listbox" aria-label="Общины" tabindex="-1" @keydown="onKey">
        <li v-for="(option, index) in options" :key="option.id">
          <button
            type="button"
            role="option"
            :aria-selected="modelValue === option.id"
            :class="{ on: index === active }"
            @click="emitValue(option.id)"
          >
            <span>{{ option.name }}</span>
            <span v-if="showCounts" class="menu-count">{{ countOf(option.id) }}</span>
            <span v-if="modelValue === option.id" aria-hidden="true">✓</span>
          </button>
        </li>
      </ul>
    </div>
  </div>
</template>
