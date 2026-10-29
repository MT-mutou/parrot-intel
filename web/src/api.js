import axios from 'axios'
import { ElMessage } from 'element-plus'

// API 基地址：默认同源 /api/v1（Nginx 同站点反代场景，无需任何配置）；
// 前端与 API 分开部署（不同域名/端口）时，通过 VITE_API_BASE 环境变量指定完整地址，
// 如 VITE_API_BASE=https://api.example.com/api/v1（后端已启用 CORS）
const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '/api/v1',
  timeout: 120000,
  withCredentials: true, // 登录会话 cookie 跨域携带（同域场景无副作用）
})

http.interceptors.response.use(
  (resp) => resp.data,
  (err) => {
    const status = err.response?.status
    const url = err.config?.url || ''
    // /auth/me 的 401 是登录页的会话探测（预期状态），静默即可，不弹提示
    if (status === 401 && url.includes('/auth/me')) {
      return Promise.reject(new Error('未登录'))
    }
    const msg = err.response?.data?.error || err.message || '请求失败'
    // 会话失效：跳转登录页（避免在登录页本身循环）
    if (status === 401 && !location.pathname.startsWith('/login')) {
      ElMessage.warning('会话已过期，请重新登录')
      location.href = '/login'
    } else {
      ElMessage.error(msg)
    }
    return Promise.reject(new Error(msg))
  }
)

// ---- 认证 ----
export const authApi = {
  login: (username, password) => http.post('/auth/login', { username, password }),
  logout: () => http.post('/auth/logout'),
  me: () => http.get('/auth/me'),
  changePassword: (old_password, new_password) => http.post('/auth/change-password', { old_password, new_password }),
}

// ---- API 封装（与 Go 后端契约一一对应）----

export const api = {
  // 总览 / 统计
  getOverview: () => http.get('/overview'),
  getStats: () => http.get('/stats'),

  // 文章
  getArticles: (params) => http.get('/articles', { params }),
  getArticle: (id) => http.get(`/articles/${id}`),
  starArticle: (id) => http.post(`/articles/${id}/star`),
  readArticle: (id) => http.post(`/articles/${id}/read`),
  analyzeArticle: (id) => http.post(`/articles/${id}/analyze`),

  // 来源
  getSources: () => http.get('/sources'),
  addSource: (data) => http.post('/sources', data),
  toggleSource: (id) => http.post(`/sources/${id}/toggle`),
  deleteSource: (id) => http.delete(`/sources/${id}`),

  // 设置
  getSettings: () => http.get('/settings'),
  saveSettings: (data) => http.post('/settings', data),

  // 收集
  triggerCollect: () => http.post('/collect'),
  collectStatus: () => http.get('/collect/status'),

  // 简报
  getBriefing: (date) => http.get('/briefing', { params: { date } }),
  getBriefingList: () => http.get('/briefings'),
  generateBriefing: (date) => http.post('/briefing/generate', { date }),
  pushBriefing: (date, channel_ids) => http.post('/briefing/push', { date, channel_ids }),

  // 推送渠道
  getChannels: () => http.get('/channels'),
  addChannel: (data) => http.post('/channels', data),
  updateChannel: (id, data) => http.put(`/channels/${id}`, data),
  deleteChannel: (id) => http.delete(`/channels/${id}`),
  testChannel: (id) => http.post(`/channels/${id}/test`),
}

export default api
