/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  // 5173 在本机常被占用（xah dev stack），dev 固定 5174。
  server: {
    port: 5174,
    proxy: {
      '/api': 'http://127.0.0.1:7300',
    },
  },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 1024,
  },
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node',
  },
})
