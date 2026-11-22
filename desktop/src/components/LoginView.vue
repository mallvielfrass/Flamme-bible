<script setup lang="ts">
import { ref } from "vue";

const props = defineProps<{ serverUrl: string; email: string; error: string; busy: boolean }>();
const emit = defineEmits<{
  submit: [mode: "login" | "register", serverUrl: string, email: string, password: string];
}>();

const mode = ref<"login" | "register">("login");
const serverUrl = ref(props.serverUrl);
const email = ref(props.email);
const password = ref("");

function submit() {
  emit("submit", mode.value, serverUrl.value, email.value, password.value);
}
</script>

<template>
  <main class="auth-main">
    <section class="auth-card">
      <div class="gold-edge"></div>
      <div class="auth-brand">
        <h1>FlammeBible</h1>
        <p class="subtitle">Песни и служение общины</p>
      </div>
      <nav class="auth-switch">
        <button type="button" :class="{ on: mode === 'login' }" @click="mode = 'login'">Вход</button>
        <button type="button" :class="{ on: mode === 'register' }" @click="mode = 'register'">Регистрация</button>
      </nav>
      <form class="auth-form" @submit.prevent="submit">
        <label>Адрес сервера
          <input v-model="serverUrl" name="server" autocomplete="url" />
        </label>
        <label>Почта
          <input v-model="email" name="email" type="email" autocomplete="username" />
        </label>
        <label>Пароль
          <input v-model="password" name="password" type="password" :autocomplete="mode === 'register' ? 'new-password' : 'current-password'" />
        </label>
        <button class="btn primary" type="submit" :disabled="busy">
          {{ mode === "register" ? "Создать учётную запись" : "Войти" }}
        </button>
        <p v-if="error" class="error">{{ error }}</p>
      </form>
    </section>
  </main>
</template>
