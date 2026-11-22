<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import ConfirmDialog from "../../../web/shared/components/ConfirmDialog.vue";
import ItemTextModal from "../../../web/shared/components/ItemTextModal.vue";
import { planItemText } from "../../../web/src/planItems.js";
import SpaceFilter from "../../../web/shared/components/SpaceFilter.vue";
import { plansApi, explain, type ImportedPlan, type PlansApi } from "./api";
import DownloadModal from "./DownloadModal.vue";
import FileModal from "./FileModal.vue";
import ItemModal from "./ItemModal.vue";
import PlanCard from "./PlanCard.vue";
import {
  appendItems,
  badgeFor,
  clipPlanText,
  cloneItems,
  copyTitle,
  createDimension,
  importEncodingNotice,
  itemTitle,
  maxPlanItems,
  moveItem,
  planCounts,
  planInFolder,
  pointsLabel,
  snap,
  visiblePlans,
  type Community,
  type LocalMark,
  type Plan,
  type PlanItem,
  type SongbookGroup,
  type SongHit,
} from "./model";
import "./plans.css";

const props = defineProps<{ loggedIn: boolean; client?: PlansApi }>();
const client = computed(() => props.client ?? plansApi);
const viewing = ref<PlanItem | null>(null);

function viewText(item: PlanItem) {
  return planItemText(item, { song: client.value.songText, verse: client.value.verseText });
}

const plans = ref<Plan[]>([]);
const communities = ref<Community[]>([]);
const locals = ref<LocalMark[]>([]);
const space = ref("all");
const openId = ref("");
const editor = ref<Plan | null>(null);
const savedSnap = ref("");
const loading = ref(false);
const busy = ref(false);
const error = ref("");
const notice = ref("");
const itemOpen = ref(false);
const itemIndex = ref(-1);
const hits = ref<SongHit[]>([]);
const groups = ref<SongbookGroup[]>([]);
const searchError = ref("");
const searching = ref(false);
const fileMode = ref<"" | "import" | "append">("");
const fileError = ref("");
const downloadId = ref("");
const repair = ref<{ warning: string; before: string; after: string; items: PlanItem[]; title: string } | null>(null);
const confirmRef = ref<{ confirm: (options: Record<string, unknown>) => Promise<boolean | "save" | "discard" | false> } | null>(null);
const fileRef = ref<{ usePlan: (plan: Plan) => void; setParsed: (items: PlanItem[]) => void } | null>(null);
let writing = false;
let again = false;
let noticeTimer = 0;

const counts = computed(() => planCounts(plans.value));
const shown = computed(() => visiblePlans(plans.value, space.value));
const communityMissing = computed(() => !communities.value.length && space.value !== "all" && space.value !== "personal");
const editingItem = computed(() => (itemIndex.value >= 0 ? editor.value?.items[itemIndex.value] || null : null));
const dirty = computed(() => Boolean(editor.value) && snap(editor.value!) !== savedSnap.value);
const otherPlans = computed(() => plans.value
  .filter((plan) => plan.id !== (editor.value?.id || ""))
  .map((plan) => ({ id: plan.id, title: plan.title, badge: badgeFor(plan.dimension_id, communities.value) })));

function copyPlan(plan: Plan): Plan {
  return {
    id: plan.id,
    dimension_id: plan.dimension_id || null,
    title: plan.title || "",
    version: plan.version || 0,
    updated_at: plan.updated_at || "",
    items: (plan.items || []).map((item) => ({ ...item })),
  };
}

function showNotice(text: string) {
  notice.value = text;
  window.clearTimeout(noticeTimer);
  noticeTimer = window.setTimeout(() => {
    if (notice.value === text) notice.value = "";
  }, 3000);
}

function replacePlan(plan: Plan) {
  const next = copyPlan(plan);
  const index = plans.value.findIndex((item) => item.id === next.id);
  if (index >= 0) plans.value.splice(index, 1, next);
  else plans.value.unshift(next);
}

async function load() {
  if (!props.loggedIn) return;
  loading.value = true;
  error.value = "";
  try {
    plans.value = ((await client.value.list()) || []).map(copyPlan);
  } catch (err) {
    error.value = explain(err);
    loading.value = false;
    return;
  }
  try {
    communities.value = (await client.value.communities()) || [];
  } catch {
    communities.value = [];
  }
  try {
    locals.value = (await client.value.folder()) || [];
  } catch {
    locals.value = [];
  }
  loading.value = false;
}

async function writeOnce(): Promise<boolean> {
  const current = editor.value;
  if (!current) return false;
  const title = current.title.trim();
  if (!title) {
    error.value = "Введите название плана.";
    return false;
  }
  if (current.items.length > maxPlanItems) {
    error.value = "В плане может быть не больше 500 пунктов.";
    return false;
  }
  const shot = snap({ title, items: current.items });
  const result = await client.value.save({ ...current, title, items: current.items.map((item) => ({ ...item })) });
  if (result.conflict) {
    editor.value = copyPlan(result.plan);
    savedSnap.value = snap(editor.value);
    replacePlan(result.plan);
    error.value = "План уже изменили. Показана последняя версия — проверьте пункты и сохраните ещё раз.";
    again = false;
    return false;
  }
  replacePlan(result.plan);
  if (!editor.value || editor.value.id !== result.plan.id) return true;
  if (snap(editor.value) === shot) {
    editor.value = copyPlan(result.plan);
    savedSnap.value = snap(editor.value);
  } else {
    editor.value.version = result.plan.version;
    editor.value.updated_at = result.plan.updated_at;
  }
  return true;
}

async function persist(): Promise<boolean> {
  if (!editor.value) return false;
  if (writing) {
    again = true;
    return true;
  }
  writing = true;
  busy.value = true;
  error.value = "";
  try {
    let ok = true;
    do {
      again = false;
      ok = await writeOnce();
    } while (again && ok);
    return ok;
  } catch (err) {
    error.value = explain(err);
    return false;
  } finally {
    writing = false;
    busy.value = false;
  }
}

async function openPlan(plan: Plan) {
  busy.value = true;
  error.value = "";
  try {
    const full = copyPlan(await client.value.read(plan.id));
    editor.value = full;
    openId.value = full.id;
    savedSnap.value = snap(full);
    repair.value = null;
    replacePlan(full);
    return true;
  } catch (err) {
    error.value = explain(err);
    return false;
  } finally {
    busy.value = false;
  }
}

async function toggle(plan: Plan) {
  if (openId.value === plan.id) {
    if (dirty.value && !(await persist())) return;
    editor.value = null;
    openId.value = "";
    savedSnap.value = "";
    return;
  }
  if (dirty.value && !(await persist())) return;
  await openPlan(plan);
}

function onTitle(value: string) {
  if (editor.value) editor.value.title = value;
}

async function commitTitle() {
  if (!editor.value || snap(editor.value) === savedSnap.value) return;
  const ok = await persist();
  if (ok) showNotice("План сохранён на сервере.");
}

async function create() {
  busy.value = true;
  error.value = "";
  try {
    if (dirty.value && !(await persist())) return;
    const dimension = createDimension(space.value, communities.value.map((item) => item.id));
    const created = copyPlan(await client.value.create("Новый план", dimension, []));
    replacePlan(created);
    editor.value = created;
    openId.value = created.id;
    savedSnap.value = snap(created);
    showNotice("План сохранён на сервере.");
  } catch (err) {
    error.value = explain(err);
  } finally {
    busy.value = false;
  }
}

async function duplicate(plan: Plan) {
  busy.value = true;
  error.value = "";
  try {
    if (dirty.value && !(await persist())) return;
    const full = await client.value.read(plan.id);
    const created = copyPlan(await client.value.create(copyTitle(full.title), full.dimension_id || "", cloneItems(full.items || [])));
    replacePlan(created);
    editor.value = created;
    openId.value = created.id;
    savedSnap.value = snap(created);
    showNotice("План сохранён на сервере.");
  } catch (err) {
    error.value = explain(err);
  } finally {
    busy.value = false;
  }
}

async function remove(plan: Plan) {
  const source = editor.value?.id === plan.id ? editor.value : plan;
  const name = source.title.trim() || "План";
  const text = source.dimension_id
    ? `Удалить план «${name}» из общины «${badgeFor(source.dimension_id, communities.value)}»? В нём ${pointsLabel(source.items.length)}. План пропадёт у всех участников общины и на компьютерах после следующей синхронизации.`
    : `Удалить план «${name}»? В нём ${pointsLabel(source.items.length)}. План пропадёт на компьютерах после следующей синхронизации.`;
  const ok = await confirmRef.value?.confirm({ title: "Удалить план?", text, action: "Удалить" });
  if (!ok) return;
  busy.value = true;
  error.value = "";
  try {
    const result = await client.value.remove(source);
    if (result.conflict) {
      editor.value = copyPlan(result.plan);
      openId.value = result.plan.id;
      replacePlan(result.plan);
      error.value = "План уже изменили. Удаление не выполнено.";
      return;
    }
    plans.value = plans.value.filter((item) => item.id !== source.id);
    if (editor.value?.id === source.id) {
      editor.value = null;
      openId.value = "";
    }
    showNotice("План удалён.");
  } catch (err) {
    error.value = explain(err);
  } finally {
    busy.value = false;
  }
}

function changeItems(items: PlanItem[]) {
  if (!editor.value) return;
  editor.value.items = items;
  void persist().then((ok) => {
    if (ok) showNotice("План сохранён на сервере.");
  });
}

function addItem(item: PlanItem) {
  if (!editor.value) return;
  changeItems([...editor.value.items, item]);
}

function changeItem(item: PlanItem) {
  if (!editor.value) return;
  const index = itemIndex.value;
  const prev = editor.value.items[index];
  if (!prev) return;
  const next = editor.value.items.slice();
  next.splice(index, 1, { ...item, id: prev.id, created_at: prev.created_at, updated_at: prev.updated_at });
  changeItems(next);
}

async function removeItem(index: number) {
  if (!editor.value) return;
  const item = editor.value.items[index];
  if (!item) return;
  const ok = await confirmRef.value?.confirm({
    title: "Убрать пункт?",
    text: `Убрать пункт №${index + 1} «${itemTitle(item)}»? Пункт пропадёт на сервере.`,
    action: "Убрать",
  });
  if (!ok) return;
  const next = editor.value.items.slice();
  next.splice(index, 1);
  changeItems(next);
}

async function ensureBooks() {
  if (groups.value.length) return;
  try {
    groups.value = await client.value.songbooks();
  } catch (err) {
    searchError.value = explain(err);
  }
}

async function openAdd() {
  itemIndex.value = -1;
  hits.value = [];
  searchError.value = "";
  itemOpen.value = true;
  await ensureBooks();
}

async function openEdit(index: number) {
  itemIndex.value = index;
  searchError.value = "";
  itemOpen.value = true;
  await ensureBooks();
}

async function onSearch(query: string, songbookId: string, sectionId: string) {
  searching.value = true;
  searchError.value = "";
  try {
    hits.value = await client.value.findSongs(query, songbookId, sectionId);
  } catch (err) {
    hits.value = [];
    searchError.value = explain(err);
  } finally {
    searching.value = false;
  }
}

function openDownload(plan: Plan) {
  downloadId.value = plan.id;
}

async function onDownload(choice: { format: string; encoding: string; saveFirst: boolean }) {
  if (choice.saveFirst && !(await persist())) return;
  const id = downloadId.value;
  if (!id) return;
  try {
    const wrote = await client.value.download(id, choice.format, choice.encoding);
    if (wrote) {
      downloadId.value = "";
      showNotice("Файл плана сохранён.");
    }
  } catch (err) {
    error.value = explain(err);
  }
}

async function openAppend(plan: Plan) {
  if (openId.value !== plan.id && !(await openPlan(plan))) return;
  fileError.value = "";
  fileMode.value = "append";
}

function openImport() {
  fileError.value = "";
  fileMode.value = "import";
}

async function onImport(body: { format: string; payload: string; title: string; dimension_id: string }) {
  if (dirty.value && !(await persist())) return;
  busy.value = true;
  fileError.value = "";
  try {
    const created = await client.value.importPlan(body) as ImportedPlan;
    const noticeBody = importEncodingNotice(body.payload, created);
    repair.value = noticeBody;
    fileMode.value = "";
    replacePlan(created);
    editor.value = copyPlan(created);
    openId.value = created.id;
    savedSnap.value = snap(editor.value);
    showNotice(repair.value ? "План импортирован. Проверьте кодировку." : "План импортирован.");
  } catch (err) {
    fileError.value = explain(err);
  } finally {
    busy.value = false;
  }
}

async function onLoadPlan(id: string) {
  fileError.value = "";
  try {
    fileRef.value?.usePlan(copyPlan(await client.value.read(id)));
  } catch (err) {
    fileError.value = explain(err);
  }
}

async function onParse(format: string, payload: string) {
  if (!format) {
    fileError.value = "Не удалось определить формат. Выберите его в списке.";
    return;
  }
  fileError.value = "";
  try {
    const parsed = await client.value.parse(format, payload);
    fileRef.value?.setParsed(parsed.items || []);
  } catch (err) {
    fileError.value = explain(err);
  }
}

function onAppend(payload: { items: PlanItem[]; kinds: string[]; skipSongs: boolean }) {
  if (!editor.value) return;
  const next = appendItems(editor.value.items, payload.items, payload);
  fileMode.value = "";
  if (next.length > maxPlanItems) {
    error.value = "В плане может быть не больше 500 пунктов.";
    return;
  }
  changeItems(next);
}

async function applyRepair() {
  if (!editor.value || !repair.value?.items.length) return;
  editor.value.title = repair.value.title || editor.value.title;
  editor.value.items = repair.value.items.map((item) => ({ ...item }));
  const ok = await persist();
  if (ok) {
    repair.value = null;
    showNotice("Кодировка исправлена.");
  }
}

function onKey(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "s") {
    event.preventDefault();
    void commitTitle();
  }
}

onMounted(() => {
  window.addEventListener("keydown", onKey);
  if (props.loggedIn) void load();
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKey);
  window.clearTimeout(noticeTimer);
});
</script>

<template>
  <section class="plans-page">
    <div class="page-head">
      <h1>Планы</h1>
      <div class="plan-actions">
        <button class="btn slim" type="button" :disabled="!loggedIn || busy" @click="openImport">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m17 8-5-5-5 5"/><path d="M12 3v12"/></svg>
          Импортировать план
        </button>
        <button class="btn slim primary" type="button" :disabled="!loggedIn || busy" @click="create">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12h14"/><path d="M12 5v14"/></svg>
          Создать план
        </button>
      </div>
    </div>
    <p class="hint">Правки сохраняются на сервере, как на сайте. В папку VisioBible их записывает только вкладка «Синхронизация».</p>
    <p v-if="!loggedIn" class="empty">Сначала войдите в учётную запись на вкладке «Синхронизация».</p>
    <template v-else>
      <SpaceFilter
        :model-value="space"
        :communities="communities"
        :counts="counts"
        show-counts
        @update:model-value="space = $event"
      />
      <p v-if="error && !editor" class="banner bad" role="alert">{{ error }}</p>
      <p v-if="notice && !editor" class="banner ok" role="status">{{ notice }}</p>
      <p v-if="loading" class="empty" role="status">Загрузка…</p>
      <div v-else-if="communityMissing" class="empty-state">
        <p>Вы пока не состоите в общине.</p>
      </div>
      <div v-else class="plan-list">
        <div v-if="repair && openId" class="banner bad" role="alert">
          <p>{{ repair.warning }}</p>
          <p class="hint">До</p>
          <p class="quote">{{ clipPlanText(repair.before) }}</p>
          <p class="hint">После</p>
          <p class="quote">{{ clipPlanText(repair.after) }}</p>
          <button class="btn slim" type="button" :disabled="busy" @click="applyRepair">Починить кодировку</button>
        </div>
        <p v-if="!shown.length" class="empty-state">Планов пока нет.</p>
        <PlanCard
          v-for="plan in shown"
          :key="plan.id"
          :open="openId === plan.id"
          :title="openId === plan.id && editor ? editor.title : plan.title"
          :badge="badgeFor(openId === plan.id && editor ? editor.dimension_id : plan.dimension_id, communities)"
          :count="openId === plan.id && editor ? editor.items.length : plan.items.length"
          :updated-at="openId === plan.id && editor ? editor.updated_at : plan.updated_at"
          :in-folder="planInFolder(plan, locals)"
          :busy="busy"
          :error="openId === plan.id ? error : ''"
          :notice="openId === plan.id ? notice : ''"
          :items="openId === plan.id && editor ? editor.items : []"
          @toggle="toggle(plan)"
          @title="onTitle"
          @commit="commitTitle"
          @download="openDownload(plan)"
          @append="openAppend(plan)"
          @duplicate="duplicate(plan)"
          @remove="remove(plan)"
          @add="openAdd"
          @edit="openEdit"
          @view="(index) => (viewing = editor?.items[index] ?? null)"
          @move="(index, delta) => editor && changeItems(moveItem(editor.items, index, delta))"
          @remove-item="removeItem"
        />
      </div>
    </template>
    <ItemModal
      v-if="itemOpen && editor"
      :item="editingItem"
      :groups="groups"
      :hits="hits"
      :search-error="searchError"
      :searching="searching"
      @close="itemOpen = false"
      @add="addItem"
      @change="changeItem"
      @search="onSearch"
    />
    <FileModal
      v-if="fileMode"
      ref="fileRef"
      :mode="fileMode"
      :plans="otherPlans"
      :current-items="editor?.items || []"
      :destinations="communities"
      :destination="createDimension(space, communities.map((item) => item.id))"
      :error-text="fileError"
      :busy="busy"
      @close="fileMode = ''"
      @import="onImport"
      @append="onAppend"
      @load-plan="onLoadPlan"
      @parse="onParse"
    />
    <DownloadModal
      v-if="downloadId"
      :dirty="Boolean(downloadId === openId && dirty)"
      @close="downloadId = ''"
      @download="onDownload"
    />
    <ItemTextModal v-if="viewing" :title="viewing.title || viewing.caption || 'Пункт'" :load="() => viewText(viewing!)" @close="viewing = null" />
    <ConfirmDialog ref="confirmRef" />
  </section>
</template>
