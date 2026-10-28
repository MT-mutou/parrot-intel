<template>
  <div class="page">
    <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 10px;">
      <div>
        <h1 class="page-title">每日简报</h1>
        <p class="page-sub">AI 汇总当日全部情报，面向管理层与安全团队</p>
      </div>
      <div style="display: flex; gap: 10px;">
        <el-date-picker v-model="date" type="date" value-format="YYYY-MM-DD" :clearable="false"
                        style="width: 150px" @change="load" />
        <el-button type="primary" :loading="generating" @click="generate">
          <el-icon style="margin-right: 4px"><MagicStick /></el-icon>生成简报
        </el-button>
        <el-dropdown @command="push">
          <el-button type="success" :loading="pushing">
            <el-icon style="margin-right: 4px"><Promotion /></el-icon>一键推送<el-icon><ArrowDown /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="all">推送到全部启用渠道</el-dropdown-item>
              <el-dropdown-item v-for="c in channels.filter(c => c.enabled)" :key="c.id" :command="c.id">
                仅推送到「{{ c.name }}」
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </div>

    <!-- 推送结果 -->
    <el-alert v-if="pushResults" :type="pushResults.every(r => r.ok) ? 'success' : 'warning'" :closable="true" style="margin-bottom: 14px;">
      <div v-for="r in pushResults" :key="r.id" style="font-size: 13px;">
        {{ r.ok ? '✅' : '❌' }} {{ r.name }}（{{ typeLabel(r.type) }}）{{ r.ok ? '推送成功' : '失败：' + r.error }}
      </div>
    </el-alert>

    <!-- 历史简报快捷切换 -->
    <div v-if="briefingList.length" style="margin-bottom: 14px; display: flex; gap: 8px; flex-wrap: wrap;">
      <el-tag v-for="b in briefingList.slice(0, 10)" :key="b.date"
              :effect="b.date === date ? 'dark' : 'plain'" style="cursor: pointer"
              @click="date = b.date; load()">
        {{ b.date.slice(5) }}
      </el-tag>
    </div>

    <!-- 简报内容 -->
    <div class="panel briefing-md" v-loading="loading">
      <div v-if="content" v-html="rendered"></div>
      <el-empty v-else-if="!loading" description="当日暂无简报，点击「生成简报」（需先收集情报并配置 AI）" />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import api from '../api'

const date = ref(new Date().toISOString().slice(0, 10))
const content = ref('')
const loading = ref(false)
const generating = ref(false)
const pushing = ref(false)
const briefingList = ref([])
const channels = ref([])
const pushResults = ref(null)

const rendered = computed(() => DOMPurify.sanitize(marked(content.value)))

function typeLabel(t) { return { dingtalk: '钉钉', wecom: '企业微信', feishu: '飞书', serverchan: '微信' }[t] || t }

async function load() {
  loading.value = true
  try {
    const r = await api.getBriefing(date.value)
    content.value = r.content
  } finally { loading.value = false }
}

async function loadList() {
  briefingList.value = await api.getBriefingList()
  channels.value = await api.getChannels()
}

async function generate() {
  generating.value = true
  try {
    const r = await api.generateBriefing(date.value)
    content.value = r.content
    ElMessage.success('简报已生成')
    loadList()
  } finally { generating.value = false }
}

async function push(cmd) {
  pushing.value = true
  pushResults.value = null
  try {
    const ids = cmd === 'all' ? undefined : [cmd]
    const r = await api.pushBriefing(date.value, ids)
    pushResults.value = r.results
    loadList()
  } finally { pushing.value = false }
}

onMounted(() => { load(); loadList() })
</script>
