<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { open, save } from "@tauri-apps/plugin-dialog";
import {
  api,
  type Dimension,
  type PlanItem,
  type PlanLink,
  type RemotePlan,
  type Scan,
  type SongHit,
} from "../commands";
import {
  clonePlanItems,
  deletePrompt,
  deleteScope,
  duplicateTitle,
  exportExtension,
  exportFileName,
  fileLabel,
  importFormatFromName,
  planFormats,
  pointsLabel,
  type PlanFormat,
} from "../planActions";
import EncodingNotice from "./EncodingNotice.vue";

const props = defineProps<{
  scan: Scan | null;
  hits: SongHit[];
  windows1251: boolean;
  loggedIn: boolean;
  dimensions: Dimension[];
  error: string;
}>();
const emit = defineEmits<{
  create: [name: string, items: PlanItem[]];
  search: [string];
  repair: [string];
  setEncoding: [boolean];
  changed: [];
}>();

const name = ref("");
const query = ref("");
const items = reactive<PlanItem[]>([]);
const localError = ref("");
const notice = ref("");
const busy = ref(false);
const links = ref<PlanLink[]>([]);
const serverPlans = ref<RemotePlan[]>([]);
const pending = ref<{ title: string; text: string; run: () => Promise<void> } | null>(null);
const draft = ref<{ path: string; format: PlanFormat | ""; title: string; dimensionId: string } | null>(null);
const exporting = ref<{ id: string; title: string; items: PlanItem[]; format: PlanFormat } | null>(null);

const communities = computed(() => props.dimensions.filter((item) => item.kind === "community"));
const localNames = computed(() => (props.scan?.plans ?? []).map((plan) => plan.name));

function blank(kind: PlanItem["kind"]): PlanItem {
  return {
    kind,
    title: "",
    song_id: "",
    songbook_id: "",
    caption: "",
    quote: "",
    reference: "",
    class: "-1",
    module_short_name: "",
    module_short_name_second: "",
    book: 0,
    chapter: 0,
    verse: 0,
  };
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function linkForPath(path: string): PlanLink | undefined {
  return links.value.find((link) => link.path === path && link.server_id);
}

function linkForServer(id: string): PlanLink | undefined {
  return links.value.find((link) => link.server_id === id && link.path);
}

function placeName(dimensionId: string): string {
  if (!dimensionId) return "";
  return props.dimensions.find((item) => item.id === dimensionId)?.name || "Община";
}

async function run(action: () => Promise<void>) {
  localError.value = "";
  notice.value = "";
  busy.value = true;
  try {
    await action();
  } catch (err) {
    localError.value = message(err);
  } finally {
    busy.value = false;
  }
}

async function refreshServer() {
  links.value = await api.planLinks();
  serverPlans.value = props.loggedIn ? await api.listPlans() : [];
}

async function pickImport() {
  localError.value = "";
  const picked = await open({
    directory: false,
    multiple: false,
    filters: [{ name: "План", extensions: ["json", "vplt", "ini", "txt"] }],
  });
  if (typeof picked !== "string") return;
  draft.value = {
    path: picked,
    format: importFormatFromName(picked),
    title: "",
    dimensionId: "",
  };
  exporting.value = null;
}

async function submitImport() {
  const current = draft.value;
  if (!current) return;
  if (!current.format) {
    localError.value = "Не удалось определить формат. Выберите его в списке.";
    return;
  }
  const format = current.format;
  await run(async () => {
    const payload = await api.readPlanFile(current.path);
    await api.importPlan(format, payload, current.title.trim(), current.dimensionId);
    draft.value = null;
    notice.value = "План импортирован.";
    await refreshServer();
  });
}

function openExport(id: string, title: string, planItems: PlanItem[]) {
  localError.value = "";
  exporting.value = { id, title, items: planItems, format: "vplt" };
  draft.value = null;
}

async function submitExport() {
  const current = exporting.value;
  if (!current) return;
  await run(async () => {
    const text = current.id
      ? await api.exportPlan(current.id, current.format)
      : await api.renderPlanExport(current.format, current.title, current.items);
    const picked = await save({
      defaultPath: exportFileName(current.format),
      filters: [{ name: "План", extensions: [exportExtension(current.format)] }],
    });
    if (typeof picked !== "string") return;
    await api.writePlanExport(picked, text);
    exporting.value = null;
    notice.value = "План сохранён в файл.";
  });
}

function saveNew() {
  localError.value = "";
  notice.value = "";
  emit("create", name.value, items);
}

async function duplicate(title: string, planItems: PlanItem[]) {
  const next = duplicateTitle(title, localNames.value);
  await run(async () => {
    await api.createPlan(next, clonePlanItems(planItems));
    notice.value = `План «${next}» сохранён на этом компьютере.`;
    emit("changed");
    await refreshServer();
  });
}

function openLocalExport(plan: Scan["plans"][number]) {
  const link = props.loggedIn ? linkForPath(plan.path) : undefined;
  openExport(link?.server_id || "", plan.name, plan.items);
}

function pullLinked(plan: Scan["plans"][number]) {
  const link = linkForPath(plan.path);
  if (!link) return;
  const remote = serverPlans.value.find((item) => item.id === link.server_id);
  return pull({
    id: link.server_id,
    title: remote?.title || plan.name,
    version: remote?.version || link.version,
    updated_at: remote?.updated_at || link.updated_at,
    deleted: false,
    dimension_id: remote?.dimension_id || link.dimension_id || undefined,
    items: remote?.items?.length ? remote.items : plan.items,
  });
}

function deleteLocal(plan: Scan["plans"][number]) {
  const link = linkForPath(plan.path);
  const remote = link ? serverPlans.value.find((item) => item.id === link.server_id) : undefined;
  askDelete({
    name: plan.name,
    path: plan.path,
    serverId: link?.server_id || "",
    version: remote?.version || link?.version || 0,
    count: plan.items.length,
    place: placeName(remote?.dimension_id || link?.dimension_id || ""),
  });
}

function deleteRemote(plan: RemotePlan) {
  const link = linkForServer(plan.id);
  askDelete({
    name: plan.title,
    path: link?.path || "",
    serverId: plan.id,
    version: plan.version,
    count: plan.items.length,
    place: placeName(plan.dimension_id || ""),
  });
}

function askDelete(target: { name: string; path: string; serverId: string; version: number; count: number; place: string }) {
  const scope = deleteScope(target.path, target.serverId, props.loggedIn);
  const prompt = deletePrompt({
    name: target.name,
    count: target.count,
    scope,
    place: target.place,
    keptOnServer: scope === "local" && target.serverId.trim() !== "",
  });
  pending.value = {
    title: prompt.title,
    text: prompt.text,
    run: async () => {
      if (scope === "server" || scope === "both") {
        if (target.version < 1) {
          throw new Error("У плана на сервере нет версии. Обновите список.");
        }
        await api.deletePlan(target.serverId, target.version);
      }
      if ((scope === "local" || scope === "both") && target.path) {
        await api.deleteLocalPlan(target.path);
        emit("changed");
      }
      notice.value = "План удалён.";
      await refreshServer();
    },
  };
}

async function runPending() {
  const job = pending.value;
  if (!job) return;
  pending.value = null;
  await run(job.run);
}

async function pull(source: RemotePlan) {
  if (source.version < 1) {
    localError.value = "У плана на сервере нет версии. Обновите список.";
    return;
  }
  await run(async () => {
    const result = await api.pullPlan(source.id, source.version, source.updated_at || "");
    const fresh = result.plan ?? source;
    const local = linkForServer(source.id);
    if (!result.changed && local && !fresh.deleted) {
      notice.value = "План уже актуален.";
      return;
    }
    if (fresh.deleted) {
      if (local) {
        await api.deleteLocalPlan(local.path);
        emit("changed");
      }
      notice.value = "План на сервере удалён.";
      await refreshServer();
      return;
    }
    if (result.changed && !result.plan) {
      throw new Error("Сервер не прислал план.");
    }
    await api.storePulledPlan(fresh);
    emit("changed");
    notice.value = `План «${fresh.title || "План"}» записан на этот компьютер.`;
    await refreshServer();
  });
}

onMounted(() => {
  void refreshServer().catch((err) => {
    localError.value = message(err);
  });
});

watch(() => props.loggedIn, () => {
  void refreshServer().catch((err) => {
    localError.value = message(err);
  });
});
</script>

<template>
  <section class="card" style="margin-top: 1rem">
    <h2>Планы</h2>
    <p v-if="localError || error" class="error" role="alert">{{ localError || error }}</p>
    <p v-if="notice" class="hint" role="status">{{ notice }}</p>
    <div v-if="loggedIn" class="row">
      <button class="btn" type="button" :disabled="busy" @click="pickImport">Импортировать план</button>
      <button class="btn" type="button" :disabled="busy" @click="run(refreshServer)">Обновить список</button>
    </div>
    <p v-else class="hint">Войдите на вкладке синхронизации, чтобы импортировать план, скачать его или забрать с сервера.</p>

    <article v-if="draft" class="conflict-card">
      <strong>Импортировать план</strong>
      <p class="hint">{{ fileLabel(draft.path) }}</p>
      <label>Формат
        <select v-model="draft.format">
          <option value="">Выберите формат</option>
          <option v-for="format in planFormats" :key="format.id" :value="format.id">{{ format.label }}</option>
        </select>
      </label>
      <label>Название
        <input v-model="draft.title" placeholder="Возьмём из файла" />
      </label>
      <label>Куда
        <select v-model="draft.dimensionId">
          <option value="">Личный</option>
          <option v-for="space in communities" :key="space.id" :value="space.id">{{ space.name || "Община" }}</option>
        </select>
      </label>
      <div class="row">
        <button class="btn" type="button" :disabled="busy" @click="draft = null">Отмена</button>
        <button class="btn primary" type="button" :disabled="busy" @click="submitImport">Импортировать</button>
      </div>
    </article>

    <article v-if="exporting" class="conflict-card">
      <strong>Скачать план</strong>
      <p class="hint">{{ exporting.title || "План" }}</p>
      <label>Формат
        <select v-model="exporting.format">
          <option v-for="format in planFormats" :key="format.id" :value="format.id">{{ format.label }}</option>
        </select>
      </label>
      <p class="hint">{{ exporting.id ? "Скачается версия с сервера." : "Скачается план с этого компьютера." }}</p>
      <div class="row">
        <button class="btn" type="button" :disabled="busy" @click="exporting = null">Отмена</button>
        <button class="btn primary" type="button" :disabled="busy" @click="submitExport">Скачать</button>
      </div>
    </article>

    <article v-if="pending" class="conflict-card">
      <strong>{{ pending.title }}</strong>
      <p>{{ pending.text }}</p>
      <div class="row">
        <button class="btn" type="button" :disabled="busy" @click="pending = null">Отмена</button>
        <button class="btn primary" type="button" :disabled="busy" @click="runPending">Удалить</button>
      </div>
    </article>

    <p v-if="!scan || scan.needs_choice" class="empty">Сначала на вкладке синхронизации выберите папку VisioBible.</p>
    <template v-else>
      <p class="hint">Планы уйдут на сервер и скачаются оттуда по кнопке «Начать синхронизацию». Файл plan.vplt хранит только подпись и текст: перевод и номер стиха VisioBible в нём не сохраняет, для них используется History или PlanDef.</p>
      <EncodingNotice :plans="scan.plans" @repair="emit('repair', $event)" />
      <label>Кодировка записи планов и PlanDef.ini
        <select :value="windows1251 ? 'windows-1251' : 'utf-8'" @change="emit('setEncoding', ($event.target as HTMLSelectElement).value === 'windows-1251')">
          <option value="utf-8">UTF-8</option>
          <option value="windows-1251">Windows-1251</option>
        </select>
      </label>
      <article v-for="plan in scan.plans" :key="plan.path" class="conflict-card">
        <strong>{{ plan.name }}</strong>
        <p class="hint">{{ plan.path }}</p>
        <p v-if="linkForPath(plan.path)" class="hint">Есть на сервере</p>
        <ul class="list">
          <li v-for="(item, index) in plan.items" :key="index">
            <template v-if="item.kind === 'song'">Песня «{{ item.title || item.caption }}»</template>
            <template v-else-if="item.kind === 'bible'">{{ item.module_short_name || "Перевод" }} {{ item.book }}:{{ item.chapter }}:{{ item.verse }} {{ item.caption }}</template>
            <template v-else>Текст «{{ item.caption || item.quote }}»</template>
          </li>
        </ul>
        <div class="row">
          <button class="btn" type="button" :disabled="busy" @click="openLocalExport(plan)">Скачать план</button>
          <button class="btn" type="button" :disabled="busy" @click="duplicate(plan.name, plan.items)">Дублировать план</button>
          <button v-if="loggedIn && linkForPath(plan.path)" class="btn" type="button" :disabled="busy" @click="pullLinked(plan)">Забрать на компьютер</button>
          <button class="btn" type="button" :disabled="busy" @click="deleteLocal(plan)">Удалить план</button>
        </div>
      </article>
      <h3 style="margin-top: 1rem">Новый план</h3>
      <label>Название<input v-model="name" /></label>
      <div class="row">
        <button class="btn" type="button" @click="items.push(blank('song'))">Добавить песню</button>
        <button class="btn" type="button" @click="items.push(blank('bible'))">Добавить место Писания</button>
        <button class="btn" type="button" @click="items.push(blank('text'))">Добавить текст</button>
      </div>
      <article v-for="(item, index) in items" :key="index" class="conflict-card">
        <template v-if="item.kind === 'song'">
          <label>Название песни<input v-model="item.title" /></label>
          <label>Сборник<input v-model="item.songbook_id" /></label>
        </template>
        <template v-else-if="item.kind === 'bible'">
          <div class="grid">
            <label>Перевод<input v-model="item.module_short_name" :placeholder="scan.plan_def.module_short_name || 'RST+'" /></label>
            <label>Второй перевод<input v-model="item.module_short_name_second" /></label>
            <label>Номер книги<input v-model.number="item.book" type="number" min="0" /></label>
            <label>Глава<input v-model.number="item.chapter" type="number" min="0" /></label>
            <label>Стих<input v-model.number="item.verse" type="number" min="0" /></label>
            <label>Подпись<input v-model="item.caption" /></label>
          </div>
        </template>
        <template v-else>
          <label>Заголовок<input v-model="item.caption" /></label>
          <label>Текст слайда<textarea v-model="item.quote" /></label>
        </template>
        <button class="btn" type="button" @click="items.splice(index, 1)">Убрать пункт</button>
      </article>
      <div class="row">
        <label>Найти песню на сервере<input v-model="query" /></label>
        <button class="btn" type="button" @click="emit('search', query)">Найти</button>
      </div>
      <ul class="list">
        <li v-for="hit in hits" :key="hit.songbook_id + hit.fbid">
          {{ hit.title }} — {{ hit.songbook_title }}
          <button class="btn" type="button" @click="items.push({ ...blank('song'), title: hit.title, song_id: hit.fbid, songbook_id: hit.songbook_id })">Добавить</button>
        </li>
      </ul>
      <button class="btn primary" type="button" @click="saveNew">Сохранить план на этом компьютере</button>
    </template>

    <template v-if="loggedIn">
      <h3 style="margin-top: 1rem">На сервере</h3>
      <p v-if="!serverPlans.length" class="empty">Планов на сервере нет.</p>
      <article v-for="plan in serverPlans" :key="plan.id" class="conflict-card">
        <strong>{{ plan.title || "План" }}</strong>
        <p class="hint">{{ pointsLabel(plan.items.length) }}<template v-if="placeName(plan.dimension_id || '')"> · {{ placeName(plan.dimension_id || '') }}</template></p>
        <p v-if="linkForServer(plan.id)" class="hint">Есть на этом компьютере</p>
        <div class="row">
          <button class="btn" type="button" :disabled="busy" @click="openExport(plan.id, plan.title, plan.items)">Скачать план</button>
          <button class="btn" type="button" :disabled="busy" @click="duplicate(plan.title, plan.items)">Дублировать план</button>
          <button class="btn" type="button" :disabled="busy" @click="pull(plan)">Забрать на компьютер</button>
          <button class="btn" type="button" :disabled="busy" @click="deleteRemote(plan)">Удалить план</button>
        </div>
      </article>
    </template>
  </section>
</template>
