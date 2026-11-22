<script setup>
import { computed } from 'vue'
import IconButton from '../../shared/components/IconButton.vue'

const props = defineProps({
  columns: { type: Array, required: true },
  rows: { type: Array, default: () => [] },
  rowKey: { type: String, default: 'id' },
  selectable: { type: Boolean, default: false },
  expandable: { type: Boolean, default: false },
  loading: { type: Boolean, default: false },
  error: { type: String, default: '' },
  empty: { type: String, default: 'Ничего не найдено.' },
  selected: { type: Array, default: () => [] },
  expanded: { type: String, default: '' },
})

const emit = defineEmits(['update:selected', 'update:expanded', 'retry'])

function keyOf(row) {
  return String(row[props.rowKey])
}

const picked = computed(() => new Set(props.selected.map(String)))
const allOn = computed(() => props.rows.length > 0 && props.rows.every((row) => picked.value.has(keyOf(row))))
const span = computed(() => props.columns.length + (props.selectable ? 1 : 0) + (props.expandable ? 1 : 0))

function toggleAll() {
  emit('update:selected', allOn.value ? [] : props.rows.map(keyOf))
}

function toggle(row) {
  const key = keyOf(row)
  const next = picked.value.has(key)
    ? props.selected.filter((id) => String(id) !== key)
    : [...props.selected, key]
  emit('update:selected', next)
}

function toggleExpand(row) {
  const key = keyOf(row)
  emit('update:expanded', props.expanded === key ? '' : key)
}

function columnClass(column) {
  const list = []
  if (column.align === 'right') list.push('num')
  if (column.class) list.push(column.class)
  return list
}
</script>

<template>
  <div class="table-wrap">
    <table class="data-table">
      <thead>
        <tr>
          <th v-if="selectable" class="check">
            <input type="checkbox" :checked="allOn" aria-label="Выбрать все" :disabled="loading || !rows.length" @change="toggleAll" />
          </th>
          <th v-if="expandable" class="chevron"></th>
          <th v-for="column in columns" :key="column.key" :class="columnClass(column)">{{ column.label }}</th>
        </tr>
      </thead>
      <tbody v-if="loading">
        <tr v-for="n in 5" :key="n" class="skeleton">
          <td :colspan="span"><span></span></td>
        </tr>
      </tbody>
      <tbody v-else-if="error">
        <tr>
          <td :colspan="span">
            <p class="error">{{ error }}</p>
            <button class="btn slim" type="button" @click="emit('retry')">Повторить</button>
          </td>
        </tr>
      </tbody>
      <tbody v-else-if="!rows.length">
        <tr>
          <td :colspan="span"><p class="empty">{{ empty }}</p></td>
        </tr>
      </tbody>
      <tbody v-else>
        <template v-for="row in rows" :key="keyOf(row)">
          <tr :class="{ open: expanded === keyOf(row) }">
            <td v-if="selectable" class="check">
              <input type="checkbox" :checked="picked.has(keyOf(row))" :aria-label="`Выбрать ${keyOf(row)}`" @change="toggle(row)" />
            </td>
            <td v-if="expandable" class="chevron">
              <IconButton
                icon="chevron"
                label="Подробнее"
                small
                :class="{ turned: expanded === keyOf(row) }"
                @click="toggleExpand(row)"
              />
            </td>
            <td v-for="column in columns" :key="column.key" :class="columnClass(column)">
              <slot :name="`cell-${column.key}`" :row="row">{{ row[column.key] }}</slot>
            </td>
          </tr>
          <tr v-if="expandable && expanded === keyOf(row)" class="expand">
            <td :colspan="span">
              <slot name="expand" :row="row" />
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>
