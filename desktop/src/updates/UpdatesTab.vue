<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { listen } from "@tauri-apps/api/event";
import { updatesApi, type DownloadDone, type UpdateOffer, type UpdateProgress } from "./api";
import { formatSize, shortHash } from "./format";

const offer = ref<UpdateOffer | null>(null);
const error = ref("");
const phase = ref<"checking" | "ready" | "downloading">("checking");
const progress = ref(0);
const installing = ref(false);
let stop: (() => void) | undefined;

const sizeLabel = computed(() => (offer.value ? formatSize(offer.value.size) : ""));
const short = computed(() => (offer.value ? shortHash(offer.value.sha256) : ""));
const showDownload = computed(() => !!offer.value?.available && !offer.value.staged && !offer.value.armed);
const showInstall = computed(
  () => !!offer.value?.available && offer.value.portable && offer.value.staged && !offer.value.armed,
);

const intro = computed(() => {
  if (!offer.value) return "Версия ещё не проверена.";
  const current = `Сейчас установлена версия ${offer.value.current}.`;
  if (offer.value.portable) {
    return `${current} Файл обновления проверяется по SHA-256 и сохраняется рядом с программой. Замена — только по кнопке, при следующем запуске.`;
  }
  return `${current} Файл обновления проверяется по SHA-256 и сохраняется в папку программы; его нужно заменить вручную.`;
});

const foot = computed(() => {
  const info = offer.value;
  if (!info) return "";
  if (info.armed) return "Замена назначена. Закройте программу и откройте её снова — проверенный файл встанет на место этой копии.";
  if (info.staged && info.portable) return "Файл проверен и лежит рядом с программой. Нажмите «Заменить при следующем запуске».";
  if (info.staged) return `Файл проверен и сохранён: ${info.staged_path}. Закройте программу и замените её этим файлом.`;
  if (info.portable) return "После проверки нажмите «Заменить при следующем запуске» и откройте программу снова.";
  return "После проверки закройте программу и замените её сохранённым файлом.";
});

function text(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function previewMode(): string {
  if (!import.meta.env.DEV) return "";
  return new URLSearchParams(location.search).get("updates-preview") ?? "";
}

function previewOffer(mode: string): UpdateOffer {
  const sha = "2c4f8a91b0d3e7f6a1c0b9e8d7c6b5a4".repeat(2);
  const base: UpdateOffer = {
    current: "0.1.0",
    latest: "0.2.0",
    notes: "Исправлена кнопка синхронизации после выбора конфликта и порядок песен с сервера.",
    available: true,
    platform: "windows",
    platform_label: "Windows",
    size: 18 * 1024 * 1024,
    sha256: sha,
    portable: true,
    staged: mode === "ready" || mode === "armed",
    armed: mode === "armed",
    staged_path: mode === "ready" || mode === "armed" ? "C:\\FlammeBible\\FlammeBible.exe.new" : "",
  };
  if (mode === "current") {
    return { ...base, current: "0.2.0", available: false, staged: false, armed: false, staged_path: "" };
  }
  return base;
}

function applyPreview(mode: string) {
  offer.value = previewOffer(mode);
  error.value = mode === "checksum" ? "Файл обновления не совпал с контрольной суммой и не сохранён." : "";
  progress.value = mode === "progress" ? 42 : 0;
  phase.value = mode === "progress" ? "downloading" : "ready";
}

async function check() {
  if (previewMode()) {
    applyPreview(previewMode());
    return;
  }
  phase.value = "checking";
  error.value = "";
  try {
    offer.value = await updatesApi.check();
  } catch (err) {
    offer.value = null;
    error.value = text(err);
  } finally {
    if (phase.value === "checking") phase.value = "ready";
  }
}

async function onDownload() {
  error.value = "";
  if (previewMode()) {
    phase.value = "downloading";
    progress.value = 35;
    await new Promise((resolve) => setTimeout(resolve, 250));
    if (previewMode() === "checksum") {
      phase.value = "ready";
      error.value = "Файл обновления не совпал с контрольной суммой и не сохранён.";
      return;
    }
    progress.value = 100;
    phase.value = "ready";
    if (offer.value) {
      offer.value = { ...offer.value, staged: true, staged_path: "C:\\FlammeBible\\FlammeBible.exe.new", armed: false };
    }
    return;
  }
  phase.value = "downloading";
  progress.value = 0;
  try {
    const done: DownloadDone = await updatesApi.download();
    if (offer.value) {
      offer.value = { ...offer.value, staged: true, staged_path: done.path, portable: done.portable, armed: false };
    }
  } catch (err) {
    error.value = text(err);
    if (offer.value) offer.value = { ...offer.value, staged: false, armed: false, staged_path: "" };
  } finally {
    phase.value = "ready";
  }
}

async function onInstall() {
  error.value = "";
  if (previewMode()) {
    if (offer.value) offer.value = { ...offer.value, armed: true };
    return;
  }
  installing.value = true;
  try {
    await updatesApi.install();
    if (offer.value) offer.value = { ...offer.value, armed: true };
  } catch (err) {
    error.value = text(err);
  } finally {
    installing.value = false;
  }
}

onMounted(async () => {
  const mode = previewMode();
  if (mode) {
    applyPreview(mode);
    return;
  }
  try {
    const unlisten = await listen<UpdateProgress>("update-progress", (event) => {
      progress.value = event.payload.percentage;
    });
    stop = unlisten;
  } catch {
    stop = undefined;
  }
  await check();
});

onUnmounted(() => stop?.());
</script>

<template>
  <section class="updates">
    <div class="page-head"><h1>Обновление программы</h1></div>
    <p v-if="phase === 'checking'" class="hint">Проверяем сервер…</p>
    <template v-else>
      <p class="hint">{{ intro }}</p>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p v-if="!offer" class="row">
        <button class="btn" type="button" @click="check">Проверить снова</button>
      </p>
      <section v-else-if="!offer.available" class="card">
        <p class="badge quiet">Актуальная версия</p>
        <h2 class="version">{{ offer.latest || offer.current }}</h2>
        <p v-if="offer.notes" class="hint notes">{{ offer.notes }}</p>
        <p class="hint">Новой версии нет.</p>
      </section>
      <section v-else class="card">
        <p class="badge">Есть новая версия</p>
        <h2 class="version">{{ offer.latest }}</h2>
        <p v-if="offer.notes" class="hint notes">{{ offer.notes }}</p>
        <div v-if="offer.sha256" class="download-row">
          <div>
            <strong>{{ offer.platform_label }}</strong>
            <p class="hint flat">Эта система<span v-if="sizeLabel"> · {{ sizeLabel }}</span></p>
            <p class="mono" :title="offer.sha256">SHA-256 {{ short }}</p>
          </div>
          <button v-if="showDownload" class="btn primary" type="button" :disabled="phase === 'downloading'" @click="onDownload">
            Скачать и проверить
          </button>
          <button v-else-if="showInstall" class="btn primary" type="button" :disabled="installing" @click="onInstall">
            Заменить при следующем запуске
          </button>
        </div>
        <div
          v-if="phase === 'downloading'"
          class="bar"
          role="progressbar"
          :aria-valuenow="progress"
          aria-valuemin="0"
          aria-valuemax="100"
        >
          <span :style="{ width: progress + '%' }"></span>
        </div>
        <p v-if="phase === 'downloading'" class="hint">Скачано {{ progress }}%</p>
        <p v-else-if="!error" class="hint">{{ foot }}</p>
      </section>
    </template>
  </section>
</template>

<style scoped>
.updates { margin-top: 1rem; }
.page-head h1 { font-size: 1.7rem; margin: 0 0 0.35rem; }
.badge {
  display: inline-block;
  margin: 0;
  padding: 0.12rem 0.55rem;
  border-radius: 999px;
  background: var(--navy);
  color: #fff;
  font-size: 0.78rem;
}
.badge.quiet {
  background: transparent;
  color: var(--navy);
  border: 1px solid var(--gold);
}
.version { margin: 0.45rem 0 0.35rem; font-size: 1.35rem; }
.notes { white-space: pre-wrap; }
.download-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  margin-top: 0.7rem;
  padding-top: 0.7rem;
  border-top: 1px solid var(--line);
}
.download-row .btn { flex: 0 0 auto; }
.flat { margin: 0; }
.mono {
  font-family: ui-monospace, "DejaVu Sans Mono", monospace;
  font-size: 0.78rem;
  color: #5c5348;
  word-break: break-all;
  margin: 0.15rem 0 0;
}
.bar { margin-top: 0.7rem; }
@media (max-width: 700px) {
  .download-row { flex-direction: column; align-items: stretch; }
}
</style>
