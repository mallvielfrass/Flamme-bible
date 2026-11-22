<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import AppModal from "../../../web/shared/components/AppModal.vue";
import IconButton from "../../../web/shared/components/IconButton.vue";
import {
  itemTitle,
  makeBible,
  makeSong,
  makeText,
  type PlanItem,
  type SongbookGroup,
  type SongHit,
} from "./model";

const props = defineProps<{
  item: PlanItem | null;
  groups: SongbookGroup[];
  hits: SongHit[];
  searchError: string;
  searching: boolean;
}>();

const emit = defineEmits<{
  close: [];
  add: [PlanItem];
  change: [PlanItem];
  search: [query: string, songbookId: string, sectionId: string];
}>();

const kinds = [
  { id: "song", label: "Песня" },
  { id: "bible", label: "Стих из Библии" },
  { id: "text", label: "Текст" },
];

const kind = ref("song");
const error = ref("");
const added = ref("");
const songbookId = ref("");
const sectionId = ref("");
const query = ref("");
const staged = ref<SongHit | null>(null);
const planTitle = ref("");
const titleTouched = ref(false);
const editing = ref(false);
const bible = reactive(blankBible());
const note = reactive({ caption: "", reference: "", quote: "", class: "" });

const books = computed(() => props.groups.flatMap((group) => group.songbooks || []));
const sections = computed(() => books.value.find((book) => book.id === songbookId.value)?.sections || []);
const filterLabel = computed(() => {
  const book = books.value.find((item) => item.id === songbookId.value);
  if (!book) return "";
  const section = sections.value.find((item) => item.id === sectionId.value);
  return section ? `${book.title} · ${section.title}` : book.title;
});

function blankBible() {
  return {
    module_short_name: "",
    module_short_name_second: "",
    book: 0,
    chapter: 0,
    verse: 0,
    reference: "",
    quote: "",
    class: "",
  };
}

function reset() {
  error.value = "";
  added.value = "";
  staged.value = null;
  planTitle.value = "";
  titleTouched.value = false;
  songbookId.value = "";
  sectionId.value = "";
  query.value = "";
  Object.assign(bible, blankBible());
  note.caption = "";
  note.reference = "";
  note.quote = "";
  note.class = "";
}

function fill(item: PlanItem) {
  reset();
  editing.value = true;
  kind.value = item.kind || "text";
  if (item.kind === "song") {
    staged.value = {
      fbid: item.song_id || "",
      songbook_id: item.songbook_id || "",
      title: item.title || "",
      songbook_title: item.songbook_title || "",
      section_title: item.section_title || "",
    };
    planTitle.value = item.title || "";
    return;
  }
  if (item.kind === "bible") {
    bible.module_short_name = item.module_short_name || "";
    bible.module_short_name_second = item.module_short_name_second || "";
    bible.book = item.book || 0;
    bible.chapter = item.chapter || 0;
    bible.verse = item.verse || 0;
    bible.reference = item.reference || "";
    bible.quote = item.quote || "";
    bible.class = item.class || "";
    return;
  }
  note.caption = item.caption || "";
  note.reference = item.reference || "";
  note.quote = item.quote || "";
  note.class = item.class || "";
}

watch(() => props.item, (item) => {
  if (item) fill(item);
  else {
    editing.value = false;
    kind.value = "song";
    reset();
  }
}, { immediate: true });

function setKind(next: string) {
  if (editing.value) return;
  kind.value = next;
  error.value = "";
}

function onBook() {
  sectionId.value = "";
}

function stage(hit: SongHit) {
  const previous = staged.value?.title || "";
  staged.value = hit;
  if (!titleTouched.value || !planTitle.value.trim() || planTitle.value === previous) {
    planTitle.value = hit.title || "";
    titleTouched.value = false;
  }
  error.value = "";
}

function build(): PlanItem {
  if (kind.value === "song") {
    if (!staged.value) throw new Error("Выберите песню.");
    return makeSong(staged.value, planTitle.value);
  }
  if (kind.value === "bible") return makeBible({ kind: "bible", ...bible });
  return makeText({ kind: "text", ...note });
}

function submit() {
  error.value = "";
  try {
    const item = build();
    if (editing.value) {
      emit("change", item);
      emit("close");
      return;
    }
    emit("add", item);
    added.value = itemTitle(item);
    if (kind.value === "song") {
      staged.value = null;
      planTitle.value = "";
      titleTouched.value = false;
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  }
}

function find() {
  emit("search", query.value, songbookId.value, sectionId.value);
}
</script>

<template>
  <AppModal :title="editing ? 'Изменить пункт' : 'Добавить пункт'" @close="emit('close')">
    <div v-if="!editing" class="tabs" role="tablist" aria-label="Тип пункта">
      <button
        v-for="item in kinds"
        :key="item.id"
        type="button"
        class="tab"
        :class="{ on: kind === item.id }"
        @click="setKind(item.id)"
      >{{ item.label }}</button>
    </div>
    <p v-else class="hint">{{ kinds.find((item) => item.id === kind)?.label }}</p>
    <p v-if="error || searchError" class="banner bad" role="alert">{{ error || searchError }}</p>
    <p v-if="added" class="banner ok" role="status">Добавлено: «{{ added }}»</p>

    <div v-if="kind === 'song'">
      <form class="search-bar" @submit.prevent="find">
        <label class="grow">Найти песню
          <input v-model="query" name="q" type="search" placeholder="Название или строчка" />
        </label>
        <label class="pick">Сборник
          <select v-model="songbookId" @change="onBook">
            <option value="">Все сборники</option>
            <option v-for="book in books" :key="book.id" :value="book.id">{{ book.title || book.short_name }}</option>
          </select>
        </label>
        <label class="pick">Раздел
          <select v-model="sectionId" :disabled="!songbookId">
            <option value="">Все разделы</option>
            <option v-for="section in sections" :key="section.id" :value="section.id">{{ section.title }}</option>
          </select>
        </label>
        <IconButton icon="search" label="Найти" :disabled="searching" @click="find" />
      </form>
      <p v-if="filterLabel" class="hint">Фильтр: {{ filterLabel }}</p>
      <p v-if="searching" class="empty" role="status">Загрузка…</p>
      <ul v-else-if="hits.length" class="results">
        <li v-for="hit in hits" :key="hit.songbook_id + hit.fbid">
          <div>
            <b class="hit-title">{{ hit.title }}</b>
            <p class="hint">{{ [hit.songbook_title, hit.section_title].filter(Boolean).join(" · ") }}</p>
          </div>
          <button class="btn slim" type="button" @click="stage(hit)">Добавить</button>
        </li>
      </ul>
      <label>Название в плане
        <input v-model="planTitle" name="plan_title" placeholder="По умолчанию — название песни" @input="titleTouched = true" />
      </label>
    </div>

    <div v-else-if="kind === 'bible'" class="fields">
      <label>Перевод
        <input v-model="bible.module_short_name" name="module_short_name" placeholder="RST" />
      </label>
      <label>Второй перевод
        <input v-model="bible.module_short_name_second" name="module_short_name_second" placeholder="необязательно" />
      </label>
      <label>Книга
        <input v-model.number="bible.book" name="book" type="number" min="0" max="200" />
      </label>
      <label>Глава
        <input v-model.number="bible.chapter" name="chapter" type="number" min="0" max="200" />
      </label>
      <label>Стих
        <input v-model.number="bible.verse" name="verse" type="number" min="0" max="500" />
      </label>
      <label>Ссылка
        <input v-model="bible.reference" name="reference" placeholder="Ин. 3:16" />
      </label>
      <label class="wide">Текст стиха
        <textarea v-model="bible.quote" name="quote" rows="3"></textarea>
      </label>
    </div>

    <div v-else>
      <label>Заголовок
        <input v-model="note.caption" name="caption" />
      </label>
      <label>Ссылка
        <input v-model="note.reference" name="reference" />
      </label>
      <label>Текст
        <textarea v-model="note.quote" name="quote" rows="3"></textarea>
      </label>
    </div>

    <template #footer>
      <button class="btn slim" type="button" @click="emit('close')">Закрыть</button>
      <button class="btn slim primary" type="button" @click="submit">{{ editing ? "Сохранить" : "Добавить" }}</button>
    </template>
  </AppModal>
</template>
