<template>
  <div class="page" v-if="a">
    <el-page-header @back="$router.back()" style="margin-bottom: 16px">
      <template #content>
        <span style="font-size: 15px; font-weight: 600;">情报详情</span>
      </template>
    </el-page-header>

    <div class="panel">
      <h2 style="margin: 0 0 10px; color: #e8f1fa; line-height: 1.5;">{{ a.title }}</h2>
      <div class="article-card meta" style="margin-bottom: 14px; display: flex; gap: 12px; flex-wrap: wrap; font-size: 13px; color: #64748b;">
        <span v-if="a.threat_level" class="threat-tag" :class="'threat-' + a.threat_level">{{ threatLabel(a.threat_level) }}</span>
        <el-tag v-if="a.category" size="small" effect="plain">{{ a.category }}</el-tag>
        <span v-if="a.region">{{ a.region }}</span>
        <span>📡 {{ a.source_name }}</span>
        <span v-if="a.published_at">🕐 {{ a.published_at }}</span>
      </div>

      <div style="display: flex; gap: 10px; margin-bottom: 18px;">
        <el-button size="small" @click.stop="star" :type="a.is_starred ? 'warning' : ''" plain>
          <el-icon style="margin-right: 4px"><Star /></el-icon>{{ a.is_starred ? '已收藏' : '收藏' }}
        </el-button>
        <el-button size="small" type="primary" plain @click="openOrigin">
          <el-icon style="margin-right: 4px"><Position /></el-icon>查看原文
        </el-button>
        <el-button v-if="!a.ai_analyzed" size="small" type="success" plain :loading="analyzing" @click="analyze">
          <el-icon style="margin-right: 4px"><MagicStick /></el-icon>AI 分析
        </el-button>
      </div>

      <!-- AI 分析结果 -->
      <template v-if="a.ai_analyzed">
        <el-alert v-if="a.plain_explanation" :title="'通俗解读：' + a.plain_explanation"
                  type="info" :closable="false" style="margin-bottom: 14px;" />

        <div v-if="a.summary" style="margin-bottom: 14px;">
          <div style="font-weight: 600; color: #38bdf8; margin-bottom: 6px;">AI 摘要</div>
          <div style="color: #c9d6e4; line-height: 1.8;">{{ a.summary }}</div>
        </div>

        <div style="display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 8px;">
          <el-tag v-for="t in a.tags" :key="t" size="small" effect="dark" type="info" round># {{ t }}</el-tag>
          <a v-for="cve in a.cve_ids" :key="cve" :href="'https://nvd.nist.gov/vuln/detail/' + cve"
             target="_blank" class="mono" style="color: #fb923c; text-decoration: none; font-size: 12px;">
            {{ cve }} ↗
          </a>
        </div>
      </template>

      <el-divider />

      <!-- 原文内容 -->
      <div style="font-weight: 600; color: #8ea2b8; margin-bottom: 8px;">原文内容</div>
      <div style="color: #a8b9cc; line-height: 1.9; white-space: pre-wrap; font-size: 14px; max-height: 520px; overflow-y: auto;">{{ a.content || '（原文内容为空，请查看原文链接）' }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'

const route = useRoute()
const a = ref(null)
const analyzing = ref(false)

function threatLabel(t) { return { high: '高危', medium: '中危', low: '低危' }[t] || t }

async function load() {
  a.value = await api.getArticle(route.params.id)
}

async function star() {
  const r = await api.starArticle(a.value.id)
  a.value.is_starred = r.starred ? 1 : 0
}

function openOrigin() {
  window.open(a.value.url, '_blank')
}

async function analyze() {
  analyzing.value = true
  try {
    const r = await api.analyzeArticle(a.value.id)
    ElMessage.success('AI 分析完成')
    Object.assign(a.value, {
      summary: r.summary, category: r.category, threat_level: r.threat_level,
      tags: r.tags, plain_explanation: r.plain_explanation, ai_analyzed: 1,
    })
  } finally { analyzing.value = false }
}

onMounted(load)
</script>
