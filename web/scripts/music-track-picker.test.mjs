import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createJiti } from 'jiti'
import ts from 'typescript'

const jiti = createJiti(import.meta.url)
const { parseMusicPickerPage } = await jiti.import('../utils/music-track-picker.ts')
const { captureMusicInsertSelection, restoreMusicInsertSelection } = await jiti.import('../utils/music-insert-selection.ts')
const track = { trackID: 'track_1', title: '歌曲', artist: '歌手', album: '专辑', available: true }
const pageData = { items: [track], total: 1, page: 1, pageSize: 25 }
assert.deepEqual(parseMusicPickerPage(pageData), pageData)
assert.throws(() => parseMusicPickerPage({ ...pageData, pageSize: 101 }))
assert.throws(() => parseMusicPickerPage({ ...pageData, items: [{ ...track, available: 'true' }] }))

// Execute the actual picker setup with controlled network and lifecycle hooks.
// No application server, mounted Nuxt tree or real music library is needed.
const source = await readFile(new URL('../components/index/MusicTrackPicker.vue', import.meta.url), 'utf8')
const setup = source.match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1].replace(/^import .*$/gm, '')
const compiled = ts.transpileModule(`${setup}\nreturn { load, search, close, select, keyword, library, loading, error };`, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.None }
}).outputText
function fixture() {
  const requests = []
  const events = []
  const watchers = []
  const capabilities = new Set(['music.view', 'music.manage'])
  const user = { isLogin: true, token: 'session', user: { userid: 1 } }
  const mounted = []
  const unmounted = []
  const names = ['ref', 'computed', 'watch', 'onMounted', 'onBeforeUnmount', 'useRuntimeConfig', 'useUserStore', 'useAdminCapabilities', 'defineEmits', 'parseMusicPickerPage', 'fetch']
  const values = [value => ({ value }), getter => ({ get value() { return getter() } }), (_sources, callback) => watchers.push(callback), callback => mounted.push(callback), callback => unmounted.push(callback), () => ({ public: { baseApi: '/api' } }), () => user, () => ({ can: name => capabilities.has(name), refreshCapabilities: async () => {} }), () => (...args) => events.push(args), parseMusicPickerPage,
    (url, options) => new Promise(resolve => requests.push({ url, options, resolve }))]
  const picker = new Function(...names, compiled)(...values)
  const respond = (index, data = pageData, status = 200) => requests[index].resolve({ ok: status === 200, status, json: async () => ({ code: 1, data }) })
  return { picker, requests, events, watchers, capabilities, user, mounted, unmounted, respond }
}

const current = fixture()
const first = current.picker.load()
assert.equal(current.picker.loading.value, true)
assert.equal(current.requests[0].options.credentials, 'include')
assert.match(current.requests[0].url, /^\/api\/music\/library\?/)
current.picker.keyword.value = '新歌曲'
current.picker.search()
assert.equal(current.requests[0].options.signal.aborted, true)
current.respond(0, { ...pageData, items: [{ ...track, title: '旧结果' }] })
await first
assert.equal(current.picker.library.value.items.length, 0, 'superseded results cannot replace the current search')
current.respond(1)
await new Promise(resolve => setImmediate(resolve))
assert.equal(current.picker.library.value.items[0].title, '歌曲')
current.picker.select({ ...track })
assert.equal(current.events.length, 0, 'a track outside the loaded result cannot be inserted')
current.picker.select(current.picker.library.value.items[0])
assert.deepEqual(current.events.map(event => event[0]), ['select', 'close'])

const cancelled = fixture()
const pending = cancelled.picker.load()
cancelled.picker.close()
cancelled.respond(0)
await pending
assert.equal(cancelled.requests[0].options.signal.aborted, true)
assert.equal(cancelled.picker.library.value.items.length, 0, 'closed picker discards late results')
assert.deepEqual(cancelled.events.map(event => event[0]), ['close'])

const revoked = fixture()
const revokedLoad = revoked.picker.load()
revoked.capabilities.delete('music.manage')
revoked.watchers.forEach(callback => callback())
revoked.respond(0)
await revokedLoad
assert.equal(revoked.requests[0].options.signal.aborted, true)
assert.equal(revoked.picker.library.value.items.length, 0)
assert.deepEqual(revoked.events.map(event => event[0]), ['close'], 'manage permission loss closes immediately')

const failed = fixture()
const failedLoad = failed.picker.load()
failed.respond(0, null, 500)
await failedLoad
assert.match(failed.picker.error.value, /失败/)
assert.equal(failed.picker.loading.value, false)
const retry = failed.picker.load()
failed.respond(1, { ...pageData, items: [], total: 0 })
await retry
assert.equal(failed.picker.error.value, '')
assert.equal(failed.picker.library.value.total, 0)

const denied = fixture()
const deniedLoad = denied.picker.load()
denied.respond(0, null, 403)
await deniedLoad
assert.deepEqual(denied.events.map(event => event[0]), ['close'])

const originalGlobals = { Element: globalThis.Element, window: globalThis.window }
try {
  class MockElement {
    isConnected = true
    constructor(parent = null) { this.parentElement = parent }
    contains(node) { return node === this || !!node?.parentElement && this.contains(node.parentElement) }
    closest() { return this }
    focus() { this.focused = true }
  }
  globalThis.Element = MockElement
  const root = new MockElement()
  const editable = new MockElement(root)
  const node = { parentElement: editable }
  const range = { commonAncestorContainer: node, cloneRange() { return { ...this } } }
  const selection = { rangeCount: 1, getRangeAt: () => range, removeAllRanges() { this.removed = true }, addRange(value) { this.restored = value } }
  globalThis.window = { getSelection: () => selection }
  const target = captureMusicInsertSelection(root)
  assert.notEqual(target.range, range, 'selection must be cloned before modal focus changes')
  assert.equal(restoreMusicInsertSelection(target), true)
  assert.equal(selection.restored, target.range)
  assert.equal(editable.focused, true)
  root.isConnected = false
  assert.equal(restoreMusicInsertSelection(target), false, 'a detached editor must never receive an insert')
  root.isConnected = true
  target.range.commonAncestorContainer = { parentElement: new MockElement() }
  assert.equal(restoreMusicInsertSelection(target), false, 'a replaced draft range cannot be restored')
} finally {
  for (const [name, value] of Object.entries(originalGlobals)) {
    if (value === undefined) delete globalThis[name]
    else globalThis[name] = value
  }
}

console.log('music picker and selection behavior tests passed')
