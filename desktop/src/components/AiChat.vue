<script setup lang="ts">
import { provide, ref } from "vue";
import { createAiApi } from "../../../web/shared/ai/aiApi.js";
import { useAiStore } from "../../../web/shared/ai/store.js";
import { aiPlanKey } from "../../../web/shared/ai/results.js";
import { planTarget } from "../../../web/src/planItems.js";
import { plansApi } from "../plans/api";
import type { Plan, PlanItem } from "../plans/model";
import { desktopFetch } from "../aiFetch";
import { AiSettingsModal, AiWorkspace } from "../shared";

const props = defineProps<{ loggedIn: boolean }>();
const chatId = ref("");
const settingsOpen = ref(false);
const ai = useAiStore();
const client = props.loggedIn ? createAiApi({ fetch: desktopFetch }) : null;
if (client) ai.useApi(client);

provide(
  aiPlanKey,
  planTarget({
    list: () => plansApi.list(),
    read: (id: string) => plansApi.read(id),
    save: async (plan: Plan) => {
      const result = await plansApi.save(plan);
      if (result.conflict) throw new Error("План только что изменили. Откройте его во вкладке «Планы» и попробуйте снова.");
    },
    create: (title: string, items: PlanItem[]) => plansApi.create(title, "", items),
  }),
);

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
