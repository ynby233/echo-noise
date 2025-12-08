// 极简 Service Worker - 只处理必要的缓存，不阻塞页面加载
const CACHE_VERSION = 'v4'
const MINIMAL_CACHE = `minimal-${CACHE_VERSION}`

// 只缓存最基本的文件
const CRITICAL_ASSETS = [
  '/manifest.json',
  '/favicon.svg'
]

// 安装事件 - 快速完成，不缓存页面
self.addEventListener('install', (event) => {
  // 立即激活，不等待
  self.skipWaiting()
  
  // 只缓存最关键的资源，且不阻塞
  event.waitUntil(
    caches.open(MINIMAL_CACHE).then(cache => {
      return cache.addAll(CRITICAL_ASSETS).catch(err => {
        // 即使缓存失败也不影响页面加载
        return Promise.resolve()
      })
    })
  )
})

// 激活事件 - 清理旧缓存
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then(keys => {
      return Promise.all(
        keys.filter(key => key !== MINIMAL_CACHE).map(key => caches.delete(key))
      )
    }).then(() => self.clients.claim())
  )
})

// 消息处理
self.addEventListener('message', (event) => {
  if (event.data === 'SKIP_WAITING') {
    self.skipWaiting()
  }
})

// 简化的fetch处理 - 主要用于离线访问，不影响在线性能
self.addEventListener('fetch', (event) => {
  const request = event.request
  
  // 只处理GET请求
  if (request.method !== 'GET') return
  
  // 只对manifest和图标等基本资源进行缓存处理
  const url = new URL(request.url)
  
  // 不处理API请求和其他动态内容
  if (url.pathname.startsWith('/api/') || url.pathname.includes('?')) {
    return
  }
  
  // 只对关键资源使用缓存优先策略
  if (CRITICAL_ASSETS.includes(url.pathname)) {
    event.respondWith(
      caches.match(request).then(cached => {
        // 如果有缓存，直接返回
        if (cached) return cached
        
        // 否则从网络获取
        return fetch(request).then(response => {
          // 检查响应是否有效
          if (!response || response.status !== 200 || response.type !== 'basic') {
            return response
          }
          
          // 缓存响应
          const responseClone = response.clone()
          caches.open(MINIMAL_CACHE).then(cache => {
            cache.put(request, responseClone)
          })
          
          return response
        }).catch(() => {
          // 网络失败时的简单处理
          return new Response('Offline', { status: 503 })
        })
      })
    )
  }
  
  // 其他资源不使用Service Worker，直接走网络
  // 这样可以最大化在线性能
})
