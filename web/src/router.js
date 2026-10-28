import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', name: 'dashboard', component: () => import('./views/Dashboard.vue'), meta: { title: '态势总览' } },
  { path: '/intel', name: 'intel', component: () => import('./views/Articles.vue'), meta: { title: '情报流' } },
  { path: '/article/:id', name: 'article', component: () => import('./views/ArticleDetail.vue'), meta: { title: '情报详情' } },
  { path: '/briefing', name: 'briefing', component: () => import('./views/Briefing.vue'), meta: { title: '每日简报' } },
  { path: '/sources', name: 'sources', component: () => import('./views/Sources.vue'), meta: { title: '来源管理' } },
  { path: '/channels', name: 'channels', component: () => import('./views/Channels.vue'), meta: { title: '推送渠道' } },
  { path: '/settings', name: 'settings', component: () => import('./views/Settings.vue'), meta: { title: '系统设置' } },
]

const router = createRouter({ history: createWebHistory(), routes })

router.afterEach((to) => {
  document.title = (to.meta.title ? to.meta.title + ' · ' : '') + '鹦鹉情报站'
})

export default router
