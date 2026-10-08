import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import test from 'node:test'
import ts from 'typescript'
import * as vue from 'vue'

const plain = value => JSON.parse(JSON.stringify(value))
const flush = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); await vue.nextTick() }
const deferred = () => {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const track = (id, extra = {}) => ({ trackID: id, title: `歌曲 ${id}`, artist: '艺术家', album: '专辑', durationMS: 180000, format: 'flac', coverURL: '', lyricsAvailable: true, available: true, ...extra })
const config = (extra = {}) => ({ version: 7, frontendSettings: { musicSource: 'local', musicEnabled: true }, scanIntervalMinutes: 60, playlist: [track('a')], scan: { state: 'succeeded' }, rootReadable: true, toolsReady: true, counts: { total: 3, available: 2, unavailable: 1 }, ...extra })
const ok = (data, status = 200) => ({ ok: status < 300, status, json: async () => ({ code: status < 300 ? 1 : 0, data }) })

// Real Vue reactivity and the complete transpiled production module. Only I/O,
// lifecycle registration, and the clock are replaced, following admin-section-state.
function runtime(file, adapters = {}) {
  const hooks = { mounted: [], activated: [], deactivated: [], unmounted: [] }
  const timers = new Map()
  let timerID = 0, now = 0
  const window = new EventTarget()
  const document = Object.assign(new EventTarget(), { visibilityState: 'visible', hidden: false })
  const setTimer = (callback, delay = 0, interval = 0) => { const id = ++timerID; timers.set(id, { callback, at: now + delay, interval }); return id }
  Object.assign(window, {
    setTimeout: (fn, delay) => setTimer(fn, delay), clearTimeout: id => timers.delete(id),
    setInterval: (fn, delay) => setTimer(fn, delay, delay), clearInterval: id => timers.delete(id),
  })
  const scope = vue.effectScope()
  const lifecycle = Object.fromEntries(Object.keys(hooks).map(key => [`on${key[0].toUpperCase()}${key.slice(1)}`, fn => hooks[key].push(fn)]))
  const module = { exports: {} }
  const sandbox = vm.createContext({
    console, module, exports: module.exports, window, document, Event, AbortController, AbortSignal, URL, URLSearchParams,
    setTimeout: window.setTimeout, clearTimeout: window.clearTimeout,
    setInterval: window.setInterval, clearInterval: window.clearInterval,
    require: name => {
      if (name === 'vue') return { ...vue, ...lifecycle }
      if (name === '#imports') return { useRuntimeConfig: () => ({ public: { baseApi: '/fixture/api/' } }) }
      throw new Error(`Unexpected production dependency: ${name}`)
    },
    ...adapters,
  })
  const code = ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText
  scope.run(() => vm.runInContext(code, sandbox, { filename: file }))
  return {
    exports: module.exports, scope, window, document, timers,
    lifecycle: async key => { for (const hook of hooks[key]) hook(); await flush() },
    advance: async milliseconds => {
      const end = now + milliseconds
      for (let turns = 0; turns < 100; turns++) {
        const next = [...timers].filter(([, timer]) => timer.at <= end).sort((a, b) => a[1].at - b[1].at)[0]
        if (!next) { now = end; await flush(); return }
        const [id, timer] = next
        now = timer.at
        if (timer.interval) timer.at += timer.interval; else timers.delete(id)
        timer.callback(); await flush()
      }
      throw new Error('Timer did not settle within 100 callbacks')
    },
    dispose: () => { for (const hook of hooks.unmounted) hook(); scope.stop() },
  }
}
function workbench(options = {}) {
  const calls = []
  let stored = config(options.config)
  const items = options.items || [track('b'), track('c'), track('broken', { available: false })]
  const io = options.io || (async call => {
    if (call.method === 'PUT') {
      const body = JSON.parse(call.init.body)
      stored = { ...stored, version: stored.version + 1, frontendSettings: body.frontendSettings, scanIntervalMinutes: body.scanIntervalMinutes, playlist: body.trackIDs.map(id => items.find(item => item.trackID === id) || track(id)) }
      return ok(plain(stored))
    }
    if (call.url.pathname.endsWith('/refresh-status')) return ok({ state: 'succeeded' })
    if (call.method === 'POST') return ok({ state: 'running', runID: 'scan-fixture', started: true }, 202)
    if (call.url.pathname.endsWith('/library')) return ok({ items: plain(items), total: 55, page: Number(call.url.searchParams.get('page')), pageSize: 25 })
    return ok(plain(stored))
  })
  const harness = runtime('../components/admin/sections/use-music-workbench.ts', { fetch: async (url, init) => {
    const call = { url: new URL(url, 'http://fixture.invalid'), method: init.method || 'GET', init }
    calls.push(call)
    return io(call)
  } })
  const account = options.account || vue.ref('account-a')
  const canManage = options.canManage || vue.ref(true)
  const canView = options.canView || vue.ref(true)
  const drafts = options.drafts || new Map()
  const subject = harness.scope.run(() => harness.exports.useMusicWorkbench({
    baseApi: '/fixture/api/', account, canManage, canView, drafts,
    refreshCapabilities: options.refreshCapabilities || (async () => {}),
  }))
  return { ...harness, ...subject, calls, account, canManage, canView, drafts,
    mount: () => harness.lifecycle('mounted'), leave: () => harness.lifecycle('deactivated'), enter: () => harness.lifecycle('activated'),
    writes: () => calls.filter(call => call.method !== 'GET'),
    libraries: () => calls.filter(call => call.url.pathname.endsWith('/library')),
  }
}

test('uninitialized or failed initialization never saves defaults; retry initializes arrays and booleans', async t => {
  let failure = true
  const panel = workbench({ io: async call => call.url.pathname.endsWith('/library') ? ok({ items: [], total: 0, page: 1, pageSize: 25 }) : failure ? ok(null, 503) : ok(config({ frontendSettings: { musicSource: 'local', musicEnabled: 'true', musicAutoplay: 'false' } })) })
  t.after(panel.dispose)
  await panel.save(); assert.equal(panel.writes().length, 0)
  await panel.mount()
  assert.equal(panel.ready.value, false); assert.match(panel.error.value, /不可用/)
  await panel.save(); assert.equal(panel.writes().length, 0)
  failure = false; await panel.loadConfig()
  assert.equal(panel.ready.value, true)
  assert.deepEqual(plain(panel.draft.value.trackIDs), ['a'])
  assert.equal(panel.draft.value.frontendSettings.musicEnabled, true)
  assert.equal(panel.draft.value.frontendSettings.musicAutoplay, false)
  assert.equal(panel.dirty.value, false)
})

test('explicit selection adds only current checked available UUIDs, deduplicates and orders by UUID', async t => {
  const panel = workbench(); t.after(panel.dispose); await panel.mount()
  panel.addSelected(); assert.deepEqual(plain(panel.draft.value.trackIDs), ['a'])
  panel.selectedIDs.value = new Set(['b', 'broken', 'off-page'])
  panel.addSelected()
  assert.deepEqual(plain(panel.draft.value.trackIDs), ['a', 'b'])
  assert.equal(panel.selectedIDs.value.size, 0)
  panel.addTrack(track('b')); panel.addTrack(track('broken', { available: false }))
  panel.addTrack(track('c')); panel.moveTrack('c', 0); panel.moveTrack('missing', 0); panel.moveTrack('a', NaN)
  assert.deepEqual(plain(panel.draft.value.trackIDs), ['c', 'a', 'b'])
  panel.removeTrack('a')
  assert.deepEqual(plain(panel.playlist.value.map(item => item.trackID)), ['c', 'b'])
  assert.equal(panel.writes().length, 0, 'selection and order remain a draft until save')
  assert.equal(panel.dirty.value, true)
})

test('playlist limit blocks the 1001st track while allowing removal and reordering', async t => {
  const panel = workbench({ config: { playlist: Array.from({ length: 1000 }, (_, i) => track(`id-${i}`)) } })
  t.after(panel.dispose); await panel.mount()
  panel.addTrack(track('extra')); assert.equal(panel.draft.value.trackIDs.length, 1000)
  panel.removeTrack('id-500'); panel.addTrack(track('extra')); panel.moveTrack('extra', 0)
  assert.equal(panel.draft.value.trackIDs.length, 1000); assert.equal(panel.draft.value.trackIDs[0], 'extra')
})

test('server search debounces 300ms, invalidates old responses immediately, and filters reset pagination', async t => {
  let stale
  const panel = workbench({ io: async call => {
    if (!call.url.pathname.endsWith('/library')) return ok(config())
    if (call.url.searchParams.get('q') === 'old') { stale = deferred(); return stale.promise }
    return ok({ items: [track(call.url.searchParams.get('q') || 'initial')], total: 55, page: Number(call.url.searchParams.get('page')), pageSize: 25 })
  } })
  t.after(panel.dispose); await panel.mount()
  panel.query.page = 2; await flush()
  panel.query.q = 'old'; const loading = panel.loadLibrary(); await flush()
  const oldCall = panel.libraries().at(-1)
  panel.search.value = ' new '; await flush()
  assert.equal(oldCall.init.signal.aborted, true)
  stale.resolve(ok({ items: [track('stale')], total: 55, page: 2, pageSize: 25 })); await loading
  assert.notEqual(panel.library.value.items[0].trackID, 'stale')
  const count = panel.libraries().length
  await panel.advance(299); assert.equal(panel.libraries().length, count)
  panel.search.value = ' newest '; await flush(); await panel.advance(299)
  assert.equal(panel.libraries().length, count)
  await panel.advance(1)
  const query = panel.libraries().at(-1).url.searchParams
  assert.equal(query.get('q'), 'newest'); assert.equal(query.get('page'), '1'); assert.equal(query.get('pageSize'), '25')
  assert.equal(panel.library.value.items[0].trackID, 'newest')
  panel.query.page = 2; await flush(); panel.selectedIDs.value = new Set(['newest'])
  panel.query.format = 'flac'; panel.query.lyrics = 'yes'; panel.query.availability = 'available'; panel.query.selected = 'no'; await flush()
  const filtered = panel.libraries().at(-1).url.searchParams
  assert.deepEqual(Object.fromEntries(['page', 'format', 'lyrics', 'availability', 'selected'].map(key => [key, filtered.get(key)])), { page: '1', format: 'flac', lyrics: 'yes', availability: 'available', selected: 'no' })
  assert.equal(panel.selectedIDs.value.size, 0)
  assert.deepEqual(plain(panel.draft.value.trackIDs), ['a'])
})

test('save submits source, preferences, interval, order and original version atomically; duplicate clicks write once', async t => {
  const pending = deferred(); let server = config()
  const panel = workbench({ io: async call => {
    if (call.method === 'PUT') return pending.promise
    if (call.url.pathname.endsWith('/library')) return ok({ items: [track('b')], total: 1, page: 1, pageSize: 25 })
    return ok(plain(server))
  } })
  t.after(panel.dispose); await panel.mount()
  panel.addTrack(track('b')); panel.moveTrack('b', 0)
  panel.draft.value.frontendSettings.musicTheme = 'dark'; panel.draft.value.scanIntervalMinutes = 30
  let notifications = 0; panel.window.addEventListener('frontend-config-updated', () => notifications++)
  const saving = panel.save(); await panel.save()
  assert.equal(panel.writes().length, 1)
  const submitted = JSON.parse(panel.writes()[0].init.body)
  assert.deepEqual(Object.keys(submitted).sort(), ['frontendSettings', 'scanIntervalMinutes', 'trackIDs', 'version'])
  assert.equal(submitted.version, 7); assert.equal(submitted.frontendSettings.musicSource, 'local')
  assert.equal(submitted.frontendSettings.musicTheme, 'dark'); assert.equal(submitted.scanIntervalMinutes, 30)
  assert.deepEqual(submitted.trackIDs, ['b', 'a'])
  server = config({ version: 8, frontendSettings: submitted.frontendSettings, scanIntervalMinutes: 30, playlist: [track('b'), track('a')] })
  pending.resolve(ok(plain(server))); await saving; await flush()
  assert.equal(panel.draft.value.version, 8); assert.equal(panel.dirty.value, false)
  assert.equal(panel.saving.value, false); assert.equal(notifications, 1); assert.ok(panel.savedAt.value)
})

test('409 retains the draft and expected version across metadata refresh and eviction until explicit discard', async t => {
  const drafts = new Map(); let current = config()
  const io = async call => call.method === 'PUT' ? ok(null, 409) : call.url.pathname.endsWith('/library') ? ok({ items: [track('b')], total: 1, page: 1, pageSize: 25 }) : ok(plain(current))
  const first = workbench({ drafts, io }); t.after(first.dispose); await first.mount()
  first.addTrack(track('b')); first.moveTrack('b', 0); await first.save()
  assert.equal(first.conflict.value, true); assert.equal(first.draft.value.version, 7)
  assert.equal(drafts.get('music-workbench').conflict, true, '409 is remembered before leaving or refreshing')
  assert.equal(first.error.value, '音乐配置已被其他管理员更新，请重新加载后保存')
  current = config({ version: 9, playlist: [track('a', { title: 'updated metadata', available: false }), track('c')] })
  await first.loadConfig()
  assert.equal(first.draft.value.version, 7); assert.deepEqual(plain(first.draft.value.trackIDs), ['b', 'a'])
  assert.equal(first.playlist.value[1].title, 'updated metadata'); assert.equal(first.playlist.value[1].available, false)
  await first.save(); assert.equal(first.writes().length, 1, '409 must not automatically rebase or resend')
  await first.leave(); first.dispose()
  const second = workbench({ drafts, io }); t.after(second.dispose); await second.mount()
  assert.equal(second.draft.value.version, 7); assert.deepEqual(plain(second.draft.value.trackIDs), ['b', 'a'])
  assert.equal(second.conflict.value, true)
  await second.save(); assert.equal(second.writes().length, 0, 'recreated conflict blocks PUT')
  await second.loadConfig({ discard: true })
  assert.equal(second.draft.value.version, 9); assert.deepEqual(plain(second.draft.value.trackIDs), ['a', 'c'])
  assert.equal(second.conflict.value, false); assert.equal(second.dirty.value, false)
  assert.equal(drafts.get('music-workbench').conflict, false)
})

test('409 on an unchanged draft survives immediate recreation and failed discard without rebasing or PUT', async t => {
  const drafts = new Map(); let failRead = false, version = 7
  const io = async call => call.method === 'PUT' ? ok(null, 409) : call.url.pathname.endsWith('/library') ? ok({ items: [], total: 0, page: 1, pageSize: 25 }) : failRead ? ok(null, 503) : ok(config({ version }))
  const first = workbench({ drafts, io }); t.after(first.dispose); await first.mount()
  await first.save()
  assert.equal(first.dirty.value, false); assert.equal(first.conflict.value, true)
  // Snapshot before unmount hooks can call remember: the 409 handler must persist it itself.
  const snapshot = new Map(plain([...drafts]))
  const second = workbench({ drafts: snapshot, io }); t.after(second.dispose)
  assert.equal(second.conflict.value, true)
  failRead = true; await second.mount(); await second.loadConfig({ discard: true }); await second.save()
  assert.equal(second.conflict.value, true); assert.equal(snapshot.get('music-workbench').conflict, true)
  assert.equal(second.writes().length, 0)
  failRead = false; version = 9; await second.loadConfig()
  assert.equal(second.conflict.value, true); await second.save(); assert.equal(second.writes().length, 0)
  assert.equal(second.draft.value.version, 7); assert.equal(second.dirty.value, false)
  await second.loadConfig({ discard: true })
  assert.equal(second.conflict.value, false); assert.equal(snapshot.get('music-workbench').conflict, false)
  assert.equal(second.draft.value.version, 9)
})

test('unsaved cross-page selection restores projected title, cover, CUE and refreshed availability outside the PUT payload', async t => {
  const drafts = new Map()
  const selected = track('cue', { title: '<b>跨页歌曲</b>', coverURL: '/fixture/api/music/cover/cue', cueTrackNumber: 3, relativePath: '/private/cue.flac', arbitrary: 'secret' })
  let unavailable = false
  const io = async call => {
    if (call.method === 'PUT') return ok(config({ version: 8, playlist: [track('a'), selected] }))
    if (call.url.pathname.endsWith('/library')) {
      const page = Number(call.url.searchParams.get('page'))
      return ok({ items: page === 2 ? [{ ...selected, available: !unavailable, failureCode: unavailable ? 'missing' : '' }] : [track('b')], total: 50, page, pageSize: 25 })
    }
    return ok(config())
  }
  const first = workbench({ drafts, io }); t.after(first.dispose); await first.mount()
  first.query.page = 2; await flush()
  first.selectedIDs.value = new Set(['cue']); first.addSelected()
  unavailable = true; await first.loadLibrary()
  assert.equal(drafts.get('music-workbench').playlist.find(item => item.trackID === 'cue').available, false, 'metadata-only refresh is remembered')
  first.query.page = 1; await flush(); first.dispose()
  const second = workbench({ drafts, io }); t.after(second.dispose)
  assert.equal(second.playlist.value.find(item => item.trackID === 'cue').title, selected.title)
  await second.mount()
  const restored = second.playlist.value.find(item => item.trackID === 'cue')
  assert.equal(restored.title, selected.title); assert.equal(restored.coverURL, selected.coverURL)
  assert.equal(restored.cueTrackNumber, 3); assert.equal(restored.available, false); assert.equal(restored.failureCode, 'missing')
  assert.equal('relativePath' in restored, false); assert.equal('arbitrary' in restored, false)
  assert.equal(JSON.stringify([...drafts]).includes('/private/'), false)
  assert.deepEqual(plain(second.draft.value.trackIDs), ['a', 'cue']); assert.equal(second.draft.value.version, 7)
  await second.save()
  const payload = JSON.parse(second.writes()[0].init.body)
  assert.deepEqual(Object.keys(payload).sort(), ['frontendSettings', 'scanIntervalMinutes', 'trackIDs', 'version'])
  assert.deepEqual(payload.trackIDs, ['a', 'cue']); assert.equal(second.conflict.value, false)
  assert.equal(drafts.get('music-workbench').conflict, false)
})

test('legacy drafts without cached metadata do not label an unknown selected track as unavailable', async t => {
  const first = workbench(); t.after(first.dispose); await first.mount(); first.addTrack(track('off-page'))
  const stored = plain(first.drafts.get('music-workbench'))
  delete stored.playlist; delete stored.conflict
  const second = workbench({ drafts: new Map([['music-workbench', stored]]) }); t.after(second.dispose); await second.mount()
  const unknown = second.playlist.value.find(item => item.trackID === 'off-page')
  assert.ok(unknown); assert.equal(unknown.title, '歌曲信息待加载'); assert.equal(unknown.available, true)
  assert.equal(second.conflict.value, false); assert.deepEqual(plain(second.draft.value.trackIDs), ['a', 'off-page'])
})

test('account changes and view revocation remove persisted conflict and cross-page metadata', async t => {
  for (const cleanup of ['account', 'view']) {
    const panel = workbench({ io: async call => call.method === 'PUT' ? ok(null, 409) : call.url.pathname.endsWith('/library') ? ok({ items: [], total: 0, page: 1, pageSize: 25 }) : ok(config()) })
    t.after(panel.dispose); await panel.mount(); panel.addTrack(track('private-cue', { cueTrackNumber: 2 })); await panel.save()
    assert.equal(panel.drafts.get('music-workbench').conflict, true)
    assert.ok(panel.drafts.get('music-workbench').playlist.some(item => item.trackID === 'private-cue'))
    if (cleanup === 'account') panel.account.value = 'account-b'; else panel.canView.value = false
    assert.equal(panel.drafts.size, 0); assert.equal(panel.conflict.value, false); assert.equal(panel.playlist.value.length, 0)
    await flush()
    assert.equal(JSON.stringify([...panel.drafts]).includes('private-cue'), false)
    panel.dispose()
    const next = workbench({ drafts: panel.drafts, account: panel.account, canView: panel.canView }); t.after(next.dispose); await next.mount()
    assert.equal(next.conflict.value, false)
    assert.equal(next.playlist.value.some(item => item.trackID === 'private-cue'), false)
  }
})

test('identical metadata is not a draft change; completed scan refresh preserves unsaved order and baseline version', async t => {
  let current = config()
  const panel = workbench({ io: async call => {
    if (call.url.pathname.endsWith('/library')) return ok({ items: [track('a'), track('b')], total: 2, page: 1, pageSize: 25 })
    if (call.url.pathname.endsWith('/refresh-status')) return ok({ state: 'succeeded', runID: 'scan-fixture' })
    if (call.method === 'POST') return ok({ state: 'running', runID: 'scan-fixture', started: true }, 202)
    return ok(plain(current))
  } })
  t.after(panel.dispose); await panel.mount(); await panel.loadLibrary()
  assert.equal(panel.dirty.value, false)
  panel.addTrack(track('b')); panel.moveTrack('b', 0)
  await panel.refreshScan(); await panel.refreshScan()
  assert.equal(panel.writes().length, 1); assert.equal(panel.timers.size, 1)
  current = config({ version: 10, playlist: [track('a', { title: 'scan metadata' })] })
  await panel.advance(2000)
  assert.equal(panel.config.value.version, 10); assert.equal(panel.draft.value.version, 7)
  assert.deepEqual(plain(panel.draft.value.trackIDs), ['b', 'a']); assert.equal(panel.dirty.value, true)
  assert.equal(panel.config.value.scan.state, 'succeeded'); assert.equal(panel.timers.size, 0)
})

test('failed read and write retain edits without a success timestamp or event', async t => {
  let fail = false
  const panel = workbench({ io: async call => fail ? ok(null, 503) : call.url.pathname.endsWith('/library') ? ok({ items: [], total: 0, page: 1, pageSize: 25 }) : ok(config()) })
  t.after(panel.dispose); await panel.mount(); panel.addTrack(track('b')); fail = true
  let notifications = 0; panel.window.addEventListener('frontend-config-updated', () => notifications++)
  await panel.loadConfig(); await panel.save()
  assert.deepEqual(plain(panel.draft.value.trackIDs), ['a', 'b']); assert.equal(panel.draft.value.version, 7)
  assert.equal(panel.ready.value, true); assert.equal(panel.savedAt.value, ''); assert.equal(notifications, 0)
  assert.equal(panel.dirty.value, true); assert.match(panel.error.value, /不可用/)
})

test('view-only can read, search, paginate and retry; every mutation handler independently refuses writes', async t => {
  const panel = workbench({ canManage: vue.ref(false) }); t.after(panel.dispose); await panel.mount()
  const before = plain(panel.draft.value)
  panel.addTrack(track('b')); panel.selectedIDs.value = new Set(['b']); panel.addSelected()
  panel.removeTrack('a'); panel.moveTrack('a', 0); await panel.refreshScan(); await panel.save()
  assert.deepEqual(plain(panel.draft.value), before); assert.equal(panel.writes().length, 0)
  panel.query.page = 2; await flush(); panel.search.value = '艺术家'; await flush(); await panel.advance(300)
  assert.equal(panel.libraries().at(-1).url.searchParams.get('q'), '艺术家')
  await panel.loadLibrary(); assert.equal(panel.library.value.items.length, 3)
})

test('superseded config/library reads abort and ignore late responses even when the transport ignores abort', async t => {
  const requests = []
  const panel = workbench({ io: call => { const waiting = deferred(); requests.push({ ...waiting, call }); return waiting.promise } })
  t.after(panel.dispose); await panel.mount()
  const first = requests[0]; const newer = panel.loadConfig(); await flush()
  assert.equal(first.call.init.signal.aborted, true)
  requests[2].resolve(ok(config({ version: 8 }))); await newer
  first.resolve(ok(config({ version: 1 }))); await flush()
  assert.equal(panel.draft.value.version, 8)
  const oldLibrary = requests[1]; const library = panel.loadLibrary(); await flush()
  assert.equal(oldLibrary.call.init.signal.aborted, true)
  requests[3].resolve(ok({ items: [track('latest')], total: 1, page: 1, pageSize: 25 })); await library
  oldLibrary.resolve(ok({ items: [track('obsolete')], total: 1, page: 1, pageSize: 25 })); await flush()
  assert.equal(panel.library.value.items[0].trackID, 'latest')
})

test('deactivation cancels scan polling, debounce and in-flight reads; reactivation reads once and keeps drafts', async t => {
  let pending
  const panel = workbench({ config: { scan: { state: 'running' } }, io: async call => {
    if (call.url.pathname.endsWith('/library')) {
      if (pending) return pending.promise
      return ok({ items: [track('b')], total: 1, page: 1, pageSize: 25 })
    }
    return ok(config({ scan: { state: 'running' } }))
  } })
  t.after(panel.dispose); await panel.mount(); panel.addTrack(track('b'))
  pending = deferred(); const loading = panel.loadLibrary(); panel.search.value = 'pending'; await flush()
  const call = panel.libraries().at(-1); await panel.leave()
  assert.equal(call.init.signal.aborted, true); assert.equal(panel.timers.size, 0)
  pending.resolve(ok({ items: [track('late')], total: 1, page: 1, pageSize: 25 })); await loading
  assert.notEqual(panel.library.value.items[0].trackID, 'late')
  const count = panel.calls.length; await panel.advance(60000); assert.equal(panel.calls.length, count)
  pending = null; await panel.enter(); await panel.enter()
  assert.equal(panel.calls.length, count + 2); assert.deepEqual(plain(panel.draft.value.trackIDs), ['a', 'b'])
  assert.equal(panel.draft.value.version, 7)
})

test('account change and view revocation clear private rows/drafts and prevent old responses from returning', async t => {
  let pending = null
  const panel = workbench({ io: async call => pending ? pending.promise : call.url.pathname.endsWith('/library') ? ok({ items: [track('b')], total: 1, page: 1, pageSize: 25 }) : ok(config()) })
  t.after(panel.dispose); await panel.mount(); panel.addTrack(track('b')); await flush()
  assert.equal(panel.drafts.size, 1)
  pending = deferred(); const old = panel.loadConfig(); const oldCall = panel.calls.at(-1)
  panel.canView.value = false
  assert.equal(oldCall.init.signal.aborted, true); assert.equal(panel.ready.value, false)
  assert.equal(panel.config.value, null); assert.equal(panel.playlist.value.length, 0); assert.equal(panel.library.value.items.length, 0); assert.equal(panel.drafts.size, 0)
  pending.resolve(ok(config({ version: 99 }))); await old
  assert.equal(panel.config.value, null); assert.equal(panel.timers.size, 0)
  pending = null; panel.account.value = 'account-b'; panel.canView.value = true; await flush()
  assert.deepEqual(plain(panel.draft.value.trackIDs), ['a']); assert.equal(panel.dirty.value, false)
})

test('403 refreshes capabilities and clears private state after music.view is revoked', async t => {
  let denied = false, refreshes = 0
  const canView = vue.ref(true)
  const panel = workbench({ canView, refreshCapabilities: async () => { refreshes++; canView.value = false }, io: async call => denied ? ok(null, 403) : call.url.pathname.endsWith('/library') ? ok({ items: [track('b')], total: 1, page: 1, pageSize: 25 }) : ok(config()) })
  t.after(panel.dispose); await panel.mount(); panel.addTrack(track('b')); denied = true; await panel.save()
  assert.equal(refreshes, 1); assert.equal(panel.ready.value, false)
  assert.equal(panel.config.value, null); assert.equal(panel.playlist.value.length, 0); assert.equal(panel.drafts.size, 0)
})

test('CUE numbers and literal tags survive metadata refresh; paths and arbitrary fields never enter drafts', async t => {
  const title = '<img src=x onerror="window.tagExecuted=true"> & 中文'
  const panel = workbench({ items: [track('cue', { title, artist: '<script>bad()</script>', album: 'A & B', cueTrackNumber: 2, relativePath: '/private/music.flac' }), track('invalid-cue', { cueTrackNumber: -1 })] })
  t.after(panel.dispose); await panel.mount()
  assert.equal(panel.library.value.items[0].title, title)
  assert.equal(panel.library.value.items[0].cueTrackNumber, 2)
  assert.equal(panel.library.value.items[1].cueTrackNumber, 0)
  assert.equal('relativePath' in panel.library.value.items[0], false)
  panel.addTrack(panel.library.value.items[0]); await panel.loadLibrary()
  assert.equal(panel.playlist.value[1].cueTrackNumber, 2)
  assert.equal(panel.playlist.value[1].album, 'A & B')
  assert.deepEqual(plain(panel.draft.value.trackIDs), ['a', 'cue'])
  assert.equal(panel.draft.value.version, 7)
  assert.equal(JSON.stringify([...panel.drafts.values()]).includes('/private/'), false)
})

test('management revocation aborts writes and refresh, clears checks and rejects late replies after regrant', async t => {
  for (const operation of ['save', 'refreshScan']) {
    const pending = deferred()
    const panel = workbench({ io: async call => call.method !== 'GET' ? pending.promise : call.url.pathname.endsWith('/library') ? ok({ items: [track('b')], total: 1, page: 1, pageSize: 25 }) : ok(config()) })
    t.after(panel.dispose); await panel.mount(); panel.addTrack(track('b'))
    panel.selectedIDs.value = new Set(['b'])
    let events = 0; panel.window.addEventListener('frontend-config-updated', () => events++)
    const writing = panel[operation](); const call = panel.writes()[0]
    panel.canManage.value = false
    assert.equal(call.init.signal.aborted, true)
    assert.equal(panel.selectedIDs.value.size, 0)
    assert.equal(panel.saving.value, false); assert.equal(panel.refreshing.value, false)
    panel.canManage.value = true
    pending.resolve(ok(operation === 'save' ? config({ version: 99 }) : { state: 'running', runID: 'obsolete' }))
    await writing; await flush()
    assert.equal(panel.draft.value.version, 7)
    assert.deepEqual(plain(panel.draft.value.trackIDs), ['a', 'b'])
    assert.equal(panel.config.value.scan.state, 'succeeded')
    assert.equal(panel.savedAt.value, ''); assert.equal(events, 0); assert.equal(panel.timers.size, 0)
  }
})

test('production cover setup permits only same-origin admin cover endpoints and resets failed images', () => {
  const props = vue.reactive({ url: '/fixture/api/music/library/abc-123/cover', baseApi: '/fixture/api/' })
  const source = readFileSync(new URL('../components/admin/sections/MusicTrackCover.vue', import.meta.url), 'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
  const code = ts.transpileModule(source.replace(/^import .*$/gm, '') + '\nmodule.exports = { safeURL, failed }', { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText
  const scope = vue.effectScope(); const module = { exports: {} }
  const context = vm.createContext({ ...vue, watch: (getter, callback) => vue.watch(getter, callback, { flush: 'sync' }), defineProps: () => props, module, URL, window: { location: { origin: 'https://fixture.invalid' } } })
  try {
    scope.run(() => vm.runInContext(code, context))
    const { safeURL, failed } = module.exports
    assert.equal(safeURL.value, 'https://fixture.invalid/fixture/api/music/library/abc-123/cover')
    for (const value of ['https://evil.invalid/fixture/api/music/library/a/cover', '//evil.invalid/a', 'javascript:alert(1)', 'data:image/svg+xml,<svg/>', '/fixture/api/music/cover/a', '/fixture/api/music/library/a/cover?token=x', '/fixture/api/music/library/a/cover#x', '/fixture/api/music/library/%2e%2e/cover', 'https://u:p@fixture.invalid/fixture/api/music/library/a/cover']) {
      props.url = value; assert.equal(safeURL.value, '', value)
    }
    failed.value = true; props.url = '/fixture/api/music/library/new-id/cover'
    assert.equal(failed.value, false)
    props.baseApi = 'https://evil.invalid/fixture/api'
    assert.equal(safeURL.value, '')
  } finally { scope.stop() }
})

const publicResponse = () => ({ code: 1, data: { source: 'local', revision: 'fixture-1', frontendSettings: { musicSource: 'local', musicEnabled: true, musicPosition: 'bottom-left', musicTheme: 'auto', musicLyric: true, musicAutoplay: false, musicDefaultMinimized: true, musicEmbed: false, musicHideOnMobile: false, musicPlaylistId: 'private-online-id' }, tracks: [{ ...track('cue', { title: '<b>literal tag</b>', cueTrackNumber: 2 }), mimeType: 'audio/mpeg', lyricsURL: '/fixture/api/music/lyrics/cue', streamURL: '/fixture/api/music/stream/cue', fallbackStreamURL: '/fixture/api/music/stream/cue?compat=1', mediaVersion: 'segment-2', relativePath: '/private/file.flac' }] } })

test('public local projection keeps literal metadata and segment representation, drops private fields and clears on failure', async t => {
  let fail = false; const calls = []
  const harness = runtime('../composables/usePublicMusic.ts', { $fetch: async (url, init) => { calls.push({ url, init }); if (fail) throw new Error('fixture unavailable'); return publicResponse() } })
  const subject = harness.scope.run(() => harness.exports.usePublicMusic())
  t.after(harness.dispose); await harness.lifecycle('mounted')
  assert.equal(subject.state.value.tracks[0].title, '<b>literal tag</b>')
  assert.equal(subject.state.value.tracks[0].mimeType, 'audio/mpeg')
  assert.equal(subject.state.value.tracks[0].mediaVersion, 'segment-2')
  assert.equal('relativePath' in subject.state.value.tracks[0], false)
  assert.equal('musicPlaylistId' in subject.state.value.frontendSettings, false)
  assert.equal(harness.timers.size, 1)
  harness.document.visibilityState = 'hidden'; harness.document.dispatchEvent(new Event('visibilitychange'))
  const count = calls.length; await harness.advance(60000)
  assert.equal(calls.length, count); assert.equal(harness.timers.size, 0)
  fail = true; harness.document.visibilityState = 'visible'; harness.document.dispatchEvent(new Event('visibilitychange')); await flush()
  assert.equal(subject.state.value, null); assert.match(subject.error.value, /不可用/)
  await harness.lifecycle('unmounted'); const stopped = calls.length
  harness.window.dispatchEvent(new Event('focus')); await harness.advance(60000)
  assert.equal(calls.length, stopped); assert.equal(harness.timers.size, 0)
})

test('startup tool checking refreshes until ready without rescan or rebasing a draft', async t => {
  let checked = false
  const panel = workbench({ io: async call => {
    if (call.url.pathname.endsWith('/library')) return ok({ items: [track('b')], total: 1, page: 1, pageSize: 25 })
    if (call.url.pathname.endsWith('/refresh-status')) return ok({ state: 'succeeded' })
    return ok(config({ toolsReady: checked, toolsStatus: checked ? 'ready' : 'checking' }))
  } })
  t.after(panel.dispose)
  await panel.mount()
  assert.equal(panel.config.value.toolsStatus, 'checking')
  panel.addTrack(track('b'))
  checked = true
  await panel.advance(2000)
  assert.equal(panel.config.value.toolsStatus, 'ready')
  assert.equal(panel.config.value.toolsReady, true)
  assert.deepEqual(plain(panel.draft.value.trackIDs), ['a', 'b'])
  assert.equal(panel.draft.value.version, 7)
  assert.equal(panel.writes().length, 0)
  assert.equal(panel.timers.size, 0)
})
