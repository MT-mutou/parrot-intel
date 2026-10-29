<template>
  <div class="page" style="max-width: 720px;">
    <h1 class="page-title">系统设置</h1>
    <p class="page-sub">AI 分析引擎 / 服务器地址 / 定时任务</p>

    <!-- AI 配置 -->
    <div class="panel" style="margin-bottom: 16px;">
      <div style="font-weight: 600; color: var(--p-accent); margin-bottom: 4px;">AI 分析引擎</div>
      <div style="font-size: 12px; color: var(--p-text-3); margin-bottom: 14px;">
        OpenAI 兼容端点：DeepSeek / 通义千问 / Kimi / OpenAI。填写端点与 API Key 后点「获取模型列表」，即可从该账号实际可用的模型中自由选择。配置后自动为情报生成中文摘要、威胁评级与通俗解读，保存即时生效。
      </div>
      <el-form label-width="90">
        <el-form-item label="服务商">
          <el-select v-model="provider" style="width: 260px" @change="onProvider">
            <el-option v-for="p in providers" :key="p.name" :label="p.name" :value="p.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="API 端点">
          <el-input v-model="form.ai_endpoint" placeholder="https://api.deepseek.com/v1" />
        </el-form-item>
        <el-form-item label="模型">
          <div style="display: flex; gap: 8px; width: 100%;">
            <el-select v-model="form.ai_model" filterable allow-create default-first-fit
                       style="flex: 1;" placeholder="点击右侧按钮获取，或直接输入模型名">
              <el-option v-for="m in modelOptions" :key="m.value" :label="m.label" :value="m.value" />
            </el-select>
            <el-button :loading="loadingModels" @click="fetchModels">获取模型列表</el-button>
          </div>
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="form.ai_api_key" type="password" show-password
                    :placeholder="form.ai_api_key_set ? '已配置（输入新值可覆盖）' : 'sk-…'" />
        </el-form-item>
        <div style="color: var(--p-text-3); font-size: 12px;" v-if="provider === '自定义'">
          任何 OpenAI 兼容服务均可：填入端点（含 /v1）与模型名即可
        </div>
      </el-form>
    </div>

    <!-- 定时任务 -->
    <div class="panel" style="margin-bottom: 16px;">
      <div style="font-weight: 600; color: var(--p-accent); margin-bottom: 4px;">定时任务</div>
      <div style="font-size: 12px; color: var(--p-text-3); margin-bottom: 14px;">进程内自动调度，无需服务器 crontab（时区 Asia/Shanghai）</div>
      <el-form label-width="120">
        <el-form-item label="每日收集时间">
          <el-select v-model="form.collect_hour" style="width: 140px">
            <el-option v-for="h in 24" :key="h - 1" :label="String(h - 1).padStart(2, '0') + ':00'" :value="String(h - 1)" />
          </el-select>
        </el-form-item>
        <el-form-item label="自动推送简报">
          <el-switch v-model="form.auto_briefing_push" />
          <span style="margin-left: 10px; font-size: 12px; color: var(--p-text-3);">
            每日收集完成后，为所有「启用」的简报模板分别生成简报并推送到全部启用渠道（模板可在简报页管理）
          </span>
        </el-form-item>
      </el-form>
    </div>

    <!-- 服务器 -->
    <div class="panel" style="margin-bottom: 20px;">
      <div style="font-weight: 600; color: var(--p-accent); margin-bottom: 4px;">服务器地址</div>
      <div style="font-size: 12px; color: var(--p-text-3); margin-bottom: 14px;">用于推送消息中回链到情报站（选填）</div>
      <el-form label-width="120">
        <el-form-item label="外网地址">
          <el-input v-model="form.server_url" placeholder="http://your-server-ip" />
        </el-form-item>
      </el-form>
    </div>

    <!-- 网络代理 -->
    <div class="panel" style="margin-bottom: 20px;">
      <div style="font-weight: 600; color: var(--p-accent); margin-bottom: 4px;">收集网络代理</div>
      <div style="font-size: 12px; color: var(--p-text-3); margin-bottom: 14px;">
        境内服务器无法直连部分国际源（feedburner / blogspot / Google 系等）。若有可用的 HTTP 代理，填入后所有情报抓取将经代理转发；留空则直连。
      </div>
      <el-form label-width="120">
        <el-form-item label="代理地址">
          <el-input v-model="form.collector_proxy" placeholder="http://127.0.0.1:7890（留空=不使用代理）" />
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

// ---- AI 服务商预设（端点 + 最新模型列表，2026-10）----
const providers = [
  {
    name: 'DeepSeek',
    endpoint: 'https://api.deepseek.com/v1',
    models: [
      { label: 'deepseek-chat（V3 通用）', value: 'deepseek-chat' },
      { label: 'deepseek-reasoner（R1 推理）', value: 'deepseek-reasoner' },
    ],
  },
  {
    name: '通义千问',
    endpoint: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    models: [
      { label: 'qwen-max-latest（旗舰）', value: 'qwen-max-latest' },
      { label: 'qwen-plus-latest（均衡，推荐）', value: 'qwen-plus-latest' },
      { label: 'qwen-turbo-latest（高速）', value: 'qwen-turbo-latest' },
      { label: 'qwen3-max', value: 'qwen3-max' },
      { label: 'qwen-max', value: 'qwen-max' },
      { label: 'qwen-plus', value: 'qwen-plus' },
    ],
  },
  {
    name: 'Kimi（月之暗面）',
    endpoint: 'https://api.moonshot.cn/v1',
    models: [
      { label: 'kimi-latest（最新版 Kimi）', value: 'kimi-latest' },
      { label: 'kimi-k2-turbo-preview（K2 高速）', value: 'kimi-k2-turbo-preview' },
      { label: 'moonshot-v1-128k（长文本）', value: 'moonshot-v1-128k' },
      { label: 'moonshot-v1-32k', value: 'moonshot-v1-32k' },
    ],
  },
  {
    name: 'OpenAI',
    endpoint: 'https://api.openai.com/v1',
    models: [
      { label: 'gpt-5', value: 'gpt-5' },
      { label: 'gpt-5-mini', value: 'gpt-5-mini' },
      { label: 'gpt-4.1', value: 'gpt-4.1' },
      { label: 'gpt-4.1-mini', value: 'gpt-4.1-mini' },
      { label: 'gpt-4o', value: 'gpt-4o' },
      { label: 'gpt-4o-mini', value: 'gpt-4o-mini' },
    ],
  },
  { name: '自定义', endpoint: '', models: [] },
]

const provider = ref('DeepSeek')

// 模型下拉选项：默认用预设，点击「获取模型列表」后替换为端点实际返回的模型
const modelOptions = ref([])
function presetModels() {
  const p = providers.find(x => x.name === provider.value)
  const list = p ? [...p.models] : []
  // 当前已填的模型不在预设里也要能显示
  if (form.value.ai_model && !list.some(m => m.value === form.value.ai_model)) {
    list.unshift({ label: form.value.ai_model, value: form.value.ai_model })
  }
  modelOptions.value = list
}

function onProvider(name) {
  const p = providers.find(x => x.name === name)
  if (p && p.endpoint) {
    form.value.ai_endpoint = p.endpoint
    if (p.models.length) form.value.ai_model = p.models[0].value
  }
  presetModels()
}

const loadingModels = ref(false)
async function fetchModels() {
  if (!form.value.ai_endpoint) {
    ElMessage.warning('请先填写 API 端点')
    return
  }
  loadingModels.value = true
  try {
    // api_key 留空时后端自动使用已保存的 Key
    const r = await api.aiModels({
      endpoint: form.value.ai_endpoint,
      api_key: form.value.ai_api_key || undefined,
    })
    const ids = r.models || []
    const list = ids.map(id => ({ label: id, value: id }))
    // 当前选中的模型不在实际列表里也保留（可对比后手动换）
    if (form.value.ai_model && !ids.includes(form.value.ai_model)) {
      list.unshift({ label: form.value.ai_model + '（当前配置，端点未返回）', value: form.value.ai_model })
    }
    modelOptions.value = list
    ElMessage.success(`已获取 ${ids.length} 个可用模型（端点与 Key 验证通过）`)
  } finally { loadingModels.value = false }
}

// 按端点反推服务商（编辑已保存配置时）
function detectProvider(endpoint) {
  if (!endpoint) return 'DeepSeek'
  const hit = providers.find(p => p.endpoint && endpoint.startsWith(p.endpoint.replace(/\/v1$/, '')))
  return hit ? hit.name : '自定义'
}

const form = ref({
  ai_endpoint: '', ai_model: '', ai_api_key: '', ai_api_key_set: false,
  collect_hour: '9', auto_briefing_push: false, server_url: '', collector_proxy: '',
})
const saving = ref(false)

async function load() {
  const s = await api.getSettings()
  Object.assign(form.value, s)
  provider.value = detectProvider(form.value.ai_endpoint)
  presetModels()
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
      collector_proxy: form.value.collector_proxy || '',
    })
    ElMessage.success('设置已保存，即时生效（无需重启）')
    form.value.ai_api_key = ''
    load()
  } finally { saving.value = false }
}

onMounted(load)
</script>
