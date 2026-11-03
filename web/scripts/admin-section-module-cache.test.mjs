import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'
import ts from 'typescript'
import * as vue from 'vue'
import { parse, compileScript } from '@vue/compiler-sfc'

const read = path => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')
const transpile = source => ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
const evaluate = source => {
  const module = { exports: {} }
  vm.runInNewContext(transpile(source), { module, exports: module.exports, require: name => { assert.equal(name, 'vue'); return vue } })
  return module.exports
}
const { createAdminSectionModuleCache } = evaluate(read('components/admin/sections/module-cache.ts'))
const hostSource = read('components/admin/sections/AdminSectionHost.vue')
const { descriptor } = parse(hostSource)
const Host = evaluate(compileScript(descriptor, { id: 'host-test', inlineTemplate: true }).content).default
const deferred = () => { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
const flush = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); await vue.nextTick() }
const node = (type, text = '') => ({ type, text, children: [], props: {}, parent: null })
const renderer = vue.createRenderer({
  createElement: type => node(type), createText: text => node('text', text), createComment: text => node('comment', text),
  setText: (target, text) => { target.text = text }, setElementText: (target, text) => { target.text = text; target.children = [] },
  parentNode: target => target.parent, nextSibling: target => target.parent?.children[target.parent.children.indexOf(target) + 1] || null,
  patchProp: (target, key, _previous, value) => { target.props[key] = value },
  insert(target, parent, anchor = null) {
    if (target.parent) target.parent.children.splice(target.parent.children.indexOf(target), 1)
    const index = anchor ? parent.children.indexOf(anchor) : -1
    parent.children.splice(index < 0 ? parent.children.length : index, 0, target)
    target.parent = parent
  },
  remove(target) { if (target.parent) target.parent.children.splice(target.parent.children.indexOf(target), 1); target.parent = null },
})
const text = target => target.type === 'comment' ? '' : target.text + target.children.map(text).join('')
const mount = (cache, loader) => {
  const root = node('root')
  const app = renderer.createApp(Host, { moduleCache: cache, loader, sectionTitle: '网站配置' })
  app.component('UIcon', { render: () => vue.h('i') })
  app.component('UButton', { render() { return vue.h('button', this.$slots.default?.()) } })
  app.mount(root)
  return { root, dispose: () => app.unmount() }
}

test('warming imports code once without mounting or requesting data; successful host mounts synchronously', async () => {
  const cache = createAdminSectionModuleCache()
  let imports = 0, mounted = 0
  const section = { setup() { mounted++; return () => vue.h('form', '真实配置表单') } }
  const loader = async () => { imports++; return { default: section } }
  const first = cache.load(loader)
  assert.equal(cache.load(loader), first)
  await first
  assert.equal(imports, 1)
  assert.equal(mounted, 0)
  for (let i = 0; i < 4; i++) {
    const host = mount(cache, loader)
    assert.equal(text(host.root), '真实配置表单', 'no async loading frame on return, even after eviction')
    host.dispose()
  }
  assert.equal(imports, 1)
  assert.equal(mounted, 4)
})

test('cold module has a labelled shell, and an unmounted host ignores its late result', async () => {
  const cache = createAdminSectionModuleCache()
  const response = deferred()
  let mounted = 0
  const host = mount(cache, () => response.promise)
  assert.match(text(host.root), /网站配置.*正在准备配置界面/)
  assert.doesNotMatch(text(host.root), /正在加载面板/)
  host.dispose()
  response.resolve({ default: { setup() { mounted++; return () => vue.h('form') } } })
  await flush()
  assert.equal(mounted, 0)
})

test('failed imports remain retryable and cache clearing rejects late cache population', async () => {
  const cache = createAdminSectionModuleCache()
  let attempts = 0
  const module = { default: { render: () => vue.h('form') } }
  const loader = async () => { if (++attempts === 1) throw new Error('network'); return module }
  await assert.rejects(cache.load(loader), /network/)
  assert.equal(cache.get(loader), undefined)
  await cache.load(loader)
  assert.equal(cache.get(loader), module)
  cache.clear()
  assert.equal(cache.get(loader), undefined)
  const old = deferred(), current = deferred()
  let count = 0
  const changing = () => (++count === 1 ? old.promise : current.promise)
  const first = cache.load(changing)
  await flush()
  cache.clear()
  const second = cache.load(changing)
  await flush()
  old.resolve(module)
  await first
  assert.equal(cache.get(changing), undefined)
  assert.equal(cache.load(changing), second, 'old finally cannot remove the new pending request')
  current.resolve(module)
  await second
  assert.equal(cache.get(changing), module)
})

test('entry warming waits for mount and permissions, filters keys, and clears drafts/code on identity change', async () => {
  const source = read('components/index/StatusPanel.vue')
  const block = source.slice(source.indexOf('const sectionContext ='), source.indexOf('const sectionGroupMap ='))
  const idle = new Map(), loaded = []
  let id = 0
  const scope = vue.effectScope()
  const account = vue.ref('1'), keys = vue.ref(['dashboard', 'site']), ready = vue.ref(false)
  const capabilities = vue.ref(['access_logs.view', 'security.manage'])
  const cache = createAdminSectionModuleCache(), drafts = new Map([['site', { value: 'draft' }]])
  const loaders = Object.fromEntries(['dashboard', 'site', 'storage'].map(key => [key, async () => { loaded.push(key); return { default: {} } }]))
  const context = vm.createContext({
    ...vue, draftAccount: account, accessibleSectionKeys: keys, adminCapabilitiesReady: ready,
    adminCapabilities: capabilities, isAdmin: vue.ref(true), isPrimaryAdmin: vue.ref(false),
    provide() {}, adminDraftAccountKey: Symbol('admin draft account'),
    userStore: { isLogin: true }, activeSection: vue.ref('dashboard'), adminSectionLoaders: loaders,
    sectionModuleCache: cache, adminDrafts: drafts,
    navigationSequence: 0, navigationError: vue.ref(''), failedSection: vue.ref(null),
    window: { requestIdleCallback: fn => { idle.set(++id, fn); return id }, cancelIdleCallback: key => idle.delete(key), clearTimeout() {} },
  })
  scope.run(() => vm.runInContext(transpile(`${block}\nglobalThis.subject = { sectionContext, warmSection, scheduleWarmup, mount: () => { mounted = true }, stop: () => cancelWarmup() }`), context))
  context.subject.warmSection('site')
  context.subject.scheduleWarmup()
  assert.equal(idle.size, 0)
  context.subject.mount()
  context.subject.scheduleWarmup()
  assert.equal(idle.size, 0, 'permissions must be ready before warming')
  ready.value = true
  await flush()
  assert.equal(idle.size, 1)
  context.subject.warmSection('storage')
  await flush()
  assert.deepEqual(loaded, [])
  const [ticket, callback] = idle.entries().next().value
  idle.delete(ticket)
  callback()
  await flush()
  assert.deepEqual(loaded, ['site'])
  assert.ok(cache.get(loaders.site))
  assert.match(source, /<KeepAlive[^>]*:key="sectionContext"/, 'the actual shell must key its instances by the complete permission context')
  let mounts = 0, unmounts = 0
  const Form = { setup() { mounts++; vue.onUnmounted(() => { unmounts++ }); return () => vue.h('form', '草稿表单') } }
  const app = renderer.createApp({ render: () => vue.h(vue.KeepAlive, { key: context.subject.sectionContext.value }, { default: () => vue.h(Form) }) })
  app.mount(node('root'))
  drafts.set('site', { value: 'unsaved draft' })
  const previousContext = context.subject.sectionContext.value
  capabilities.value = ['security.manage', 'access_logs.view']
  await flush()
  assert.equal(context.subject.sectionContext.value, previousContext, 'reordered but identical snapshots remain stable')
  assert.equal(drafts.size, 1)
  assert.ok(cache.get(loaders.site))
  assert.equal(mounts, 1)
  assert.equal(unmounts, 0)
  capabilities.value = ['access_logs.view']
  assert.deepEqual(keys.value, ['dashboard', 'site'], 'the navigation keys did not change')
  assert.equal(drafts.size, 0, 'revoking an action capability immediately clears drafts')
  assert.equal(cache.get(loaders.site), undefined)
  await flush()
  assert.equal(mounts, 2, 'the still-visible section gets a fresh instance after action revocation')
  assert.equal(unmounts, 1, 'the cached instance owning the old permissions is destroyed')
  app.unmount()
  account.value = '2'
  assert.equal(drafts.size, 0)
  assert.equal(cache.get(loaders.site), undefined)
  keys.value = ['dashboard']
  await flush()
  assert.equal(idle.size, 0, 'revoking access cancels pending warmup')
  context.subject.warmSection('site')
  await flush()
  assert.deepEqual(loaded, ['site'])
  context.subject.stop()
  scope.stop()
})

test('navigation keeps the current form during cold imports and failures, then commits a synchronously available module', async () => {
  const source = read('components/index/StatusPanel.vue')
  const block = source.slice(source.indexOf('const setActive ='), source.indexOf('const onHashChange ='))
  const cache = createAdminSectionModuleCache()
  let response = deferred()
  const loader = () => response.promise
  const activeSection = vue.ref('dashboard'), navigationError = vue.ref(''), failedSection = vue.ref(null)
  const context = vm.createContext({
    accessibleSectionKeys: vue.ref(['dashboard', 'site']), adminSectionLoaders: { site: loader },
    sectionModuleCache: cache, sectionContext: vue.ref('account-1'), navigationSequence: 0,
    navigationError, failedSection, activeSection, sectionGroupMap: vue.ref({}), navGroupOpen: {},
    mainScroll: vue.ref(null), window: { innerWidth: 1200 },
  })
  vm.runInContext(transpile(`${block}\nglobalThis.navigate = setActive`), context)
  const first = context.navigate('site', false)
  assert.equal(activeSection.value, 'dashboard', 'old real panel remains mounted while code is pending')
  response.reject(new Error('offline'))
  await first
  assert.equal(activeSection.value, 'dashboard')
  assert.equal(navigationError.value, 'offline')
  assert.equal(failedSection.value, 'site')
  response = deferred()
  const retry = context.navigate(failedSection.value, false)
  assert.equal(activeSection.value, 'dashboard')
  response.resolve({ default: { render: () => vue.h('form', '新表单') } })
  await retry
  assert.equal(activeSection.value, 'site')
  assert.equal(navigationError.value, '')
  const host = mount(cache, loader)
  assert.equal(text(host.root), '新表单', 'new panel renders in the commit frame without loading')
  host.dispose()
  activeSection.value = 'dashboard'
  context.navigate('site', false)
  assert.equal(activeSection.value, 'site', 'warm navigation commits immediately')
})
