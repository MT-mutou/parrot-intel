import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 开发模式代理到 Go 后端；生产构建产物输出到 internal/web/dist（go:embed）
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1500,
  },
})
