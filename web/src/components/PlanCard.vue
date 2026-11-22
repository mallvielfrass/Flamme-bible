<script setup>
import { computed } from 'vue'
import { clipPlanText } from '../checks'
import { itemKindLabel, itemStamp, itemTitle, planUpdatedLabel, pointsLabel } from '../planItems'
import IconButton from '../../shared/components/IconButton.vue'
import { icons } from '../../shared/icons.js'

const props = defineProps({
  title: { type: String, default: '' },
  badge: { type: String, default: '' },
  count: { type: Number, default: 0 },
  updatedAt: { type: String, default: '' },
  open: { type: Boolean, default: false },
  saved: { type: Boolean, default: false },
  dirty: { type: Boolean, default: false },
  busy: { type: Boolean, default: false },
  error: { type: String, default: '' },
  notice: { type: String, default: '' },
  repair: { type: Object, default: null },
  place: { type: String, default: '' },
  showDestination: { type: Boolean, default: false },
  destination: { type: String, default: '' },
  communities: { type: Array, default: () => [] },
  items: { type: Array, default: () => [] },
})

const emit = defineEmits([
  'toggle', 'save', 'download', 'append', 'duplicate', 'remove', 'cancel',
  'title', 'destination', 'add', 'edit', 'move', 'remove-item', 'repair',
])

const when = computed(() => planUpdatedLabel(props.updatedAt))
const saveLabel = computed(() => (props.dirty ? 'Сохранить (есть изменения)' : 'Сохранить'))

function stamp(item) {
  return itemStamp(item)
}

function subtitle(item) {
  if (item.kind === 'song') return [item.songbook_title, item.section_title].filter(Boolean).join(' · ')
  if (item.kind === 'bible' && item.module_short_name_second) return `Второй перевод: ${item.module_short_name_second}`
  return ''
}
</script>

<template>
  <article class="card plan-card">
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
        <path v-for="(d, index) in icons.chevron" :key="index" :d="d" />
      </svg>
      <h2 class="plan-title title-serif">{{ title || 'Новый план' }}</h2>
      <span class="badge">{{ badge }}</span>
      <span class="plan-meta">{{ pointsLabel(count) }}</span>
      <span v-if="when" class="plan-meta">{{ when }}</span>
      <div class="plan-card-icons" @click.stop @keydown.stop>
        <template v-if="open && !saved">
          <IconButton icon="save" :label="saveLabel" :dirty="dirty" :busy="busy" :disabled="busy" @click="emit('save')" />
          <IconButton icon="cross" tone="red" label="Отменить" :disabled="busy" @click="emit('cancel')" />
        </template>
        <template v-else-if="saved">
          <IconButton v-if="open" icon="save" :label="saveLabel" :dirty="dirty" :busy="busy" :disabled="busy" @click="emit('save')" />
          <IconButton icon="download" label="Скачать план" :disabled="busy" @click="emit('download')" />
          <IconButton icon="append" label="Дополнить пунктами из другого плана" :disabled="busy" @click="emit('append')" />
          <IconButton icon="copy" label="Дублировать план" :disabled="busy" @click="emit('duplicate')" />
          <IconButton icon="trash" tone="red" label="Удалить план" :disabled="busy" @click="emit('remove')" />
        </template>
      </div>
    </div>

    <div v-if="open" class="plan-card-body">
      <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
      <p v-if="notice" class="banner ok" role="status">{{ notice }}</p>
      <div v-if="repair" class="banner bad" role="alert">
        <p>{{ repair.warning }}</p>
        <p class="hint">До</p>
        <p class="quote">{{ clipPlanText(repair.before) }}</p>
        <p class="hint">После</p>
        <p class="quote">{{ clipPlanText(repair.after) }}</p>
        <button class="btn slim" type="button" :disabled="busy" @click="emit('repair')">Починить кодировку</button>
      </div>
      <div class="place-line">
        <p class="hint">{{ place }}</p>
        <label v-if="showDestination" class="inline-field">Куда
          <select :value="destination || 'personal'" @change="emit('destination', $event.target.value === 'personal' ? '' : $event.target.value)">
            <option value="personal">Личный</option>
            <option v-for="space in communities" :key="space.id" :value="space.id">{{ space.name }}</option>
          </select>
        </label>
      </div>
      <label>Название
        <input :value="title" name="title" @input="emit('title', $event.target.value)" />
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
            <span class="badge">{{ itemKindLabel(item.kind) }}</span>
            <IconButton icon="up" tone="green" small label="Выше" :disabled="index === 0 || busy" @click="emit('move', index, -1)" />
            <IconButton icon="down" tone="green" small label="Ниже" :disabled="index === items.length - 1 || busy" @click="emit('move', index, 1)" />
            <IconButton icon="pencil" small label="Изменить пункт" :disabled="busy" @click="emit('edit', index)" />
            <IconButton icon="cross" tone="red" small label="Убрать пункт" :disabled="busy" @click="emit('remove-item', index)" />
          </div>
          <strong class="title-serif">{{ itemTitle(item) }}</strong>
          <p v-if="subtitle(item)" class="item-sub">{{ subtitle(item) }}</p>
          <p v-if="item.quote" class="quote">{{ item.quote }}</p>
          <p class="stamp" :title="stamp(item).title">{{ stamp(item).text }}</p>
        </li>
      </ol>
    </div>
  </article>
</template>
