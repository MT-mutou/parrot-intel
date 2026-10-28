<template>
  <div class="page" style="max-width: 720px;">
    <h1 class="page-title">系统设置</h1>
    <p class="page-sub">AI 分析引擎 / 服务器地址 / 定时任务</p>

    <!-- AI 配置 -->
    <div class="panel" style="margin-bottom: 16px;">
      <div style="font-weight: 600; color: #38bdf8; margin-bottom: 4px;">AI 分析引擎</div>
      <div style="font-size: 12px; color: #64748b; margin-bottom: 14px;">
        OpenAI 兼容端点：DeepSeek / 通义千问 / Kimi / OpenAI 等。配置后自动为情报生成中文摘要、威胁评级与通俗解读。
      </div>
      <el-form label-width="90">
        <el-form-item label="API 端点">
          <el-input v-model="form.ai_endpoint" placeholder="https://api.deepseek.com/v1" />
        </el-form-item>
        <el-form-item label="模型">
          <el-input v-model="form.ai_model" placeholder="deepseek-chat" />
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="form.ai_api_key" type="password" show-password
                    :placeholder="form.ai_api_key_set ? '已配置（输入新值可覆盖）' : 'sk-…'" />
        </el-form-item>
        <div style="color: #475569; font-size: 12px;">
          常用端点：DeepSeek <code>https://api.deepseek.com/v1</code> · 通义 <code>https://dashscope.aliyuncs.com/compatible-mode/v1</code> · Kimi <code>https://api.moonshot.cn/v1</code>
        </div>
      </el-form>
    </div>

    <!-- 定时任务 -->
    <div class="panel" style="margin-bottom: 16px;">
      <div style="font-weight: 600; color: #38bdf8; margin-bottom: 4px;">定时任务</div>
      <div style="font-size: 12px; color: #64748b; margin-bottom: 14px;">进程内自动调度，无需服务器 crontab（时区 Asia/Shanghai）</div>
      <el-form label-width="120">
        <el-form-item label="每日收集时间">
          <el-select v-model="form.collect_hour" style="width: 140px">
            <el-option v-for="h in 24" :key="h - 1" :label="String(h - 1).padStart(2, '0') + ':00'" :value="String(h - 1)" />
          </el-select>
        </el-form-item>
        <el-form-item label="自动推送简报">
          <el-switch v-model="form.auto_briefing_push" />
          <span style="margin-left: 10px; font-size: 12px; color: #64748b;">
            每日收集完成后自动生成简报并推送到全部启用渠道
          </span>
        </el-form-item>
      </el-form>
    </div>

    <!-- 服务器 -->
    <div class="panel" style="margin-bottom: 20px;">
      <div style="font-weight: 600; color: #38bdf8; margin-bottom: 4px;">服务器地址</div>
      <div style="font-size: 12px; color: #64748b; margin-bottom: 14px;">用于推送消息中回链到情报站（选填）</div>
      <el-form label-width="120">
        <el-form-item label="外网地址">
          <el-input v-model="form.server_url" placeholder="http://your-server-ip:8080" />
        </el-form-item>
      </el-form>
    </div>

    <el-button type="primary" size="large" :loading="saving" @click="save" style="width: 160px;">保存设置</el-button>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const form = ref({
  ai_endpoint: '', ai_model: '', ai_api_key: '', ai_api_key_set: false,
  collect_hour: '9', auto_briefing_push: false, server_url: '',
})
const saving = ref(false)

async function load() {
  const s = await api.getSettings()
  Object.assign(form.value, s)
}

async function save() {
  saving.value = true
  try {
    await api.saveSettings({
      ai_endpoint: form.value.ai_endpoint,
      ai_model: form.value.ai_model,
      ai_api_key: form.value.ai_api_key || undefined,
      collect_hour: form.value.collect_hour,
      auto_briefing_push: String(form.value.auto_briefing_push),
      server_url: form.value.server_url,
    })
    ElMessage.success('设置已保存，即时生效（无需重启）')
    form.value.ai_api_key = ''
    load()
  } finally { saving.value = false }
}

onMounted(load)
</script>
