<script setup lang="ts">
import type { Progress } from "../commands";
import { AppModal } from "../shared";
import SyncProgress from "./SyncProgress.vue";

defineProps<{ progress: Progress | null; error: string }>();
const emit = defineEmits<{ cancel: []; close: [] }>();
</script>

<template>
  <AppModal :title="error ? 'Синхронизация не завершилась' : 'Идёт синхронизация'" @close="emit('cancel')">
    <template v-if="error">
      <p class="error">{{ error }}</p>
      <p class="confirm-text">Можно закрыть программу или остаться и повторить синхронизацию.</p>
    </template>
    <template v-else>
      <p class="confirm-text">Программа закроется сама, когда синхронизация завершится. Прерывать запись в папку VisioBible нельзя.</p>
      <SyncProgress v-if="progress" :progress="progress" />
      <p v-else class="hint">Готовимся…</p>
    </template>
    <template #footer>
      <button class="btn slim" type="button" data-autofocus @click="emit('cancel')">Не закрывать</button>
      <button v-if="error" class="btn slim danger" type="button" @click="emit('close')">Закрыть программу</button>
    </template>
  </AppModal>
</template>
