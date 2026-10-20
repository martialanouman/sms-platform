import { tanstackRouter } from '@tanstack/router-plugin/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [
    tanstackRouter({ target: 'react', autoCodeSplitting: true, routeFileIgnorePattern: '\\.test\\.tsx?$' }),
    react(),
  ],
  build: { outDir: 'dist' },
  server: { port: 3000, strictPort: true, proxy: { '/api': 'http://localhost:8080' } },
  test: { environment: 'jsdom' },
})
