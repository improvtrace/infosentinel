import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发模式:模板资产由 Vite 提供,/api 代理到本地 Go 服务
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: { '/api': 'http://127.0.0.1:8080' },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
