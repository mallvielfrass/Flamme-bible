<script setup lang="ts">
import { reactive, ref } from "vue";
import type { PlanItem, Scan, SongHit } from "../commands";
import EncodingNotice from "./EncodingNotice.vue";

defineProps<{ scan: Scan | null; hits: SongHit[]; windows1251: boolean }>();
const emit = defineEmits<{
  create: [name: string, items: PlanItem[]];
  search: [string];
  repair: [string];
  setEncoding: [boolean];
}>();

const name = ref("");
const query = ref("");
const items = reactive<PlanItem[]>([]);

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
</script>

<template>
  <section class="card" style="margin-top: 1rem">
    <h2>Планы</h2>
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
        <ul class="list">
          <li v-for="(item, index) in plan.items" :key="index">
            <template v-if="item.kind === 'song'">Песня «{{ item.title || item.caption }}»</template>
            <template v-else-if="item.kind === 'bible'">{{ item.module_short_name || "Перевод" }} {{ item.book }}:{{ item.chapter }}:{{ item.verse }} {{ item.caption }}</template>
            <template v-else>Текст «{{ item.caption || item.quote }}»</template>
          </li>
        </ul>
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
      <button class="btn primary" type="button" @click="emit('create', name, items)">Сохранить план на этом компьютере</button>
    </template>
  </section>
</template>
