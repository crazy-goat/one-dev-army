import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: '/new/',
  build: {
    outDir: 'dist',
    rollupOptions: {
      output: {
        manualChunks: undefined,
        entryFileNames: 'assets/app.js',
        chunkFileNames: 'assets/app.js',
        assetFileNames: 'assets/app[extname]',
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:7000',
        rewrite: (path) => {
          // Map /api/v2/settings to /api/v2/settings (no change needed)
          return path
        },
      },
      '/ws': { target: 'ws://localhost:7000', ws: true },
      '/task': 'http://localhost:7000',
      '/wizard': 'http://localhost:7000',
      '/settings': 'http://localhost:7000',
      '/sprint': 'http://localhost:7000',
      '/decline': 'http://localhost:7000',
      '/approve-merge': 'http://localhost:7000',
      '/block': 'http://localhost:7000',
      '/unblock': 'http://localhost:7000',
      '/sync': 'http://localhost:7000',
      '/plan-sprint': 'http://localhost:7000',
    },
  },
})
