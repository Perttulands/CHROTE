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
      // One Go server serves /api and /terminal on one port (ADR-0018), and it
      // registers the route as /terminal/, so the prefix must survive.
      // changeOrigin stays off deliberately: the terminal upgrade compares the
      // browser's Origin against the Host it was addressed by, so forwarding the
      // dev server's own Host keeps the dev socket same-origin by construction
      // and needs no CORS_ORIGINS entry.
      '/terminal': {
        target: 'http://localhost:8090',
        ws: true,
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
