export default defineNuxtConfig({
  ssr: false,
  app: {
    head: {
      link: [
        { rel: 'preconnect', href: 'https://cdn.jsdelivr.net', crossorigin: '' },
        { rel: 'dns-prefetch', href: 'https://cdn.jsdelivr.net' },
        { rel: 'preconnect', href: 'https://unpkg.com', crossorigin: '' },
        { rel: 'dns-prefetch', href: 'https://unpkg.com' },
        { rel: 'preconnect', href: 'https://www.noisework.cn' },
        { rel: 'dns-prefetch', href: 'https://www.noisework.cn' },
        { rel: 'stylesheet', href: 'https://www.noisework.cn/css/APlayer.min.css' },
        { rel: 'stylesheet', href: 'https://unpkg.com/@waline/client@v3/dist/waline.css' },
        { rel: 'stylesheet', href: 'https://cdn.jsdelivr.net/npm/@fancyapps/ui@5.0/dist/fancybox/fancybox.css' },
        { rel: 'icon', href: '/favicon.svg' },
        { rel: 'apple-touch-icon', href: '/apple-touch-icon.png' },
        { rel: 'manifest', href: '/manifest.json' },
      ],
      script: [
        { src: 'https://cdn.jsdelivr.net/npm/jquery@3.2.1/dist/jquery.min.js', body: true, defer: true },
        { src: 'https://cdn.jsdelivr.net/npm/aplayer@1.10.1/dist/APlayer.min.js', body: true, defer: true },
        { src: 'https://cdn.jsdelivr.net/npm/meting@2.0.1/dist/Meting.min.js', body: true, defer: true },
        { 
          src: 'https://unpkg.com/@waline/client@v3/dist/waline.js',
          body: true,
          defer: true
        },
        { 
          src: 'https://cdn.jsdelivr.net/npm/@fancyapps/ui@5.0/dist/fancybox/fancybox.umd.js',
          body: true,
          defer: true 
        },
        { src: 'https://unpkg.com/medium-zoom/dist/medium-zoom.min.js', body: true, defer: true },
        { src: 'https://cdn.jsdelivr.net/npm/bcryptjs@2.4.3/dist/bcrypt.min.js', body: true, defer: true },
      ],
      meta: [
        { name: "viewport", content: "width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no" }
      ],
      title: '说说笔记'
    }
  },
  compatibilityDate: '2024-11-01',
  devtools: { enabled: true },
  vite: {
    server: {
      proxy: {
        '/api': {
          target: 'http://localhost:1314',
          changeOrigin: true
        },
        '/rss': {
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
    
  ],
  css: [
    '@/assets/fonts/result.css',
  ],
  colorMode: {
    preference: 'light'
  },
  runtimeConfig: {
    public: {
      baseApi: process.env.NODE_ENV === 'development' ? '/api' : (process.env.BASE_API || '/api'),
    }
  },
  // 添加以下配置
  nitro: {
    preset: process.env.NODE_ENV === 'production' ? 'static' : undefined,
    devProxy: {
      '/api': {
        target: 'http://localhost:1314',
        changeOrigin: true
      },
      '/rss': {
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
