<script setup lang="ts">
import type { UpdateView } from "../commands";

defineProps<{ info: UpdateView | null; error: string; busy: boolean; savedPath: string }>();
defineEmits<{ check: []; download: [] }>();
</script>

<template>
  <section class="card" style="margin-top: 1rem">
    <h2>Обновление программы</h2>
    <p v-if="info">Сейчас установлена версия {{ info.current }}.</p>
    <p v-else class="empty">Версия ещё не проверена.</p>
    <div class="row">
      <button class="btn" type="button" :disabled="busy" @click="$emit('check')">Проверить обновление</button>
      <button v-if="info?.available" class="btn primary" type="button" :disabled="busy" @click="$emit('download')">Скачать и проверить</button>
    </div>
    <p v-if="busy" class="hint">Проверяем…</p>
    <p v-if="info && !info.available && info.latest" class="hint">Новой версии нет. На сервере {{ info.latest }}.</p>
    <article v-if="info?.available" class="conflict-card">
      <strong>Есть версия {{ info.latest }}</strong>
      <p>{{ info.notes }}</p>
    </article>
    <p v-if="savedPath" class="found">Файл проверен и сохранён: {{ savedPath }}. Закройте программу и замените её этим файлом.</p>
    <p v-if="error" class="error">{{ error }}</p>
  </section>
</template>
