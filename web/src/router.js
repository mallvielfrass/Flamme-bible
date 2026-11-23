import { createRouter, createWebHistory } from 'vue-router'
import { homePath, redirectFor } from './guard'
import { useSessionStore } from './stores/session'
import LoginPage from './pages/LoginPage.vue'
import RegisterPage from './pages/RegisterPage.vue'
import SpacePage from './pages/SpacePage.vue'
import AcceptInvitePage from './pages/AcceptInvitePage.vue'
import SyncPage from './pages/SyncPage.vue'
import SongbooksPage from './pages/SongbooksPage.vue'
import BiblePage from './pages/BiblePage.vue'
import PlansPage from './pages/PlansPage.vue'
import AdminPage from './pages/AdminPage.vue'
import AdminTab from './pages/AdminTab.vue'
import DownloadPage from './pages/DownloadPage.vue'
import AiChatPage from './pages/AiChatPage.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: LoginPage, meta: { public: true } },
    { path: '/register', component: RegisterPage, meta: { public: true } },
    { path: '/catalog', component: SongbooksPage, meta: { public: true, catalog: true } },
    { path: '/download', component: DownloadPage, meta: { public: true } },
    { path: '/songbooks', component: SongbooksPage },
    { path: '/bible', component: BiblePage },
    { path: '/plans', component: PlansPage },
    { path: '/ai', component: AiChatPage },
    { path: '/ai/:id', component: AiChatPage },
    { path: '/communities', component: SpacePage },
    { path: '/admin', redirect: '/admin/songbooks' },
    {
      path: '/admin/:tab',
      component: AdminPage,
      meta: { admin: true },
      children: [
        { path: '', component: AdminTab },
      ],
    },
    { path: '/dimensions/:id', component: SpacePage },
    { path: '/dimensions/:id/invite', component: SpacePage },
    { path: '/dimensions/:id/sync', component: SyncPage },
    { path: '/invite/:token', component: AcceptInvitePage },
    { path: '/', redirect: () => homePath(useSessionStore()) },
  ],
})

router.beforeEach((to) => {
  const session = useSessionStore()
  return redirectFor(to, { token: session.token, role: session.role })
})
