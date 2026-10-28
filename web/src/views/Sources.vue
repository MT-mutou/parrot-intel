<template>
  <div class="page">
    <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 10px;">
      <div>
        <h1 class="page-title">来源管理</h1>
        <p class="page-sub">{{ enabledCount }}/{{ sources.length }} 个来源启用中 · RSS 订阅 / 网页爬虫 / 漏洞库 API</p>
      </div>
      <el-button type="primary" @click="dialog = true">
        <el-icon style="margin-right: 4px"><Plus /></el-icon>添加来源
      </el-button>
    </div>

    <!-- 分类分组 -->
    <div v-for="(list, cat) in grouped" :key="cat" style="margin-bottom: 18px;">
      <div style="font-weight: 600; color: var(--p-text-2); margin-bottom: 8px; font-size: 13px;">{{ cat }}（{{ list.length }}）</div>
      <el-table :data="list" size="small" class="panel">
        <el-table-column prop="name" label="名称" width="170" />
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag size="small" effect="plain" :type="typeTag(row.type).t">{{ typeTag(row.type).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="url" label="地址" min-width="260" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono" style="font-size: 12px; color: var(--p-text-3);">{{ row.url }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态 / 错误" min-width="220">
          <template #default="{ row }">
            <el-tag v-if="row.last_status === 'success'" size="small" type="success" effect="plain">✓ 正常</el-tag>
            <div v-else-if="row.last_status === 'error'" style="display: flex; align-items: center; gap: 6px;">
              <el-tag size="small" type="danger" effect="plain">✗ 异常</el-tag>
              <span class="mono" style="font-size: 11px; color: var(--p-danger, #f87171); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 150px;">{{ row.last_error || '未知错误' }}</span>
            </div>
            <el-tag v-else size="small" type="info" effect="plain">未抓取</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80">
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
    <el-dialog v-model="dialog" title="添加情报来源" width="560">
      <el-form label-width="90">
        <el-form-item label="来源类型">
          <el-radio-group v-model="form.type">
            <el-radio-button value="rss">RSS 订阅</el-radio-button>
            <el-radio-button value="html">网页爬虫</el-radio-button>
            <el-radio-button value="nvd">NVD 漏洞库</el-radio-button>
            <el-radio-button value="ghsa">GitHub 通告</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="如 奇安信 CERT" />
        </el-form-item>
        <el-form-item :label="form.type === 'rss' ? 'RSS URL' : (form.type === 'html' ? '列表页 URL' : 'API 地址')">
          <el-input v-model="form.url" :placeholder="urlPlaceholder" />
        </el-form-item>
        <el-form-item v-if="form.type === 'nvd' || form.type === 'ghsa'">
          <div style="font-size: 12px; color: var(--p-text-3); line-height: 1.7;">
            {{ form.type === 'nvd'
              ? '官方 CVE API 2.0，无需 Key。自动拉取最近 72 小时新披露漏洞，CVSE 严重级别自动映射为威胁等级。'
              : 'GitHub 安全通告 API（开源生态漏洞/供应链风险），无需 Key。自动拉取最近 7 天通告。' }}
            地址一般无需修改。
          </div>
        </el-form-item>
        <el-form-item v-if="form.type === 'html'" label="爬虫配置">
          <el-input v-model="form.config" type="textarea" :rows="8" class="mono" :placeholder="htmlConfigExample" />
          <div style="font-size: 12px; color: var(--p-text-3); margin-top: 6px; line-height: 1.7;">
            CSS 选择器 JSON：item=列表条目、title/link=条目内的标题与链接（link_attr 默认 href），
            可选 date（发布时间）、summary（摘要）、date_format（Go 时间布局）、base（相对链接基准）。
            保存后到「立即收集」验证，失败原因会显示在列表里。
          </div>
        </el-form-item>
        <el-form-item label="分类">
          <el-select v-model="form.category" style="width: 100%">
            <el-option v-for="c in ['安全媒体', '技术研究', '厂商动态', '厂商研究', '漏洞通报', '政策法规', '国际媒体', '国际通报', 'AI 安全']" :key="c" :label="c" :value="c" />
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
const form = ref({ name: '', url: '', category: '安全媒体', type: 'rss', config: '' })

const htmlConfigExample = `{
  "list_url": "https://example.com/news",
  "item": ".news-list li",
  "title": "a",
  "link": "a",
  "link_attr": "href",
  "date": ".date",
  "summary": ".desc"
}`

const urlPlaceholder = computed(() => ({
  rss: 'https://example.com/rss',
  html: 'https://example.com/news（列表页）',
  nvd: 'https://services.nvd.nist.gov/rest/json/cves/2.0',
  ghsa: 'https://api.github.com/advisories',
}[form.value.type] || ''))

function typeTag(t) {
  return {
    rss: { label: 'RSS', t: 'info' },
    html: { label: '爬虫', t: 'warning' },
    nvd: { label: 'NVD', t: 'danger' },
    ghsa: { label: 'GitHub', t: 'primary' },
  }[t || 'rss'] || { label: 'RSS', t: 'info' }
}

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
  if (form.value.type === 'html') {
    try { JSON.parse(form.value.config || '{}') }
    catch { ElMessage.warning('爬虫配置不是合法 JSON'); return }
  }
  await api.addSource(form.value)
  ElMessage.success('已添加，可到概览页点「立即收集」验证')
  dialog.value = false
  form.value = { name: '', url: '', category: '安全媒体', type: 'rss', config: '' }
  load()
}

onMounted(load)
</script>
