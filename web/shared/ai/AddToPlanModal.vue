<script setup>
import { inject, onMounted, ref } from 'vue'
import AppModal from '../components/AppModal.vue'
import { explain } from '../http.js'
import { aiPlanKey } from './results.js'

const props = defineProps({
  item: { type: Object, required: true },
})
const emit = defineEmits(['close', 'added'])

const target = inject(aiPlanKey)
const plans = ref([])
const picked = ref('')
const title = ref('')
const places = ref([])
const place = ref('')
const loading = ref(true)
const busy = ref(false)
const error = ref('')

onMounted(async () => {
  loadPlaces()
  try {
    plans.value = await target.list()
    picked.value = plans.value[0]?.id || 'new'
  } catch (err) {
    error.value = explain(err)
    picked.value = 'new'
  } finally {
    loading.value = false
  }
})

async function loadPlaces() {
  try {
    places.value = (await target.places?.()) || []
  } catch {
    places.value = []
  }
  const wanted = target.defaultPlace?.() || ''
  place.value = places.value.some((item) => item.id === wanted) ? wanted : ''
}

async function submit() {
  const name = title.value.trim()
  if (picked.value === 'new' && !name) {
    error.value = 'Напишите название нового плана.'
    return
  }
  busy.value = true
  error.value = ''
  try {
    if (picked.value === 'new') {
      await target.create(name, props.item, place.value)
      emit('added', name)
    } else {
      await target.add(picked.value, props.item)
      emit('added', plans.value.find((plan) => plan.id === picked.value)?.title || '')
    }
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AppModal title="В план" @close="emit('close')">
    <p class="hint">{{ item.kind === 'song' ? 'Песня' : 'Стих' }} «{{ item.title }}»</p>
    <p v-if="loading" class="hint">Загрузка планов…</p>
    <div v-else class="ai-plan-pick">
      <label v-for="plan in plans" :key="plan.id" class="ai-plan-opt">
        <input v-model="picked" type="radio" name="ai-plan" :value="plan.id" />
        <span>{{ plan.title || 'План без названия' }}</span>
      </label>
      <label class="ai-plan-opt">
        <input v-model="picked" type="radio" name="ai-plan" value="new" />
        <span>Новый план</span>
      </label>
      <template v-if="picked === 'new'">
        <input v-model="title" class="ai-plan-title" placeholder="Название плана" aria-label="Название нового плана" />
        <label class="ai-plan-place">Куда сохранить
          <select v-model="place">
            <option value="">Личное</option>
            <option v-for="item in places" :key="item.id" :value="item.id">{{ item.name }}</option>
          </select>
        </label>
      </template>
    </div>
    <p v-if="error" class="error">{{ error }}</p>
    <template #footer>
      <button class="btn slim" type="button" @click="emit('close')">Отмена</button>
      <button class="btn slim primary" type="button" :disabled="loading || busy" @click="submit">Добавить</button>
    </template>
  </AppModal>
</template>
