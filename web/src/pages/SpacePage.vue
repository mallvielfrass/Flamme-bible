<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, explain } from '../api'
import { inviteToken } from '../checks'
import { useSessionStore } from '../stores/session'
import InvitePanel from '../components/InvitePanel.vue'

const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const space = ref(null)
const tab = ref('now')
const error = ref('')
const notice = ref('')
const loading = ref(false)
const busy = ref(false)
const name = ref('')
const address = ref('')
const email = ref('')
const confession = ref('')
const description = ref('')
const code = ref('')

const spaceId = computed(() => route.params.id || session.communityId || '')

async function load() {
  error.value = ''
  if (!spaceId.value) {
    space.value = null
    loading.value = false
    return
  }
  loading.value = true
  try {
    const view = await api(`/api/dimensions/${spaceId.value}`, { token: session.token })
    space.value = view
    session.remember(view, { select: Boolean(route.params.id) })
  } catch (err) {
    space.value = null
    error.value = explain(err)
  } finally {
    loading.value = false
  }
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
    name.value = ''
    address.value = ''
    email.value = ''
    confession.value = ''
    description.value = ''
    tab.value = 'now'
    notice.value = 'Община создана. Теперь можно пригласить людей.'
    router.push(`/dimensions/${view.id}`)
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
    const view = await session.acceptInvite(token)
    code.value = ''
    tab.value = 'now'
    notice.value = 'Вы вступили в общину.'
    router.push(`/dimensions/${view.id}`)
  } catch (err) {
    error.value = explain(err)
  } finally {
    busy.value = false
  }
}

watch(spaceId, load, { immediate: true })
watch(() => route.path, (path) => {
  if (path.endsWith('/invite')) tab.value = 'now'
})
</script>

<template>
  <h1>Общины</h1>
  <p v-if="notice" class="banner ok" role="status">{{ notice }}</p>
  <p v-if="error" class="banner bad" role="alert">{{ error }}</p>
  <div class="tabs" role="tablist">
    <button type="button" class="tab" :class="{ on: tab === 'now' }" @click="tab = 'now'">Это пространство</button>
    <button type="button" class="tab" :class="{ on: tab === 'new' }" @click="tab = 'new'">Новая община</button>
    <button type="button" class="tab" :class="{ on: tab === 'join' }" @click="tab = 'join'">Вступить</button>
  </div>

  <section v-if="tab === 'now'">
    <p v-if="loading" class="empty" role="status">Загрузка…</p>
    <div v-else-if="!space" class="empty-state">
      <p>Вы пока не состоите в общине.</p>
      <router-link to="/communities" @click="tab = 'new'">Создать или вступить в общину</router-link>
    </div>
    <div v-else class="stack">
      <section class="card" :data-kind="space.kind">
        <p class="badge">{{ space.kind === 'personal' ? 'Личное' : 'Община' }}</p>
        <h2 class="space-name">{{ space.kind === 'personal' ? 'Личное' : (space.name || 'Община') }}</h2>
        <p v-if="space.kind === 'personal'" class="hint">Здесь ваши личные сборники и планы. Общину можно создать на соседней вкладке.</p>
        <ul v-else class="facts">
          <li v-if="space.address">Адрес: {{ space.address }}</li>
          <li v-if="space.email">Почта: {{ space.email }}</li>
          <li v-if="space.confession">Конфессия: {{ space.confession }}</li>
          <li v-if="space.description">Описание: {{ space.description }}</li>
        </ul>
        <p>
          <router-link :to="`/dimensions/${space.id}/sync`">Синхронизация с программой</router-link>
        </p>
      </section>
      <InvitePanel v-if="space.kind === 'community'" :dimension-id="space.id" />
      <section v-else class="card">
        <h2>Приглашения</h2>
        <p class="hint">В личное пространство никого пригласить нельзя. Создайте общину и пришлите ссылку.</p>
        <button class="btn slim primary" type="button" @click="tab = 'new'">Создать общину</button>
      </section>
    </div>
  </section>

  <section v-else-if="tab === 'new'" class="card">
    <h2>Новая община</h2>
    <p class="hint">После создания вы станете её администратором и сможете пригласить остальных.</p>
    <form @submit.prevent="createCommunity">
      <label>Имя
        <input v-model="name" name="name" required />
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
      <button class="btn primary" type="submit" :disabled="busy">Создать общину</button>
    </form>
  </section>

  <section v-else class="card">
    <h2>Вступить по приглашению</h2>
    <p class="hint">Вставьте код или всю ссылку, которую вам прислали.</p>
    <form @submit.prevent="join">
      <label>Код или ссылка
        <input v-model="code" name="token" placeholder="Код или ссылка приглашения" required />
      </label>
      <button class="btn primary" type="submit" :disabled="busy">Вступить</button>
    </form>
  </section>
</template>
