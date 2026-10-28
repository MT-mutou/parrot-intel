<template>
  <div class="login-wrap">
    <div class="login-card">
      <div style="text-align: center; margin-bottom: 6px;">
        <span style="font-size: 44px;">🦜</span>
      </div>
      <div class="login-title">鹦鹉情报站</div>
      <div class="login-sub">ParrotIntel · 鹦鹉安全实验室 · 威胁情报聚合平台</div>

      <!-- 第一步：登录 -->
      <el-form v-if="step === 'login'" @submit.prevent="doLogin">
        <el-form-item>
          <el-input v-model="username" placeholder="账号" size="large" @keyup.enter="doLogin">
            <template #prefix><el-icon><User /></el-icon></template>
          </el-input>
        </el-form-item>
        <el-form-item>
          <el-input v-model="password" type="password" placeholder="密码" size="large" show-password @keyup.enter="doLogin">
            <template #prefix><el-icon><Lock /></el-icon></template>
          </el-input>
        </el-form-item>
        <el-button type="primary" size="large" style="width: 100%; margin-top: 6px;"
                   :loading="loading" @click="doLogin">登 录</el-button>
        <div style="font-size: 12px; color: var(--p-text-3); text-align: center; margin-top: 14px;">
          默认账号 yingwu · 密码 yingwu（首次登录需修改）
        </div>
      </el-form>

      <!-- 第二步：强制修改密码 -->
      <el-form v-else @submit.prevent="doChange">
        <el-alert type="warning" :closable="false" style="margin-bottom: 16px;">
          首次登录（或密码已重置），请先修改默认密码后继续使用
        </el-alert>
        <el-form-item>
          <el-input v-model="oldPassword" type="password" placeholder="当前密码" size="large" show-password @keyup.enter="doChange">
            <template #prefix><el-icon><Lock /></el-icon></template>
          </el-input>
        </el-form-item>
        <el-form-item>
          <el-input v-model="newPassword" type="password" placeholder="新密码（至少 6 位）" size="large" show-password @keyup.enter="doChange">
            <template #prefix><el-icon><Key /></el-icon></template>
          </el-input>
        </el-form-item>
        <el-form-item>
          <el-input v-model="confirmPassword" type="password" placeholder="确认新密码" size="large" show-password @keyup.enter="doChange">
            <template #prefix><el-icon><Key /></el-icon></template>
          </el-input>
        </el-form-item>
        <el-button type="primary" size="large" style="width: 100%; margin-top: 6px;"
                   :loading="loading" @click="doChange">修改密码并进入系统</el-button>
      </el-form>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api, { authApi } from '../api'
import { setAuthed } from '../router'

const router = useRouter()
const step = ref('login')
const loading = ref(false)

const username = ref('')
const password = ref('')
const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')

onMounted(async () => {
  // 已有有效会话：直接进入（或进入强制改密步骤）
  try {
    const me = await authApi.me()
    if (me.must_change_password) step.value = 'change'
    else router.replace('/')
  } catch { /* 未登录，停留登录页 */ }
})

async function doLogin() {
  if (!username.value || !password.value) {
    ElMessage.warning('请输入账号与密码')
    return
  }
  loading.value = true
  try {
    const r = await authApi.login(username.value, password.value)
    if (r.must_change_password) {
      step.value = 'change'
      ElMessage.info('首次登录，请修改默认密码')
    } else {
      ElMessage.success('登录成功')
      setAuthed(true)
      router.replace('/')
    }
  } finally { loading.value = false }
}

async function doChange() {
  if (!oldPassword.value || !newPassword.value) {
    ElMessage.warning('请填写当前密码与新密码')
    return
  }
  if (newPassword.value.length < 6) {
    ElMessage.warning('新密码长度至少 6 位')
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  loading.value = true
  try {
    await authApi.changePassword(oldPassword.value, newPassword.value)
    ElMessage.success('密码修改成功，欢迎进入鹦鹉情报站')
    setAuthed(true)
    router.replace('/')
  } finally { loading.value = false }
}
</script>
