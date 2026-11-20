<script setup lang="ts">
import { ref } from "vue";
import type { SongRow } from "../commands";

defineProps<{ row: SongRow }>();
const emit = defineEmits<{ choose: [kind: "keep_local" | "take_server" | "keep_both" | "manual", title: string, lyrics: string]; close: [] }>();
const title = ref("");
const lyrics = ref("");
</script>

<template>
  <div class="modal-back" @click.self="emit('close')">
    <section class="modal" role="dialog" aria-labelledby="conflict-title">
      <h2 id="conflict-title">{{ row.title || "Песня" }}</h2>
      <p class="hint">Выберите, какой текст оставить. Обе версии останутся, если сохранить их как разные песни.</p>
      <div class="split">
        <article>
          <p class="who">На этом компьютере</p>
          <strong>{{ row.local_title || "Песни нет" }}</strong>
          <pre>{{ row.local_lyrics || "Песни нет" }}</pre>
        </article>
        <article>
          <p class="who">На сервере</p>
          <strong>{{ row.remote_title || "Песни нет" }}</strong>
          <pre>{{ row.remote_lyrics || "Песни нет" }}</pre>
        </article>
      </div>
      <div class="row" style="margin-top: 0.9rem">
        <button class="btn primary" type="button" @click="emit('choose', 'keep_local', '', '')">Оставить мою версию</button>
        <button class="btn primary" type="button" @click="emit('choose', 'take_server', '', '')">Взять с сервера</button>
        <button class="btn gold" type="button" @click="emit('choose', 'keep_both', '', '')">Сохранить обе как разные песни</button>
      </div>
      <h3 style="margin-top: 1rem">Исправить текст вручную</h3>
      <div class="grid">
        <label>Название<input v-model="title" /></label>
        <label>Текст<textarea v-model="lyrics" /></label>
      </div>
      <button class="btn" type="button" :disabled="title.trim().length === 0" @click="emit('choose', 'manual', title, lyrics)">Сохранить этот текст</button>
    </section>
  </div>
</template>
