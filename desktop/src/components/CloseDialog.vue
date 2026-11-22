<script setup lang="ts">
import { AppModal } from "../shared";

const emit = defineEmits<{ sync: []; discard: []; close: [] }>();
</script>

<template>
  <AppModal title="Есть несинхронизированные изменения" @close="emit('close')">
    <p class="confirm-text">На сервере есть правки, которые ещё не записаны в VisioBible, и изменения из папки, которые ещё не отправлены. Если закрыть программу сейчас, они никуда не денутся, но папка VisioBible останется как есть.</p>
    <template #footer>
      <button class="btn slim" type="button" data-autofocus @click="emit('close')">Отмена</button>
      <button class="btn slim" type="button" @click="emit('discard')">Закрыть без синхронизации</button>
      <button class="btn slim primary" type="button" @click="emit('sync')">Синхронизировать</button>
    </template>
  </AppModal>
</template>
