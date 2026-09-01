/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const mockedPlaywright = process.env.CHROTE_PLAYWRIGHT_MOCKED === '1'

export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
    exclude: ['tests/**', 'node_modules/**', 'dist/**'],
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    forwardConsole: false,
    proxy: mockedPlaywright ? undefined : {
      '/terminal': {
        target: 'http://localhost:7691',
        changeOrigin: true,
        ws: true,
        rewrite: (path) => path.replace(/^\/terminal/, ''),
      },
      '/bv-terminal': {
        target: 'http://localhost:8090',
        changeOrigin: true,
        ws: true,
      },
      '/api': {
        target: 'http://localhost:8090',
        changeOrigin: true,
      },
    },
  },
})
