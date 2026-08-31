/// <reference lib="WebWorker" />

import { clientsClaim, setCacheNameDetails } from 'workbox-core'
import { cleanupOutdatedCaches, matchPrecache, precacheAndRoute } from 'workbox-precaching'
import { NavigationRoute, registerRoute, setCatchHandler, setDefaultHandler } from 'workbox-routing'
import { NetworkFirst, NetworkOnly } from 'workbox-strategies'

declare let self: ServiceWorkerGlobalScope & {
  __WB_MANIFEST: Array<{ url: string; revision?: string | null }>
}
declare const __PWA_BUILD_ID__: string

type PushPayload = {
  title?: string
  body?: string
  icon?: string
  badge?: string
  tag?: string
  url?: string
  unreadCount?: number
}

setCacheNameDetails({ prefix: 'site-app', suffix: __PWA_BUILD_ID__ })
cleanupOutdatedCaches()
precacheAndRoute(self.__WB_MANIFEST)
clientsClaim()

registerRoute(new NavigationRoute(
  new NetworkFirst({
    cacheName: 'site-app-navigation',
    networkTimeoutSeconds: 5,
  }),
  { denylist: [/^\/api\//, /^\/mcp(?:\/|$)/] },
))
setDefaultHandler(new NetworkOnly())
setCatchHandler(async ({ request }) => {
  if (request.destination === 'document') {
    return (await matchPrecache('/offline')) || (await matchPrecache('/offline.html')) || Response.error()
  }
  return Response.error()
})

self.addEventListener('message', (event) => {
  if (event.data?.type === 'SKIP_WAITING' || event.data === 'SKIP_WAITING') {
    void self.skipWaiting()
  }
})

const safeTargetURL = (raw?: string) => {
  try {
    const target = new URL(raw || '/', self.location.origin)
    return target.origin === self.location.origin ? target.href : self.location.origin + '/'
  } catch {
    return self.location.origin + '/'
  }
}

self.addEventListener('push', (event) => {
  event.waitUntil((async () => {
    let payload: PushPayload = {}
    try {
      payload = event.data?.json() || {}
    } catch {
      payload = {}
    }
    const title = String(payload.title || '个人站点')
    const body = String(payload.body || '你有一条新通知')
    const unreadCount = Number(payload.unreadCount || 0)
    if ('setAppBadge' in navigator) {
      if (unreadCount > 0) await navigator.setAppBadge(unreadCount).catch(() => undefined)
      else if ('clearAppBadge' in navigator) await navigator.clearAppBadge().catch(() => undefined)
    }
    await self.registration.showNotification(title, {
      body,
      icon: payload.icon || '/android-chrome-192x192.png',
      badge: payload.badge || '/android-chrome-192x192.png',
      tag: payload.tag,
      data: { url: safeTargetURL(payload.url) },
    })
  })())
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  event.waitUntil((async () => {
    const targetURL = safeTargetURL(event.notification.data?.url)
    const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
    for (const client of windows) {
      if ('navigate' in client) await client.navigate(targetURL)
      return client.focus()
    }
    return self.clients.openWindow(targetURL)
  })())
})
