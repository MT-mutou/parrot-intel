<template>
  <!-- 登录页：独立全屏布局 -->
  <router-view v-if="isLoginRoute" />

  <div v-else style="display: flex; height: 100vh; overflow: hidden;">
    <!-- 侧边栏 -->
    <aside class="sidebar">
      <div class="brand" style="padding: 6px 14px 16px;">
        <span class="logo">🦜</span>
        <div>
          <div>鹦鹉情报站</div>
          <div class="sub">ParrotIntel · 鹦鹉安全实验室</div>
        </div>
      </div>
      <div v-for="item in navs" :key="item.path" class="nav-item"
           :class="{ active: isActive(item.path) }" @click="$router.push(item.path)">
        <el-icon><component :is="item.icon" /></el-icon>
        <span>{{ item.label }}</span>
      </div>
      <div style="flex: 1"></div>
      <div class="nav-item" @click="logout" style="color: var(--p-text-3);">
        <el-icon><SwitchButton /></el-icon>
        <span>退出登录</span>
      </div>
      <div class="nav-item" style="color: var(--p-text-3); font-size: 12px; cursor: default;">
        <span>v2.2 · Go + Vue3</span>
      </div>
    </aside>

    <div style="flex: 1; display: flex; flex-direction: column; min-width: 0;">
      <!-- 顶栏 -->
      <header class="topbar">
        <div style="flex: 1"></div>

        <!-- 主题切换 -->
        <el-tooltip :content="isLight ? '切换到夜间模式' : '切换到白天模式'" placement="bottom">
          <el-button circle @click="toggleTheme">
            <el-icon><Sunny v-if="isLight" /><Moon v-else /></el-icon>
          </el-button>
        </el-tooltip>

        <!-- 搜索（跳转情报流） -->
        <el-input v-model="search" placeholder="搜索情报…" style="width: 260px" clearable
                  @keyup.enter="goSearch">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>

        <!-- 收集状态 -->
        <el-tag v-if="col.running" type="warning" effect="dark" size="small" class="mono">
          <el-icon class="is-loading" style="vertical-align: -2px"><Loading /></el-icon>
          {{ col.status || '收集中…' }}
        </el-tag>

        <!-- 立即收集 -->
        <el-button type="primary" :loading="col.running" @click="collect">
          <el-icon style="margin-right: 4px"><Refresh /></el-icon>立即收集
        </el-button>
      </header>

      <router-view />
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import api, { authApi } from './api'
import { setAuthed } from './router'

const router = useRouter()
const route = useRoute()

const navs = [
  { path: '/', icon: 'Odometer', label: '态势总览' },
  { path: '/intel', icon: 'Document', label: '情报流' },
  { path: '/briefing', icon: 'MagicStick', label: '每日简报' },
  { path: '/sources', icon: 'Link', label: '来源管理' },
  { path: '/channels', icon: 'Promotion', label: '推送渠道' },
  { path: '/settings', icon: 'Setting', label: '系统设置' },
]

const search = ref('')
const col = reactive({ running: false, status: '' })
let timer = null

const isLoginRoute = computed(() => route.path === '/login')
const isLight = ref(document.documentElement.classList.contains('light'))

function toggleTheme() {
  const next = isLight.value ? 'dark' : 'light'
  document.documentElement.classList.remove('light', 'dark')
  document.documentElement.classList.add(next)
  localStorage.setItem('parrot-theme', next)
  isLight.value = next === 'light'
  window.dispatchEvent(new Event('theme-change'))
}

function isActive(p) {
  if (p === '/') return route.path === '/'
  return route.path.startsWith(p)
}

function goSearch() {
  router.push({ path: '/intel', query: search.value ? { search: search.value } : {} })
}

async function logout() {
  try { await authApi.logout() } catch { /* 忽略 */ }
  setAuthed(false)
  ElMessage.success('已退出登录')
  router.push('/login')
}

async function collect() {
  try {
    const r = await api.triggerCollect()
    if (!r.started) ElMessage.info(r.msg || '收集已在进行中')
    else ElMessage.success('收集已启动，后台运行中')
  } catch { /* interceptor 已提示 */ }
}

async function poll() {
  try {
    const s = await api.collectStatus()
    col.running = s.running
    col.status = s.status
  } catch { /* 忽略 */ }
}

onMounted(() => {
  poll()
  timer = setInterval(poll, 3000)
})
onUnmounted(() => clearInterval(timer))
</script>
