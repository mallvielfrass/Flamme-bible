<script setup lang="ts">
import { ref } from "vue";
import { createAiApi } from "../../../web/shared/ai/aiApi.js";
import { useAiStore } from "../../../web/shared/ai/store.js";
import { desktopFetch } from "../aiFetch";
import { AiSettingsModal, AiWorkspace } from "../shared";

const props = defineProps<{ loggedIn: boolean }>();
const chatId = ref("");
const settingsOpen = ref(false);
const ai = useAiStore();
const client = props.loggedIn ? createAiApi({ fetch: desktopFetch }) : null;
if (client) ai.useApi(client);

function openChat(id: string) {
  chatId.value = id || "";
}

async function closeSettings() {
  settingsOpen.value = false;
  try {
    await ai.loadProviders();
  } catch {
    /* ошибку показывает сам чат */
  }
}
</script>

<template>
  <section class="chat-pane">
    <p v-if="!loggedIn" class="hint">Войдите на вкладке «Синхронизация», чтобы открыть чат.</p>
    <template v-else>
      <AiWorkspace :chat-id="chatId" @open-chat="openChat" @open-settings="settingsOpen = true" />
      <AiSettingsModal v-if="settingsOpen && client" :client="client" @close="closeSettings" />
    </template>
  </section>
</template>
