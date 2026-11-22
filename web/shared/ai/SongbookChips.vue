<script setup>
import { computed, ref } from 'vue'
import SongbookSongsModal from './SongbookSongsModal.vue'

const props = defineProps({
  items: { type: Array, default: () => [] },
})

const first = 6
const opened = ref(null)
const all = ref(false)

const shown = computed(() => (all.value ? props.items : props.items.slice(0, first)))
const hidden = computed(() => props.items.length - shown.value.length)
</script>

<template>
  <div v-if="items.length" class="ai-books">
    <p class="ai-books-label">Сборники</p>
    <div class="ai-books-list">
      <button
        v-for="book in shown"
        :key="book.id"
        class="ai-book"
        type="button"
        :title="book.note || undefined"
        @click="opened = book"
      >{{ book.title }}</button>
      <button v-if="hidden > 0" class="ai-book more" type="button" @click="all = true">ещё {{ hidden }}</button>
      <button v-else-if="items.length > first" class="ai-book more" type="button" @click="all = false">свернуть</button>
    </div>
  </div>
  <SongbookSongsModal v-if="opened" :book="opened" @close="opened = null" />
</template>
