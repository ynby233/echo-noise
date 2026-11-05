import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createJiti } from 'jiti'
import ts from 'typescript'
import * as Vue from 'vue'
import { compile } from '@vue/compiler-dom'

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
const compiled = ts.transpileModule(`${setup}\nreturn { load, search, changePage, close, select, keyword, query, page, pageCount, library, loading, error };`, {
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
  const values = [Vue.ref, Vue.computed, (_sources, callback) => watchers.push(callback), callback => mounted.push(callback), callback => unmounted.push(callback), () => ({ public: { baseApi: '/api' } }), () => user, () => ({ can: name => capabilities.has(name), refreshCapabilities: async () => {} }), () => (...args) => events.push(args), parseMusicPickerPage,
    (url, options) => new Promise(resolve => requests.push({ url, options, resolve }))]
  const picker = new Function(...names, compiled)(...values)
  const respond = (index, data = pageData, status = 200) => requests[index].resolve({ ok: status === 200, status, json: async () => ({ code: 1, data }) })
  return { picker, requests, events, watchers, capabilities, user, mounted, unmounted, respond }
}

// Render the real template into an in-memory host so node retention is observable.
// Layout dimensions still require browser verification; this host does not simulate CSS.
const template = source.match(/<template>([\s\S]*?)<\/template>/)[1]
const render = new Function('Vue', compile(template, { mode: 'function', prefixIdentifiers: true }).code)(Vue)
const node = type => ({ type, tagName: type.toUpperCase(), children: [], props: {}, parent: null, text: '', value: '', addEventListener() {}, removeEventListener() {} })
const detach = child => {
  if (child.parent) child.parent.children.splice(child.parent.children.indexOf(child), 1)
  child.parent = null
}
const renderer = Vue.createRenderer({
  createElement: node,
  createText: text => ({ ...node('#text'), text }),
  createComment: text => ({ ...node('#comment'), text }),
  setText: (target, text) => { target.text = text },
  setElementText: (target, text) => { target.text = text; target.children = [] },
  patchProp: (target, key, _previous, value) => { target.props[key] = value },
  insert: (child, parent, anchor = null) => {
    detach(child)
    parent.children.splice(anchor ? parent.children.indexOf(anchor) : parent.children.length, 0, child)
    child.parent = parent
  },
  remove: detach,
  parentNode: target => target.parent,
  nextSibling: target => target.parent?.children[target.parent.children.indexOf(target) + 1] || null
})
function mountPicker(current) {
  const root = node('root')
  const app = renderer.createApp({ setup: () => current.picker, render })
  app.component('UModal', { inheritAttrs: false, setup: (_props, { slots }) => () => slots.default?.() })
  app.component('UIcon', { render: () => null })
  app.mount(root)
  const find = predicate => {
    const visit = target => predicate(target) ? target : target.children.map(visit).find(Boolean)
    return visit(root)
  }
  return { find, unmount: () => app.unmount() }
}
const settle = async () => { await new Promise(resolve => setImmediate(resolve)); await Vue.nextTick() }

const paged = fixture()
const view = mountPicker(paged)
const body = view.find(target => target.props.class?.includes('music-picker-body'))
const scroller = view.find(target => target.props.class === 'music-picker-results')
const initialPage = paged.picker.load()
await Vue.nextTick()
assert.equal(body.props['aria-busy'], true)
assert.equal(view.find(target => target.props.role === 'status').text, '正在读取本地音乐库…')
paged.respond(0, { ...pageData, total: 51 })
await initialPage
await Vue.nextTick()
const oldLibrary = paged.picker.library.value
const oldList = view.find(target => target.type === 'ul')
scroller.scrollTop = 120
paged.picker.changePage(-1)
assert.equal(paged.requests.length, 1, 'first-page previous action must not refetch')
paged.picker.changePage(1)
paged.picker.changePage(1)
paged.picker.changePage(-1)
await paged.picker.load()
await Vue.nextTick()
assert.equal(paged.requests.length, 2, 'pagination and retry cannot duplicate an in-flight request')
assert.equal(new URL(paged.requests[1].url, 'https://local.test').searchParams.get('page'), '2')
assert.equal(paged.picker.page.value, 1, 'page indicator stays with the visible data until success')
assert.equal(paged.picker.library.value, oldLibrary)
assert.equal(view.find(target => target.type === 'ul'), oldList, 'loading must retain the mounted list')
assert.equal(view.find(target => target.props.class === 'music-picker-results'), scroller)
assert.equal(scroller.scrollTop, 120, 'loading must retain the scroll container and its position')
assert.equal(view.find(target => target.props['aria-label'] === '插入 歌曲').props.disabled, true)
paged.picker.select(oldLibrary.items[0])
assert.equal(paged.events.length, 0, 'insertion is blocked while paging')
paged.respond(1, null, 500)
await settle()
assert.equal(paged.picker.loading.value, false)
assert.equal(paged.picker.page.value, 1, 'failed pagination keeps the old page number')
assert.equal(paged.picker.library.value, oldLibrary, 'failed pagination keeps the old result data')
assert.equal(view.find(target => target.type === 'ul'), oldList, 'errors must retain the mounted list')
assert.ok(view.find(target => target.props.role === 'alert'))
const retryButton = view.find(target => target.type === 'button' && target.text === '重试')
retryButton.props.onClick({ type: 'click' })
assert.equal(new URL(paged.requests[2].url, 'https://local.test').searchParams.get('page'), '2', 'retry targets the failed page rather than the visible old page')
paged.respond(2, { ...pageData, items: [{ ...track, trackID: 'track_2', title: '第二页' }], total: 51, page: 2 })
await settle()
assert.equal(paged.picker.page.value, 2)
assert.equal(paged.picker.library.value.items[0].title, '第二页')
assert.equal(paged.picker.error.value, '')
paged.picker.changePage(1)
paged.respond(3, { ...pageData, items: [], total: 51, page: 3 })
await settle()
assert.equal(paged.picker.page.value, 3)
assert.equal(view.find(target => target.props.class?.includes('music-picker-body')), body, 'empty pages retain the same panel body')
assert.equal(view.find(target => target.props.class === 'music-picker-results'), scroller, 'empty pages retain the scroll viewport')
assert.equal(view.find(target => target.props.role === 'status').text, '本地音乐库暂无歌曲。')
paged.picker.changePage(1)
assert.equal(paged.requests.length, 4, 'last-page next action must not refetch')
paged.picker.changePage(-1)
paged.respond(4, { ...pageData, items: [], total: 1, page: 2 })
await settle()
assert.equal(paged.picker.loading.value, true, 'page-count correction remains busy while fetching a valid page')
assert.equal(paged.picker.page.value, 3, 'an out-of-range intermediate response must not replace the visible page')
assert.equal(new URL(paged.requests[5].url, 'https://local.test').searchParams.get('page'), '1')
paged.picker.changePage(-1)
assert.equal(paged.requests.length, 6, 'page-count correction also blocks duplicate pagination')
paged.respond(5)
await settle()
assert.equal(paged.picker.page.value, 1)
assert.equal(paged.picker.loading.value, false)
view.unmount()

// CSS contracts cover viewport reservation; pixel sizes and mobile reachability need a real browser.
const styles = source.match(/<style scoped>([\s\S]*?)<\/style>/)[1]
assert.match(styles, /\.music-picker-body\s*\{[^}]*height:\s*min\(50dvh,\s*26rem\)/)
assert.match(styles, /\.music-picker\s*\{[^}]*max-height:\s*calc\(100dvh\s*-\s*2rem\)/)
assert.match(styles, /\.music-picker-results\s*\{[^}]*height:\s*100%;[^}]*overflow-y:\s*auto/)

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
const failedView = mountPicker(failed)
const failedBody = failedView.find(target => target.props.class?.includes('music-picker-body'))
const failedScroller = failedView.find(target => target.props.class === 'music-picker-results')
const failedLoad = failed.picker.load()
failed.respond(0, null, 500)
await failedLoad
await Vue.nextTick()
assert.ok(failedView.find(target => target.props.role === 'alert'))
assert.equal(failedView.find(target => target.props.class?.includes('music-picker-body')), failedBody)
assert.equal(failedView.find(target => target.props.class === 'music-picker-results'), failedScroller, 'first-load errors retain the reserved viewport')
assert.match(failed.picker.error.value, /失败/)
assert.equal(failed.picker.loading.value, false)
const retry = failed.picker.load()
failed.respond(1, { ...pageData, items: [], total: 0 })
await retry
await Vue.nextTick()
assert.equal(failedView.find(target => target.props.class?.includes('music-picker-body')), failedBody)
assert.equal(failedView.find(target => target.props.class === 'music-picker-results'), failedScroller)
assert.equal(failedView.find(target => target.props.role === 'status').text, '本地音乐库暂无歌曲。')
failedView.unmount()
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
