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
    if (url === '/api/frontend/config') return response({ frontendSettings: { pwaEnabled: true } })
    if (url === '/api/web-push/config') {
      return response({ configured: true, public_key: 'AQ', session_subscribed: false, preferences: {} })
    }
    return response(null)
  },
}

const moduleURL = `data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`
const plugin = (await import(moduleURL)).default
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

console.log('PWA push busy settlement test passed')
