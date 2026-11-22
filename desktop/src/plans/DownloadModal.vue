<script setup lang="ts">
import { computed, ref } from "vue";
import AppModal from "../../../web/shared/components/AppModal.vue";

defineProps<{ dirty: boolean }>();
const emit = defineEmits<{
  close: [];
  download: [choice: { format: string; encoding: string; saveFirst: boolean }];
}>();

const storageKey = "flammebible.planExport";

function readChoice() {
  try {
    const saved = JSON.parse(localStorage.getItem(storageKey) || "");
    return { format: saved?.format || "vplt", encoding: saved?.encoding || "windows-1251" };
  } catch {
    return { format: "vplt", encoding: "windows-1251" };
  }
}

const choice = readChoice();
const format = ref(choice.format);
const encoding = ref(choice.encoding);
const showEncoding = computed(() => format.value !== "json");

function send(saveFirst: boolean) {
  const next = { format: format.value, encoding: showEncoding.value ? encoding.value : "" };
  localStorage.setItem(storageKey, JSON.stringify({ format: next.format, encoding: encoding.value || "windows-1251" }));
  emit("download", { ...next, saveFirst });
}
</script>

<template>
  <AppModal title="Скачать план" @close="emit('close')">
    <label>Формат
      <select v-model="format">
        <option value="vplt">plan.vplt</option>
        <option value="history">History.ini</option>
        <option value="plandef">PlanDef.ini</option>
        <option value="json">JSON</option>
      </select>
    </label>
    <label v-if="showEncoding">Кодировка
      <select v-model="encoding">
        <option value="windows-1251">Windows-1251</option>
        <option value="utf-8">UTF-8</option>
      </select>
    </label>
    <p class="hint">Файл сохранится туда, куда вы укажете. Папка VisioBible не меняется.</p>
    <p v-if="dirty" class="banner bad" role="status">Сначала сохранится правка названия.</p>
    <template #footer>
      <button class="btn slim" type="button" @click="emit('close')">Отмена</button>
      <button v-if="dirty" class="btn slim primary" type="button" @click="send(true)">Сохранить и скачать</button>
      <button v-else class="btn slim primary" type="button" @click="send(false)">Скачать</button>
    </template>
  </AppModal>
</template>
