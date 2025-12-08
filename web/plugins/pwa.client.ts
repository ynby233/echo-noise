export default defineNuxtPlugin(() => {
  // 默认启用PWA，避免不必要的延迟
  let pwaEnabled = true
  let configLoaded = false
  
  // 立即注册Service Worker，不等待配置
  if ('serviceWorker' in navigator && process.client) {
    // 延迟注册，避免阻塞页面加载
    setTimeout(() => {
      navigator.serviceWorker.getRegistrations().then(async regs => {
        if (!regs || regs.length === 0) {
          try {
            await navigator.serviceWorker.register('/sw.js', { scope: '/', updateViaCache: 'none' })
            console.log('Service Worker registered successfully')
          } catch (error) {
            console.error('Service Worker registration failed:', error)
          }
        }
      })
    }, 1000) // 延迟1秒，确保页面优先加载
  }

  const applyHeadFromConfig = (fs: any) => {
    const enabled = typeof fs.pwaEnabled === 'boolean' ? fs.pwaEnabled : true
    
    // 如果配置没有改变，不需要重新应用
    if (configLoaded && pwaEnabled === enabled) return
    
    pwaEnabled = enabled
    configLoaded = true
    
    const title = (fs.pwaTitle || fs.siteTitle || '说说笔记').trim()
    const icon = (fs.pwaIconURL || '/favicon.svg')
    const description = (fs.pwaDescription || fs.description || '').trim()

  try {
    // 在 Nuxt 插件中使用 useHead 更新全局 Head
    if (enabled && useHead) {
      useHead({
        title,
        meta: [
          { name: 'description', content: description },
          { name: 'theme-color', content: '#000000' }
        ],
        link: [
          { rel: 'manifest', href: '/manifest.json' }
        ]
      })
    } else {
      // 移除可能存在的 manifest 链接
      const manifestEl = document.querySelector('link[rel="manifest"]')
      if (manifestEl) manifestEl.parentElement?.removeChild(manifestEl)
      
      // 如果禁用PWA，取消注册Service Worker
      if ('serviceWorker' in navigator) {
        navigator.serviceWorker.getRegistrations().then(async regs => {
          for (const r of regs) await r.unregister()
          const keys = await caches.keys()
          await Promise.all(keys.map(k => caches.delete(k)))
        })
      }
    }
    } catch (error) {
      console.error('Failed to apply PWA config:', error)
    }
  }

  // 使用节流机制，避免频繁请求配置
  let configTimeout = null
  const loadAndApplyThrottled = () => {
    if (configTimeout) clearTimeout(configTimeout)
    configTimeout = setTimeout(() => {
      loadAndApply()
    }, 500) // 延迟500ms
  }

  const loadAndApply = async () => {
    try {
      const res = await fetch('/api/frontend/config', { credentials: 'include' })
      const data = await res.json()
      const fs = data?.data?.frontendSettings || {}
      applyHeadFromConfig(fs)
      
      try {
        const last = localStorage.getItem('pwaEnabledLast')
        const prev = last === null ? null : (last === 'true')
        const curr = typeof fs.pwaEnabled === 'boolean' ? fs.pwaEnabled : true
        if (prev !== curr) {
          localStorage.setItem('pwaEnabledLast', curr ? 'true' : 'false')
          if (!curr && 'serviceWorker' in navigator) {
            const regs = await navigator.serviceWorker.getRegistrations()
            for (const r of regs) await r.unregister()
            const keys = await caches.keys()
            await Promise.all(keys.map(k => caches.delete(k)))
          }
        }
      } catch {}
    } catch (error) {
      console.error('Failed to load PWA config:', error)
      // 加载失败时使用默认配置
      if (!configLoaded) {
        applyHeadFromConfig({ pwaEnabled: true })
      }
    }
  }

  // 延迟加载配置，避免阻塞页面渲染
  if (process.client) {
    setTimeout(loadAndApplyThrottled, 2000) // 延迟2秒，确保页面完全加载后再请求配置
  }
  
  // 监听后台面板触发的配置更新事件
  if (process.client) {
    window.addEventListener('frontend-config-updated', loadAndApplyThrottled)
  }
})
import { useHead } from '#imports'
