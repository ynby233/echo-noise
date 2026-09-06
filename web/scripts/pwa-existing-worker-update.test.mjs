import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

let callbacks
let registrations = 0
let applied = 0
const registration = {
  active: { scriptURL: 'https://example.test/sw.js' },
  update: async () => {},
  pushManager: { getSubscription: async () => null },
}
globalThis.__existingWorkerTest = {
  registerSW: options => {
    callbacks = options
    registrations++
    return async reload => { if (reload) applied++ }
  },
  navigator: {
    onLine: true, userAgent: '',
    serviceWorker: { getRegistration: async () => registration, ready: Promise.resolve(registration) },
  },
  window: {
    isSecureContext: true, addEventListener: () => {},
    matchMedia: () => ({ matches: false, addEventListener: () => {} }),
  },
  fetch: async () => ({ ok: true, json: async () => ({ code: 1, data: { frontendSettings: { pwaEnabled: true } } }) }),
}
const source = readFileSync(new URL('../plugins/pwa.client.ts', import.meta.url), 'utf8').replace(/^import .*$/gm, '')
const prelude = `
const { registerSW, navigator, window, fetch } = globalThis.__existingWorkerTest
const ref = value => ({ value })
const computed = getter => ({ get value() { return getter() } })
const useHead = () => {}
const defineNuxtPlugin = fn => fn
const setInterval = () => 0
`
const compiled = ts.transpileModule(prelude + source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const plugin = (await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)).default
const manager = plugin().provide.pwaManager
await manager.refreshConfiguration()
assert.equal(registrations, 1, 'return visits must attach update listeners even with an active worker')
callbacks.onNeedRefresh()
assert.equal(manager.needRefresh.value, true, 'a waiting update must be surfaced')
await manager.applyUpdate()
assert.equal(applied, 1, 'the update action must activate the new worker and reload')
await manager.refreshConfiguration()
assert.equal(registrations, 1, 'configuration refresh must reuse the update listener')
console.log('existing PWA worker update tests passed')
