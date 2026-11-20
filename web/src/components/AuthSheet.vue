<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AuthForm from './AuthForm.vue'

const route = useRoute()
const mode = computed(() => (route.path === '/register' ? 'register' : 'login'))
</script>

<template>
  <section class="auth-card">
    <div class="gold-edge"></div>
    <div class="auth-brand">
      <h1>FlammeBible</h1>
      <p class="subtitle">Песни и служение общины</p>
    </div>
    <nav class="auth-switch">
      <router-link :to="{ path: '/login', query: route.query }">Вход</router-link>
      <router-link :to="{ path: '/register', query: route.query }">Регистрация</router-link>
    </nav>
    <AuthForm :mode="mode" />
  </section>
</template>
