import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  define: {
    'process.env.NODE_ENV': '"production"',
  },
  build: {
    lib: {
      entry: 'src/main.ts',
      name: 'TokenTally',
      formats: ['iife'],
    },
    outDir: '../web',
    emptyOutDir: false,
    rollupOptions: {
      output: {
        entryFileNames: 'app.bundle.js',
        assetFileNames: 'app.[ext]',
      },
    },
  },
})
