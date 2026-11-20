<script setup>
import { computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { explain } from '../api'
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
  if (!song) return 'нет текста'
  const parts = (song.parts || []).map((part) => part.text).filter(Boolean)
  return [song.title, ...parts].filter(Boolean).join('\n') || 'нет текста'
}

function answerLabel(answer) {
  if (answer === 'server') return 'оставить версию сервера'
  if (answer === 'new') return 'сохранить как новую песню'
  return ''
}

onMounted(() => sync.load(route.params.id))
watch(() => route.params.id, (id) => sync.load(id))
</script>

<template>
  <div class="page-head">
    <h1>Синхронизация</h1>
    <router-link class="back" :to="`/dimensions/${route.params.id}`">К общине</router-link>
  </div>
  <p class="hint">
    Полная синхронизация папки VisioBible делается в программе на компьютере, не в браузере.
    <router-link to="/download">Скачать программу</router-link>.
    Здесь видно, совпадает ли то, что уже учтено на этом сайте, с сервером.
  </p>

  <div class="sync-bar">
    <div class="sync-tools">
      <label v-if="sync.songbooks.length">
        Сборник
        <select :value="sync.bookId" @change="sync.selectBook($event.target.value)">
          <option v-for="book in sync.songbooks" :key="book.id" :value="book.id">{{ book.title || 'Сборник' }}</option>
        </select>
      </label>
      <p v-else-if="sync.ready" class="empty">Сборников пока нет</p>
    </div>
    <div v-if="sync.serverCursor !== null" class="sync-status">
      <p v-if="gap > 0" class="status">На сервере есть {{ gap }} {{ pluralChanges(gap) }}. Их ещё нет на этом сайте.</p>
      <p v-else-if="gap === 0 && !blocked" class="status">На этом сайте всё совпадает с сервером.</p>
    </div>
  </div>

  <p v-if="!sync.ready && !sync.readError" class="empty" role="status">Загрузка…</p>

  <div class="sync-grid" :class="{ 'has-conflict': sync.conflicts.length }">
    <section data-queue-in>
      <h2>Изменения на сервере</h2>
      <p v-if="sync.events.length === 0" class="empty">Новых изменений нет</p>
      <ul v-else class="plain">
        <li v-for="event in sync.events" :key="event.seq">
          {{ event.body?.song?.title || 'Правка сборника' }}
        </li>
      </ul>
      <button class="btn primary slim" type="button" :disabled="sync.events.length === 0" @click="sync.applyPage">Забрать изменения</button>
      <h3>Ещё не отправлено</h3>
      <p v-if="sync.outgoing.length === 0" class="empty">Отправлять нечего</p>
      <ul v-else class="plain">
        <li v-for="op in sync.outgoing" :key="op.operation_id">
          {{ op.song?.title || 'Правка' }}
        </li>
      </ul>
      <button class="btn primary slim" type="button" :disabled="sync.outgoing.length === 0" @click="sync.send">Отправить на сервер</button>
    </section>

    <section v-if="sync.conflicts.length" class="conflict" data-conflict>
      <h2>Нужно выбрать версию</h2>
      <p class="hint">Одну и ту же песню изменили и у вас, и на сервере. Выберите, что оставить.</p>
      <div v-for="item in sync.conflicts" :key="item.operation_id">
        <article>
          <p class="who">На сервере</p>
          <pre class="lyric">{{ songText(item.server) }}</pre>
        </article>
        <article>
          <p class="who">У вас</p>
          <pre class="lyric">{{ songText(item.client) }}</pre>
        </article>
        <p v-if="!item.answer" class="btn-row">
          <button class="btn primary slim" type="button" @click="sync.answer(item.operation_id, 'server')">Оставить версию сервера</button>
          <button class="btn slim" type="button" @click="sync.answer(item.operation_id, 'new')">Сохранить как новую песню</button>
        </p>
        <p v-else>Выбрано: {{ answerLabel(item.answer) }}</p>
      </div>
    </section>
  </div>

  <section v-if="sync.readError || sync.pushError" data-refusal>
    <h2>Не получилось</h2>
    <p class="banner bad">{{ explain(sync.readError || sync.pushError) }}</p>
    <button v-if="sync.readError === 'reload'" class="btn primary slim" type="button" @click="sync.reloadSnapshot">Начать сверку заново</button>
  </section>

  <p v-if="sync.done" data-sync-done>Изменения учтены. Папку VisioBible по-прежнему обновляет программа на компьютере.</p>
</template>
