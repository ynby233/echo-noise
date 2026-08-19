import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { computed, reactive, ref, watch } from 'vue'
import { transformSync } from 'esbuild'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const source = transformSync(fs.readFileSync(path.join(root, 'composables/useAdminCapabilities.ts'), 'utf8'), { loader: 'ts', format: 'esm' }).code
  .replace(/^import .*\r?\n/gm, '')
  .replace('export const useAdminCapabilities', 'const useAdminCapabilities')
  .replace(/export \{[\s\S]*?\};?\s*$/, '')

const store = reactive({ isLogin: true, user: { id: 3, is_admin: true } })
const pending = []
const listeners = new Map()
const useUserStore = () => store
const getRequest = () => new Promise((resolve) => pending.push(resolve))
globalThis.window = { addEventListener(name, listener) { listeners.set(name, listener) } }

const factory = new Function('ref', 'computed', 'watch', 'useUserStore', 'getRequest', `${source}\nreturn useAdminCapabilities`)
const useAdminCapabilities = factory(ref, computed, watch, useUserStore, getRequest)

const first = useAdminCapabilities()
assert.equal(pending.length, 1, 'the first consumer must request the capability snapshot')
assert.equal(first.isLoading.value, true, 'a delegated administrator without a snapshot must be loading, not denied')
pending.shift()({ code: 1, data: { capabilities: ['notes.view'] } })
await Promise.resolve()
assert.equal(first.can('notes.view'), true, 'the loaded snapshot must grant the note section')
assert.equal(first.isReady.value, true)

useAdminCapabilities()
assert.equal(first.can('notes.view'), true, 'mounting a second consumer must not clear a ready shared snapshot')
assert.equal(pending.length, 0, 'mounting a second consumer must not refetch an already-ready snapshot')

listeners.get('admin-capabilities-invalidated')()
assert.equal(first.can('notes.view'), false, 'invalidation must not retain a stale granted capability')
assert.equal(first.isLoading.value, true, 'invalidation must expose an explicit loading state')
assert.equal(pending.length, 1, 'invalidation must refresh the server-authoritative snapshot')
pending.shift()({ code: 1, data: { capabilities: [] } })
await Promise.resolve()
assert.equal(first.isReady.value, true)
assert.equal(first.can('notes.view'), false, 'the refreshed revoked snapshot must remain denied')

console.log('admin capability generation tests passed')
