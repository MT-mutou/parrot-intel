<template>
  <div class="page">
    <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 10px;">
      <div>
        <h1 class="page-title">来源管理</h1>
        <p class="page-sub">{{ enabledCount }}/{{ sources.length }} 个来源启用中 · 涵盖国内媒体/厂商/漏洞通报/国际媒体/CISA 等</p>
      </div>
      <el-button type="primary" @click="dialog = true">
        <el-icon style="margin-right: 4px"><Plus /></el-icon>添加来源
      </el-button>
    </div>

    <!-- 分类分组 -->
    <div v-for="(list, cat) in grouped" :key="cat" style="margin-bottom: 18px;">
      <div style="font-weight: 600; color: var(--p-text-2); margin-bottom: 8px; font-size: 13px;">{{ cat }}（{{ list.length }}）</div>
      <el-table :data="list" size="small" class="panel">
        <el-table-column prop="name" label="名称" width="180" />
        <el-table-column prop="url" label="RSS 地址" min-width="300" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono" style="font-size: 12px; color: var(--p-text-3);">{{ row.url }}</span>
          </template>
        </el-table-column>
        <el-table-column label="最近抓取" width="170">
          <template #default="{ row }">
            <el-tag v-if="row.last_status === 'success'" size="small" type="success" effect="plain">✓ 正常</el-tag>
            <el-tooltip v-else-if="row.last_status === 'error'" :content="row.last_error || ''">
              <el-tag size="small" type="danger" effect="plain">✗ 异常</el-tag>
            </el-tooltip>
            <el-tag v-else size="small" type="info" effect="plain">未抓取</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="90">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled === 1" @change="toggle(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80">
          <template #default="{ row }">
            <el-popconfirm title="确定删除该来源？" @confirm="del(row)">
              <template #reference>
                <el-button size="small" type="danger" text>删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 添加对话框 -->
    <el-dialog v-model="dialog" title="添加 RSS 来源" width="460">
      <el-form label-width="70">
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="如 奇安信 CERT" />
        </el-form-item>
        <el-form-item label="RSS URL">
          <el-input v-model="form.url" placeholder="https://example.com/rss" />
        </el-form-item>
        <el-form-item label="分类">
          <el-select v-model="form.category" style="width: 100%">
            <el-option v-for="c in ['安全媒体', '技术研究', '厂商动态', '漏洞通报', '政策法规', '国际媒体', '国际通报', 'AI 安全']" :key="c" :label="c" :value="c" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" @click="add">添加</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const sources = ref([])
const dialog = ref(false)
const form = ref({ name: '', url: '', category: '安全媒体' })

const grouped = computed(() => {
  const g = {}
  for (const s of sources.value) {
    const c = s.category || '未分类'
    ;(g[c] = g[c] || []).push(s)
  }
  return g
})
const enabledCount = computed(() => sources.value.filter(s => s.enabled === 1).length)

async function load() { sources.value = await api.getSources() }

async function toggle(row) {
  await api.toggleSource(row.id)
  row.enabled = row.enabled === 1 ? 0 : 1
}

async function del(row) {
  await api.deleteSource(row.id)
  ElMessage.success('已删除')
  load()
}

async function add() {
  if (!form.value.name || !form.value.url) {
    ElMessage.warning('名称和 URL 必填')
    return
  }
  await api.addSource(form.value)
  ElMessage.success('已添加')
  dialog.value = false
  form.value = { name: '', url: '', category: '安全媒体' }
  load()
}

onMounted(load)
</script>
