<script setup lang="ts">
import { bibleStatus, type BibleLink } from "../bibleSync";

defineProps<{
  bibles: { path: string; name: string; short_name: string }[];
  links: BibleLink[];
  loggedIn: boolean;
  busy: boolean;
  error: string;
}>();

const emit = defineEmits<{ sync: [] }>();

function linkFor(links: BibleLink[], path: string): BibleLink | undefined {
  return links.find((link) => link.path === path);
}
</script>

<template>
  <section class="card">
    <div class="step-head"><span class="num">4</span><h2>Модули Библии</h2></div>
    <p class="hint">Если такой модуль уже есть на сервере, файл не отправляется. В настройках этого компьютера сохраняется только его id.</p>
    <p v-if="bibles.length === 0" class="empty">В выбранной папке модулей Библии не видно.</p>
    <ul v-else class="list">
      <li v-for="bible in bibles" :key="bible.path">
        <strong>{{ bible.short_name || bible.name || "Модуль" }}</strong>
        <span v-if="bible.name && bible.name !== bible.short_name"> — {{ bible.name }}</span>
        <p class="hint">{{ bibleStatus(linkFor(links, bible.path)) }}</p>
      </li>
    </ul>
    <button class="btn" type="button" :disabled="busy || !loggedIn || bibles.length === 0" @click="emit('sync')">Сверить с сервером</button>
    <p v-if="!loggedIn" class="hint">Сначала войдите в учётную запись.</p>
    <p v-if="error" class="error">{{ error }}</p>
  </section>
</template>
