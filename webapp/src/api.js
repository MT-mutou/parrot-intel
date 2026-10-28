import axios from 'axios'
import { ElMessage } from 'element-plus'

const http = axios.create({ baseURL: '/api/v1', timeout: 120000 })

http.interceptors.response.use(
  (resp) => resp.data,
  (err) => {
    const msg = err.response?.data?.error || err.message || '请求失败'
    ElMessage.error(msg)
    return Promise.reject(new Error(msg))
  }
)

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
