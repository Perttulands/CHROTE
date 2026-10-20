/* global process */
// Disposable preview: no backend proxy, no production entrypoint changes.
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
export default defineConfig({
  plugins: [react(), {
    name: 'send-prototype-entry',
    transformIndexHtml(html) {
      return html.replace('/src/main.tsx', '/src/components/SendDrawerPrototype/main.tsx')
    },
  }],
  server: { host: process.env.CHROTE_PROTOTYPE_HOST || '127.0.0.1', port: Number(process.env.CHROTE_PROTOTYPE_PORT || 5188), strictPort: true },
})
