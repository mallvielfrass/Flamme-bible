<script setup lang="ts">
import IconButton from "../../../web/shared/components/IconButton.vue";
import {
  folderHint,
  itemTitle,
  kindLabel,
  planUpdatedLabel,
  pointsLabel,
  type PlanItem,
} from "./model";

const props = defineProps<{
  title: string;
  badge: string;
  count: number;
  updatedAt: string;
  open: boolean;
  inFolder: boolean;
  busy: boolean;
  error: string;
  notice: string;
  items: PlanItem[];
}>();

const emit = defineEmits<{
  toggle: [];
  title: [string];
  commit: [];
  download: [];
  append: [];
  duplicate: [];
  remove: [];
  add: [];
  edit: [number];
  view: [number];
  move: [number, number];
  removeItem: [number];
}>();

function heading(item: PlanItem): string {
  if (item.kind === "bible") {
    const place = item.reference || [item.book, item.chapter, item.verse].filter(Boolean).join(":");
    return place || "Стих";
  }
  return itemTitle(item);
}

function subtitle(item: PlanItem): string {
  if (item.kind === "song") return [item.songbook_title, item.section_title].filter(Boolean).join(" · ");
  if (item.kind === "bible") {
    return [item.module_short_name, item.module_short_name_second].filter(Boolean).join(" · ");
  }
  return "";
}

function onTitle(event: Event) {
  emit("title", (event.target as HTMLInputElement).value);
}
</script>

<template>
  <article class="card plan-card" :class="{ open }">
    <div
      class="plan-card-head"
      role="button"
      tabindex="0"
      :aria-expanded="open"
      @click="emit('toggle')"
      @keydown.enter.prevent="emit('toggle')"
      @keydown.space.prevent="emit('toggle')"
    >
      <svg class="chevron" :class="{ open }" viewBox="0 0 24 24" aria-hidden="true">
        <path d="m9 18 6-6-6-6" />
      </svg>
      <h2 class="plan-title title-serif">{{ title || "План" }}</h2>
      <span class="badge">{{ badge }}</span>
      <span class="plan-meta">{{ pointsLabel(count) }}</span>
      <span v-if="planUpdatedLabel(updatedAt)" class="plan-meta">{{ planUpdatedLabel(updatedAt) }}</span>
      <span class="plan-meta">{{ inFolder ? "есть в VisioBible" : "только на сервере" }}</span>
      <div class="plan-card-icons" @click.stop @keydown.stop>
        <IconButton icon="download" label="Скачать план" :disabled="busy" @click="emit('download')" />
        <IconButton icon="append" label="Дополнить пунктами из другого плана" :disabled="busy" @click="emit('append')" />
        <IconButton icon="copy" label="Дублировать план" :disabled="busy" @click="emit('duplicate')" />
        <IconButton icon="trash" tone="red" label="Удалить план" :disabled="busy" @click="emit('remove')" />
      </div>
    </div>
    <div v-if="open" class="plan-card-body">
      <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
      <p v-if="notice" class="banner ok" role="status">{{ notice }}</p>
      <div class="place-line">
        <p class="hint">{{ folderHint(inFolder) }}</p>
        <span class="badge">Есть на сервере</span>
      </div>
      <label>Название
        <input :value="props.title" name="title" @input="onTitle" @blur="emit('commit')" />
      </label>
      <div class="points-head">
        <h2>Пункты</h2>
        <IconButton icon="plus" round label="Добавить пункт" :disabled="busy" @click="emit('add')" />
      </div>
      <div v-if="!items.length" class="empty-state">
        <p>В плане пока пусто. Добавьте песню, стих или заметку.</p>
      </div>
      <ol v-else class="items">
        <li v-for="(item, index) in items" :key="item.id || index">
          <div class="item-row">
            <span class="num">{{ index + 1 }}</span>
            <span class="badge">{{ kindLabel(item.kind) }}</span>
            <IconButton icon="up" tone="green" small label="Выше" :disabled="index === 0 || busy" @click="emit('move', index, -1)" />
            <IconButton icon="down" tone="green" small label="Ниже" :disabled="index === items.length - 1 || busy" @click="emit('move', index, 1)" />
            <IconButton icon="eye" small label="Показать текст" title="Показать текст" @click="emit('view', index)" />
            <IconButton icon="pencil" small label="Изменить пункт" :disabled="busy" @click="emit('edit', index)" />
            <IconButton icon="cross" tone="red" small label="Убрать пункт" :disabled="busy" @click="emit('removeItem', index)" />
          </div>
          <strong class="title-serif">{{ heading(item) }}</strong>
          <p v-if="subtitle(item)" class="item-sub">{{ subtitle(item) }}</p>
          <p v-if="item.quote" class="quote">{{ item.quote }}</p>
        </li>
      </ol>
    </div>
  </article>
</template>
