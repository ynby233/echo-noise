import { scopedModulePreload } from './build/scoped-module-preload.mjs'

const env = (globalThis as typeof globalThis & {
  process?: {
    env?: Record<string, string | undefined>
  }
}).process?.env ?? {}

const isDevelopment = env.NODE_ENV === 'development'
const buildIdentity = String(env.VITE_APP_VERSION || env.APP_VERSION || 'development')
  .trim()
  .replace(/[^0-9A-Za-z._-]+/g, '-')
  .slice(0, 64) || 'development'

export default defineNuxtConfig({
  ssr: false,
  app: {
    head: {
      link: [
        { rel: 'preconnect', href: 'https://cdn.jsdelivr.net', crossorigin: '' },
        { rel: 'dns-prefetch', href: 'https://cdn.jsdelivr.net' },
        { rel: 'preconnect', href: 'https://unpkg.com', crossorigin: '' },
        { rel: 'dns-prefetch', href: 'https://unpkg.com' },
        { rel: 'icon', href: '/favicon.svg' },
        { rel: 'apple-touch-icon', href: '/apple-touch-icon.png' },
      ],
      meta: [
        { name: "viewport", content: "width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no, viewport-fit=cover" }
      ],
      title: '个人站点'
    }
  },
  compatibilityDate: '2024-11-01',
  devtools: { enabled: true },
  vite: {
    plugins: [scopedModulePreload()],
    define: {
      __PWA_BUILD_ID__: JSON.stringify(buildIdentity),
    },
    server: {
      proxy: {
        '/api': {
          target: 'http://localhost:1314',
          changeOrigin: true
        },
        '/rss': {
          target: 'http://localhost:1314',
          changeOrigin: true
        },
        '/manifest.json': {
          target: 'http://localhost:1314',
          changeOrigin: true
        },
        '/manifest.webmanifest': {
          target: 'http://localhost:1314',
          changeOrigin: true
        }
      }
    }
  },
  plugins: [
    '~/plugins/fetch.ts',
    '~/plugins/pwa.client'
  ],
  modules: [
    '@nuxt/ui',
    '@nuxtjs/tailwindcss',
    '@pinia/nuxt',
    '@vite-pwa/nuxt',
  ],
  pwa: {
    registerType: 'prompt',
    strategies: 'injectManifest',
    srcDir: 'service-worker',
    filename: 'sw.ts',
    injectRegister: false,
    manifest: false,
    includeAssets: ['offline.html'],
    client: {
      registerPlugin: false,
      installPrompt: false,
    },
    injectManifest: {
      // Online pages use NetworkFirst; precached HTML would bypass that route.
      globPatterns: ['**/*.{js,css,ico,png,svg,webp}', 'offline.html'],
      // Recovery modules are requested only after an online chunk failure.
      globIgnores: ['_nuxt/__retry__/**'],
      maximumFileSizeToCacheInBytes: 6 * 1024 * 1024,
    },
    devOptions: {
      enabled: false,
    },
  },
  css: [
    '@/assets/fonts/result.css',
    '@/assets/css/attachment-audio-player.css',
    '@/assets/css/attachment-failure.css',
    '@/assets/css/markdown-line-spacing.css',
  ],
  colorMode: {
    preference: 'light'
  },
  runtimeConfig: {
    public: {
      baseApi: isDevelopment ? '/api' : (env.BASE_API || '/api'),
    }
  },
  // 添加以下配置
  nitro: {
    preset: isDevelopment ? undefined : 'static',
    devProxy: {
      '/api': {
        target: 'http://localhost:1314',
        changeOrigin: true
      },
      '/rss': {
        target: 'http://localhost:1314',
        changeOrigin: true
      },
      '/manifest.json': {
        target: 'http://localhost:1314',
        changeOrigin: true
      },
      '/manifest.webmanifest': {
        target: 'http://localhost:1314',
        changeOrigin: true
      }
    },
    routeRules: {
      '/**': {
        headers: {
          'Content-Security-Policy': "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https:; style-src 'self' 'unsafe-inline' https:; img-src 'self' data: https: http:; font-src 'self' https:; connect-src 'self' http: https:; frame-src 'self' https:;"
        }
      }
    }
  },
  build: {
    transpile: ['@heroicons/vue'],
  },
  experimental: {
    payloadExtraction: false
  },
  devServer: {
    port: 1314,
    host: '0.0.0.0'
  },
})
