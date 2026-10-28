import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/login', name: 'login', component: () => import('./views/Login.vue'), meta: { title: '登录', public: true } },
  { path: '/', name: 'dashboard', component: () => import('./views/Dashboard.vue'), meta: { title: '态势总览' } },
  { path: '/intel', name: 'intel', component: () => import('./views/Articles.vue'), meta: { title: '情报流' } },
  { path: '/article/:id', name: 'article', component: () => import('./views/ArticleDetail.vue'), meta: { title: '情报详情' } },
  { path: '/briefing', name: 'briefing', component: () => import('./views/Briefing.vue'), meta: { title: '每日简报' } },
  { path: '/sources', name: 'sources', component: () => import('./views/Sources.vue'), meta: { title: '来源管理' } },
  { path: '/channels', name: 'channels', component: () => import('./views/Channels.vue'), meta: { title: '推送渠道' } },
  { path: '/settings', name: 'settings', component: () => import('./views/Settings.vue'), meta: { title: '系统设置' } },
]

const router = createRouter({ history: createWebHistory(), routes })

// ---- 登录守卫 ----
let authChecked = false   // 首次进入应用时校验一次会话
let authed = false

router.beforeEach(async (to) => {
  if (to.meta.public) return true
  if (!authChecked) {
    authChecked = true
    try {
      const me = await fetch((import.meta.env.VITE_API_BASE || '/api/v1') + '/auth/me', { credentials: 'include' })
      authed = me.ok
      if (me.ok) {
        const info = await me.json()
        // 强制改密状态回登录页第二步
        if (info.must_change_password) return { path: '/login' }
      }
    } catch { authed = false }
  }
  if (!authed) return { path: '/login' }
  return true
})

// 登录成功 / 退出后由调用方重置（避免整页刷新时误判）
export function setAuthed(v) {
  authed = v
  authChecked = v ? authChecked : true
}

router.afterEach((to) => {
  document.title = (to.meta.title ? to.meta.title + ' · ' : '') + '鹦鹉情报站'
})

export default router
