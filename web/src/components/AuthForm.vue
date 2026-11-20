<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSessionStore } from '../stores/session'

const props = defineProps({
  mode: {
    type: String,
    required: true,
    validator: (value) => value === 'login' || value === 'register',
  },
})

const session = useSessionStore()
const route = useRoute()
const router = useRouter()
const email = ref('')
const password = ref('')
const passwordAgain = ref('')
const emailError = ref('')
const passwordError = ref('')
const againError = ref('')
const formError = ref('')

function nextPath() {
  const next = route.query.next
  return typeof next === 'string' && next.startsWith('/') ? next : '/songbooks'
}

async function submit() {
  const mail = email.value.trim()
  emailError.value = mail ? '' : 'Введите почту'
  passwordError.value = password.value ? '' : 'Введите пароль'
  againError.value = props.mode === 'register' && passwordAgain.value !== password.value
    ? 'Пароли не совпадают'
    : ''
  formError.value = ''
  if (emailError.value || passwordError.value || againError.value) return
  try {
    if (props.mode === 'register') await session.register(mail, password.value)
    else await session.login(mail, password.value)
    router.push(nextPath())
  } catch (err) {
    formError.value = err.message
  }
}
</script>

<template>
  <form class="auth-form" novalidate @submit.prevent="submit">
    <label>
      Почта
      <input v-model="email" name="email" type="email" autocomplete="username" />
      <span v-if="emailError" class="error">{{ emailError }}</span>
    </label>
    <label>
      Пароль
      <input
        v-model="password"
        name="password"
        type="password"
        :autocomplete="mode === 'register' ? 'new-password' : 'current-password'"
      />
      <span v-if="passwordError" class="error">{{ passwordError }}</span>
    </label>
    <label v-if="mode === 'register'">
      Пароль ещё раз
      <input v-model="passwordAgain" name="passwordAgain" type="password" autocomplete="new-password" />
      <span v-if="againError" class="error">{{ againError }}</span>
    </label>
    <button class="btn primary" type="submit">
      {{ mode === 'register' ? 'Создать учётную запись' : 'Войти' }}
    </button>
    <p v-if="formError" class="error">{{ formError }}</p>
  </form>
</template>
