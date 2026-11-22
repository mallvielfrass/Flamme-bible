<script setup lang="ts">
import { ref } from "vue";
import { open } from "@tauri-apps/plugin-dialog";
import type { Dimension, Scan, Settings } from "../commands";
import { AppModal } from "../shared";
import { foundLine } from "../syncState";

export interface SettingsDraft {
  serverUrl: string;
  path: string;
  windows1251: boolean;
  dimensionId: string;
}

const props = defineProps<{
  settings: Settings;
  scan: Scan | null;
  dimensions: Dimension[];
  error: string;
  busy: boolean;
}>();

const emit = defineEmits<{
  close: [];
  logout: [];
  save: [SettingsDraft];
}>();

const serverUrl = ref(props.settings.server_url);
const path = ref(props.settings.visio_path);
const windows1251 = ref(props.settings.plan_windows1251);
const dimensionId = ref(props.settings.dimension_id);
const pickError = ref("");

const samePath = () => path.value === props.settings.visio_path;

async function pick() {
  pickError.value = "";
  try {
    const picked = await open({ directory: true, multiple: false });
    if (typeof picked === "string") path.value = picked;
  } catch (err) {
    pickError.value = err instanceof Error ? err.message : String(err);
  }
}

function save() {
  emit("save", {
    serverUrl: serverUrl.value,
    path: path.value,
    windows1251: windows1251.value,
    dimensionId: dimensionId.value,
  });
}
</script>

<template>
  <AppModal title="Настройки" @close="emit('close')">
    <div class="path-row">
      <label>Папка VisioBible, файл VisioBible.ini или каталог Modules
        <input v-model="path" />
      </label>
      <button class="btn slim" type="button" :disabled="busy" @click="pick">Выбрать</button>
    </div>
    <p v-if="scan && samePath() && !scan.needs_choice" class="hint">{{ foundLine(scan.songbooks.length, scan.bibles.length, scan.plans.length) }}</p>
    <p v-if="scan?.needs_choice && samePath()" class="hint">Найдено несколько папок. Выберите одну:</p>
    <div v-if="scan?.needs_choice && samePath()" class="row">
      <button v-for="catalog in scan.catalogs" :key="catalog" class="btn slim" type="button" @click="path = catalog">{{ catalog }}</button>
    </div>
    <p v-if="scan && samePath() && !scan.needs_choice && scan.songbooks.length === 0" class="hint">В выбранной папке сборников не видно. Проверьте, что это Modules, а не сама программа.</p>
    <label>Адрес сервера
      <input v-model="serverUrl" autocomplete="url" />
    </label>
    <label>Кодировка записи планов в VisioBible
      <select :value="windows1251 ? 'windows-1251' : 'utf-8'" @change="windows1251 = ($event.target as HTMLSelectElement).value === 'windows-1251'">
        <option value="utf-8">UTF-8</option>
        <option value="windows-1251">Windows-1251</option>
      </select>
    </label>
    <p class="hint">Кодировка действует только когда синхронизация записывает файлы в папку VisioBible.</p>
    <label v-if="dimensions.length">Пространство для синхронизации
      <select v-model="dimensionId">
        <option v-for="space in dimensions" :key="space.id" :value="space.id">
          {{ space.name || (space.kind === "personal" ? "Личное" : "Община") }}
        </option>
      </select>
    </label>
    <p v-if="settings.email" class="hint">Вы вошли как {{ settings.email }}.</p>
    <p v-if="pickError || error" class="error">{{ pickError || error }}</p>
    <template #footer>
      <button class="btn slim danger" type="button" :disabled="busy" @click="emit('logout')">Выйти</button>
      <button class="btn slim" type="button" :disabled="busy" @click="emit('close')">Отмена</button>
      <button class="btn slim primary" type="button" :disabled="busy" @click="save">Сохранить</button>
    </template>
  </AppModal>
</template>
