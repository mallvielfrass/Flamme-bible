<script setup lang="ts">
import { computed } from "vue";
import type { Dimension, Preview, Progress, Scan, Settings, SongRow } from "../commands";
import { actionLabel, foundLine, groupRows } from "../syncState";

const props = defineProps<{
  settings: Settings;
  serverUrl: string;
  email: string;
  password: string;
  scan: Scan | null;
  preview: Preview | null;
  dimensions: Dimension[];
  progress: Progress | null;
  log: string[];
  error: string;
  busy: string;
  conflict: SongRow | null;
}>();

const emit = defineEmits<{
  "update:serverUrl": [string];
  "update:email": [string];
  "update:password": [string];
  saveServer: [];
  login: [];
  register: [];
  logout: [];
  pickRoot: [];
  pickModules: [];
  chooseCatalog: [string];
  selectDimension: [string];
  refresh: [];
  start: [];
  openConflict: [SongRow];
  resolvePlan: [string, "keep_local" | "take_server" | "keep_both"];
}>();

const VISIOBIBLE_OPEN = "Закройте VisioBible и нажмите „Повторить“";

const songs = computed(() => groupRows(props.preview?.songs ?? []));
const plans = computed(() => groupRows(props.preview?.plans ?? []));
const conflictsLeft = computed(() => (props.preview?.songs ?? []).filter((row) => row.action === "conflict").length
  + (props.preview?.plans ?? []).filter((row) => row.action === "conflict").length);
const visiobibleOpen = computed(() => props.error === VISIOBIBLE_OPEN);
</script>

<template>
  <div class="steps">
    <section class="card">
      <div class="step-head"><span class="num">1</span><h2>Подключение и папка VisioBible</h2></div>
      <div class="grid">
        <label>Адрес сервера
          <input :value="serverUrl" @input="emit('update:serverUrl', ($event.target as HTMLInputElement).value)" />
        </label>
        <label>Почта<input :value="email" autocomplete="username" @input="emit('update:email', ($event.target as HTMLInputElement).value)" /></label>
        <label>Пароль<input :value="password" type="password" autocomplete="current-password" @input="emit('update:password', ($event.target as HTMLInputElement).value)" /></label>
      </div>
      <div class="row" style="margin-top: 0.7rem">
        <button class="btn" type="button" @click="emit('saveServer')">Сохранить адрес</button>
        <button class="btn primary" type="button" :disabled="busy !== ''" @click="emit('login')">Войти</button>
        <button class="btn" type="button" :disabled="busy !== ''" @click="emit('register')">Создать учётную запись</button>
        <button v-if="settings.logged_in" class="btn" type="button" @click="emit('logout')">Выйти</button>
      </div>
      <p v-if="settings.logged_in" class="found">Вы вошли как {{ settings.email || "участник" }}.</p>
      <label v-if="dimensions.length" style="margin-top: 0.7rem">Пространство
        <select :value="settings.dimension_id" @change="emit('selectDimension', ($event.target as HTMLSelectElement).value)">
          <option v-for="space in dimensions" :key="space.id" :value="space.id">
            {{ space.name || (space.kind === "personal" ? "Личное" : "Община") }}
          </option>
        </select>
      </label>
      <p class="hint">Файл VisioBible.ini обычно лежит в папке программы, а сборники — в подпапке Modules или в C:\ProgramData\VisioBible V…\Modules.</p>
      <div class="row">
        <button class="btn primary" type="button" @click="emit('pickRoot')">Выбрать папку VisioBible</button>
        <button class="btn" type="button" @click="emit('pickModules')">Выбрать папку Modules</button>
      </div>
      <p v-if="!scan" class="empty">Папка ещё не выбрана.</p>
      <p v-else-if="scan.needs_choice" class="empty">Найдено несколько папок. Выберите одну:</p>
      <div v-if="scan?.needs_choice" class="row">
        <button v-for="catalog in scan.catalogs" :key="catalog" class="btn" type="button" @click="emit('chooseCatalog', catalog)">{{ catalog }}</button>
      </div>
      <p v-else-if="scan" class="found">{{ foundLine(scan.songbooks.length, scan.bibles.length, scan.plans.length) }}</p>
      <p v-if="scan && !scan.needs_choice && scan.songbooks.length === 0" class="empty">В выбранной папке сборников не видно. Проверьте, что это Modules, а не сама программа.</p>
    </section>

    <section class="card">
      <div class="step-head"><span class="num">2</span><h2>Список изменений перед запуском</h2></div>
      <button class="btn" type="button" :disabled="busy !== '' || !settings.logged_in" @click="emit('refresh')">Показать, что изменится</button>
      <p v-if="busy === 'preview'" class="hint">Смотрим, что изменилось…</p>
      <p v-if="error && !visiobibleOpen" class="error">{{ error }}</p>
      <p v-for="warning in preview?.warnings ?? []" :key="warning" class="error">{{ warning }}</p>
      <template v-if="preview">
        <div class="group">
          <h3>Что отправится на сервер</h3>
          <p v-if="songs.upload.length + plans.upload.length === 0" class="empty">Отправлять нечего.</p>
          <ul class="list">
            <li v-for="row in songs.upload" :key="row.book_path + row.fbid">{{ row.book_title }} — {{ row.title }} · {{ actionLabel(row.action) }}</li>
            <li v-for="row in plans.upload" :key="row.key">План «{{ row.name }}» · {{ row.summary }}</li>
          </ul>
        </div>
        <div class="group">
          <h3>Что обновится с сервера</h3>
          <p v-if="songs.download.length + plans.download.length === 0" class="empty">Скачивать нечего.</p>
          <ul class="list">
            <li v-for="row in songs.download" :key="row.book_path + row.fbid">{{ row.book_title }} — {{ row.title }} · {{ actionLabel(row.action) }}</li>
            <li v-for="row in plans.download" :key="row.key">План «{{ row.name }}» · {{ row.remote_summary }}</li>
          </ul>
        </div>
        <div class="group">
          <h3>Конфликты песен</h3>
          <p v-if="songs.conflicts.length === 0" class="empty">Конфликтов песен нет.</p>
          <ul class="list">
            <li v-for="row in songs.conflicts" :key="row.book_path + row.fbid">
              <div class="row">
                <span>{{ row.title }}</span>
                <button class="btn" type="button" @click="emit('openConflict', row)">Сравнить</button>
              </div>
            </li>
          </ul>
        </div>
        <div class="group">
          <h3>Конфликты планов</h3>
          <p v-if="plans.conflicts.length === 0" class="empty">Конфликтов планов нет.</p>
          <article v-for="row in plans.conflicts" :key="row.key" class="conflict-card">
            <strong>{{ row.name }}</strong>
            <p>На этом компьютере: {{ row.local_summary }}</p>
            <p>На сервере: {{ row.remote_summary }}</p>
            <div class="row">
              <button class="btn" type="button" @click="emit('resolvePlan', row.key, 'keep_local')">Оставить мою версию</button>
              <button class="btn" type="button" @click="emit('resolvePlan', row.key, 'take_server')">Взять с сервера</button>
              <button class="btn" type="button" @click="emit('resolvePlan', row.key, 'keep_both')">Сохранить оба списка</button>
            </div>
          </article>
        </div>
        <p class="hint">Без изменений: {{ songs.unchanged.length }} песен, {{ plans.unchanged.length }} планов.</p>
      </template>
      <p v-else-if="busy !== 'preview'" class="empty">Список появится после входа и выбора папки.</p>
    </section>

    <section class="card">
      <div class="step-head"><span class="num">3</span><h2>Запись</h2></div>
      <p class="hint">Перед заменой сборника или плана программа сохранит рядом копию с окончанием .bak. Её можно вернуть, если новая запись не подойдёт.</p>
      <button class="btn primary" type="button" :disabled="busy !== '' || !preview || conflictsLeft > 0" @click="emit('start')">Начать синхронизацию</button>
      <p v-if="conflictsLeft" class="error">Сначала выберите, что делать с конфликтами.</p>
      <p v-if="visiobibleOpen" class="error">{{ error }}</p>
      <button v-if="visiobibleOpen" class="btn primary" type="button" :disabled="busy !== ''" @click="emit('start')">Повторить</button>
      <p v-if="busy === 'sync'" class="hint">Идёт синхронизация…</p>
      <div v-if="progress" class="bar" role="progressbar" :aria-valuenow="progress.percentage" aria-valuemin="0" aria-valuemax="100">
        <span :style="{ width: progress.percentage + '%' }"></span>
      </div>
      <p v-if="progress">{{ progress.stage }}: {{ progress.current }} — {{ progress.percentage }}%</p>
      <ul class="list log">
        <li v-for="(line, index) in log" :key="index">{{ line }}</li>
      </ul>
    </section>
  </div>
</template>
