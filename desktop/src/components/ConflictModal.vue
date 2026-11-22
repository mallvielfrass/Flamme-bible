<script setup lang="ts">
import { ref } from "vue";
import type { SongRow } from "../commands";
import { AppModal } from "../shared";

const props = defineProps<{ row: SongRow }>();
const emit = defineEmits<{
  choose: [kind: "keep_local" | "take_server" | "keep_both" | "manual", title: string, lyrics: string];
  close: [];
}>();

const title = ref(props.row.local_title || props.row.remote_title || props.row.title);
const lyrics = ref(props.row.local_lyrics || props.row.remote_lyrics);
</script>

<template>
  <AppModal wide :title="row.title || 'Песня'" @close="emit('close')">
    <div class="conflict">
      <p class="hint">Выберите, какой текст оставить. Обе версии останутся, если сохранить их как разные песни.</p>
      <div class="conflict-split">
        <article>
          <p class="who">В VisioBible</p>
          <strong class="entity-title">{{ row.local_title || "Песни нет" }}</strong>
          <pre class="lyric">{{ row.local_lyrics || "Песни нет" }}</pre>
        </article>
        <article>
          <p class="who">На сервере</p>
          <strong class="entity-title">{{ row.remote_title || "Песни нет" }}</strong>
          <pre class="lyric">{{ row.remote_lyrics || "Песни нет" }}</pre>
        </article>
      </div>
      <div class="btn-row">
        <button class="btn slim primary" type="button" @click="emit('choose', 'keep_local', '', '')">Оставить мою версию</button>
        <button class="btn slim primary" type="button" @click="emit('choose', 'take_server', '', '')">Взять с сервера</button>
        <button class="btn slim gold" type="button" @click="emit('choose', 'keep_both', '', '')">Сохранить обе как разные песни</button>
      </div>
      <h3 class="manual-title">Исправить текст вручную</h3>
      <label>Название<input v-model="title" /></label>
      <label>Текст<textarea v-model="lyrics" rows="3"></textarea></label>
    </div>
    <template #footer>
      <button class="btn slim" type="button" @click="emit('close')">Отмена</button>
      <button class="btn slim primary" type="button" :disabled="title.trim().length === 0" @click="emit('choose', 'manual', title, lyrics)">Сохранить этот текст</button>
    </template>
  </AppModal>
</template>
