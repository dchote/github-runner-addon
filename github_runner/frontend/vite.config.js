import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vuetify from 'vite-plugin-vuetify'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  base: './',
  test: {
    environment: 'jsdom',
  },
  css: {
    preprocessorOptions: {
      scss: {
        api: 'modern-compiler',
      },
    },
  },
  plugins: [
    vue(),
    vuetify({
      autoImport: true,
      styles: { configFile: 'src/styles/settings.scss' },
    }),
    {
      name: 'vuetify-layer-order',
      transformIndexHtml: {
        order: 'pre',
        handler: () => [
          {
            tag: 'style',
            injectTo: 'head-prepend',
            children:
              '@layer vuetify-core, vuetify-components, vuetify-overrides, vuetify-utilities, vuetify-final;',
          },
        ],
      },
    },
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8099', changeOrigin: true },
      '/docs': { target: 'http://127.0.0.1:8099', changeOrigin: true },
      '/health': { target: 'http://127.0.0.1:8099', changeOrigin: true },
      '/ws': { target: 'ws://127.0.0.1:8099', ws: true },
    },
  },
})
