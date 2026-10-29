import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite-plus'

const backend = process.env.GG_BACKEND ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  build: {
    outDir: '../web/dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1500,
  },
  server: {
    port: 9000,
    proxy: {
      '/api': { target: backend, ws: true, changeOrigin: false },
      '/health': backend,
    },
  },
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node',
  },
  lint: {
    ignorePatterns: ['dist/**', 'e2e/**', 'playwright-report/**', 'test-results/**'],
  },
  fmt: {
    semi: false,
    singleQuote: true,
    printWidth: 110,
    ignorePatterns: ['playwright-report/**', 'test-results/**'],
  },
})
