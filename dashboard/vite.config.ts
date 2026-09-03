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
    // Dev mode reads the component under the pointer off the React fiber, and
    // all a fiber carries is the function itself. The minifier renames every
    // function it can, so without this the served bundle could only ever answer
    // "a" and the annotation an agent receives would name nothing. Rolldown
    // spells esbuild's keepNames as an output option.
    rollupOptions: {
      output: { keepNames: true },
    },
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
