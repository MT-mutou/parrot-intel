<template>
  <div class="page">
    <div style="display: flex; align-items: baseline; justify-content: space-between; flex-wrap: wrap; gap: 8px;">
      <div>
        <h1 class="page-title">情报流</h1>
        <p class="page-sub">共 {{ total }} 条情报 · 自动收集自 40+ 国内外安全信息源</p>
      </div>
    </div>

    <!-- 筛选栏 -->
    <div class="panel" style="padding: 12px 16px; margin-bottom: 14px; display: flex; gap: 10px; flex-wrap: wrap; align-items: center;">
      <el-radio-group v-model="filter" @change="reload">
        <el-radio-button value="all">全部</el-radio-button>
        <el-radio-button value="unread">未读</el-radio-button>
        <el-radio-button value="starred">收藏</el-radio-button>
      </el-radio-group>
      <el-select v-model="category" placeholder="全部分类" clearable style="width: 160px" @change="reload">
        <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
      </el-select>
      <el-select v-model="threat" placeholder="全部等级" clearable style="width: 130px" @change="reload">
        <el-option label="高危" value="high" />
        <el-option label="中危" value="medium" />
        <el-option label="低危" value="low" />
      </el-select>
      <div style="flex: 1"></div>
      <el-input v-model="search" placeholder="搜索标题/摘要…" style="width: 240px" clearable
                @keyup.enter="reload" @clear="reload">
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
    </div>

    <!-- 情报列表 -->
    <div v-loading="loading" style="min-height: 300px">
      <el-empty v-if="!loading && articles.length === 0" description="暂无情报，点右上角「立即收集」抓取第一批" />
      <el-card v-for="a in articles" :key="a.id" class="article-card panel"
               shadow="never" style="margin-bottom: 12px" @click="$router.push('/article/' + a.id)">
        <div style="display: flex; justify-content: space-between; gap: 12px; align-items: flex-start;">
          <div class="title">
            <span v-if="!a.is_read" style="color: var(--p-accent); margin-right: 4px;">●</span>{{ a.title }}
          </div>
          <span v-if="a.threat_level" class="threat-tag" :class="'threat-' + a.threat_level">{{ threatLabel(a.threat_level) }}</span>
        </div>
        <div class="summary" v-if="a.summary">{{ a.summary }}</div>
        <div class="meta">
          <el-tag v-if="a.category" size="small" effect="plain" round>{{ a.category }}</el-tag>
          <span v-if="a.region" class="mono">{{ a.region }}</span>
          <span>📡 {{ a.source_name }}</span>
          <span v-if="a.published_at">🕐 {{ fmtTime(a.published_at) }}</span>
          <span v-for="cve in (a.cve_ids || []).slice(0, 2)" :key="cve"
                class="mono" style="color: #fb923c;">{{ cve }}</span>
          <div style="flex: 1"></div>
          <el-icon v-if="a.is_starred" color="#facc15" :size="16"><Star /></el-icon>
          <el-icon color="var(--p-text-3)" :size="15"><View /></el-icon>
        </div>
      </el-card>
    </div>

    <!-- 分页 -->
    <div style="display: flex; justify-content: center; margin-top: 18px" v-if="totalPages > 1">
      <el-pagination background layout="prev, pager, next" :total="total" :page-size="limit"
                     :current-page="page" @current-change="p => { page = p; load() }" />
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import dayjs from 'dayjs'
import api from '../api'

const route = useRoute()
const articles = ref([])
const total = ref(0), page = ref(1), totalPages = ref(1), limit = 20
const loading = ref(false)
const filter = ref('all'), category = ref(''), threat = ref(''), search = ref('')
const categories = ['安全事件·漏洞', '技术研究', '政策法规', '厂商动态', 'AI安全', '漏洞通报', '安全媒体', '国际媒体', '国际通报']

function threatLabel(t) { return { high: '高危', medium: '中危', low: '低危' }[t] || t }
function fmtTime(s) { return dayjs(s).format('MM-DD HH:mm') }

function reload() { page.value = 1; load() }

async function load() {
  loading.value = true
  try {
    const r = await api.getArticles({
      page: page.value, limit, filter: filter.value,
      category: category.value || undefined,
      threat: threat.value || undefined,
      search: search.value || undefined,
    })
    articles.value = r.articles || []
    total.value = r.total
    totalPages.value = r.total_pages
  } finally { loading.value = false }
}

onMounted(() => {
  if (route.query.search) { search.value = route.query.search; filter.value = 'all' }
  if (route.query.filter) filter.value = route.query.filter
  load()
})
</script>
