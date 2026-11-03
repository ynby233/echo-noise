import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { stripTypeScriptTypes } from 'node:module'
import test from 'node:test'
import vm from 'node:vm'
import ts from 'typescript'
import { computed, effectScope, nextTick, reactive, ref, watch } from 'vue'

// Execute the complete setup and real data/navigation helpers. Only imports and
// TypeScript syntax are removed; no behavior is selected by source-text matches.
const paths = {
  component: '../components/index/MessageList.vue',
  store: '../store/message.ts',
  engagement: '../utils/message-list-engagement.ts',
  navigation: '../utils/message-target-navigation.ts',
  hash: '../utils/message-route-hash.ts',
}
const sources = Object.fromEntries(await Promise.all(Object.entries(paths).map(async ([key, path]) => {
  let source = await readFile(new URL(path, import.meta.url), 'utf8')
  if (key === 'component') {
    const opening = '<script setup lang="ts">'
    const start = source.indexOf(opening)
    const end = source.indexOf('</script>', start)
    assert.ok(start >= 0 && end > start, 'MessageList must contain a TypeScript setup block')
    source = source.slice(start + opening.length, end)
  }
  const ast = ts.createSourceFile(path, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  const removals = []
  for (const statement of ast.statements) {
    if (ts.isImportDeclaration(statement)) removals.push([statement.getStart(ast), statement.end])
    else for (const modifier of statement.modifiers || []) {
      if (modifier.kind === ts.SyntaxKind.ExportKeyword) removals.push([modifier.getStart(ast), modifier.end])
    }
  }
  for (const [start, end] of removals.sort((a, b) => b[0] - a[0])) source = source.slice(0, start) + source.slice(end)
  return [key, stripTypeScriptTypes(source, { mode: 'transform' })]
})))

function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

// A host task boundary drains Vue jobs and the chained promises from requests,
// engagement and navigation without wall-clock sleeps or executing retry timers.
async function flush() {
  await nextTick()
  await new Promise(resolve => setImmediate(resolve))
  await nextTick()
}

function fixture(t, { props: overrides = {}, hash = '', viewer = {}, commentSummary = id => ({ id, count: 0 }) } = {}) {
  const props = reactive({
    initialPage: 1, siteConfig: { commentEnabled: true }, targetMessageId: null,
    targetCommentId: null, masonry: false, wide: false, pageReady: true,
    activeTab: 'latest', calendarDate: '', searchKeyword: '', selectedTag: '', ...overrides,
  })
  const route = reactive({ hash })
  const user = reactive({ isLogin: true, token: 'viewer-a', user: { userid: 7, username: 'alice' }, ...viewer })
  const calls = [], unexpected = [], events = [], toasts = [], timers = new Map()
  const mounted = [], beforeUnmount = []
  const componentScope = effectScope(), storeScope = effectScope()
  const capability = deferred()
  let capabilityCalls = 0, exposed, store, stopped = false
  const schedule = callback => { const id = Symbol('timer'); timers.set(id, callback); return id }
  const request = (method, path, body, options = {}) => {
    const flight = { method, path, body, options, ...deferred(), settled: false }
    calls.push(flight)
    // Deliberately ignore AbortSignal: a server/transport may finish after abort.
    if (method === 'POST' && path === 'messages/comments/counts') {
      flight.settled = true
      flight.resolve({ code: 1, data: body.ids.map(commentSummary) })
    } else if (!(method === 'POST' && ['messages/page', 'messages/locate'].includes(path)) &&
      !(method === 'GET' && /^messages\/\d+$/.test(path)) &&
      !(method === 'FETCH' && path === '/api/guestbook/message')) {
      unexpected.push(`${method} ${path}`)
      flight.settled = true
      flight.resolve(method === 'FETCH' ? { ok: false } : { code: 0 })
    }
    return flight.promise
  }
  const windowEvents = new EventTarget()
  const lifecycleStub = () => ({ mount() {}, update() {}, dispose() {} })
  const context = vm.createContext({
    ref, computed, reactive, watch, nextTick, AbortController, URL, Event, queueMicrotask, console,
    setTimeout: schedule, clearTimeout: id => timers.delete(id),
    window: {
      matchMedia: () => ({ matches: false }),
      addEventListener: windowEvents.addEventListener.bind(windowEvents),
      removeEventListener: windowEvents.removeEventListener.bind(windowEvents),
      dispatchEvent: windowEvents.dispatchEvent.bind(windowEvents),
      setTimeout: schedule, clearTimeout: id => timers.delete(id),
      requestAnimationFrame: schedule, cancelAnimationFrame: id => timers.delete(id),
    },
    localStorage: { getItem: () => null },
    useRuntimeConfig: () => ({ public: { baseApi: '/api' } }),
    useUserStore: () => user,
    useMessageStore: () => store,
    // Vue reactive unwraps setup-store refs like Pinia does; the store's own
    // request generation, identity watch and append logic remain real.
    defineStore: (_, setup) => () => reactive(setup()),
    useMessage: () => ({ deleteMessage() {} }),
    useAdminCapabilities: () => ({ can: () => false, refreshCapabilities: () => { capabilityCalls++; return capability.promise } }),
    useRoute: () => route,
    useToast: () => ({ add: toast => toasts.push(toast) }),
    defineProps: () => props,
    defineEmits: () => (...args) => events.push(args),
    defineExpose: value => { exposed = value },
    inject: (_, fallback) => fallback,
    onMounted: callback => mounted.push(callback),
    onBeforeUnmount: callback => beforeUnmount.push(callback),
    asyncFeature: () => ({}),
    createMessageToolbox: () => ({ ...lifecycleStub(), openId: ref(null), toggle() {} }),
    createMessageListMedia: lifecycleStub,
    createMessageContentLayout: () => ({ ...lifecycleStub(), expanded: ref({}), showExpandButton: ref({}) }),
    getRequest: (path, body, options) => request('GET', path, body, options),
    postRequest: (path, body, options) => request('POST', path, body, options),
    fetch: (path, options) => request('FETCH', path, undefined, options),
  })
  const evaluate = (key, names) => vm.runInContext(`(() => {\n${sources[key]}\nreturn { ${names.join(', ')} };\n})()`, context, { filename: paths[key] })
  Object.assign(context, evaluate('hash', ['getMessageIdFromRouteHash']))
  Object.assign(context, evaluate('navigation', ['createMessageTargetNavigation', 'getMessageListScrollContainer', 'isMessageListScrollable']))
  Object.assign(context, evaluate('engagement', ['createMessageListEngagement']))
  store = storeScope.run(() => evaluate('store', ['useMessageStore']).useMessageStore())
  // Read-only observations plus the template's continuous-load callback and
  // comments ref callback. No production exports or test branches are needed.
  const state = componentScope.run(() => evaluate('component', [
    'displayMessages', 'isPageLoading', 'targetListReady', 'continuousError',
    'continuousLoading', 'loadContinuousPage', 'expandedCommentsMap', 'builtinCommentsRefFor',
    'toggleComment',
  ]))
  const all = path => calls.filter(call => call.path === path)
  const pending = path => {
    const matches = all(path).filter(call => !call.settled && !call.options.signal?.aborted)
    assert.equal(matches.length, 1, `expected exactly one pending ${path}`)
    return matches[0]
  }
  const respond = (call, response) => { call.settled = true; call.resolve(response) }
  const page = (items, total = 45) => respond(pending('messages/page'), { code: 1, data: { items, total } })
  const detail = (call, item) => respond(call, { code: 1, data: item })
  const mount = () => { for (const callback of mounted) callback() }
  const ready = async () => {
    mount()
    respond(pending('/api/guestbook/message'), { ok: true, json: async () => ({ data: { id: 999 } }) })
    await flush()
  }
  const unmount = () => {
    if (stopped) return
    stopped = true
    for (const callback of beforeUnmount) callback()
    componentScope.stop()
  }
  t.after(async () => {
    unmount()
    storeScope.stop()
    capability.resolve()
    await flush()
    assert.deepEqual(unexpected, [], 'no status health checks, per-message comments requests or other unplanned network work')
    assert.equal(timers.size, 0, 'unmount must release scheduled work')
  })
  return {
    props, route, user, store, state, exposed, events, toasts, calls, all, pending,
    respond, page, detail, mount, ready, unmount, capabilityCalls: () => capabilityCalls,
    ids: () => Array.from(state.displayMessages.value, item => item.id),
  }
}

const item = (id, extra = {}) => ({ id, content: `message ${id}`, user_id: 7, like_count: 3, liked: false, can_interact: true, ...extra })
const consumed = f => f.events.filter(([name]) => name === 'target-consumed').length

test('setup waits for mount and guestbook metadata; route changes coalesce into one page request', async t => {
  const f = fixture(t, { props: { initialPage: 3 } })
  await f.exposed.refreshList()
  f.route.hash = '#/messages/12'
  await flush()
  f.route.hash = ''
  await flush()
  assert.equal(f.calls.length, 0, 'setup and pre-mount watches must not fetch')
  f.mount()
  await flush()
  assert.equal(f.all('messages/page').length, 0, 'guestbook exclusion must be known before paging')
  assert.equal(f.capabilityCalls(), 1)
  f.route.hash = '#ordinary-anchor'
  f.respond(f.pending('/api/guestbook/message'), { ok: true, json: async () => ({ data: { id: 999 } }) })
  await flush()
  assert.equal(f.all('messages/page').length, 1, 'mounted and route scheduling must not duplicate the first page')
  assert.deepEqual({ ...f.pending('messages/page').body }, { page: 3, pageSize: 15, pinScope: 'latest', excludeId: 999 })
  assert.equal(f.state.targetListReady.value, false)
  f.page([item(31), item(32)], 60)
  await flush()
  assert.deepEqual(f.ids(), [31, 32], 'capability refresh may remain unresolved without blocking the list')
  assert.equal(f.state.targetListReady.value, true)
  assert.equal(f.state.isPageLoading.value, false)
  const counts = f.all('messages/comments/counts')
  assert.equal(counts.length, 1, 'visible messages use a single batch for comment counts')
  assert.deepEqual(Array.from(counts[0].body.ids), [31, 32])
  f.route.hash = '#another-anchor'
  await flush()
  assert.equal(f.all('messages/page').length, 1, 'unrelated hash changes must not reload')
})

test('pageReady gates page requests and continuous loading, including a late page response', async t => {
  const f = fixture(t, { props: { pageReady: false, masonry: true } })
  await f.ready()
  await f.exposed.refreshList()
  await f.state.loadContinuousPage()
  assert.equal(f.all('messages/page').length, 0)
  f.props.pageReady = true
  await flush()
  const old = f.pending('messages/page')
  f.props.pageReady = false
  await flush()
  assert.equal(old.options.signal.aborted, true)
  f.respond(old, { code: 1, data: { items: [item(1)], total: 45 } })
  await flush()
  assert.deepEqual(f.ids(), [])
  await f.state.loadContinuousPage()
  assert.equal(f.all('messages/page').length, 1)
  assert.equal(f.state.targetListReady.value, false)
  f.props.pageReady = true
  await flush()
  assert.equal(f.all('messages/page').length, 2)
  f.page([item(2)])
  await flush()
  assert.deepEqual(f.ids(), [2])
})

test('route detail waits for readiness, loads once and ignores unrelated hash suffixes', async t => {
  const f = fixture(t, { hash: '#/messages/41', props: { pageReady: false } })
  await f.ready()
  assert.equal(f.all('messages/41').length, 0)
  f.props.pageReady = true
  await flush()
  assert.equal(f.all('messages/41').length, 1)
  assert.equal(f.all('messages/page').length, 0)
  f.detail(f.pending('messages/41'), item(41))
  await flush()
  assert.deepEqual(f.ids(), [41])
  assert.equal(f.store.total, 1)
  assert.equal(f.store.hasMore, false)
  f.route.hash = '#/messages/41?comment=2'
  await flush()
  assert.equal(f.all('messages/41').length, 1)
  f.route.hash = '#/messages/42'
  await flush()
  f.detail(f.pending('messages/42'), item(42))
  await flush()
  assert.deepEqual(f.ids(), [42])
  f.route.hash = ''
  await flush()
  assert.equal(f.all('messages/page').length, 1, 'leaving detail restores normal paging')
  f.page([item(1), item(2)], 30)
  await flush()
  assert.deepEqual(f.ids(), [1, 2], 'normal list replaces the deep-link snapshot')
  assert.equal(f.store.total, 30)
  assert.equal(f.store.hasMore, true)
  f.route.hash = '#grid-message-1'
  await flush()
  assert.equal(f.all('messages/page').length, 1, 'media hashes preserve the restored page')
  assert.deepEqual(f.ids(), [1, 2])
})

for (const [name, change] of [
  ['logout', f => { f.user.isLogin = false }],
  ['user id', f => { f.user.user.userid = 8 }],
  ['username', f => { f.user.user.username = 'bob' }],
  ['token', f => { f.user.token = 'viewer-b' }],
  ['admin role', f => { f.user.user.is_admin = true }],
]) {
  test(`identity change (${name}) synchronously rejects an already queued detail response`, async t => {
    const f = fixture(t, { hash: '#/messages/51' })
    await f.ready()
    const old = f.pending('messages/51')
    f.detail(old, item(51, { content: 'private old viewer response' }))
    change(f) // Before the resolved request continuation or batched route watch.
    assert.equal(old.options.signal.aborted, true, 'invalidation must happen in the identity mutation stack')
    assert.deepEqual(f.ids(), [])
    await flush()
    assert.deepEqual(f.ids(), [], 'the queued old response must never restore private content')
    assert.equal(f.state.isPageLoading.value, true, 'old finally must not finish the new request')
    assert.equal(f.state.targetListReady.value, false)
    f.detail(f.pending('messages/51'), item(51, { content: 'new viewer response' }))
    await flush()
    assert.equal(f.store.messages[0].content, 'new viewer response')
    assert.equal(f.all('messages/51').length, 2)
    assert.deepEqual(f.toasts, [])
  })
}

test('role-only changes clear old viewer data and reload the current list', async t => {
  const f = fixture(t)
  await f.ready()
  f.page([item(1)], 1)
  await flush()
  f.user.user.is_admin = true
  assert.deepEqual(f.ids(), [], 'role change synchronously removes the old permission view')
  await flush()
  assert.equal(f.all('messages/page').length, 2, 'component must reload after the store identity reset')
  f.page([item(2)], 1)
  await flush()
  assert.deepEqual(f.ids(), [2])
  assert.equal(f.state.isPageLoading.value, false)
})

test('layout and filter round trips retain cached visible-comment expansion and user collapse', async t => {
  const f = fixture(t, {
    commentSummary: id => ({ id, count: id === 2 ? 0 : 1, hasVisibleComments: true }),
  })
  const messages = [item(1), item(2), item(3)]
  await f.ready()
  f.page(messages, 3)
  await flush()
  assert.equal(f.state.expandedCommentsMap.value[1], true)
  assert.equal(f.state.expandedCommentsMap.value[2], true, 'visible replies under a tombstone expand at count zero')
  await f.state.toggleComment(3)
  assert.equal(f.state.expandedCommentsMap.value[3], false)
  f.props.masonry = true
  await flush()
  f.page(messages, 3)
  await flush()
  assert.equal(f.state.expandedCommentsMap.value[1], true)
  assert.equal(f.state.expandedCommentsMap.value[2], true)
  assert.equal(f.state.expandedCommentsMap.value[3], false)
  f.props.searchKeyword = 'other'
  await flush()
  f.page([item(4)], 1)
  await flush()
  f.props.searchKeyword = ''
  await flush()
  f.page(messages, 3)
  await flush()
  assert.deepEqual(f.ids(), [1, 2, 3])
  assert.equal(f.state.expandedCommentsMap.value[1], true)
  assert.equal(f.state.expandedCommentsMap.value[2], true)
  assert.equal(f.state.expandedCommentsMap.value[3], false, 'explicit collapse survives cached summary reuse')
  assert.equal(f.all('messages/comments/counts').length, 2, 'returning messages reuse their summaries')
})

test('unmount rejects late details and cannot publish readiness or loading events', async t => {
  const f = fixture(t, { hash: '#/messages/61' })
  await f.ready()
  const old = f.pending('messages/61')
  f.unmount()
  assert.equal(old.options.signal.aborted, true)
  const events = f.events.length
  f.detail(old, item(61))
  await flush()
  assert.deepEqual(f.ids(), [])
  assert.equal(f.state.targetListReady.value, false)
  assert.equal(f.events.length, events)
  assert.deepEqual(f.toasts, [])
})

test('unmount during guestbook bootstrap never starts paging', async t => {
  const f = fixture(t)
  f.mount()
  const metadata = f.pending('/api/guestbook/message')
  f.unmount()
  assert.equal(metadata.options.signal.aborted, true)
  f.respond(metadata, { ok: true, json: async () => ({ data: { id: 999 } }) })
  await flush()
  assert.equal(f.all('messages/page').length, 0)
})

test('filter changes reset to page one, preserve query fields and reject stale pages', async t => {
  const f = fixture(t, { props: { initialPage: 3 } })
  await f.ready()
  f.page([item(31)], 60)
  await flush()
  f.props.searchKeyword = ' first '
  await flush()
  const old = f.pending('messages/page')
  Object.assign(f.props, { searchKeyword: ' second ', selectedTag: ' #work ', calendarDate: '2026-10-01', activeTab: 'personal' })
  await flush()
  assert.equal(old.options.signal.aborted, true)
  const current = f.pending('messages/page')
  assert.deepEqual({ ...current.body }, {
    page: 1, pageSize: 15, pinScope: 'personal', excludeId: 999,
    authorId: 7, date: '2026-10-01', keyword: 'second', tag: 'work',
  })
  f.respond(current, { code: 1, data: { items: [item(80)], total: 1 } })
  await flush()
  f.respond(old, { code: 1, data: { items: [item(70)], total: 60 } })
  await flush()
  assert.deepEqual(f.ids(), [80])
  assert.equal(f.store.page, 1)
  assert.equal(f.store.total, 1)
  assert.equal(f.all('messages/page').length, 3, 'several filter changes in one tick cause one refresh')
})

test('continuous loading appends once, deduplicates, retries failure and stops at the last page', async t => {
  const f = fixture(t, { props: { masonry: true } })
  await f.ready()
  f.page([item(1), item(2)])
  await flush()
  const load = f.state.loadContinuousPage()
  await f.state.loadContinuousPage()
  assert.equal(f.all('messages/page').length, 2, 'repeated observer callbacks share the pending load')
  assert.equal(f.pending('messages/page').body.page, 2)
  f.page([item(2), item(3)])
  await load
  await flush()
  assert.deepEqual(f.ids(), [1, 2, 3])
  const failed = f.state.loadContinuousPage()
  f.respond(f.pending('messages/page'), { code: 0 })
  await failed
  assert.deepEqual(f.ids(), [1, 2, 3])
  assert.equal(f.store.page, 2)
  assert.equal(f.state.continuousError.value, true)
  const retry = f.state.loadContinuousPage()
  assert.equal(f.pending('messages/page').body.page, 3)
  f.page([item(4)], 4)
  await retry
  await flush()
  assert.deepEqual(f.ids(), [1, 2, 3, 4])
  assert.equal(f.state.continuousError.value, false)
  assert.equal(f.store.hasMore, false)
  await f.state.loadContinuousPage()
  assert.equal(f.all('messages/page').length, 4)
})

test('changing filters during continuous loading cannot append an old segment', async t => {
  const f = fixture(t, { props: { masonry: true } })
  await f.ready()
  f.page([item(1)])
  await flush()
  const append = f.state.loadContinuousPage()
  const old = f.pending('messages/page')
  f.props.selectedTag = 'new'
  await flush()
  assert.equal(old.options.signal.aborted, true)
  const current = f.pending('messages/page')
  f.respond(old, { code: 1, data: { items: [item(2)], total: 45 } })
  await append
  assert.equal(f.state.targetListReady.value, false)
  assert.equal(f.state.isPageLoading.value, true)
  f.respond(current, { code: 1, data: { items: [item(20)], total: 1 } })
  await flush()
  assert.deepEqual(f.ids(), [20])
  assert.equal(f.store.page, 1)
  assert.equal(f.state.continuousLoading.value, false)
})

for (const masonry of [false, true]) {
  test(`notification target retains page location and comment focus (${masonry ? 'continuous' : 'paged'})`, async t => {
    const f = fixture(t, { props: { masonry, targetMessageId: '91', targetCommentId: 901 } })
    await f.ready()
    assert.equal(f.all('messages/locate').length, 0, 'notification waits for initial page readiness')
    await f.state.loadContinuousPage()
    f.page([item(1)])
    await flush()
    const location = f.pending('messages/locate')
    assert.equal(location.body.messageId, 91)
    assert.equal(location.body.excludeId, 999)
    f.respond(location, { code: 1, data: { page: 2 } })
    await flush()
    if (masonry) {
      assert.equal(f.pending('messages/page').body.page, 1)
      f.page([item(1)])
      await flush()
    }
    assert.equal(f.pending('messages/page').body.page, 2)
    const focused = []
    f.state.builtinCommentsRefFor(91)({ focusCommentById: async (id, options) => { focused.push([id, options.scroll]); return true } })
    f.page([item(91)])
    await flush()
    assert.deepEqual(f.ids(), masonry ? [1, 91] : [91])
    assert.deepEqual(focused, [[901, false]])
    assert.equal(f.state.expandedCommentsMap.value[91], true)
    assert.equal(consumed(f), 1)
    assert.equal(f.all('messages/locate').length, 1)
    assert.equal(f.all('messages/91').length, 0, 'a located target needs no redundant detail fetch')
  })
}

test('unmount cancels notification location without consuming a late target', async t => {
  const f = fixture(t, { props: { targetMessageId: '92' } })
  await f.ready()
  f.page([item(1)])
  await flush()
  const location = f.pending('messages/locate')
  f.unmount()
  assert.equal(location.options.signal.aborted, true)
  f.respond(location, { code: 1, data: { page: 2 } })
  await flush()
  assert.equal(consumed(f), 0)
  assert.equal(f.all('messages/page').length, 1)
  assert.equal(f.all('messages/92').length, 0)
})
