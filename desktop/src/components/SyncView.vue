<script setup lang="ts">
import { computed } from "vue";
import type { Preview, Progress, Scan, SongRow } from "../commands";
import { placeLine, syncScreen, type ListedRow } from "../syncState";
import type { Choice } from "../syncState";

const props = defineProps<{
  scan: Scan | null;
  preview: Preview | null;
  progress: Progress | null;
  log: string[];
  error: string;
  busy: string;
  chosen: Record<string, Choice>;
  adopted: Record<string, true>;
  folder: string;
  space: string;
}>();

const emit = defineEmits<{
  refresh: [];
  start: [];
  openConflict: [SongRow];
  resolvePlan: [string, "keep_local" | "take_server" | "keep_both"];
  resolveGroup: [bookPath: string, sectionId: string, choice: "keep_local" | "take_server"];
  adopt: [kind: "songbook" | "plan", id: string];
  skip: [kind: "songbook" | "plan", id: string];
  restore: [kind: "songbook" | "plan", id: string];
}>();

const VISIOBIBLE_OPEN = "Закройте VisioBible и нажмите „Повторить“";

const visiobibleOpen = computed(() => props.error === VISIOBIBLE_OPEN);
const place = computed(() => placeLine(props.folder, props.space));
const screen = computed(() =>
  props.preview ? syncScreen(props.preview, props.chosen, props.adopted) : null,
);

function conflictSong(row: ListedRow): SongRow | null {
  return props.preview?.songs.find((song) => song.book_path === row.key && song.fbid === row.conflictId) ?? null;
}

function openConflict(row: ListedRow) {
  const song = conflictSong(row);
  if (song) emit("openConflict", song);
}

function act(name: "adopt" | "skip", row: ListedRow) {
  if ((row.kind !== "songbook" && row.kind !== "plan") || !row.serverId) return;
  if (name === "adopt") emit("adopt", row.kind, row.serverId);
  else emit("skip", row.kind, row.serverId);
}

function restore(kind: string, id: string) {
  if (kind !== "songbook" && kind !== "plan") return;
  emit("restore", kind, id);
}
</script>

<template>
  <section class="sync-page">
    <div class="page-head">
      <div>
        <h1>Синхронизация</h1>
        <p class="hint place">{{ place }}</p>
      </div>
      <button class="btn slim" type="button" :disabled="busy !== ''" @click="emit('refresh')">Обновить список</button>
    </div>

    <div class="sync-body">
      <p v-if="scan?.needs_choice" class="hint">Найдено несколько папок. Выберите одну в настройках.</p>
      <p v-if="busy === 'preview'" class="hint">Смотрим, что изменилось…</p>
      <p v-if="error && !visiobibleOpen" class="error">{{ error }}</p>
      <p v-for="warning in preview?.warnings ?? []" :key="warning" class="error">{{ warning }}</p>

      <template v-if="screen && preview">
        <div v-if="visiobibleOpen" class="banner bad">
          <strong>{{ VISIOBIBLE_OPEN }}.</strong>
          <p class="hint">Пока программа открыта, файлы в папке VisioBible не меняются. Правки на сервере уже отправлены, запись на компьютер ещё не закончена.</p>
          <button class="btn slim primary" type="button" :disabled="busy !== ''" @click="emit('start')">Повторить</button>
        </div>
        <div v-else-if="screen.banner" class="banner ok">
          <strong>На сервере есть изменения — синхронизируйте.</strong>
          <p class="hint">{{ screen.banner }}</p>
        </div>

        <div class="stat-chips">
          <span v-for="chip in screen.chips" :key="chip.id" class="stat-chip" :class="chip.tone === 'plain' ? '' : chip.tone">{{ chip.text }}</span>
        </div>

        <div class="block">
          <div class="block-head"><h2>Сборники</h2></div>
          <p v-if="screen.books.length === 0" class="empty">Изменений в сборниках нет.</p>
          <div v-else class="entity-list">
            <article v-for="row in screen.books" :key="row.key" class="entity-row" :class="row.rowClass">
              <div>
                <p class="entity-title">{{ row.title }}</p>
                <p class="entity-meta">{{ row.meta }}</p>
              </div>
              <span class="action" :class="row.tone">{{ row.status }}</span>
              <div class="entity-actions">
                <button v-if="row.mode === 'compare'" class="btn slim" type="button" @click="openConflict(row)">Сравнить</button>
                <template v-else-if="row.mode === 'name'">
                  <button class="btn slim" type="button" @click="emit('resolveGroup', row.conflictId, row.serverId, 'keep_local')">Оставить моё название</button>
                  <button class="btn slim" type="button" @click="emit('resolveGroup', row.conflictId, row.serverId, 'take_server')">Взять с сервера</button>
                </template>
                <template v-else-if="row.mode === 'adopt'">
                  <button class="btn slim primary" type="button" @click="act('adopt', row)">Добавить в Modules</button>
                  <button class="btn slim" type="button" @click="act('skip', row)">Пропустить</button>
                </template>
                <span v-else-if="row.aside" class="choice">{{ row.aside }}</span>
              </div>
            </article>
          </div>
        </div>

        <div class="block">
          <div class="block-head"><h2>Планы</h2></div>
          <p v-if="screen.plans.length === 0" class="empty">Изменений в планах нет.</p>
          <div v-else class="entity-list">
            <article v-for="row in screen.plans" :key="row.key" class="entity-row" :class="row.rowClass">
              <div>
                <p class="entity-title">{{ row.title }}</p>
                <p class="entity-meta">{{ row.meta }}</p>
              </div>
              <span class="action" :class="row.tone">{{ row.status }}</span>
              <div class="entity-actions">
                <template v-if="row.mode === 'choose'">
                  <button class="btn slim" type="button" @click="emit('resolvePlan', row.key, 'keep_local')">Оставить мою версию</button>
                  <button class="btn slim" type="button" @click="emit('resolvePlan', row.key, 'take_server')">Взять с сервера</button>
                  <button class="btn slim" type="button" @click="emit('resolvePlan', row.key, 'keep_both')">Сохранить оба списка</button>
                </template>
                <template v-else-if="row.mode === 'adopt'">
                  <button class="btn slim primary" type="button" @click="act('adopt', row)">Добавить в Modules</button>
                  <button class="btn slim" type="button" @click="act('skip', row)">Пропустить</button>
                </template>
                <span v-else-if="row.aside" class="choice">{{ row.aside }}</span>
              </div>
            </article>
          </div>
        </div>

        <slot name="bibles" />

        <details v-if="preview.skipped.length" class="skipped-block">
          <summary>Пропущенные ({{ preview.skipped.length }})</summary>
          <div class="entity-list">
            <article v-for="item in preview.skipped" :key="item.kind + item.id" class="entity-row skip">
              <div>
                <p class="entity-title">{{ item.title }}</p>
                <p class="entity-meta">{{ item.kind === "plan" ? "План" : "Сборник" }} · не копируется на этот компьютер</p>
              </div>
              <span class="action skip">Пропущен</span>
              <button class="btn slim" type="button" @click="restore(item.kind, item.id)">Вернуть</button>
            </article>
          </div>
        </details>
      </template>
      <p v-else-if="busy !== 'preview'" class="empty">Список появится после выбора папки.</p>
    </div>

    <div class="sync-foot">
      <div v-if="!visiobibleOpen" class="btn-row">
        <button class="btn primary slim" type="button" :disabled="busy !== '' || !screen?.ready" @click="emit('start')">Начать синхронизацию</button>
        <p v-if="screen && !screen.ready" class="hint">{{ screen.hint }}</p>
      </div>
      <div v-if="progress">
        <div class="progress" role="progressbar" :aria-valuenow="progress.percentage" aria-valuemin="0" aria-valuemax="100">
          <span :style="{ width: progress.percentage + '%' }"></span>
        </div>
        <p class="progress-label">{{ progress.stage }}: {{ progress.current }} — {{ progress.percentage }}%</p>
      </div>
      <ul v-if="log.length" class="list log">
        <li v-for="(line, index) in log" :key="index">{{ line }}</li>
      </ul>
    </div>
  </section>
</template>
