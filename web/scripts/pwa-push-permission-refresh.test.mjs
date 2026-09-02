import assert from 'node:assert/strict'
import fs from 'node:fs'
import ts from 'typescript'

const pluginURL = new URL('../plugins/pwa.client.ts', import.meta.url)
let source = fs.readFileSync(pluginURL, 'utf8')
source = source.replace(/^import .*$/gm, '')

const windowHandlers = new Map()
const documentHandlers = new Map()
const prelude = `
const ref = value => ({ value })
const computed = getter => ({ get value() { return getter() } })
const useHead = () => undefined
const defineNuxtPlugin = callback => callback
const shouldRecoverMissingSubscription = () => false
const registerSW = () => async () => undefined
const window = globalThis.__PWA_TEST__.window
const document = globalThis.__PWA_TEST__.document
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

const subscription = {
  endpoint: 'https://push.example/current-device',
  options: { applicationServerKey: Uint8Array.from([1]).buffer },
  toJSON: () => ({ endpoint: 'https://push.example/current-device', keys: {} }),
  unsubscribe: async () => true,
}
const registration = {
  active: { scriptURL: 'https://example.test/sw.js' },
  waiting: null,
  installing: null,
  pushManager: {
    getSubscription: async () => subscription,
    subscribe: async () => subscription,
  },
  unregister: async () => true,
  update: async () => undefined,
}
const response = data => ({
  ok: true,
  json: async () => ({ code: 1, data }),
})

globalThis.__PWA_TEST__ = {
  window: {
    isSecureContext: true,
    PushManager: function PushManager() {},
    Notification: function Notification() {},
    matchMedia: () => ({ matches: true, addEventListener: () => undefined }),
    addEventListener: (name, handler) => windowHandlers.set(name, handler),
  },
  document: {
    visibilityState: 'visible',
    addEventListener: (name, handler) => documentHandlers.set(name, handler),
  },
  navigator: {
    onLine: true,
    userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)',
    standalone: true,
    serviceWorker: {
      ready: Promise.resolve(registration),
      getRegistration: async () => registration,
      getRegistrations: async () => [registration],
    },
  },
  Notification: {
    permission: 'granted',
    requestPermission: async () => globalThis.__PWA_TEST__.Notification.permission,
  },
  caches: { keys: async () => [], delete: async () => true },
  fetch: async url => {
    if (url === '/api/frontend/config') return response({ frontendSettings: { pwaEnabled: true } })
    if (url === '/api/web-push/config') {
      return response({ configured: true, public_key: 'AQ', session_subscribed: true, preferences: {} })
    }
    return response(null)
  },
}

const moduleURL = `data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`
const plugin = (await import(moduleURL)).default
const manager = plugin().provide.pwaManager
await manager.loadPushConfig()

assert.equal(manager.pushSubscribed.value, true, 'a granted device with a matching subscription starts enabled')
assert.equal(typeof windowHandlers.get('focus'), 'function', 'the app must refresh notification state when it regains focus')
assert.equal(typeof windowHandlers.get('pageshow'), 'function', 'the app must refresh notification state whenever it is entered again')
assert.equal(typeof documentHandlers.get('visibilitychange'), 'function', 'the app must refresh after returning from system settings')

globalThis.__PWA_TEST__.Notification.permission = 'denied'
windowHandlers.get('focus')()
await new Promise(resolve => setTimeout(resolve, 0))

assert.equal(manager.notificationPermission.value, 'denied', 'the displayed permission must reflect the current system setting')
assert.equal(manager.pushSubscribed.value, false, 'a denied system permission must never remain displayed as enabled')

globalThis.__PWA_TEST__.Notification.permission = 'granted'
documentHandlers.get('visibilitychange')()
await new Promise(resolve => setTimeout(resolve, 0))

assert.equal(manager.notificationPermission.value, 'granted', 'returning from settings must detect a restored permission')
assert.equal(manager.pushSubscribed.value, true, 'a restored permission with an existing subscription must display as enabled')

console.log('PWA push permission refresh test passed')
