<script setup>
import { computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useSyncStore } from '../stores/sync'

const route = useRoute()
const sync = useSyncStore()

const gap = computed(() => {
  if (sync.serverCursor === null) return null
  return Number(sync.serverCursor) - Number(sync.appliedCursor)
})

const blocked = computed(() => sync.conflicts.length > 0 || !!sync.readError || !!sync.pushError)

function pluralChanges(n) {
  const mod10 = n % 10
  const mod100 = n % 100
  if (mod10 === 1 && mod100 !== 11) return 'изменение'
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return 'изменения'
  return 'изменений'
}

function songText(song) {
  if (!song) return 'нет'
  const parts = (song.parts || []).map((part) => part.text).filter(Boolean)
  return [song.title, ...parts].filter(Boolean).join('\n') || 'нет'
}

function answerLabel(answer) {
  if (answer === 'server') return 'серверная версия'
  if (answer === 'new') return 'новая песня'
  return ''
}

onMounted(() => sync.load(route.params.id))
watch(() => route.params.id, (id) => sync.load(id))
</script>

<template>
  <div class="page-head">
    <h1>Состояние синхронизации</h1>
    <router-link class="back" :to="`/dimensions/${route.params.id}`">← К пространству</router-link>
  </div>
  <div class="sync-bar">
    <div class="sync-tools">
      <label v-if="sync.songbooks.length">
        Сборник
        <select :value="sync.bookId" @change="sync.selectBook($event.target.value)">
          <option v-for="book in sync.songbooks" :key="book.id" :value="book.id">{{ book.title || book.fbid }}</option>
        </select>
      </label>
      <p v-else class="empty">Сборников нет</p>
    </div>
    <div v-if="sync.serverCursor !== null" class="sync-status">
      <p v-if="gap > 0" class="status">На сервере есть {{ gap }} {{ pluralChanges(gap) }}, они ещё не попали в песенник на этом компьютере.</p>
      <p v-else-if="gap === 0 && !blocked" class="status">Песенник на этом компьютере совпадает с сервером.</p>
      <details>
        <summary>Подробности</summary>
        <p>Курсор сервера: {{ sync.serverCursor }}. Применено: {{ sync.appliedCursor }}.</p>
      </details>
    </div>
  </div>

  <div class="sync-grid" :class="{ 'has-conflict': sync.conflicts.length }">
    <section data-queue-in>
      <h2>Очередь</h2>
      <p v-if="sync.events.length === 0" class="empty">Входящих событий нет</p>
      <ul v-else class="plain">
        <li v-for="event in sync.events" :key="event.seq">
          {{ event.seq }} {{ event.body?.song?.title || event.songbook_id }}
        </li>
      </ul>
      <button class="btn primary" type="button" :disabled="sync.events.length === 0" @click="sync.applyPage">Применить страницу</button>
      <h3>Исходящие</h3>
      <p v-if="sync.outgoing.length === 0" class="empty">Исходящих операций нет</p>
      <ul v-else class="plain">
        <li v-for="op in sync.outgoing" :key="op.operation_id">
          {{ op.song?.fbid }} {{ op.song?.title }}
        </li>
      </ul>
      <button class="btn primary" type="button" :disabled="sync.outgoing.length === 0" @click="sync.send">Отправить очередь</button>
    </section>

    <section v-if="sync.conflicts.length" class="conflict" data-conflict>
      <h2>Конфликт</h2>
      <div v-for="item in sync.conflicts" :key="item.operation_id">
        <article>
          <p class="who">Сервер</p>
          <pre>{{ songText(item.server) }}</pre>
        </article>
        <article>
          <p class="who">Клиент</p>
          <pre>{{ songText(item.client) }}</pre>
        </article>
        <p v-if="!item.answer" class="btn-row">
          <button class="btn primary" type="button" @click="sync.answer(item.operation_id, 'server')">Серверная версия</button>
          <button class="btn primary" type="button" @click="sync.answer(item.operation_id, 'new')">Новая песня</button>
        </p>
        <p v-else>Ответ: {{ answerLabel(item.answer) }}</p>
      </div>
    </section>
  </div>

  <section v-if="sync.readError || sync.pushError" data-refusal>
    <h2>Отказ</h2>
    <p class="error">{{ sync.readError || sync.pushError }}</p>
    <button v-if="sync.readError === 'reload'" class="btn primary slim" type="button" @click="sync.reloadSnapshot">Загрузить снимок заново</button>
  </section>

  <p v-if="sync.done" data-sync-done>Синхронизация завершена</p>
</template>
