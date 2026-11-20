import { createRouter, createWebHistory } from 'vue-router'
import { useSessionStore } from './stores/session'
import LoginPage from './pages/LoginPage.vue'
import RegisterPage from './pages/RegisterPage.vue'
import SpacePage from './pages/SpacePage.vue'
import InvitePage from './pages/InvitePage.vue'
import AcceptInvitePage from './pages/AcceptInvitePage.vue'
import SyncPage from './pages/SyncPage.vue'
import SongbooksPage from './pages/SongbooksPage.vue'
import PlansPage from './pages/PlansPage.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: LoginPage, meta: { public: true } },
    { path: '/register', component: RegisterPage, meta: { public: true } },
    { path: '/songbooks', component: SongbooksPage },
    { path: '/plans', component: PlansPage },
    { path: '/dimensions/:id', component: SpacePage },
    { path: '/dimensions/:id/invite', component: InvitePage },
    { path: '/dimensions/:id/sync', component: SyncPage },
    { path: '/invite/:token', component: AcceptInvitePage },
    { path: '/', redirect: '/login' },
  ],
})

router.beforeEach((to) => {
  const session = useSessionStore()
  if (!to.meta.public && !session.token) {
    return { path: '/login', query: { next: to.fullPath } }
  }
  if (to.meta.public && session.token && (to.path === '/login' || to.path === '/register')) {
    return { path: '/songbooks' }
  }
})
