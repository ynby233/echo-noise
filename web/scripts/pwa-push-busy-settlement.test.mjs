import assert from 'node:assert/strict'
import fs from 'node:fs'
import ts from 'typescript'

const pluginURL = new URL('../plugins/pwa.client.ts', import.meta.url)
let source = fs.readFileSync(pluginURL, 'utf8')
source = source
  .replace(/^import .*$/gm, '')
  .replace('const pushOperationTimeoutMs = 15_000', 'const pushOperationTimeoutMs = 50')
  .replace('export default defineNuxtPlugin', 'export default defineNuxtPlugin')

const prelude = `
const ref = value => ({ value })
const computed = getter => ({ get value() { return getter() } })
const useHead = () => undefined
const defineNuxtPlugin = callback => callback
const shouldRecoverMissingSubscription = () => false
const registerSW = options => {
  options.onRegisteredSW?.('/sw.js', globalThis.__PWA_TEST__.registration)
  return async () => undefined
}
const window = globalThis.__PWA_TEST__.window
const navigator = globalThis.__PWA_TEST__.navigator
const Notification = globalThis.__PWA_TEST__.Notification
const fetch = globalThis.__PWA_TEST__.fetch
const caches = globalThis.__PWA_TEST__.caches
const atob = globalThis.atob
const setInterval = () => 0
`

const compiled = ts.transpileModule(prelude + source, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
  fileName: pluginURL.pathname,
}).outputText

const never = new Promise(() => undefined)
const registration = {
  active: { scriptURL: 'https://example.test/sw.js' },
  waiting: null,
  installing: null,
  scope: 'https://example.test/',
  pushManager: {
    getSubscription: async () => null,
    subscribe: () => never,
  },
  unregister: async () => true,
  update: async () => undefined,
}

const response = data => ({
  ok: true,
  json: async () => ({ code: 1, data }),
})

let releaseFrontendConfig
const frontendConfigGate = new Promise(resolve => {
  releaseFrontendConfig = () => resolve(response({ frontendSettings: { pwaEnabled: true } }))
})
let delayFrontendConfig = true
let pwaEnabledValue = true

globalThis.__PWA_TEST__ = {
  registration,
  window: {
    isSecureContext: true,
    PushManager: function PushManager() {},
    Notification: function Notification() {},
    matchMedia: () => ({ matches: true, addEventListener: () => undefined }),
    addEventListener: () => undefined,
  },
  navigator: {
    onLine: true,
    userAgent: 'Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X)',
    standalone: true,
    serviceWorker: {
      ready: Promise.resolve(registration),
      getRegistration: async () => registration,
      getRegistrations: async () => [registration],
    },
  },
  Notification: {
    permission: 'default',
    requestPermission: async () => {
      globalThis.__PWA_TEST__.Notification.permission = 'granted'
      return 'granted'
    },
  },
  caches: { keys: async () => [], delete: async () => true },
  fetch: async url => {
    if (url === '/api/frontend/config') {
      if (delayFrontendConfig) return frontendConfigGate
      return response({ frontendSettings: { pwaEnabled: pwaEnabledValue } })
    }
    if (url === '/api/web-push/config') {
      return response({ configured: true, public_key: 'AQ', session_subscribed: false, preferences: {} })
    }
    return response(null)
  },
}

const moduleURL = `data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`
const plugin = (await import(moduleURL)).default

const startupRaceSubscription = {
  endpoint: 'https://push.example/ipad-startup-race',
  options: { applicationServerKey: Uint8Array.from([1]).buffer },
  toJSON: () => ({
    endpoint: 'https://push.example/ipad-startup-race',
    keys: { p256dh: 'startup-p256dh', auth: 'startup-auth' },
  }),
  unsubscribe: async () => true,
}
registration.pushManager.subscribe = async () => startupRaceSubscription
let startupPermissionRequestedBeforeConfig = false
globalThis.__PWA_TEST__.Notification.permission = 'default'
globalThis.__PWA_TEST__.Notification.requestPermission = async () => {
  startupPermissionRequestedBeforeConfig = delayFrontendConfig
  globalThis.__PWA_TEST__.Notification.permission = 'granted'
  return 'granted'
}

const startupRaceManager = plugin().provide.pwaManager
await startupRaceManager.loadPushConfig()
let startupRaceError
const startupRaceOperation = startupRaceManager.enableNotifications().catch(error => { startupRaceError = error })
await startupRaceOperation
delayFrontendConfig = false
releaseFrontendConfig()
await new Promise(resolve => setTimeout(resolve, 0))

assert.equal(startupRaceManager.workerRegistered.value, true, 'the delayed startup refresh must eventually register the owned worker')
assert.equal(startupPermissionRequestedBeforeConfig, true, 'the iPad permission prompt must remain directly attached to the user action')
assert.equal(
  startupRaceError?.message,
  undefined,
  'an iPad push action with an active worker must not wait for an unrelated frontend configuration request',
)
assert.equal(startupRaceManager.pushSubscribed.value, true, 'the startup-race retry must establish and persist the PushSubscription')

pwaEnabledValue = false
let disabledSubscriptionAttempted = false
registration.pushManager.getSubscription = async () => null
registration.pushManager.subscribe = async () => {
  disabledSubscriptionAttempted = true
  return startupRaceSubscription
}
const disabledManager = plugin().provide.pwaManager
await disabledManager.refreshConfiguration()
await disabledManager.loadPushConfig()
let disabledError
await disabledManager.enableNotifications().catch(error => { disabledError = error })
assert.match(disabledError?.message || '', /应用服务尚未准备完成/, 'an explicitly disabled PWA must not reuse a residual worker for push')
assert.equal(disabledSubscriptionAttempted, false, 'an explicitly disabled PWA must not create a PushSubscription')
pwaEnabledValue = true

registration.pushManager.subscribe = () => never
globalThis.__PWA_TEST__.Notification.permission = 'default'
const manager = plugin().provide.pwaManager
await manager.loadPushConfig()

let settled = false
let operationError
const operation = manager.enableNotifications()
  .catch(error => { operationError = error })
  .finally(() => { settled = true })
await new Promise(resolve => setTimeout(resolve, 250))

assert.equal(settled, true, 'an iPad push operation must settle instead of spinning forever when a browser Push API stalls')
assert.equal(manager.pushBusy.value, false, 'pushBusy must reset after a stalled browser Push API operation')
assert.match(operationError?.message || '', /创建推送订阅超时/, 'the user must be told which iPad push stage stalled')
await operation

let permissionRequestCount = 0
globalThis.__PWA_TEST__.Notification.permission = 'granted'
globalThis.__PWA_TEST__.Notification.requestPermission = async () => {
  permissionRequestCount += 1
  return 'granted'
}
const recoveredSubscription = {
  endpoint: 'https://push.example/ipad-retry',
  options: { applicationServerKey: Uint8Array.from([1]).buffer },
  toJSON: () => ({
    endpoint: 'https://push.example/ipad-retry',
    keys: { p256dh: 'retry-p256dh', auth: 'retry-auth' },
  }),
  unsubscribe: async () => true,
}
registration.pushManager.getSubscription = async () => null
registration.pushManager.subscribe = async () => recoveredSubscription

const retryManager = plugin().provide.pwaManager
await retryManager.loadPushConfig()
await retryManager.enableNotifications()

assert.equal(permissionRequestCount, 0, 'a retry after iPadOS already granted permission must not request permission again')
assert.equal(retryManager.pushSubscribed.value, true, 'a granted-permission retry must establish and persist the PushSubscription')
assert.equal(retryManager.pushBusy.value, false, 'a successful retry must leave the push action idle')

registration.pushManager.getSubscription = async () => null
registration.pushManager.subscribe = async () => {
  throw new DOMException('', 'AbortError')
}

const failedManager = plugin().provide.pwaManager
await failedManager.loadPushConfig()
let failedStageError
await failedManager.enableNotifications().catch(error => { failedStageError = error })

assert.match(
  failedStageError?.message || '',
  /创建推送订阅失败.*AbortError/,
  'an immediate iPad Push API rejection must identify the failed stage even when Safari returns an empty message',
)
assert.equal(failedManager.pushBusy.value, false, 'an immediate iPad Push API rejection must leave the push action idle')

globalThis.__PWA_TEST__.Notification.permission = 'default'
globalThis.__PWA_TEST__.Notification.requestPermission = async () => {
  globalThis.__PWA_TEST__.Notification.permission = 'granted'
  return 'granted'
}
registration.active = null
registration.waiting = null
registration.installing = null
registration.pushManager.getSubscription = async () => null
registration.pushManager.subscribe = async () => recoveredSubscription
globalThis.__PWA_TEST__.navigator.serviceWorker.ready = new Promise(resolve => {
  setTimeout(() => {
    registration.active = { scriptURL: 'https://example.test/sw.js' }
    registration.installing = null
    resolve(registration)
  }, 20)
})

const readinessManager = plugin().provide.pwaManager
await readinessManager.loadPushConfig()
let readinessError
await readinessManager.enableNotifications().catch(error => { readinessError = error })

assert.equal(
  readinessError?.message,
  undefined,
  'an iPad registration callback must not bypass waiting for the owned service worker to become active',
)
assert.equal(readinessManager.pushSubscribed.value, true, 'an iPad push subscription must continue after the owned worker becomes active')
assert.equal(readinessManager.pushBusy.value, false, 'the readiness retry must leave the push action idle')

console.log('PWA push busy settlement test passed')
