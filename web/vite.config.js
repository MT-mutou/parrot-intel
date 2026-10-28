import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 前端独立工程：
// - 开发（npm run dev）:  /api 代理到本地 Go 后端 :8080
// - 预览（npm run preview）: /api 同样代理，用于发布前本机验证
// - 生产构建（npm run build）: 产物输出到 ./dist，由 Nginx 独立部署
const proxy = {
  '/api': process.env.API_PROXY_TARGET || 'http://127.0.0.1:8080',
}

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy,
  },
  preview: {
    port: 4173,
    proxy,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1500,
  },
})
