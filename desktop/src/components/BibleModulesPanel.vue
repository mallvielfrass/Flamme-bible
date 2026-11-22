<script setup lang="ts">
import { bibleStatus, type BibleLink } from "../bibleSync";

defineProps<{
  bibles: { path: string; name: string; short_name: string }[];
  links: BibleLink[];
  busy: boolean;
  error: string;
}>();

const emit = defineEmits<{ sync: [] }>();

function linkFor(links: BibleLink[], path: string): BibleLink | undefined {
  return links.find((link) => link.path === path);
}
</script>

<template>
  <div class="block">
    <div class="block-head">
      <h2>Модули Библии</h2>
      <button class="btn slim" type="button" :disabled="busy || bibles.length === 0" @click="emit('sync')">Сверить с сервером</button>
    </div>
    <p class="hint">Если такой модуль уже есть на сервере, файл не отправляется — сохраняется только его id.</p>
    <p v-if="bibles.length === 0" class="empty">В папке модулей Библии не видно.</p>
    <div v-else class="entity-list">
      <article v-for="bible in bibles" :key="bible.path" class="entity-row">
        <div>
          <p class="entity-title">{{ bible.short_name || bible.name || "Модуль" }}</p>
          <p class="entity-meta">{{ bible.name && bible.name !== bible.short_name ? bible.name : bible.path }}</p>
        </div>
        <span class="action" :class="linkFor(links, bible.path)?.server_id ? 'ok' : 'skip'">{{ bibleStatus(linkFor(links, bible.path)) }}</span>
      </article>
    </div>
    <p v-if="error" class="error">{{ error }}</p>
  </div>
</template>
