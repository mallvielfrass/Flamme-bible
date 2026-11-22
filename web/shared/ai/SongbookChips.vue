<script setup>
import { ref } from 'vue'
import SongbookSongsModal from './SongbookSongsModal.vue'

defineProps({
  items: { type: Array, default: () => [] },
})

const opened = ref(null)
</script>

<template>
  <div v-if="items.length" class="ai-books">
    <p class="ai-books-label">Сборники</p>
    <div class="ai-books-list">
      <button
        v-for="book in items"
        :key="book.id"
        class="ai-book"
        type="button"
        @click="opened = book"
      >
        <span class="ai-hit-k">Сборник</span>
        <span class="ai-hit-t">{{ book.title }}</span>
        <span v-if="book.note" class="ai-hit-n">{{ book.note }}</span>
      </button>
    </div>
  </div>
  <SongbookSongsModal v-if="opened" :book="opened" @close="opened = null" />
</template>
