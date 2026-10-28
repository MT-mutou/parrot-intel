<template>
  <div class="page">
    <h1 class="page-title">态势总览</h1>
    <p class="page-sub">鹦鹉安全实验室 · 威胁情报态势一屏掌握</p>

    <!-- 指标卡 -->
    <el-row :gutter="14" style="margin-bottom: 14px;">
      <el-col :span="4" v-for="m in metrics" :key="m.label">
        <div class="panel stat-card">
          <div class="stat-icon" :style="{ background: m.bg }">{{ m.icon }}</div>
          <div>
            <div class="num" :style="{ color: m.color }">{{ m.value }}</div>
            <div class="label">{{ m.label }}</div>
          </div>
        </div>
      </el-col>
    </el-row>

    <!-- 图表 -->
    <el-row :gutter="14">
      <el-col :span="14">
        <div class="panel">
          <div style="font-weight: 600; margin-bottom: 10px;">近 7 日情报收集趋势</div>
          <div ref="trendRef" style="height: 300px"></div>
        </div>
      </el-col>
      <el-col :span="10">
        <div class="panel" style="margin-bottom: 14px;">
          <div style="font-weight: 600; margin-bottom: 10px;">威胁等级分布</div>
          <div ref="threatRef" style="height: 132px"></div>
        </div>
        <div class="panel">
          <div style="font-weight: 600; margin-bottom: 10px;">情报分类分布</div>
          <div ref="catRef" style="height: 132px"></div>
        </div>
      </el-col>
    </el-row>

    <!-- 来源 TOP -->
    <div class="panel" style="margin-top: 14px;">
      <div style="font-weight: 600; margin-bottom: 10px;">情报来源 TOP 10</div>
      <el-table :data="topSources" size="small">
        <el-table-column type="index" width="50" />
        <el-table-column prop="name" label="来源" />
        <el-table-column label="情报数" width="120" sortable :sort-by="(r) => r.count">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ row.count }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="占比" min-width="200">
          <template #default="{ row }">
            <el-progress :percentage="Math.round(row.count / (stats.total || 1) * 100)"
                         :stroke-width="8" :color="accentColor" />
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart, PieChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([LineChart, PieChart, BarChart, GridComponent, TooltipComponent, CanvasRenderer])
const { LinearGradient } = echarts.graphic
import api from '../api'

const stats = reactive({ total: 0, today: 0, unread: 0, starred: 0, high_threat: 0, by_category: [], by_source: [], by_threat: [], last_7days: [] })
const topSources = ref([])
const trendRef = ref(), threatRef = ref(), catRef = ref()
let charts = []
const accentColor = ref(document.documentElement.classList.contains('light') ? '#0284c7' : '#38bdf8')

const metrics = reactive([
  { label: '情报总量', icon: '📋', bg: 'rgba(56,189,248,.12)', color: '#38bdf8', value: 0 },
  { label: '今日新增', icon: '🆕', bg: 'rgba(129,140,248,.12)', color: '#818cf8', value: 0 },
  { label: '高危情报', icon: '🚨', bg: 'rgba(239,68,68,.12)', color: '#f87171', value: 0 },
  { label: '未读', icon: '📬', bg: 'rgba(245,158,11,.12)', color: '#fbbf24', value: 0 },
  { label: '收藏', icon: '⭐', bg: 'rgba(234,179,8,.12)', color: '#facc15', value: 0 },
  { label: '今日简报', icon: '📝', bg: 'rgba(34,197,94,.12)', color: '#4ade80', value: '—' },
])

async function load() {
  const s = await api.getStats()
  Object.assign(stats, s)
  topSources.value = (s.by_source || []).map(x => ({ name: x.category, count: x.count }))
  metrics[0].value = s.total
  metrics[1].value = s.today
  metrics[2].value = s.high_threat
  metrics[3].value = s.unread
  metrics[4].value = s.starred
  const today = new Date().toISOString().slice(0, 10)
  metrics[5].value = (await api.getBriefingList()).some(b => b.date === today) ? '已生成' : '未生成'
  render()
}

function render() {
  charts.forEach(c => c.dispose())
  charts = []
  // 主题感知：读取当前主题的图表配色
  const light = document.documentElement.classList.contains('light')
  const accent = light ? '#0284c7' : '#38bdf8'
  const axis = light
    ? { axisLine: { lineStyle: { color: '#c3cfdd' } }, axisLabel: { color: '#5a6b80' }, splitLine: { lineStyle: { color: '#e7edf4' } } }
    : { axisLine: { lineStyle: { color: '#24354a' } }, axisLabel: { color: '#7b8ba0' }, splitLine: { lineStyle: { color: '#16202e' } } }
  const labelColor = light ? '#5a6b80' : '#8ea2b8'
  // 趋势
  const trend = echarts.init(trendRef.value)
  trend.setOption({
    grid: { left: 40, right: 16, top: 20, bottom: 28 },
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: stats.last_7days.map(d => d.day.slice(5)), ...axis },
    yAxis: { type: 'value', ...axis },
    series: [{
      type: 'line', smooth: true, symbolSize: 7,
      data: stats.last_7days.map(d => d.count),
      lineStyle: { color: accent, width: 2.5 },
      itemStyle: { color: accent },
      areaStyle: { color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
        { offset: 0, color: light ? 'rgba(2,132,199,.18)' : 'rgba(56,189,248,.28)' }, { offset: 1, color: 'rgba(56,189,248,0)' }
      ]) },
    }],
  })
  charts.push(trend)

  // 威胁分布（饼）
  const tmap = { high: '高危', medium: '中危', low: '低危', '未评': '未评' }
  const tcolors = { high: '#ef4444', medium: '#f59e0b', low: '#22c55e', '未评': light ? '#8494a7' : '#475569' }
  const threat = echarts.init(threatRef.value)
  threat.setOption({
    tooltip: { trigger: 'item' },
    series: [{
      type: 'pie', radius: ['52%', '78%'], center: ['50%', '50%'],
      label: { color: labelColor, fontSize: 11, formatter: '{b} {c}' },
      data: (stats.by_threat || []).map(t => ({ name: tmap[t.category] || t.category, value: t.count, itemStyle: { color: tcolors[t.category] || (light ? '#8494a7' : '#475569') } })),
    }],
  })
  charts.push(threat)

  // 分类分布（横向条形）
  const cat = echarts.init(catRef.value)
  const cats = (stats.by_category || []).slice(0, 5)
  cat.setOption({
    grid: { left: 90, right: 30, top: 6, bottom: 20 },
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'value', ...axis, splitLine: { show: false }, axisLabel: { show: false }, axisLine: { show: false } },
    yAxis: { type: 'category', data: cats.map(c => c.category).reverse(), ...axis, splitLine: { show: false } },
    series: [{
      type: 'bar', barWidth: 10,
      data: cats.map(c => c.count).reverse(),
      itemStyle: { color: '#818cf8', borderRadius: [0, 5, 5, 0] },
      label: { show: true, position: 'right', color: labelColor, fontSize: 11 },
    }],
  })
  charts.push(cat)
}

onMounted(async () => {
  await load()
  window.addEventListener('resize', resize)
  // 主题切换时重绘图表
  window.addEventListener('theme-change', () => {
    accentColor.value = document.documentElement.classList.contains('light') ? '#0284c7' : '#38bdf8'
    render()
  })
})
function resize() { charts.forEach(c => c.resize()) }
</script>
