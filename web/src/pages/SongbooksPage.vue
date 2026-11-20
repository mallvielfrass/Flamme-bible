<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { useSessionStore } from '../stores/session'

const session = useSessionStore()
const books = ref([])
const error = ref('')
const loading = ref(true)

onMounted(async () => {
  const spaces = session.spaces.length
    ? session.spaces
    : (session.dimensionId ? [{ id: session.dimensionId }] : [])
  try {
    const lists = await Promise.all(spaces.map((space) =>
      api(`/dimensions/${space.id}/songbooks`, { token: session.token }),
    ))
    books.value = lists.flat().filter((book) => !book.hidden)
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <h1>Сборники</h1>
  <p v-if="error" class="error">{{ error }}</p>
  <p v-else-if="loading" class="empty">Загрузка…</p>
  <p v-else-if="!books.length" class="empty">Сборников пока нет.</p>
  <ul v-else class="plain">
    <li v-for="book in books" :key="book.id">
      <strong>{{ book.title || book.short_name || book.fbid }}</strong>
      <span v-if="book.short_name && book.title"> · {{ book.short_name }}</span>
    </li>
  </ul>
</template>
