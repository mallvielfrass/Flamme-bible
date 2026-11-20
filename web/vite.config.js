import { defineConfig } from 'vitest/config'
import { loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const target = env.VITE_API_TARGET || 'https://127.0.0.1:8080'
  const proxy = {
    target,
    changeOrigin: true,
    secure: false,
    bypass(req) {
      if ((req.headers.accept || '').includes('text/html')) return '/index.html'
    },
  }
  return {
    plugins: [vue()],
    server: {
      proxy: {
        '/api': proxy,
        '/register': proxy,
        '/login': proxy,
        '/refresh': proxy,
        '/logout': proxy,
        '/dimensions': proxy,
        '/invites': proxy,
        '/songbooks': proxy,
      },
    },
    test: {
      environment: 'node',
      include: ['src/**/*.test.js'],
    },
  }
})
