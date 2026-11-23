<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppModal from '../../shared/components/AppModal.vue'
import IconButton from '../../shared/components/IconButton.vue'
import { api, explain } from '../api'
import { inviteToken } from '../checks'
import { canManageCommunity, communityCards } from '../communities'
import CommunityCard from '../components/CommunityCard.vue'
import InviteModal from '../components/InviteModal.vue'
import { useSessionStore } from '../stores/session'

const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const error = ref('')
const notice = ref('')
const loading = ref(false)
const busy = ref(false)
const modal = ref('')
const inviteFor = ref(null)
const name = ref('')
const address = ref('')
const email = ref('')
const confession = ref('')
const description = ref('')
const code = ref('')

const communities = computed(() => communityCards(session.spaces))

function resetForms() {
  name.value = ''
  address.value = ''
  email.value = ''
  confession.value = ''
  description.value = ''
  code.value = ''
}

function openInviteIfNeeded() {
  if (!route.path.endsWith('/invite')) return
  const id = route.params.id
  const space = communities.value.find((item) => item.id === id)
  if (space && canManageCommunity(space, session.role)) inviteFor.value = space
}

async function load() {
  error.value = ''
  loading.value = true
  try {
    await session.refreshSpaces()
    openInviteIfNeeded()
  } catch (err) {
    error.value = explain(err)
  } finally {
    loading.value = false
  }
}

function closeModals() {
  modal.value = ''
  inviteFor.value = null
  if (route.path.endsWith('/invite')) router.replace('/communities')
}

async function createCommunity() {
  error.value = ''
  notice.value = ''
  if (!name.value.trim()) {
    error.value = 'Введите имя общины.'
    return
  }
  busy.value = true
  try {
    const view = await api('/api/communities', {
      method: 'POST',
      token: session.token,
      body: {
        name: name.value.trim(),
        address: address.value.trim(),
        email: email.value.trim(),
        confession: confession.value.trim(),
        description: description.value.trim(),
      },
    })
    session.remember(view)
    await session.refreshSpaces()
    resetForms()
    modal.value = ''
    notice.value = 'Община создана. Теперь можно пригласить людей.'
    if (route.path !== '/communities') router.replace('/communities')
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

async function join() {
  const token = inviteToken(code.value)
  error.value = ''
  notice.value = ''
  if (!token) {
    error.value = 'Вставьте код или ссылку приглашения.'
    return
  }
  busy.value = true
  try {
    await session.acceptInvite(token)
    await session.refreshSpaces()
    resetForms()
    modal.value = ''
    notice.value = 'Вы вступили в общину.'
    if (route.path !== '/communities') router.replace('/communities')
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

watch(() => [route.path, route.params.id], load, { immediate: true })
</script>

<template>
  <div class="page-head tight">
    <h1>Общины</h1>
    <div class="page-tools">
      <IconButton icon="plus" label="Новая община" @click="modal = 'new'" />
      <IconButton icon="user" label="Вступить" @click="modal = 'join'" />
    </div>
  </div>
  <p v-if="notice" class="banner ok" role="status">{{ notice }}</p>
  <p v-if="error && !modal && !inviteFor" class="banner bad" role="alert">{{ error }}</p>
  <p v-if="loading" class="empty" role="status">Загрузка…</p>
  <div v-else-if="!communities.length" class="empty-state">
    <p>Вы пока не состоите в общине.</p>
    <p>
      <button class="linkish" type="button" @click="modal = 'new'">Создать общину</button>
      или
      <button class="linkish" type="button" @click="modal = 'join'">вступить по приглашению</button>.
    </p>
  </div>
  <div v-else class="stack">
    <CommunityCard
      v-for="space in communities"
      :key="space.id"
      :space="space"
      @invite="inviteFor = $event"
      @changed="session.refreshSpaces()"
    />
  </div>

  <AppModal v-if="modal === 'new'" title="Новая община" @close="modal = ''">
    <p class="hint">После создания вы станете её администратором и сможете пригласить остальных.</p>
    <form @submit.prevent="createCommunity">
      <label>Имя
        <input v-model="name" name="name" required data-autofocus />
      </label>
      <div class="fields">
        <label>Адрес
          <input v-model="address" name="address" />
        </label>
        <label>Почта
          <input v-model="email" name="email" type="email" />
        </label>
        <label>Конфессия
          <input v-model="confession" name="confession" />
        </label>
        <label>Описание
          <input v-model="description" name="description" />
        </label>
      </div>
    </form>
    <p v-if="error && modal === 'new'" class="banner bad" role="alert">{{ error }}</p>
    <template #footer>
      <button class="btn slim" type="button" @click="modal = ''">Отмена</button>
      <button class="btn slim primary" type="button" :disabled="busy" @click="createCommunity">Создать общину</button>
    </template>
  </AppModal>

  <AppModal v-if="modal === 'join'" title="Вступить по приглашению" @close="modal = ''">
    <p class="hint">Вставьте код или всю ссылку, которую вам прислали.</p>
    <form @submit.prevent="join">
      <label>Код или ссылка
        <input v-model="code" name="token" placeholder="Код или ссылка приглашения" required data-autofocus />
      </label>
    </form>
    <p v-if="error && modal === 'join'" class="banner bad" role="alert">{{ error }}</p>
    <template #footer>
      <button class="btn slim" type="button" @click="modal = ''">Отмена</button>
      <button class="btn slim primary" type="button" :disabled="busy" @click="join">Вступить</button>
    </template>
  </AppModal>

  <InviteModal v-if="inviteFor" :space="inviteFor" @close="closeModals" />
</template>
