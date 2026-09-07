import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('../components/admin/AttachmentManager.vue', import.meta.url), 'utf8')

assert.match(source, /item\.logical_id/, 'attachment cards must display or use the logical id')
assert.match(source, /附件 ID[：:]\s*\{\{\s*item\.logical_id\s*\}\}/, 'attachment cards must visibly show the full logical id')
assert.match(source, /attachments\/references\/\$\{encodeURIComponent\(item\.logical_id\)\}/, 'logical references must use the reference-only delete endpoint')
assert.match(source, /logical_id:\s*entry\.item\?\.logical_id/, 'zip downloads must identify logical references explicitly')
assert.match(source, /const itemIdentity = \(item: any\).*logical_id/s, 'same-name cards must use logical identity for selection and expansion')
assert.doesNotMatch(source, /attachments\/\$\{endpointForKind\(kind\)\}/, 'removed filename-delete routes must not remain reachable from the manager')
assert.doesNotMatch(source, /legacyGroups|const legacy\s*=/, 'batch actions must not retain dead legacy deletion branches')
assert.match(source, /item\?\.logical_id/, 'management actions must be limited to registered logical attachments')

console.log('attachment manager logical-reference checks passed')

// Exercise the compiled components through rendered controls and their events.
// Only the external list API and Nuxt/UI adapters are substituted; selection code is real.
const { createRequire } = await import('node:module')
const require = createRequire(import.meta.url)
const vue = require('vue')
const { parse, compileScript } = require('@vue/compiler-sfc')
const ts = require('typescript')
const fixture = (logical_id, name) => ({ logical_id, name, url: '/' + name, size: 10, modified_at: '2026-01-01', belongs: [] })
const lists = {
  'attachments/images': [fixture('image-one', 'one.png'), fixture('image-two', 'two.png'), { name: 'unmanaged.png', url: '/unmanaged.png' }],
  'attachments/video': [fixture('video-one', 'one.mp4')],
}
function component(source, filename) {
  const { descriptor } = parse(source, { filename })
  const compiled = compileScript(descriptor, { id: filename, inlineTemplate: true }).content
  const js = ts.transpileModule(compiled, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const module = { exports: {} }
  const adapters = {
    vue,
    '#imports': { useToast: () => ({ add() {} }) },
    '~/store/user': { useUserStore: () => ({}) },
    '~/utils/media-url': { resolveManagedAttachmentURL: (_, url) => url },
    '~/composables/useAdminCapabilities': { useAdminCapabilities: () => ({ can: () => true, refreshCapabilities() {} }) },
    '~/utils/api': { getRequest: async endpoint => ({ code: 1, data: lists[endpoint] || [] }) },
    '~/utils/note-manager-permission': { createAdminModulePermissionHandler: () => Object.assign(() => {}, { reset() {} }) },
  }
  new Function('require', 'module', 'exports', 'computed', 'useRuntimeConfig', 'window', js)(name => {
    assert.ok(name in adapters, 'unexpected adapter: ' + name)
    return adapters[name]
  }, module, module.exports, vue.computed, () => ({ public: { baseApi: '/api' } }), { addEventListener() {}, removeEventListener() {} })
  return module.exports.default
}
const barSource = await readFile(new URL('../components/admin/AdminSelectionBar.vue', import.meta.url), 'utf8')
const SelectionBar = component(barSource, 'AdminSelectionBar.vue')
const Manager = component(source, 'AttachmentManager.vue')
const node = (type, text = '') => ({ type, text, props: {}, children: [], parent: null })
const renderer = vue.createRenderer({
  createElement: type => node(type), createText: text => node('#text', text), createComment: text => node('#comment', text),
  setText: (el, text) => { el.text = text }, setElementText: (el, text) => { el.text = text; el.children = [] },
  patchProp: (el, key, prev, value) => { el.props[key] = value },
  insert(el, parent, anchor) { if (el.parent) this.remove(el); el.parent = parent; const i = parent.children.indexOf(anchor); parent.children.splice(i < 0 ? parent.children.length : i, 0, el) },
  remove(el) { const i = el.parent?.children.indexOf(el); if (i >= 0) el.parent.children.splice(i, 1); el.parent = null },
  parentNode: el => el.parent, nextSibling: el => el.parent?.children[el.parent.children.indexOf(el) + 1],
})
const root = node('root')
const app = renderer.createApp(Manager)
app.component('AdminSelectionBar', SelectionBar)
for (const name of ['UButton', 'UInput', 'USelect', 'UIcon', 'UBadge', 'UCard', 'AdminModuleHeader']) {
  app.component(name, { setup: (_, { attrs, slots }) => () => vue.h(name === 'UButton' ? 'button' : name, attrs, slots.default?.()) })
}
app.component('UModal', { setup: () => () => null })
app.mount(root)
await new Promise(resolve => setImmediate(resolve))
await vue.nextTick()
function find(predicate, el = root) { if (predicate(el)) return el; for (const child of el.children) { const match = find(predicate, child); if (match) return match } }
function text(el) { return el.text + el.children.map(text).join('') }
const button = label => { const el = find(el => el.type === 'button' && text(el) === label); assert.ok(el, label); return el }
const checkbox = label => { const el = find(el => el.type === 'input' && el.props['aria-label'] === label); assert.ok(el, label); return el }
async function click(label) { button(label).props.onClick(); await vue.nextTick() }
async function changeAll(value) { checkbox('全选当前分类').props.onChange({ target: { checked: value } }); await vue.nextTick() }
async function choose(id) { checkbox('选择逻辑附件 ' + id).props.onChange(); await vue.nextTick() }
const failures = []
async function scenario(name, fn) { try { await fn(); console.log('PASS ' + name) } catch (e) { failures.push(e); console.error('FAIL ' + name + ': ' + e.message) } finally { await click('取消选择'); await click('图片') } }
await scenario('deselecting images retains a video selected in another category', async () => {
  await click('视频'); await choose('video-one'); await click('图片'); await changeAll(true); await changeAll(false); await click('视频')
  assert.equal(checkbox('选择逻辑附件 video-one').props.checked, true)
})
await scenario('only a video selected leaves the images scope checkbox empty', async () => {
  await click('视频'); await choose('video-one'); await click('图片')
  assert.equal(checkbox('全选当前分类').props.checked, false)
  assert.equal(checkbox('全选当前分类').props.indeterminate, false)
  assert.equal(button('取消选择').props.disabled, false, 'global clear remains enabled')
})
await scenario('filtered deselection preserves hidden images and other categories', async () => {
  await click('视频'); await choose('video-one'); await click('图片'); await changeAll(true)
  const search = find(el => el.type === 'UInput' && el.props['aria-label'] === '搜索文件名或附件 ID')
  search.props['onUpdate:modelValue']('one.png'); await vue.nextTick(); await changeAll(false)
  search.props['onUpdate:modelValue'](''); await vue.nextTick()
  assert.equal(checkbox('选择逻辑附件 image-one').props.checked, false)
  assert.equal(checkbox('选择逻辑附件 image-two').props.checked, true)
  assert.equal(checkbox('全选当前分类').props.indeterminate, true)
  await click('视频'); assert.equal(checkbox('选择逻辑附件 video-one').props.checked, true)
  await click('取消选择'); assert.equal(checkbox('选择逻辑附件 video-one').props.checked, false)
  await click('图片'); assert.equal(checkbox('选择逻辑附件 image-two').props.checked, false)
})
app.unmount()
// Existing callers omit partialSelected and must retain the original inference.
for (const [props, expected] of [[{ selected: 1, total: 3 }, true], [{ selected: 0, total: 3 }, false], [{ selected: 3, total: 3 }, false], [{ selected: 1, total: 3, partialSelected: false }, false]]) {
  const holder = node('root'), barApp = renderer.createApp(SelectionBar, props)
  barApp.component('UButton', { setup: (_, { attrs, slots }) => () => vue.h('button', attrs, slots.default?.()) })
  barApp.mount(holder)
  try { assert.equal(find(el => el.type === 'input', holder).props.indeterminate, expected) } catch (e) { failures.push(e) }
  barApp.unmount()
}
assert.equal(failures.length, 0, failures.map(e => e.message).join('\n'))
console.log('attachment selection interaction scenarios passed')
