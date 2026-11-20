<script setup>
import { onMounted, ref } from 'vue'
import { api, explain } from '../api'
import { statusText } from '../checks'

defineProps({
  guest: { type: Boolean, default: false },
})
const emit = defineEmits(['switch'])

const books = ref([])
const modules = ref([])
const error = ref('')
const loading = ref(true)

onMounted(async () => {
  try {
    const [songbooks, bible] = await Promise.all([
      api('/api/catalog/songbooks'),
      api('/api/catalog/modules'),
    ])
    books.value = Array.isArray(songbooks) ? songbooks : []
    modules.value = Array.isArray(bible) ? bible : []
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <p class="hint">Здесь только то, что уже одобрено и открыто всем. Войти для просмотра не нужно.</p>
  <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  <p v-else-if="loading" class="empty" role="status">Загрузка…</p>
  <div v-else class="split">
    <section>
      <h2>Сборники</h2>
      <div v-if="!books.length" class="empty-state">
        <p>Одобренных сборников пока нет.</p>
        <button v-if="!guest" class="btn slim primary" type="button" @click="emit('switch', 'books')">К моим сборникам</button>
        <router-link v-else class="btn slim primary" to="/login">Войти, чтобы добавить свой</router-link>
      </div>
      <ul v-else class="plain">
        <li v-for="book in books" :key="book.id">
          <strong class="title-serif">{{ book.title || book.short_name || 'Сборник' }}</strong>
          <p v-if="book.short_name" class="hint">{{ book.short_name }}</p>
        </li>
      </ul>
    </section>
    <section>
      <h2>Модули Библии</h2>
      <div v-if="!modules.length" class="empty-state">
        <p>Одобренных переводов пока нет.</p>
        <button v-if="!guest" class="btn slim primary" type="button" @click="emit('switch', 'modules')">К моим модулям</button>
      </div>
      <ul v-else class="plain">
        <li v-for="item in modules" :key="item.id">
          <div class="card-head">
            <strong class="title-serif">{{ item.title || item.short_name }}</strong>
            <span class="badge approved">{{ statusText(item.status) }}</span>
          </div>
          <p class="hint">{{ item.short_name }}</p>
        </li>
      </ul>
    </section>
  </div>
</template>
