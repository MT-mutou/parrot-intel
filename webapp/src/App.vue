<template>
  <div style="display: flex; height: 100vh; overflow: hidden;">
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
      <div class="nav-item" style="color: #475569; font-size: 12px; cursor: default;">
        <span>v2.0 · Go</span>
      </div>
    </aside>

    <div style="flex: 1; display: flex; flex-direction: column; min-width: 0;">
      <!-- 顶栏 -->
      <header class="topbar">
        <div style="flex: 1"></div>

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
import { ref, reactive, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from './api'

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

function isActive(p) {
  if (p === '/') return route.path === '/'
  return route.path.startsWith(p)
}

function goSearch() {
  router.push({ path: '/intel', query: search.value ? { search: search.value } : {} })
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
