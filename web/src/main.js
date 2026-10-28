import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import * as Icons from '@element-plus/icons-vue'
import App from './App.vue'
import router from './router'
import './styles/theme.css'

// 主题初始化：默认暗色，用户选择持久化在 localStorage
const saved = localStorage.getItem('parrot-theme')
const theme = saved === 'light' || saved === 'dark' ? saved : 'dark'
document.documentElement.classList.remove('light', 'dark')
document.documentElement.classList.add(theme)

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(ElementPlus)
for (const [name, comp] of Object.entries(Icons)) {
  app.component(name, comp)
}
app.mount('#app')
