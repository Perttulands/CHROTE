/* global process */
// Disposable preview: the normal build and runtime never load this adapter.
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import brainPrototypeServer from './brain-prototype-server.mjs'

if (!process.env.CHROTE_PROTOTYPE_BACKEND) throw new Error('Set CHROTE_PROTOTYPE_BACKEND to the selected CHROTE runtime URL')

export default defineConfig({
  plugins: [react(), brainPrototypeServer()],
  server: {
    host: process.env.CHROTE_PROTOTYPE_HOST || '127.0.0.1',
    port: Number(process.env.CHROTE_PROTOTYPE_PORT || 5178),
    strictPort: true,
    allowedHosts: (process.env.CHROTE_PROTOTYPE_ALLOWED_HOSTS || '').split(',').filter(Boolean),
    proxy: {
      '/api': { target: process.env.CHROTE_PROTOTYPE_BACKEND, changeOrigin: true },
      '/terminal': { target: process.env.CHROTE_PROTOTYPE_BACKEND, ws: true },
    },
  },
})
