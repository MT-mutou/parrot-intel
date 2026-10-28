<template>
  <div class="page">
    <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 10px;">
      <div>
        <h1 class="page-title">推送渠道</h1>
        <p class="page-sub">每日简报一键推送到钉钉 / 企业微信 / 飞书 / 微信（Server酱）</p>
      </div>
      <el-button type="primary" @click="dialog = true">
        <el-icon style="margin-right: 4px"><Plus /></el-icon>添加渠道
      </el-button>
    </div>

    <el-empty v-if="!channels.length" description="还没有推送渠道，点右上角添加" />

    <el-row :gutter="14">
      <el-col :span="12" v-for="c in channels" :key="c.id" style="margin-bottom: 14px;">
        <div class="panel" style="height: 100%;">
          <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 12px;">
            <span style="font-size: 22px;">{{ icon(c.type) }}</span>
            <div style="flex: 1">
              <div style="font-weight: 600; color: var(--p-card-title);">{{ c.name }}</div>
              <div style="font-size: 12px; color: var(--p-text-3);">{{ typeLabel(c.type) }}</div>
            </div>
            <el-switch :model-value="c.enabled === 1" @change="toggleEnabled(c)" />
          </div>
          <div class="mono" style="font-size: 12px; color: var(--p-text-3); margin-bottom: 6px; word-break: break-all;">
            {{ maskWebhook(c.webhook) }}
          </div>
          <div style="font-size: 12px; margin-bottom: 12px;">
            <el-tag v-if="c.last_status === 'success'" size="small" type="success" effect="plain">最近推送成功</el-tag>
            <el-tooltip v-else-if="c.last_status" :content="c.last_status">
              <el-tag size="small" type="danger" effect="plain">最近推送失败</el-tag>
            </el-tooltip>
            <el-tag v-else size="small" type="info" effect="plain">未推送过</el-tag>
          </div>
          <div style="display: flex; gap: 8px;">
            <el-button size="small" type="primary" plain :loading="testing === c.id" @click="test(c)">测试推送</el-button>
            <el-button size="small" plain @click="edit(c)">编辑</el-button>
            <el-popconfirm title="确定删除该渠道？" @confirm="del(c)">
              <template #reference>
                <el-button size="small" type="danger" text>删除</el-button>
              </template>
            </el-popconfirm>
          </div>
        </div>
      </el-col>
    </el-row>

    <!-- 添加/编辑对话框 -->
    <el-dialog v-model="dialog" :title="editing ? '编辑渠道' : '添加推送渠道'" width="480">
      <el-form label-width="90">
        <el-form-item label="渠道类型">
          <el-select v-model="form.type" style="width: 100%" :disabled="!!editing">
            <el-option label="钉钉机器人" value="dingtalk" />
            <el-option label="企业微信机器人" value="wecom" />
            <el-option label="飞书机器人" value="feishu" />
            <el-option label="微信（Server酱）" value="serverchan" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="如：公司安全群" />
        </el-form-item>
        <el-form-item :label="form.type === 'serverchan' ? 'SendKey' : 'Webhook'">
          <el-input v-model="form.webhook"
                    :placeholder="form.type === 'serverchan' ? 'SCTxxxxxxxx' : 'https://…/webhook/send?key=…'" />
        </el-form-item>
        <el-form-item v-if="form.type === 'dingtalk' || form.type === 'feishu'" label="加签密钥">
          <el-input v-model="form.secret" placeholder="选填；机器人安全设置中的加签 Secret" show-password />
        </el-form-item>
        <el-alert v-if="form.type" :title="tips[form.type]" type="info" :closable="false" style="margin-top: 4px" />
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" @click="save">{{ editing ? '保存' : '添加' }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const channels = ref([])
const dialog = ref(false)
const editing = ref(null)
const testing = ref(0)
const form = ref({ type: 'dingtalk', name: '', webhook: '', secret: '' })

const tips = {
  dingtalk: '钉钉群 → 群设置 → 智能群助手 → 添加机器人（自定义，安全设置勾「加签」，把 Webhook 和加签密钥填到左侧）',
  wecom: '企业微信群 → 右键群 → 添加群机器人 → 复制 Webhook 地址',
  feishu: '飞书群 → 设置 → 群机器人 → 添加「自定义机器人」，开启签名校验则填加签密钥',
  serverchan: '个人微信推送：登录 sct.ftqq.com 微信扫码获得 SendKey，推送到微信「方糖」服务号',
}

function icon(t) { return { dingtalk: '💬', wecom: '🟢', feishu: '🐦', serverchan: '📩' }[t] }
function typeLabel(t) { return { dingtalk: '钉钉', wecom: '企业微信', feishu: '飞书', serverchan: '微信（Server酱）' }[t] }
function maskWebhook(s) { return s && s.length > 50 ? s.slice(0, 46) + '…' : s }

async function load() { channels.value = await api.getChannels() }

function edit(c) {
  editing.value = c
  form.value = { type: c.type, name: c.name, webhook: c.webhook, secret: '' }
  dialog.value = true
}

async function save() {
  if (!form.value.name || !form.value.webhook) {
    ElMessage.warning('名称和 Webhook 必填')
    return
  }
  if (editing.value) {
    await api.updateChannel(editing.value.id, { ...form.value, enabled: editing.value.enabled })
  } else {
    await api.addChannel(form.value)
  }
  ElMessage.success(editing.value ? '已保存' : '已添加')
  dialog.value = false
  editing.value = null
  form.value = { type: 'dingtalk', name: '', webhook: '', secret: '' }
  load()
}

async function toggleEnabled(c) {
  await api.updateChannel(c.id, { name: c.name, webhook: c.webhook, enabled: c.enabled === 1 ? 0 : 1 })
  c.enabled = c.enabled === 1 ? 0 : 1
}

async function test(c) {
  testing.value = c.id
  try {
    const r = await api.testChannel(c.id)
    if (r.ok) ElMessage.success(`测试消息已推送到「${c.name}」，请查收`)
    else ElMessage.error('推送失败：' + r.error)
    load()
  } finally { testing.value = 0 }
}

async function del(c) {
  await api.deleteChannel(c.id)
  ElMessage.success('已删除')
  load()
}

onMounted(load)
</script>
