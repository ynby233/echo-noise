const CACHE_VERSION = 'v2'
const ASSET_CACHE = `assets-${CACHE_VERSION}`
const PAGE_CACHE = `pages-${CACHE_VERSION}`

const isAsset = (p) => p.includes('/_nuxt/') || /\.(js|css|png|jpg|jpeg|svg|ico|woff2?)$/i.test(p)
const isCacheable = (req, res) => req.method === 'GET' && res && res.ok && (res.type === 'basic' || res.type === 'default')

self.addEventListener('install', (event) => {
  self.skipWaiting()
  event.waitUntil(
    caches.open(ASSET_CACHE).then((cache) => cache.addAll(['/','/status','/auth/login','/auth/register']).catch(() => null))
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil((async () => {
    // 清理旧缓存
    const keys = await caches.keys()
    await Promise.all(keys.filter(k => ![ASSET_CACHE, PAGE_CACHE].includes(k)).map(k => caches.delete(k)))
    // 启用导航预加载以提升首屏速度
    if ('navigationPreload' in self.registration) {
      try { await self.registration.navigationPreload.enable() } catch {}
    }
    // 立即接管
    await self.clients.claim()
  })())
})

self.addEventListener('message', (event) => {
  if (event.data === 'SKIP_WAITING') self.skipWaiting()
})

self.addEventListener('fetch', (event) => {
  const req = event.request
  const url = new URL(req.url)
  if (req.method !== 'GET') return

  // 导航请求：优先使用导航预加载，其次网络优先，最后缓存兜底
  if (req.mode === 'navigate') {
    event.respondWith((async () => {
      try {
        const preload = 'navigationPreload' in event ? await event.preloadResponse : null
        if (preload) return preload
      } catch {}
      try {
        const res = await fetch(req)
        const cache = await caches.open(PAGE_CACHE)
        if (isCacheable(req, res)) cache.put(req, res.clone())
        return res
      } catch {
        const cache = await caches.open(PAGE_CACHE)
        const cached = await cache.match(req)
        return cached || Response.error()
      }
    })())
    return
  }

  // 资源请求：立即返回缓存，同时后台更新（stale-while-revalidate）
  if (isAsset(url.pathname)) {
    event.respondWith((async () => {
      const cache = await caches.open(ASSET_CACHE)
      const cached = await cache.match(req)
      const fetchPromise = fetch(req).then(res => {
        if (isCacheable(req, res)) cache.put(req, res.clone())
        return res
      }).catch(() => null)
      if (cached) {
        event.waitUntil(fetchPromise)
        return cached
      }
      const res = await fetchPromise
      return res || new Response('', { status: 504 })
    })())
    return
  }

  // 其它 GET：网络优先，缓存兜底
  event.respondWith((async () => {
    const cache = await caches.open(PAGE_CACHE)
    try {
      const res = await fetch(req)
      if (isCacheable(req, res)) cache.put(req, res.clone())
      return res
    } catch {
      const cached = await cache.match(req)
      return cached || new Response('', { status: 504 })
    }
  })())
})
