<template>
  <div class="page">
    <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 10px;">
      <div>
        <h1 class="page-title">每日简报</h1>
        <p class="page-sub">按角色定制：领导看形势政策，安全管理人员看漏洞处置；生成后按日期留存回看</p>
      </div>
      <div style="display: flex; gap: 10px; flex-wrap: wrap; align-items: center;">
        <el-select v-model="profileId" style="width: 170px" placeholder="选择简报模板" @change="onProfileChange">
          <el-option v-for="p in profiles" :key="p.id" :value="p.id" :label="p.name"
                     :disabled="p.enabled !== 1" />
        </el-select>
        <el-date-picker v-model="date" type="date" value-format="YYYY-MM-DD" :clearable="false"
                        style="width: 150px" @change="load" />
        <el-button type="primary" :loading="generating" :disabled="!profileId" @click="generate">
          <el-icon style="margin-right: 4px"><MagicStick /></el-icon>生成简报
        </el-button>
        <el-dropdown @command="push">
          <el-button type="success" :loading="pushing" :disabled="!content">
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
        <el-button plain @click="openManage">
          <el-icon style="margin-right: 4px"><Setting /></el-icon>管理模板
        </el-button>
      </div>
    </div>

    <!-- 当前模板说明 -->
    <div v-if="currentProfile" class="panel" style="padding: 10px 14px; margin-bottom: 14px; display: flex; align-items: center; gap: 10px; flex-wrap: wrap;">
      <el-tag :type="roleTagType(currentProfile.role)" effect="dark" size="small">{{ roleLabel(currentProfile.role) }}</el-tag>
      <span style="font-size: 13px; color: var(--p-text-2);">{{ roleDesc(currentProfile.role, currentProfile.custom_prompt) }}</span>
      <span style="flex: 1"></span>
      <el-tag v-for="k in currentProfile.sections" :key="k" size="small" effect="plain" style="margin-left: 4px;">
        {{ sectionLabel(k) }}
      </el-tag>
      <el-tag size="small" type="info" effect="plain">{{ detailLabel(currentProfile.detail) }}</el-tag>
    </div>

    <!-- 推送结果 -->
    <el-alert v-if="pushResults" :type="pushResults.every(r => r.ok) ? 'success' : 'warning'" :closable="true" style="margin-bottom: 14px;">
      <div v-for="r in pushResults" :key="r.id" style="font-size: 13px;">
        {{ r.ok ? '✅' : '❌' }} {{ r.name }}（{{ typeLabel(r.type) }}）{{ r.ok ? '推送成功' : '失败：' + r.error }}
      </div>
    </el-alert>

    <!-- 历史简报快捷切换（当前模板） -->
    <div v-if="briefingList.length" style="margin-bottom: 14px; display: flex; gap: 8px; flex-wrap: wrap; align-items: center;">
      <span style="font-size: 12px; color: var(--p-text-3);">历史留存：</span>
      <el-tag v-for="b in briefingList.slice(0, 14)" :key="b.date"
              :effect="b.date === date ? 'dark' : 'plain'" style="cursor: pointer"
              @click="date = b.date; load()">
        {{ b.date.slice(5) }}
      </el-tag>
    </div>

    <!-- 简报内容 -->
    <div class="panel briefing-md" v-loading="loading">
      <div v-if="content" v-html="rendered"></div>
      <el-empty v-else-if="!loading" :description="emptyDesc" />
    </div>

    <!-- 模板管理对话框 -->
    <el-dialog v-model="manageDialog" title="简报模板管理" width="860">
      <el-alert type="info" :closable="false" style="margin-bottom: 12px"
                title="每个模板对应一类读者（角色）+ 自选内容模块。生成与自动推送均按「启用」的模板分别产出，互不覆盖，按日期留存。" />
      <el-table :data="profiles" size="small">
        <el-table-column label="名称" prop="name" width="130">
          <template #default="{ row }">
            <span style="font-weight: 600;">{{ row.name }}</span>
            <el-tag v-if="row.id === profileId" size="small" type="primary" effect="plain" style="margin-left: 4px;">当前</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="角色" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="roleTagType(row.role)" effect="plain">{{ roleLabel(row.role) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="内容模块" min-width="230">
          <template #default="{ row }">
            <el-tag v-for="k in row.sections" :key="k" size="small" effect="plain" style="margin: 1px 2px;">{{ sectionLabel(k) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="详略" width="70">
          <template #default="{ row }">{{ detailLabel(row.detail) }}</template>
        </el-table-column>
        <el-table-column label="启用" width="60">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled === 1" @change="toggleProfile(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button size="small" plain @click="editProfile(row)">编辑</el-button>
            <el-popconfirm title="删除模板将同时删除其历史简报，确定？" @confirm="delProfile(row)">
              <template #reference>
                <el-button size="small" type="danger" text>删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
      <div style="margin-top: 12px;">
        <el-button type="primary" plain size="small" @click="addProfile">
          <el-icon style="margin-right: 4px"><Plus /></el-icon>新增模板
        </el-button>
      </div>
    </el-dialog>

    <!-- 模板编辑对话框 -->
    <el-dialog v-model="editDialog" :title="editingId ? '编辑简报模板' : '新增简报模板'" width="560" append-to-body>
      <el-form label-width="92">
        <el-form-item label="模板名称">
          <el-input v-model="form.name" placeholder="如：领导决策版 / 周报·技术版" maxlength="30" />
        </el-form-item>
        <el-form-item label="目标读者">
          <el-select v-model="form.role" style="width: 100%" @change="onRoleChange">
            <el-option v-for="r in roleOptions" :key="r.value" :value="r.value" :label="r.label" />
          </el-select>
          <div style="font-size: 12px; color: var(--p-text-3); line-height: 1.6; margin-top: 4px;">
            {{ roleDesc(form.role, '') }}
          </div>
        </el-form-item>
        <el-form-item label="内容模块">
          <el-checkbox-group v-model="form.sections">
            <el-checkbox v-for="s in sectionCatalog" :key="s.key" :value="s.key" style="width: 46%; margin-right: 0;">
              {{ s.label }}
            </el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item label="详略程度">
          <el-radio-group v-model="form.detail">
            <el-radio value="brief">要点式（约 400 字）</el-radio>
            <el-radio value="normal">标准</el-radio>
            <el-radio value="detailed">详尽（含技术细节）</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="form.role === 'custom' ? '读者描述' : '补充要求'">
          <el-input v-model="form.custom_prompt" type="textarea" :rows="3"
                    :placeholder="form.role === 'custom' ? '描述这份简报给谁看、关注什么，如：面向集团风控委员会，关注金融行业监管处罚与数据合规…' : '选填；对写作的额外要求，如：突出金融行业相关情报…'" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialog = false">取消</el-button>
        <el-button type="primary" @click="saveProfile">{{ editingId ? '保存' : '添加' }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import api from '../api'

// ---- 角色 / 模块目录（与后端 ai.BriefingRoles / BriefingSectionCatalog 对齐）----
const roleOptions = [
  { value: 'executive', label: '领导决策层' },
  { value: 'manager', label: '安全管理人员' },
  { value: 'technical', label: '技术运营团队' },
  { value: 'compliance', label: '合规风控岗' },
  { value: 'custom', label: '自定义' },
]
const roleDescs = {
  executive: '面向单位领导：宏观形势、政策趋势、整体风险影响与决策建议，不含技术细节',
  manager: '面向安全管理人员：漏洞风险等级、处置优先级、资源协调与可落地行动清单',
  technical: '面向一线技术团队：CVE 编号、受影响版本、攻击手法与修复验证方式',
  compliance: '面向合规风控：政策法规、监管处罚、合规义务与上报要求',
  custom: '自定义读者：请在下方「读者描述」中写明这份简报给谁看、关注什么',
}
const roleDefaults = {
  executive: ['overview', 'policy', 'incidents', 'actions'],
  manager: ['overview', 'vulnerabilities', 'incidents', 'exploitation', 'actions'],
  technical: ['vulnerabilities', 'exploitation', 'research', 'actions'],
  compliance: ['policy', 'incidents', 'overview', 'actions'],
  custom: ['overview', 'vulnerabilities', 'actions'],
}
const sectionCatalog = [
  { key: 'overview', label: '当日形势总览' },
  { key: 'policy', label: '政策法规与监管' },
  { key: 'vulnerabilities', label: '高危漏洞与修复' },
  { key: 'incidents', label: '攻击与泄露事件' },
  { key: 'exploitation', label: '在野利用与威胁趋势' },
  { key: 'vendors', label: '厂商与产品动态' },
  { key: 'ai', label: 'AI 安全' },
  { key: 'research', label: '技术研究' },
  { key: 'actions', label: '处置建议与行动清单' },
]

function roleLabel(r) { return roleOptions.find(o => o.value === r)?.label || r }
function roleDesc(r, custom) { return r === 'custom' && custom ? custom : (roleDescs[r] || '') }
function roleTagType(r) {
  return { executive: 'warning', manager: 'primary', technical: 'success', compliance: 'danger', custom: 'info' }[r] || 'info'
}
function sectionLabel(k) { return sectionCatalog.find(s => s.key === k)?.label || k }
function detailLabel(d) { return { brief: '要点式', normal: '标准', detailed: '详尽' }[d] || d }
function typeLabel(t) { return { dingtalk: '钉钉', wecom: '企业微信', feishu: '飞书', serverchan: '微信' }[t] || t }

// ---- 状态 ----
const date = ref(new Date().toISOString().slice(0, 10))
const profileId = ref(0)
const profiles = ref([])
const content = ref('')
const loading = ref(false)
const generating = ref(false)
const pushing = ref(false)
const briefingList = ref([])
const channels = ref([])
const pushResults = ref(null)
const manageDialog = ref(false)
const editDialog = ref(false)
const editingId = ref(0)
const form = ref({ name: '', role: 'manager', sections: [...roleDefaults.manager], detail: 'normal', custom_prompt: '' })

const rendered = computed(() => DOMPurify.sanitize(marked(content.value)))
const currentProfile = computed(() => profiles.value.find(p => p.id === profileId.value))
const emptyDesc = computed(() =>
  profileId.value ? '当日暂无该模板简报，点击「生成简报」（需先收集情报并配置 AI）' : '请先在「管理模板」中创建或启用一个简报模板')

// ---- 数据加载 ----
async function loadProfiles() {
  const r = await api.getBriefingProfiles()
  profiles.value = r.profiles || []
  if (!profiles.value.some(p => p.id === profileId.value)) {
    const first = profiles.value.find(p => p.enabled === 1)
    profileId.value = first ? first.id : 0
  }
}

async function load() {
  if (!profileId.value) { content.value = ''; briefingList.value = []; return }
  loading.value = true
  try {
    const [r, list] = await Promise.all([
      api.getBriefing(date.value, profileId.value),
      api.getBriefingList(profileId.value),
    ])
    content.value = r.content
    briefingList.value = list || []
  } finally { loading.value = false }
}

function onProfileChange() { pushResults.value = null; load() }

async function generate() {
  generating.value = true
  try {
    const r = await api.generateBriefing(date.value, profileId.value)
    content.value = r.content
    ElMessage.success('简报已生成并留存')
    load()
  } finally { generating.value = false }
}

async function push(cmd) {
  pushing.value = true
  pushResults.value = null
  try {
    const ids = cmd === 'all' ? undefined : [cmd]
    const r = await api.pushBriefing(date.value, profileId.value, ids)
    pushResults.value = r.results
    load()
  } finally { pushing.value = false }
}

// ---- 模板管理 ----
function openManage() { manageDialog.value = true }

function onRoleChange() {
  // 切换角色时预填该角色的推荐模块（用户仍可自由调整）
  form.value.sections = [...roleDefaults[form.value.role]]
}

function addProfile() {
  editingId.value = 0
  form.value = { name: '', role: 'manager', sections: [...roleDefaults.manager], detail: 'normal', custom_prompt: '' }
  editDialog.value = true
}

function editProfile(p) {
  editingId.value = p.id
  form.value = { name: p.name, role: p.role, sections: [...(p.sections || [])], detail: p.detail, custom_prompt: p.custom_prompt || '' }
  editDialog.value = true
}

async function saveProfile() {
  const f = form.value
  if (!f.name.trim()) { ElMessage.warning('模板名称必填'); return }
  if (!f.sections.length) { ElMessage.warning('至少选择一个内容模块'); return }
  if (f.role === 'custom' && !f.custom_prompt.trim()) { ElMessage.warning('自定义角色需填写读者描述'); return }
  const payload = { name: f.name.trim(), role: f.role, sections: f.sections, detail: f.detail, custom_prompt: f.custom_prompt.trim(), enabled: 1 }
  if (editingId.value) {
    const cur = profiles.value.find(p => p.id === editingId.value)
    await api.updateBriefingProfile(editingId.value, { ...payload, enabled: cur ? cur.enabled : 1 })
  } else {
    await api.addBriefingProfile(payload)
  }
  ElMessage.success(editingId.value ? '模板已保存' : '模板已添加')
  editDialog.value = false
  await loadProfiles()
  load()
}

async function toggleProfile(p) {
  const next = p.enabled === 1 ? 0 : 1
  await api.updateBriefingProfile(p.id, {
    name: p.name, role: p.role, sections: p.sections, detail: p.detail,
    custom_prompt: p.custom_prompt || '', enabled: next,
  })
  p.enabled = next
  if (next === 0 && p.id === profileId.value) {
    // 当前模板被停用：切到第一个启用模板
    await loadProfiles()
    load()
  }
}

async function delProfile(p) {
  await api.deleteBriefingProfile(p.id)
  ElMessage.success('已删除')
  await loadProfiles()
  load()
}

onMounted(async () => {
  await loadProfiles()
  await load()
  channels.value = await api.getChannels()
})
</script>
