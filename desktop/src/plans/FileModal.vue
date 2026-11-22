<script setup lang="ts">
import { computed, ref, watch } from "vue";
import AppModal from "../../../web/shared/components/AppModal.vue";
import {
  appendItems,
  countByKind,
  importFormatFromName,
  maxPlanItems,
  readPlanFile,
  type Community,
  type Plan,
  type PlanItem,
} from "./model";

const props = defineProps<{
  mode: "import" | "append";
  plans: { id: string; title: string; badge: string }[];
  currentItems: PlanItem[];
  destinations: Community[];
  destination: string;
  errorText: string;
  busy: boolean;
}>();

const emit = defineEmits<{
  close: [];
  import: [body: { format: string; payload: string; title: string; dimension_id: string }];
  append: [payload: { items: PlanItem[]; kinds: string[]; skipSongs: boolean }];
  loadPlan: [id: string];
  parse: [format: string, payload: string];
}>();

const tab = ref<"file" | "plan">("file");
const format = ref("auto");
const encoding = ref("auto");
const title = ref("");
const where = ref(props.destination || "");
const fileName = ref("");
const raw = ref<Uint8Array | null>(null);
const payload = ref("");
const over = ref(false);
const query = ref("");
const kinds = ref<string[]>(["song", "bible", "text"]);
const skipSongs = ref(true);
const sourceItems = ref<PlanItem[]>([]);

const kindOptions = [
  { id: "song", label: "Песни" },
  { id: "bible", label: "Стихи из Библии" },
  { id: "text", label: "Текст и заметки" },
];

const shownPlans = computed(() => {
  const q = query.value.trim().toLowerCase();
  if (!q || props.plans.length <= 8) return props.plans;
  return props.plans.filter((plan) => plan.title.toLowerCase().includes(q));
});

const counts = computed(() => countByKind(sourceItems.value));
const added = computed(() => appendItems(props.currentItems, sourceItems.value, { kinds: kinds.value, skipSongs: skipSongs.value }).length - props.currentItems.length);
const tooMany = computed(() => props.currentItems.length + added.value > maxPlanItems);

function resolvedFormat() {
  if (format.value !== "auto") return format.value;
  return importFormatFromName(fileName.value);
}

function decodeRaw() {
  if (!raw.value) return;
  payload.value = readPlanFile(raw.value, encoding.value).text;
}

function takeFile(file: File | undefined) {
  if (!file) return;
  fileName.value = file.name;
  sourceItems.value = [];
  file.arrayBuffer().then((buffer) => {
    raw.value = new Uint8Array(buffer);
    decodeRaw();
    const guessed = importFormatFromName(file.name);
    if (guessed) format.value = guessed;
  }).catch(() => {
    raw.value = null;
    payload.value = "";
  });
}

watch(encoding, () => {
  sourceItems.value = [];
  decodeRaw();
});

function onFile(event: Event) {
  const input = event.target as HTMLInputElement;
  takeFile(input.files?.[0]);
  input.value = "";
}

function submitImport() {
  const chosen = resolvedFormat();
  if (!payload.value) return;
  if (!chosen) return;
  emit("import", {
    format: chosen,
    payload: payload.value,
    title: title.value.trim(),
    dimension_id: where.value,
  });
}

function submitAppend() {
  if (!added.value || tooMany.value) return;
  emit("append", { items: sourceItems.value, kinds: kinds.value.slice(), skipSongs: skipSongs.value });
}

function toggleKind(id: string, checked: boolean) {
  kinds.value = checked ? [...kinds.value, id] : kinds.value.filter((item) => item !== id);
}

function usePlan(plan: Plan) {
  sourceItems.value = plan.items || [];
  kinds.value = kindOptions.map((item) => item.id).filter((id) => countByKind(plan.items || [])[id as "song" | "bible" | "text"] > 0);
}

defineExpose({ usePlan, setParsed(items: PlanItem[]) {
  sourceItems.value = items;
  kinds.value = kindOptions.map((item) => item.id).filter((id) => countByKind(items)[id as "song" | "bible" | "text"] > 0);
} });
</script>

<template>
  <AppModal :title="mode === 'import' ? 'Импортировать план' : 'Дополнить пунктами из другого плана'" @close="emit('close')">
    <div v-if="mode === 'append'" class="tabs" role="tablist">
      <button type="button" class="tab" :class="{ on: tab === 'file' }" @click="tab = 'file'">Из файла</button>
      <button type="button" class="tab" :class="{ on: tab === 'plan' }" @click="tab = 'plan'">Из другого плана</button>
    </div>
    <p v-if="errorText" class="banner bad" role="alert">{{ errorText }}</p>
    <template v-if="mode === 'import' || tab === 'file'">
      <label class="drop" :class="{ over }" @dragover.prevent="over = true" @dragleave.prevent="over = false" @drop.prevent="over = false; takeFile(($event as DragEvent).dataTransfer?.files?.[0])">
        {{ fileName || "Перетащите файл или выберите" }}
        <input type="file" accept=".json,.vplt,.ini,.txt,text/plain" @change="onFile" />
      </label>
      <div class="fields">
        <label>Формат
          <select v-model="format">
            <option value="auto">Определить по имени файла</option>
            <option value="vplt">plan.vplt</option>
            <option value="history">History.ini</option>
            <option value="plandef">PlanDef.ini</option>
            <option value="json">JSON</option>
          </select>
        </label>
        <label>Кодировка
          <select v-model="encoding">
            <option value="auto">Определить автоматически</option>
            <option value="utf-8">UTF-8</option>
            <option value="windows-1251">Windows-1251</option>
          </select>
        </label>
      </div>
      <template v-if="mode === 'import'">
        <label>Название
          <input v-model="title" name="import_title" placeholder="Возьмём из файла" />
        </label>
        <label>Куда
          <select v-model="where">
            <option value="">Личный</option>
            <option v-for="item in destinations" :key="item.id" :value="item.id">{{ item.name }}</option>
          </select>
        </label>
      </template>
      <button v-if="mode === 'append' && payload" class="btn slim" type="button" :disabled="busy" @click="emit('parse', resolvedFormat(), payload)">Показать пункты</button>
    </template>
    <template v-if="mode === 'append' && tab === 'plan'">
      <label v-if="plans.length > 8">Название
        <input v-model="query" name="plan_query" type="search" placeholder="Найти план" />
      </label>
      <ul class="plain">
        <li v-for="plan in shownPlans" :key="plan.id">
          <button class="text-button" type="button" @click="emit('loadPlan', plan.id)">
            <strong class="title-serif">{{ plan.title }}</strong>
          </button>
          <p class="hint"><span class="badge">{{ plan.badge }}</span></p>
        </li>
      </ul>
      <p v-if="!shownPlans.length" class="empty">Других планов нет.</p>
    </template>
    <template v-if="mode === 'append' && sourceItems.length">
      <label v-for="item in kindOptions" :key="item.id" class="check-line">
        <input type="checkbox" :disabled="!counts[item.id as 'song']" :checked="kinds.includes(item.id)" @change="toggleKind(item.id, ($event.target as HTMLInputElement).checked)" />
        {{ item.label }} ({{ counts[item.id as "song"] || 0 }})
      </label>
      <label class="check-line">
        <input v-model="skipSongs" type="checkbox" />
        Пропускать песни, которые уже есть в плане
      </label>
      <p class="hint">{{ tooMany ? `В плане может быть не больше ${maxPlanItems} пунктов.` : `Будет добавлено ${added} пунктов` }}</p>
    </template>
    <template #footer>
      <button class="btn slim" type="button" @click="emit('close')">Отмена</button>
      <button v-if="mode === 'import'" class="btn slim primary" type="button" :disabled="busy || !payload || !resolvedFormat()" @click="submitImport">Импортировать</button>
      <button v-else class="btn slim primary" type="button" :disabled="busy || !sourceItems.length || !added || tooMany" @click="submitAppend">Дополнить</button>
    </template>
  </AppModal>
</template>
