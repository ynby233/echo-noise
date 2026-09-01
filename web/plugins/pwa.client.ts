/// <reference types="vite-plugin-pwa/vanillajs" />

import { computed, ref } from 'vue'
import { useHead } from '#imports'
import { registerSW } from 'virtual:pwa-register'
import type { PwaInstallResult, PwaManager, WebPushPreferences } from '~/types/pwa'
import { shouldRecoverMissingSubscription } from '~/utils/pwaPushReconciliation'

type DeferredInstallPrompt = Event & {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

const defaultPreferences = (): WebPushPreferences => ({
  enabled: true,
  comment_enabled: true,
  reply_enabled: true,
  guestbook_enabled: true,
  like_enabled: false,
  announcement_enabled: true,
  account_security_enabled: true,
  show_preview: false,
})

const unwrap = <T>(payload: any): T => payload?.data as T

const fetchJSON = async <T>(url: string, options: RequestInit = {}): Promise<T> => {
  const response = await fetch(url, { credentials: 'include', ...options })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok || payload?.code !== 1) throw new Error(payload?.msg || '请求失败')
  return unwrap<T>(payload)
}

const base64URLToBytes = (value: string) => {
  const padding = '='.repeat((4 - (value.length % 4)) % 4)
  const decoded = atob((value + padding).replace(/-/g, '+').replace(/_/g, '/'))
  return Uint8Array.from(decoded, char => char.charCodeAt(0))
}

const detectPlatform = () => {
  const agent = navigator.userAgent.toLowerCase()
  if (/iphone|ipad|ipod/.test(agent)) return /ipad/.test(agent) ? 'ipados' : 'ios'
  if (/android/.test(agent)) return 'android'
  if (/windows/.test(agent)) return 'windows'
  if (/macintosh|mac os x/.test(agent)) return 'macos'
  if (/linux/.test(agent)) return 'linux'
  return 'unknown'
}

const pushOperationTimeoutMs = 15_000
const pushPermissionTimeoutMs = 120_000

const pushFailureDetail = (error: unknown) => {
  if (!error || typeof error !== 'object') return String(error || '').trim()
  const candidate = error as { name?: unknown; message?: unknown }
  const name = typeof candidate.name === 'string' ? candidate.name.trim() : ''
  const message = typeof candidate.message === 'string' ? candidate.message.trim() : ''
  if (name && name !== 'Error' && message && !message.includes(name)) return `${name}：${message}`
  if (message) return message
  if (name && name !== 'Error') return name
  return ''
}

const withPushTimeout = async <T>(operation: PromiseLike<T>, stage: string, timeoutMs = pushOperationTimeoutMs): Promise<T> => {
  let timer: ReturnType<typeof setTimeout> | undefined
  let timeoutError: Error | undefined
  try {
    return await Promise.race([
      Promise.resolve(operation),
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => {
          timeoutError = new Error(`${stage}超时，请关闭应用并从主屏幕重新打开后重试`)
          reject(timeoutError)
        }, timeoutMs)
      }),
    ])
  } catch (error) {
    if (error === timeoutError) throw error
    const detail = pushFailureDetail(error)
    throw new Error(`${stage}失败${detail ? `（${detail}）` : '，浏览器未提供详细原因'}`)
  } finally {
    if (timer) clearTimeout(timer)
  }
}

export default defineNuxtPlugin(() => {
  const enabled = ref(false)
  const supported = ref('serviceWorker' in navigator)
  const secureContext = ref(window.isSecureContext)
  const online = ref(navigator.onLine)
  const standalone = ref(window.matchMedia('(display-mode: standalone)').matches || (navigator as Navigator & { standalone?: boolean }).standalone === true)
  const ios = ref(/iphone|ipad|ipod/i.test(navigator.userAgent))
  const installable = ref(false)
  const installed = ref(standalone.value)
  const needRefresh = ref(false)
  const offlineReady = ref(false)
  const registrationError = ref(false)
  const workerRegistered = ref(false)
  const notificationPermission = ref<NotificationPermission | 'unsupported'>('Notification' in window ? Notification.permission : 'unsupported')
  const pushConfigured = ref(false)
  const pushSubscribed = ref(false)
  const pushBusy = ref(false)
  const preferences = ref<WebPushPreferences>(defaultPreferences())
  const title = ref('个人站点')
  const description = ref('')
  let publicKey = ''
  let deferredInstallPrompt: DeferredInstallPrompt | null = null
  let updateServiceWorker: ((reloadPage?: boolean) => Promise<void>) | null = null

  useHead(computed(() => ({
    htmlAttrs: { 'data-pwa-worker': !enabled.value ? 'disabled' : registrationError.value ? 'error' : workerRegistered.value ? 'active' : 'pending' },
    title: title.value,
    meta: [
      { key: 'pwa-description', name: 'description', content: description.value },
      { key: 'pwa-theme-color', name: 'theme-color', content: '#000000' },
    ],
    link: enabled.value ? [{ key: 'pwa-manifest', rel: 'manifest', href: '/manifest.webmanifest' }] : [],
  })))

  const ownedRegistration = (registration: ServiceWorkerRegistration) => {
    const worker = registration.active || registration.waiting || registration.installing
    if (!worker) return false
    try { return new URL(worker.scriptURL).pathname === '/sw.js' } catch { return false }
  }

  const disablePwaRuntime = async () => {
    const subscription = await currentSubscription().catch(() => null)
    if (subscription) {
      await removeSubscriptionFromServer(subscription.endpoint).catch(() => undefined)
      await subscription.unsubscribe().catch(() => false)
    }
    if ('serviceWorker' in navigator) {
      const registrations = await navigator.serviceWorker.getRegistrations()
      await Promise.all(registrations.filter(ownedRegistration).map(registration => registration.unregister()))
    }
    if ('caches' in window) {
      const keys = await caches.keys()
      await Promise.all(keys.filter(key => key.startsWith('site-app-')).map(key => caches.delete(key)))
    }
    workerRegistered.value = false
    pushSubscribed.value = false
  }

  const ensureServiceWorker = async () => {
    if (!enabled.value || !supported.value || !secureContext.value || workerRegistered.value) return
    if (!updateServiceWorker) {
      updateServiceWorker = registerSW({
        immediate: true,
        onNeedRefresh: () => { needRefresh.value = true },
        onOfflineReady: () => { offlineReady.value = true },
        onRegisterError: () => { registrationError.value = true },
        onRegisteredSW: () => { workerRegistered.value = true },
      })
    }
    const registration = await navigator.serviceWorker.ready
    workerRegistered.value = ownedRegistration(registration)
  }

  const refreshConfiguration = async () => {
    try {
      const data = await fetchJSON<any>('/api/frontend/config')
      const settings = data?.frontendSettings || {}
      enabled.value = typeof settings.pwaEnabled === 'boolean' ? settings.pwaEnabled : true
      title.value = String(settings.pwaTitle || settings.siteTitle || '个人站点').trim()
      description.value = String(settings.pwaDescription || settings.description || '').trim()
    } catch {
      enabled.value = true
    }
    if (enabled.value) await ensureServiceWorker()
    else await disablePwaRuntime()
  }

  const install = async (): Promise<PwaInstallResult> => {
    if (installed.value || standalone.value) return 'already-installed'
    if (deferredInstallPrompt) {
      await deferredInstallPrompt.prompt()
      const choice = await deferredInstallPrompt.userChoice
      deferredInstallPrompt = null
      installable.value = false
      return choice.outcome === 'accepted' ? 'installed' : 'dismissed'
    }
    if (ios.value) return 'ios-guide'
    return 'unsupported'
  }

  const applyUpdate = async () => {
    if (updateServiceWorker) await updateServiceWorker(true)
    needRefresh.value = false
  }

  const dismissUpdate = () => { needRefresh.value = false }

  const currentSubscription = async () => {
    if (!('serviceWorker' in navigator)) return null
    const registration = await navigator.serviceWorker.getRegistration('/')
    if (!registration) return null
    return registration.pushManager.getSubscription()
  }

  const persistSubscription = async (subscription: PushSubscription) => {
    const serialized = subscription.toJSON()
    await fetchJSON('/api/web-push/subscriptions', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...serialized, platform: detectPlatform() }),
    })
  }

  const removeSubscriptionFromServer = async (endpoint: string) => {
    await fetchJSON('/api/web-push/subscriptions', {
      method: 'DELETE', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ endpoint }),
    })
  }

  const subscriptionUsesPublicKey = (subscription: PushSubscription, expected: string) => {
    const actual = subscription.options.applicationServerKey
    if (!actual || !expected) return false
    const actualBytes = new Uint8Array(actual)
    const expectedBytes = base64URLToBytes(expected)
    return actualBytes.length === expectedBytes.length && actualBytes.every((value, index) => value === expectedBytes[index])
  }

  const loadPushConfig = async () => {
    const data = await fetchJSON<any>('/api/web-push/config')
    pushConfigured.value = data?.configured === true
    publicKey = String(data?.public_key || '')
    preferences.value = { ...defaultPreferences(), ...(data?.preferences || {}) }
    const permission = 'Notification' in window ? Notification.permission : 'unsupported'
    let subscription = await currentSubscription()
    let publicKeyRotated = false
    if (subscription && pushConfigured.value && publicKey && !subscriptionUsesPublicKey(subscription, publicKey)) {
      await removeSubscriptionFromServer(subscription.endpoint).catch(() => undefined)
      await subscription.unsubscribe().catch(() => false)
      subscription = null
      publicKeyRotated = true
    }
    if (publicKeyRotated && permission === 'granted') {
      await ensureServiceWorker()
      const registration = await navigator.serviceWorker.ready
      subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: base64URLToBytes(publicKey) as BufferSource,
      })
      await persistSubscription(subscription)
    }
    if (shouldRecoverMissingSubscription({
      configured: pushConfigured.value,
      hasPublicKey: publicKey !== '',
      permission,
      serverSubscribed: data?.session_subscribed === true,
      localSubscribed: !!subscription,
    })) {
      await ensureServiceWorker()
      const registration = await navigator.serviceWorker.ready
      subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: base64URLToBytes(publicKey) as BufferSource,
      })
      await persistSubscription(subscription)
    }
    pushSubscribed.value = !!subscription && data?.session_subscribed === true
    if (subscription && permission === 'granted' && !pushSubscribed.value && pushConfigured.value) {
      await persistSubscription(subscription)
      pushSubscribed.value = true
    }
    notificationPermission.value = permission
  }

  const enableNotifications = async () => {
    if (!pushConfigured.value || !publicKey) throw new Error('站点尚未配置系统推送')
    if (!supported.value || !secureContext.value || !('PushManager' in window) || !('Notification' in window)) {
      throw new Error('当前浏览器或连接方式不支持系统推送')
    }
    pushBusy.value = true
    try {
      const permission = Notification.permission === 'granted'
        ? 'granted'
        : await withPushTimeout(Notification.requestPermission(), '等待系统通知授权', pushPermissionTimeoutMs)
      notificationPermission.value = permission
      if (permission !== 'granted') throw new Error('通知权限未允许，请在系统设置中重新开启')
      await withPushTimeout(ensureServiceWorker(), '准备应用服务')
      const registration = await withPushTimeout(navigator.serviceWorker.getRegistration('/'), '读取应用服务')
      if (!registration || !ownedRegistration(registration)) throw new Error('应用服务尚未准备完成，请关闭应用并从主屏幕重新打开后重试')
      let subscription = await withPushTimeout(registration.pushManager.getSubscription(), '读取推送订阅')
      if (!subscription) {
        subscription = await withPushTimeout(registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: base64URLToBytes(publicKey) as BufferSource,
        }), '创建推送订阅')
      }
      await withPushTimeout(persistSubscription(subscription), '保存推送订阅')
      pushSubscribed.value = true
    } finally {
      pushBusy.value = false
    }
  }

  const disableNotifications = async () => {
    pushBusy.value = true
    try {
      const subscription = await currentSubscription()
      if (subscription) {
        await removeSubscriptionFromServer(subscription.endpoint)
        await subscription.unsubscribe()
      }
      pushSubscribed.value = false
      await syncBadge(0)
    } finally {
      pushBusy.value = false
    }
  }

  const savePreferences = async (next: WebPushPreferences) => {
    const saved = await fetchJSON<WebPushPreferences>('/api/web-push/preferences', {
      method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(next),
    })
    preferences.value = { ...defaultPreferences(), ...saved }
  }

  const sendTestNotification = async () => {
    await fetchJSON('/api/web-push/test', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })
  }

  async function syncBadge(unreadCount: number) {
    const badgeNavigator = navigator as Navigator & { setAppBadge?: (count?: number) => Promise<void>; clearAppBadge?: () => Promise<void> }
    try {
      if (unreadCount > 0 && badgeNavigator.setAppBadge) await badgeNavigator.setAppBadge(unreadCount)
      else if (badgeNavigator.clearAppBadge) await badgeNavigator.clearAppBadge()
    } catch {}
  }

  window.addEventListener('beforeinstallprompt', (event) => {
    event.preventDefault()
    deferredInstallPrompt = event as DeferredInstallPrompt
    installable.value = true
  })
  window.addEventListener('appinstalled', () => {
    installed.value = true
    standalone.value = true
    installable.value = false
    deferredInstallPrompt = null
  })
  window.addEventListener('frontend-config-updated', () => { void refreshConfiguration() })
  window.addEventListener('online', () => { online.value = true })
  window.addEventListener('offline', () => { online.value = false })
  const displayMode = window.matchMedia('(display-mode: standalone)')
  const handleDisplayModeChange = (event: MediaQueryListEvent | MediaQueryList) => {
    standalone.value = event.matches
    installed.value = event.matches
  }
  displayMode.addEventListener('change', handleDisplayModeChange)
  setInterval(() => {
    if (enabled.value && 'serviceWorker' in navigator) void navigator.serviceWorker.getRegistration('/').then(registration => registration?.update())
  }, 60 * 60 * 1000)

  const manager: PwaManager = {
    enabled, supported, secureContext, online, standalone, ios, installable, installed, needRefresh,
    offlineReady, registrationError, workerRegistered, notificationPermission, pushConfigured, pushSubscribed,
    pushBusy, preferences, refreshConfiguration, install, applyUpdate, dismissUpdate,
    loadPushConfig, enableNotifications, disableNotifications, savePreferences,
    sendTestNotification, syncBadge,
  }
  void refreshConfiguration()
  return { provide: { pwaManager: manager } }
})

declare module '#app' {
  interface NuxtApp { $pwaManager: PwaManager }
}

declare module 'vue' {
  interface ComponentCustomProperties { $pwaManager: PwaManager }
}
