<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { listen } from "@tauri-apps/api/event";
import { open } from "@tauri-apps/plugin-dialog";
import AiChat from "./components/AiChat.vue";
import ConflictModal from "./components/ConflictModal.vue";
import PlansView from "./components/PlansView.vue";
import SyncView from "./components/SyncView.vue";
import UpdatesView from "./components/UpdatesView.vue";
import { api, type Dimension, type PlanItem, type Preview, type Progress, type Scan, type Settings, type SongHit, type SongRow, type UpdateView } from "./commands";
import { canStart, conflictRows, decisionsFrom, planKey, songKey, type Choice } from "./syncState";

const tab = ref<"sync" | "plans" | "updates" | "ai">("sync");
const settings = ref<Settings>({
  server_url: "http://localhost:8080",
  email: "",
  logged_in: false,
  visio_path: "",
  user_id: "",
  dimension_id: "",
  role: "",
  plan_windows1251: false,
});
const serverUrl = ref(settings.value.server_url);
const email = ref("");
const password = ref("");
const scan = ref<Scan | null>(null);
const preview = ref<Preview | null>(null);
const dimensions = ref<Dimension[]>([]);
const progress = ref<Progress | null>(null);
const log = ref<string[]>([]);
const error = ref("");
const busy = ref("");
const chosen = ref<Record<string, Choice>>({});
const conflict = ref<SongRow | null>(null);
const hits = ref<SongHit[]>([]);
const updateInfo = ref<UpdateView | null>(null);
const updateError = ref("");
const updateBusy = ref(false);
const savedPath = ref("");

const chosenRows = computed(() => conflictRows(preview.value?.songs ?? [], preview.value?.plans ?? []));

function note(line: string) {
  log.value = [...log.value, line];
}

async function boot() {
  try {
    settings.value = await api.loadSettings();
    serverUrl.value = settings.value.server_url;
    email.value = settings.value.email;
    if (settings.value.visio_path) {
      scan.value = await api.openVisio(settings.value.visio_path);
    }
    if (settings.value.logged_in) {
      dimensions.value = await api.dimensions();
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  }
}

async function saveServer() {
  settings.value = await api.saveSettings(serverUrl.value);
  note("Адрес сервера сохранён.");
}

async function enter(create: boolean) {
  busy.value = "auth";
  error.value = "";
  try {
    await saveServer();
    settings.value = create ? await api.register(email.value, password.value) : await api.login(email.value, password.value);
    password.value = "";
    dimensions.value = await api.dimensions();
    note(create ? "Учётная запись создана." : "Вход выполнен.");
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    busy.value = "";
  }
}

async function pick(directory: boolean) {
  const picked = await open(directory ? { directory: true, multiple: false } : { directory: false, multiple: false, filters: [{ name: "VisioBible", extensions: ["ini"] }] });
  if (typeof picked !== "string") return;
  await choose(picked);
}

async function choose(path: string) {
  error.value = "";
  try {
    scan.value = await api.openVisio(path);
    preview.value = null;
    if (scan.value.needs_choice) note("Найдено несколько папок, выберите одну.");
    else note(scan.value.modules_dir ? "Папка открыта." : "Папка открыта, сборники не найдены.");
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  }
}

async function refresh() {
  busy.value = "preview";
  error.value = "";
  try {
    preview.value = await api.preview();
    chosen.value = {};
    note("Список изменений готов.");
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    busy.value = "";
  }
}

function onPlanChoice(key: string, choice: "keep_local" | "take_server" | "keep_both") {
  remember(planKey(key), { kind: choice });
}

function remember(key: string, choice: Choice) {
  chosen.value = { ...chosen.value, [key]: choice };
}

async function start() {
  if (!preview.value || !canStart(chosenRows.value, chosen.value)) return;
  busy.value = "sync";
  error.value = "";
  const payload = decisionsFrom(
    chosen.value,
    preview.value.songs,
    preview.value.plans,
  );
  try {
    const report = await api.start(payload.songs, payload.plans);
    for (const line of report.notes) note(line);
    if (report.backups.length) note("Копии .bak сохранены рядом с изменёнными файлами.");
    if (settings.value.visio_path) scan.value = await api.openVisio(settings.value.visio_path);
    preview.value = await api.preview();
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    busy.value = "";
  }
}

async function repairEncoding(path: string) {
  error.value = "";
  try {
    scan.value = await api.repairPlanEncoding(path);
    preview.value = null;
    note("Кодировка плана исправлена. Файл записан в UTF-8.");
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  }
}

async function setPlanEncoding(windows1251: boolean) {
  try {
    settings.value = await api.setPlanEncoding(windows1251);
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  }
}

async function createPlan(name: string, items: PlanItem[]) {
  error.value = "";
  try {
    await api.createPlan(name, items);
    if (settings.value.visio_path) scan.value = await api.openVisio(settings.value.visio_path);
    note(`План «${name}» сохранён на этом компьютере.`);
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  }
}

async function search(query: string) {
  try {
    hits.value = await api.searchSongs(query);
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  }
}

async function checkUpdate() {
  updateBusy.value = true;
  updateError.value = "";
  try {
    updateInfo.value = await api.checkUpdate();
  } catch (err) {
    updateError.value = err instanceof Error ? err.message : String(err);
  } finally {
    updateBusy.value = false;
  }
}

async function downloadUpdate() {
  updateBusy.value = true;
  updateError.value = "";
  try {
    savedPath.value = await api.downloadUpdate();
  } catch (err) {
    updateError.value = err instanceof Error ? err.message : String(err);
  } finally {
    updateBusy.value = false;
  }
}

onMounted(() => {
  void boot();
  void listen<Progress>("sync-progress", (event) => {
    progress.value = event.payload;
  });
});
</script>

<template>
  <div class="shell" :class="{ 'is-chat': tab === 'ai' }">
    <header class="topbar">
      <div class="brand">FlammeBible</div>
      <nav class="tabs">
        <button class="tab" :class="{ active: tab === 'sync' }" type="button" @click="tab = 'sync'">Синхронизация</button>
        <button class="tab" :class="{ active: tab === 'plans' }" type="button" @click="tab = 'plans'">Планы</button>
        <button class="tab" :class="{ active: tab === 'updates' }" type="button" @click="tab = 'updates'">Обновление</button>
        <button class="tab" :class="{ active: tab === 'ai' }" type="button" @click="tab = 'ai'">AI Чат</button>
      </nav>
    </header>
    <SyncView
      v-if="tab === 'sync'"
      :settings="settings"
      :server-url="serverUrl"
      :email="email"
      :password="password"
      :scan="scan"
      :preview="preview"
      :dimensions="dimensions"
      :progress="progress"
      :log="log"
      :error="error"
      :busy="busy"
      :conflict="conflict"
      :chosen="chosen"
      @update:server-url="serverUrl = $event"
      @update:email="email = $event"
      @update:password="password = $event"
      @save-server="saveServer"
      @login="enter(false)"
      @register="enter(true)"
      @logout="api.logout().then((value) => { settings = value; dimensions = []; })"
      @pick-root="pick(true)"
      @pick-modules="pick(true)"
      @choose-catalog="choose"
      @select-dimension="api.selectDimension($event).then((value) => { settings = value; })"
      @refresh="refresh"
      @start="start"
      @open-conflict="conflict = $event"
      @resolve-plan="onPlanChoice"
      @repair="repairEncoding"
      @set-encoding="setPlanEncoding"
    />
    <PlansView v-else-if="tab === 'plans'" :scan="scan" :hits="hits" :windows1251="settings.plan_windows1251" @create="createPlan" @search="search" @repair="repairEncoding" @set-encoding="setPlanEncoding" />
    <UpdatesView v-else-if="tab === 'updates'" :info="updateInfo" :error="updateError" :busy="updateBusy" :saved-path="savedPath" @check="checkUpdate" @download="downloadUpdate" />
    <AiChat v-else-if="tab === 'ai'" :logged-in="settings.logged_in" />
    <ConflictModal
      v-if="conflict"
      :row="conflict"
      @close="conflict = null"
      @choose="(kind, title, lyrics) => { if (conflict) remember(songKey(conflict.book_path, conflict.fbid), kind === 'manual' ? { kind, title, lyrics } : { kind }); conflict = null; }"
    />
  </div>
</template>
