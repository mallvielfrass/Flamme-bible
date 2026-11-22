<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { listen } from "@tauri-apps/api/event";
import AiChat from "./components/AiChat.vue";
import CloseDialog from "./components/CloseDialog.vue";
import ConflictModal from "./components/ConflictModal.vue";
import LoginView from "./components/LoginView.vue";
import PlansView from "./components/PlansView.vue";
import SettingsModal, { type SettingsDraft } from "./components/SettingsModal.vue";
import SyncView from "./components/SyncView.vue";
import UpdatesView from "./components/UpdatesView.vue";
import { api, type Dimension, type PlanItem, type Preview, type Progress, type Scan, type Settings, type SongHit, type SongRow, type UpdateView } from "./commands";
import { canStart, conflictRows, decisionsFrom, hasUnsyncedChanges, planKey, songKey, type Choice } from "./syncState";

const tab = ref<"sync" | "plans" | "updates" | "ai">("sync");
const ready = ref(false);
const settingsOpen = ref(false);
const closeAsk = ref(false);
const closeBusy = ref(false);
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
const scan = ref<Scan | null>(null);
const preview = ref<Preview | null>(null);
const dimensions = ref<Dimension[]>([]);
const progress = ref<Progress | null>(null);
const log = ref<string[]>([]);
const error = ref("");
const authError = ref("");
const settingsError = ref("");
const busy = ref("");
const settingsBusy = ref(false);
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

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

let booting: Promise<void> | null = null;

function startBoot() {
  booting ??= boot();
  return booting;
}

async function boot() {
  try {
    settings.value = await api.loadSettings();
    if (settings.value.visio_path) {
      scan.value = await api.openVisio(settings.value.visio_path);
    }
    if (settings.value.logged_in) {
      dimensions.value = await api.dimensions();
    }
  } catch (err) {
    const text = message(err);
    if (settings.value.logged_in) error.value = text;
    else authError.value = text;
  } finally {
    ready.value = true;
  }
}

async function enter(mode: "login" | "register", serverUrl: string, email: string, password: string) {
  if (busy.value) return;
  busy.value = "auth";
  authError.value = "";
  try {
    settings.value = await api.saveSettings(serverUrl);
    settings.value = mode === "register" ? await api.register(email, password) : await api.login(email, password);
    preview.value = null;
    tab.value = "sync";
    dimensions.value = await api.dimensions();
    note(mode === "register" ? "Учётная запись создана." : "Вход выполнен.");
  } catch (err) {
    const text = message(err);
    if (settings.value.logged_in) error.value = text;
    else authError.value = text;
  } finally {
    busy.value = "";
  }
}

async function saveSettings(draft: SettingsDraft) {
  settingsBusy.value = true;
  settingsError.value = "";
  try {
    if (draft.windows1251 !== settings.value.plan_windows1251) {
      settings.value = await api.setPlanEncoding(draft.windows1251);
    }
    if (draft.dimensionId && draft.dimensionId !== settings.value.dimension_id) {
      settings.value = await api.selectDimension(draft.dimensionId);
    }
    if (draft.path !== settings.value.visio_path) {
      scan.value = await api.openVisio(draft.path);
      preview.value = null;
      settings.value = await api.loadSettings();
      if (scan.value.needs_choice) note("Найдено несколько папок, выберите одну.");
      else note(scan.value.modules_dir ? "Папка открыта." : "Папка открыта, сборники не найдены.");
    }
    if (draft.serverUrl !== settings.value.server_url) {
      settings.value = await api.saveSettings(draft.serverUrl);
      note("Адрес сервера сохранён.");
      if (!settings.value.logged_in) {
        dimensions.value = [];
        settingsOpen.value = false;
        return;
      }
    }
    if (!scan.value?.needs_choice) settingsOpen.value = false;
  } catch (err) {
    settingsError.value = message(err);
  } finally {
    settingsBusy.value = false;
  }
}

async function logout() {
  settingsBusy.value = true;
  settingsError.value = "";
  try {
    settings.value = await api.logout();
    dimensions.value = [];
    settingsOpen.value = false;
    tab.value = "sync";
  } catch (err) {
    settingsError.value = message(err);
  } finally {
    settingsBusy.value = false;
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
    error.value = message(err);
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
  const payload = decisionsFrom(chosen.value, preview.value.songs, preview.value.plans);
  try {
    const report = await api.start(payload.songs, payload.plans);
    for (const line of report.notes) note(line);
    if (report.backups.length) note("Копии .bak сохранены рядом с изменёнными файлами.");
    if (settings.value.visio_path) scan.value = await api.openVisio(settings.value.visio_path);
    preview.value = await api.preview();
  } catch (err) {
    error.value = message(err);
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
    error.value = message(err);
  }
}

async function createPlan(name: string, items: PlanItem[]) {
  error.value = "";
  try {
    await api.createPlan(name, items);
    if (settings.value.visio_path) scan.value = await api.openVisio(settings.value.visio_path);
    note(`План «${name}» сохранён на этом компьютере.`);
  } catch (err) {
    error.value = message(err);
  }
}

async function search(query: string) {
  try {
    hits.value = await api.searchSongs(query);
  } catch (err) {
    error.value = message(err);
  }
}

async function checkUpdate() {
  updateBusy.value = true;
  updateError.value = "";
  try {
    updateInfo.value = await api.checkUpdate();
  } catch (err) {
    updateError.value = message(err);
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
    updateError.value = message(err);
  } finally {
    updateBusy.value = false;
  }
}

async function unsyncedNow(): Promise<boolean> {
  if (busy.value === "sync") return true;
  if (settings.value.logged_in && scan.value && !scan.value.needs_choice) {
    try {
      preview.value = await api.preview();
    } catch {
      // Остаётся последний удачный список.
    }
  }
  return hasUnsyncedChanges(preview.value);
}

async function onCloseRequested() {
  if (closeAsk.value || closeBusy.value) return;
  closeBusy.value = true;
  try {
    await startBoot();
    if (await unsyncedNow()) closeAsk.value = true;
    else await api.finishClose();
  } catch (err) {
    const text = message(err);
    if (settings.value.logged_in) error.value = text;
    else authError.value = text;
  } finally {
    closeBusy.value = false;
  }
}

function goSync() {
  closeAsk.value = false;
  settingsOpen.value = false;
  tab.value = "sync";
}

async function closeAnyway() {
  closeAsk.value = false;
  try {
    await api.finishClose();
  } catch (err) {
    const text = message(err);
    if (settings.value.logged_in) error.value = text;
    else authError.value = text;
  }
}

onMounted(() => {
  void startBoot();
  void listen<Progress>("sync-progress", (event) => {
    progress.value = event.payload;
  }).catch(() => undefined);
  void listen("close-requested", () => {
    void onCloseRequested();
  }).then(() => api.setCloseGuard(true).catch(() => undefined)).catch(() => undefined);
});
</script>

<template>
  <p v-if="!ready" class="hint boot">Открываем…</p>
  <LoginView
    v-else-if="!settings.logged_in"
    :server-url="settings.server_url"
    :email="settings.email"
    :error="authError"
    :busy="busy === 'auth'"
    @submit="enter"
  />
  <div v-else class="shell" :class="{ 'is-chat': tab === 'ai' }">
    <header class="topbar">
      <button class="brand" type="button" @click="tab = 'sync'">FlammeBible</button>
      <nav>
        <button class="nav-link" :class="{ on: tab === 'sync' }" type="button" @click="tab = 'sync'">Синхронизация</button>
        <button class="nav-link" :class="{ on: tab === 'plans' }" type="button" @click="tab = 'plans'">Планы</button>
        <button class="nav-link" :class="{ on: tab === 'updates' }" type="button" @click="tab = 'updates'">Обновление</button>
        <button class="nav-link" :class="{ on: tab === 'ai' }" type="button" @click="tab = 'ai'">AI Чат</button>
        <div class="nav-end">
          <span v-if="settings.email" class="who">{{ settings.email }}</span>
          <button class="gear" :class="{ on: settingsOpen }" type="button" aria-label="Настройки" @click="settingsOpen = !settingsOpen">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z" />
              <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9c.3.6.9 1 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z" />
            </svg>
          </button>
        </div>
      </nav>
    </header>
    <main>
      <SyncView
        v-if="tab === 'sync'"
        :scan="scan"
        :preview="preview"
        :progress="progress"
        :log="log"
        :error="error"
        :busy="busy"
        :conflict="conflict"
        :chosen="chosen"
        @refresh="refresh"
        @start="start"
        @open-conflict="conflict = $event"
        @resolve-plan="onPlanChoice"
        @repair="repairEncoding"
      />
      <PlansView v-else-if="tab === 'plans'" :scan="scan" :hits="hits" @create="createPlan" @search="search" @repair="repairEncoding" />
      <UpdatesView v-else-if="tab === 'updates'" :info="updateInfo" :error="updateError" :busy="updateBusy" :saved-path="savedPath" @check="checkUpdate" @download="downloadUpdate" />
      <AiChat v-else-if="tab === 'ai'" :logged-in="settings.logged_in" />
    </main>
    <SettingsModal
      v-if="settingsOpen"
      :settings="settings"
      :scan="scan"
      :dimensions="dimensions"
      :error="settingsError"
      :busy="settingsBusy"
      @close="settingsOpen = false"
      @logout="logout"
      @save="saveSettings"
    />
    <CloseDialog v-if="closeAsk" @close="closeAsk = false" @discard="closeAnyway" @sync="goSync" />
    <ConflictModal
      v-if="conflict"
      :row="conflict"
      @close="conflict = null"
      @choose="(kind, title, lyrics) => { if (conflict) remember(songKey(conflict.book_path, conflict.fbid), kind === 'manual' ? { kind, title, lyrics } : { kind }); conflict = null; }"
    />
  </div>
</template>
