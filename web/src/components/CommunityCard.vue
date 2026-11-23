<script setup>
import { computed, ref } from 'vue'
import IconButton from '../../shared/components/IconButton.vue'
import { api, explain } from '../api'
import { canManageCommunity, memberCountLabel, roleLabel } from '../communities'
import { useSessionStore } from '../stores/session'

const props = defineProps({
  space: { type: Object, required: true },
})
const emit = defineEmits(['invite', 'changed'])

const session = useSessionStore()
const membersOpen = ref(false)
const members = ref(null)
const loadingMembers = ref(false)
const error = ref('')
const busy = ref(false)
const manageable = computed(() => canManageCommunity(props.space, session.role))

async function loadMembers() {
  if (!manageable.value) {
    members.value = null
    return
  }
  loadingMembers.value = true
  error.value = ''
  try {
    const data = await api(`/api/communities/${props.space.id}/members`, { token: session.token })
    members.value = data.members || []
  } catch (err) {
    if (err.status === 404 || err.status === 403) members.value = null
    else error.value = explain(err)
  } finally {
    loadingMembers.value = false
  }
}

async function toggleMembers() {
  membersOpen.value = !membersOpen.value
  if (membersOpen.value && members.value == null) await loadMembers()
}

async function exclude(member) {
  error.value = ''
  busy.value = true
  try {
    await api(`/api/communities/${props.space.id}/members/${member.user_id}/exclude`, {
      method: 'POST',
      token: session.token,
    })
    await loadMembers()
    emit('changed')
  } catch (err) {
    if (err.status === 404) {
      try {
        await api(`/dimensions/${props.space.id}/members/${member.user_id}/exclude`, {
          method: 'POST',
          token: session.token,
        })
        await loadMembers()
        emit('changed')
        return
      } catch (inner) {
        error.value = explain(inner)
        return
      }
    }
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="card community-card" :data-kind="space.kind">
    <div class="card-head">
      <div>
        <p class="badge">Община</p>
        <h2 class="community-name">{{ space.name || 'Община' }}</h2>
        <p class="community-meta">
          <span v-if="space.role">{{ roleLabel(space.role) }}</span>
          <span v-if="space.role && space.memberCount"> · </span>
          <span v-if="space.memberCount">{{ memberCountLabel(space.memberCount) }}</span>
        </p>
      </div>
      <IconButton
        v-if="manageable"
        icon="link"
        label="Пригласить"
        @click="emit('invite', space)"
      />
    </div>
    <ul v-if="space.address || space.email || space.confession || space.description" class="facts">
      <li v-if="space.address">Адрес: {{ space.address }}</li>
      <li v-if="space.email">Почта: {{ space.email }}</li>
      <li v-if="space.confession">Конфессия: {{ space.confession }}</li>
      <li v-if="space.description">Описание: {{ space.description }}</li>
    </ul>
    <div v-if="manageable" class="members">
      <button type="button" class="members-toggle" :aria-expanded="membersOpen" @click="toggleMembers">
        Участники
        <span aria-hidden="true">{{ membersOpen ? '▴' : '▾' }}</span>
      </button>
      <div v-if="membersOpen">
        <p v-if="loadingMembers" class="empty" role="status">Загрузка…</p>
        <p v-else-if="members && !members.length" class="hint">Участников пока нет.</p>
        <ul v-else-if="members" class="member-list">
          <li v-for="member in members" :key="member.user_id" class="member-row">
            <span>
              {{ member.email }} · {{ roleLabel(member.role) }}
              <span v-if="member.excluded"> · исключён</span>
            </span>
            <button
              v-if="!member.excluded"
              class="btn slim"
              type="button"
              :disabled="busy"
              @click="exclude(member)"
            >Исключить</button>
          </li>
        </ul>
      </div>
    </div>
    <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  </section>
</template>
