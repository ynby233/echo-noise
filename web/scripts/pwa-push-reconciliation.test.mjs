import assert from 'node:assert/strict'
import fs from 'node:fs'
import ts from 'typescript'

const sourceURL = new URL('../utils/pwaPushReconciliation.ts', import.meta.url)
const source = fs.readFileSync(sourceURL, 'utf8')
const pluginSource = fs.readFileSync(new URL('../plugins/pwa.client.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
  fileName: sourceURL.pathname,
}).outputText
const moduleURL = `data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`
const { shouldRecoverMissingSubscription } = await import(moduleURL)

assert.match(pluginSource, /shouldRecoverMissingSubscription\s*\(/, 'the PWA manager must apply the tested reconciliation decision')

assert.equal(shouldRecoverMissingSubscription({
  configured: true,
  hasPublicKey: true,
  permission: 'granted',
  serverSubscribed: true,
  localSubscribed: false,
}), true, 'a browser that previously subscribed on this login must recover a missing local PushSubscription after reload')

for (const state of [
  { configured: false, hasPublicKey: true, permission: 'granted', serverSubscribed: true, localSubscribed: false },
  { configured: true, hasPublicKey: false, permission: 'granted', serverSubscribed: true, localSubscribed: false },
  { configured: true, hasPublicKey: true, permission: 'default', serverSubscribed: true, localSubscribed: false },
  { configured: true, hasPublicKey: true, permission: 'denied', serverSubscribed: true, localSubscribed: false },
  { configured: true, hasPublicKey: true, permission: 'granted', serverSubscribed: false, localSubscribed: false },
  { configured: true, hasPublicKey: true, permission: 'granted', serverSubscribed: true, localSubscribed: true },
]) {
  assert.equal(shouldRecoverMissingSubscription(state), false, `must not auto-subscribe for ${JSON.stringify(state)}`)
}

console.log('PWA push reconciliation tests passed')
