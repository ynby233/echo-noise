import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import test from 'node:test'
import ts from 'typescript'
import * as vue from 'vue'

const directory = new URL('../components/admin/sections/', import.meta.url)
const deferred = () => {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); await vue.nextTick() }
const plain = value => JSON.parse(JSON.stringify(value))

// Execute the actual SFC setup and composable, with only I/O and lifecycle controlled.
function component(name, expose, options = {}) {
  const hooks = { mounted: [], activated: [], deactivated: [], unmounted: [] }
  const timers = new Map()
  const timerDelays = new Map()
  const schedule = (fn, delay) => { timers.set(++timerID, fn); timerDelays.set(timerID, delay); return timerID }
  const unschedule = id => { timers.delete(id); timerDelays.delete(id) }
  let timerID = 0
  const events = new EventTarget()
  const account = options.account || vue.ref('1')
  const drafts = options.drafts || new Map()
  const props = vue.reactive({ theme: { text: 'light', mutedText: 'muted' }, adminShellCardClass: ['light'], adminPanelCardClass: ['light'], adminSubtleCardClass: ['light'] })
  const scope = vue.effectScope()
  const sandbox = vm.createContext({
    ...vue, console, exports: {}, Event, AbortSignal, AbortController, URL, URLSearchParams, Date, Intl,
    defineProps: () => props,
    defineEmits: () => () => {},
    useToast: () => ({ add() {} }),
    useRuntimeConfig: () => ({ public: { baseApi: '/api' } }),
    useAdminCapabilities: () => ({ isPrimaryAdmin: vue.ref(true), can: () => true, refreshCapabilities: async () => {} }),
    useUserStore: () => ({ get user() { return { userid: account.value } } }),
    localStorage: { getItem: () => null },
    inject: key => key.description === 'admin drafts' ? drafts : account,
    onMounted: fn => hooks.mounted.push(fn),
    onActivated: fn => hooks.activated.push(fn),
    onDeactivated: fn => hooks.deactivated.push(fn),
    onUnmounted: fn => hooks.unmounted.push(fn),
    window: Object.assign(events, {
      setTimeout: schedule,
      clearTimeout: unschedule,
      confirm: () => true,
    }),
    setTimeout: schedule,
    setInterval: schedule,
    clearInterval: unschedule,
    clearTimeout: unschedule,
    resolveManagedAttachmentURL: (_base, value) => value,
    resolveUploadedMediaUrl: value => value,
    booleanSetting: (value, fallback = false) => value == null ? fallback : [true, 'true', 1, '1'].includes(value),
    loadFrontendSettings: async () => ({ frontendSettings: {}, raw: {} }),
    saveFrontendSettings: async () => {},
    ...options.io,
  })
  const run = (source, filename) => {
    const code = ts.transpileModule(source.replace(/^import .*$/gm, ''), {
      compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
    }).outputText
    scope.run(() => vm.runInContext(`{\n${code}\n}`, sandbox, { filename }))
  }
  run(readFileSync(new URL('config-draft.ts', directory), 'utf8'), 'config-draft.ts')
  sandbox.useConfigDraft = sandbox.exports.useConfigDraft
  sandbox.adminDraftsKey = sandbox.exports.adminDraftsKey
  sandbox.adminDraftAccountKey = sandbox.exports.adminDraftAccountKey
  if (name === 'MusicSection') {
    run(readFileSync(new URL('use-music-workbench.ts', directory), 'utf8'), 'use-music-workbench.ts')
    sandbox.useMusicWorkbench = sandbox.exports.useMusicWorkbench
    sandbox.conflictMessage = sandbox.exports.conflictMessage
  }
  const source = readFileSync(new URL(`${name}.vue`, directory), 'utf8').split('<script setup lang="ts">')[1].split('</script>')[0]
  // A block avoids colliding with the composable module's local variables.
  run(`{\n${source}\nglobalThis.subject = { ${expose} }\n}`, `${name}.vue`)
  return {
    ...sandbox.subject, props, timers, timerDelays, drafts, account,
    fireTimer: async id => { const callback = timers.get(id); unschedule(id); await callback(); await flush() },
    mount: async () => { for (const hook of hooks.mounted) await hook(); await flush() },
    activate: async () => { for (const hook of hooks.activated) await hook(); await flush() },
    deactivate: async () => { for (const hook of hooks.deactivated) await hook(); await flush() },
    dispose: () => { for (const hook of hooks.unmounted) hook(); scope.stop() },
  }
}

test('feed unlimited and numeric limits pass through the actual component save and reload', async () => {
  let server = { feedLimit: 0 }
  const writes = []
  const panel = component('FeedSection', 'form, load, save, ready, error', { io: {
    loadFrontendSettings: async () => ({ frontendSettings: server }),
    saveFrontendSettings: async fields => { writes.push(plain(fields)); server = { ...server, ...plain(fields) } },
  } })
  await panel.save()
  assert.equal(writes.length, 0, 'uninitialized default values must never be saved')
  await panel.load()
  assert.equal(panel.form.feedLimit, '')
  panel.form.feedPageTitle = 'changed title'
  await panel.save()
  assert.equal(writes.at(-1).feedLimit, 0)
  assert.equal(panel.form.feedLimit, '')
  for (const value of [1, 100, '']) {
    panel.form.feedLimit = value
    await panel.save()
    assert.equal(writes.at(-1).feedLimit, value === '' ? 0 : value)
    assert.equal(server.feedLimit, value === '' ? 0 : value)
  }
  panel.dispose()
})

test('initialization failure blocks writes, retry recovers, and failed reload preserves edits', async () => {
  let fail = true
  let writes = 0
  const panel = component('FeedSection', 'form, load, save, ready, error', { io: {
    loadFrontendSettings: async () => { if (fail) throw new Error('503 unavailable'); return { frontendSettings: { feedLimit: 1 } } },
    saveFrontendSettings: async () => { writes++ },
  } })
  await panel.load()
  assert.equal(panel.ready.value, false)
  assert.match(panel.error.value, /503/)
  await panel.save()
  assert.equal(writes, 0)
  fail = false
  await panel.load()
  assert.equal(panel.ready.value, true)
  panel.form.feedPageTitle = 'unsaved'
  fail = true
  await panel.load()
  assert.equal(panel.form.feedPageTitle, 'unsaved')
  fail = false
  await panel.save()
  assert.equal(writes, 1)
  panel.dispose()
})

test('evicted feed restores source rows and drafts; theme follows replacement props', async () => {
  const drafts = new Map()
  const io = { loadFrontendSettings: async () => ({ frontendSettings: { feedLimit: 100, feedSources: [] } }) }
  const first = component('FeedSection', 'form, sources, groupDraft, theme, adminPanelCardClass, load', { drafts, io })
  await first.load()
  first.form.feedPageTitle = 'unsaved title'
  first.sources.value.push({ type: 'rss', group: 'draft', name: '', url: '', enabled: true, visible: true })
  first.groupDraft.value = 'next group'
  await first.deactivate()
  first.dispose()
  const next = component('FeedSection', 'form, sources, groupDraft, theme, adminPanelCardClass, load', { drafts, io })
  await next.load()
  assert.equal(next.form.feedPageTitle, 'unsaved title')
  assert.equal(next.sources.value[0].group, 'draft')
  assert.equal(next.groupDraft.value, 'next group')
  next.props.theme = { text: 'dark' }
  next.props.adminPanelCardClass = ['dark']
  assert.equal(next.theme.value.text, 'dark')
  assert.deepEqual(plain(next.adminPanelCardClass.value), ['dark'])
  next.dispose()
})

test('saving one site-info field preserves a different unsaved field and reloads the saved value', async () => {
  let server = { siteTitle: 'old title', subtitleText: 'old subtitle' }
  const panel = component('SiteInfoSection', 'form, load, save', { io: {
    loadFrontendSettings: async () => ({ frontendSettings: server }),
    saveFrontendSettings: async fields => { server = { ...server, ...plain(fields) } },
  } })
  await panel.load()
  panel.form.siteTitle = 'new title'
  panel.form.subtitleText = 'unsaved subtitle'
  await panel.save('siteTitle')
  assert.equal(server.siteTitle, 'new title')
  assert.equal(server.subtitleText, 'old subtitle')
  assert.equal(panel.form.subtitleText, 'unsaved subtitle')
  panel.dispose()
})

test('late initialization is ignored after newer response, eviction, or account change', async () => {
  const pending = [deferred(), deferred(), deferred()]
  let index = 0
  const panel = component('FeedSection', 'form, load, ready', { io: {
    loadFrontendSettings: () => pending[index++].promise,
  } })
  const old = panel.load(), current = panel.load()
  pending[1].resolve({ frontendSettings: { feedPageTitle: 'current' } })
  await current
  pending[0].resolve({ frontendSettings: { feedPageTitle: 'obsolete' } })
  await old
  assert.equal(panel.form.feedPageTitle, 'current')
  const last = panel.load()
  panel.account.value = '2'
  panel.drafts.clear()
  panel.dispose()
  pending[2].resolve({ frontendSettings: { feedPageTitle: 'wrong account' } })
  await last
  assert.equal(panel.form.feedPageTitle, 'current')
  assert.equal(panel.drafts.size, 0)
})

test('registration late running response never restarts polling after deactivation or eviction', async () => {
  let request = deferred()
  const panel = component('RegistrationSection', 'loadRuntime, runtime', { io: { getRequest: () => request.promise } })
  const loading = panel.loadRuntime()
  await panel.deactivate()
  request.resolve({ code: 1, data: { provisioning_run: { status: 'running' } } })
  await loading
  assert.equal(panel.timers.size, 0)
  assert.equal(panel.runtime.runStatus, '')
  request = deferred()
  await panel.activate()
  request.resolve({ code: 1, data: { provisioning_run: { status: 'running' } } })
  await flush()
  assert.equal(panel.timers.size, 1)
  request = deferred()
  const last = panel.loadRuntime()
  panel.dispose()
  request.resolve({ code: 1, data: { provisioning_run: { status: 'completed' } } })
  await last
  assert.equal(panel.timers.size, 0)
  assert.equal(panel.runtime.runStatus, 'running')
})

test('storage initialization and poll response cannot restart a hidden or disposed panel', async () => {
  let request = deferred()
  const panel = component('StorageSection', 'load, refreshLastSyncOnly, lastCloudSyncText, storageEnabled', { io: { fetch: () => request.promise } })
  const loading = panel.load()
  await panel.deactivate()
  request.resolve({ ok: true, json: async () => ({ code: 1, data: { storageEnabled: true, storageConfig: { autoSyncEnabled: true, syncRole: 'primary' } } }) })
  await loading
  await flush()
  assert.equal(panel.timers.size, 0)
  await panel.activate()
  assert.equal(panel.timers.size, 1)
  request = deferred()
  const poll = panel.refreshLastSyncOnly()
  panel.dispose()
  request.resolve({ ok: true, json: async () => ({ code: 1, data: { storageConfig: { lastSyncTime: '2026-09-09T01:00:00Z' } } }) })
  await poll
  assert.equal(panel.lastCloudSyncText.value, '')
  assert.equal(panel.timers.size, 0)
})

test('storage save rereads configured flags and clears submitted secrets without losing the other draft', async () => {
  let stored = { storageConfig: {}, attachmentStorageConfig: {} }
  const writes = []
  const panel = component('StorageSection', 'load, saveAttachmentStorageConfig, saveStorageConfig, storageConfig, attachmentStorageConfig', { io: {
    fetch: async (_url, options) => {
      if (options.method === 'PUT') {
        const body = JSON.parse(options.body)
        writes.push(body)
        if (body.attachmentStorageConfig) stored.attachmentStorageConfig = { accessKeyConfigured: true, secretKeyConfigured: true }
        if (body.storageConfig) stored.storageConfig = { accessKeyConfigured: true, secretKeyConfigured: true }
      }
      return { ok: true, json: async () => ({ code: 1, data: stored }) }
    },
  } })
  await panel.load()
  panel.storageConfig.bucket = 'unsaved database bucket'
  panel.attachmentStorageConfig.accessKey = 'synthetic-access'
  panel.attachmentStorageConfig.secretKey = 'synthetic-secret'
  await panel.saveAttachmentStorageConfig()
  assert.equal(writes.length, 1)
  assert.equal(writes[0].attachmentStorageConfig.secretKey, 'synthetic-secret')
  assert.equal(panel.attachmentStorageConfig.accessKeyConfigured, true)
  assert.equal(panel.attachmentStorageConfig.secretKeyConfigured, true)
  assert.equal(panel.attachmentStorageConfig.secretKey, '')
  assert.equal(panel.storageConfig.bucket, 'unsaved database bucket')
  panel.storageConfig.secretKey = 'synthetic-secret'
  await panel.saveStorageConfig()
  assert.equal(panel.storageConfig.secretKeyConfigured, true)
  assert.equal(panel.storageConfig.secretKey, '')
  panel.dispose()
})

test('music custom CDN draft and preset selection survive eviction together and save through music API', async () => {
  const drafts = new Map()
  const calls = []
  let stored = {
    version: 1, frontendSettings: { musicCssCdnURL: 'https://cdn.example/music.css', musicJsCdnURL: 'https://cdn.example/music.js' },
    scanIntervalMinutes: 60, playlist: [], scan: { state: 'idle' }, rootReadable: true, toolsReady: true,
    counts: { total: 0, available: 0, unavailable: 0 },
  }
  const io = {
    loadFrontendSettings: async () => { throw new Error('music must use its dedicated API') },
    saveFrontendSettings: async () => { throw new Error('music must use its dedicated API') },
    fetch: async (url, init) => {
      calls.push({ url, init })
      assert.equal(init.credentials, 'include')
      assert.ok(init.signal instanceof AbortSignal)
      let data
      if (url === '/api/music/config') {
        if (init.method === 'PUT') {
          const submitted = JSON.parse(init.body)
          assert.equal(init.headers['Content-Type'], 'application/json')
          assert.equal(submitted.version, stored.version)
          assert.deepEqual(submitted.trackIDs, [])
          stored = { ...stored, ...submitted, version: stored.version + 1 }
        } else assert.equal(init.method || 'GET', 'GET')
        data = plain(stored)
      } else {
        assert.ok(url.startsWith('/api/music/library?'), `unexpected music endpoint: ${url}`)
        assert.equal(init.method || 'GET', 'GET')
        data = { items: [], total: 0, page: 1, pageSize: 25 }
      }
      return { ok: true, status: 200, json: async () => ({ code: 1, data }) }
    },
  }
  const expose = 'draft, cdnPreset, loadConfig, save, ready, dirty'
  const first = component('MusicSection', expose, { drafts, io })
  await first.activate()
  assert.equal(first.ready.value, true)
  assert.equal(first.cdnPreset.value, 'custom')
  first.draft.value.frontendSettings.musicJsCdnURL = 'https://custom.example/player.js'
  first.draft.value.frontendSettings.musicCssCdnURL = 'https://custom.example/player.css'
  await flush()
  assert.equal(first.dirty.value, true)
  first.dispose()
  const custom = component('MusicSection', expose, { drafts, io })
  await custom.activate()
  assert.equal(custom.cdnPreset.value, 'custom')
  assert.equal(custom.draft.value.frontendSettings.musicJsCdnURL, 'https://custom.example/player.js')
  assert.equal(custom.draft.value.frontendSettings.musicCssCdnURL, 'https://custom.example/player.css')
  custom.cdnPreset.value = 'jsdelivr'
  await flush()
  const expected = plain(custom.draft.value.frontendSettings)
  assert.match(expected.musicJsCdnURL, /npm\/netease-mini-player@2\.0\.4/)
  assert.match(expected.musicCssCdnURL, /npm\/netease-mini-player@2\.0\.4/)
  custom.dispose()
  const next = component('MusicSection', expose, { drafts, io })
  await next.activate()
  assert.equal(next.cdnPreset.value, 'jsdelivr')
  assert.deepEqual(plain(next.draft.value.frontendSettings), expected)
  await next.save()
  await flush()
  assert.equal(calls.filter(call => call.init.method === 'PUT').length, 1)
  assert.deepEqual(stored.frontendSettings, expected)
  assert.equal(next.draft.value.version, 2)
  assert.equal(next.dirty.value, false)
  next.dispose()
  const saved = component('MusicSection', expose, { drafts, io })
  await saved.activate()
  assert.equal(saved.cdnPreset.value, 'jsdelivr')
  assert.deepEqual(plain(saved.draft.value.frontendSettings), expected)
  assert.equal(saved.dirty.value, false)
  saved.dispose()
})

test('async dashboard refreshes on its first return even without an initial activated callback', async () => {
  let enabled = true
  const panel = component('DashboardSection', 'loadDashboard, registerEnabled', { io: {
    useUserStore: () => ({ isLogin: true, user: {}, status: {}, getStatus: async () => {} }),
    resolveAdminDashboardPresentation: () => ({ operationCards: [] }),
    fetch: async () => ({ json: async () => ({ code: 1, data: { allowRegistration: enabled } }) }),
  } })
  await panel.loadDashboard()
  assert.equal(panel.registerEnabled.value, true)
  await panel.deactivate()
  enabled = false
  await panel.activate()
  assert.equal(panel.registerEnabled.value, false)
  panel.dispose()
})

const versionIntent = { channel: 'edge', revision: 'a'.repeat(40), digest: `sha256:${'b'.repeat(64)}` }
const versionNow = Date.parse('2026-10-11T01:00:00Z')
const versionRequestedAt = new Date(versionNow).toISOString()
const versionAvailable = { available: true, reason: '' }
const versionExpired = { available: false, reason: 'executor_offline' }
function versionPanel(options = {}) {
  let now = versionNow
  class Clock extends Date {
    constructor(...args) { super(...(args.length ? args : [now])) }
    static now() { return now }
  }
  const posts = [], gets = []
  let state = {
    executor: { id: 7, checked_at: new Date(now - 240000).toISOString(), check_ok: true },
    installation: { ...versionExpired },
    preparation: { credential_id: 7 },
    task: null,
    ...options.state,
  }
  const pendingReply = (wake_status = 'sent') => ({ code: 1, data: { credential_id: 7, requested_at: versionRequestedAt, installation: { ...versionExpired }, wake_status } })
  const panel = component('VersionSection', 'canInstall, canPrepare, confirmInstall, install, prepareInstallation, cancelPreparation, refresh, poll, applyState, channels, selected, target, draftConfirmed, installation, executor, task, busy, preparing, posting, preparationAttempt, pendingInstall, preparationError, preparationHint, uncertain, error, rejection', {
    account: options.account,
    io: {
      Date: Clock,
      getRequest: async (path, _params, requestOptions) => {
        gets.push({ path, options: requestOptions })
        if (options.get) return options.get(path)
        if (path === 'updates/state') return { code: 1, data: plain(state) }
        if (path === 'updates') return { code: 1, data: { report: { channels: [] }, follow_channel: 'stable' } }
        return { code: 1, data: { isContainer: true } }
      },
      postRequest: async (path, body, requestOptions) => {
        posts.push({ path, body: plain(body), options: requestOptions })
        if (options.post) return options.post(path, body)
        if (path === 'updates/prepare') return pendingReply()
        state.task = { id: 'created', status: 'pending', target_revision: body.revision }
        return { code: 1, data: state.task }
      },
      ...options.io,
    },
  })
  panel.applyState(plain(state))
  panel.channels.value = [{ name: 'edge', ...versionIntent, installable: true, status: 'update_available' }]
  return Object.assign(panel, {
    posts, gets, pendingReply,
    setNow: value => { now = value },
    setState: patch => { state = { ...state, ...patch } },
    report: async patch => { state = { ...state, ...patch }; await panel.refresh(); await flush() },
    confirm: () => { panel.confirmInstall('edge'); panel.draftConfirmed.value = true },
    taskPosts: () => posts.filter(request => request.path === 'updates/tasks'),
  })
}

test('version permits expired checks but requires draft confirmation before any prepare or task request', async () => {
  const panel = versionPanel()
  assert.equal(panel.canInstall('edge'), true)
  panel.confirmInstall('edge')
  assert.equal(panel.selected.value, 'edge')
  await panel.install()
  assert.equal(panel.posts.length, 0)
  assert.equal(panel.preparing.value, false)
  panel.draftConfirmed.value = true
  await panel.install()
  assert.deepEqual(panel.posts.map(request => [request.path, request.body]), [['updates/prepare', {}]])
  assert.equal(panel.busy.value, true)
  assert.equal(panel.posting.value, false)
  assert.equal(panel.taskPosts().length, 0)
  for (const reason of ['credential_expired', 'platform_unsupported', 'database_unsupported', 'instance_mismatch', 'executor_upgrade_required', 'executor_unconfigured']) {
    panel.cancelPreparation()
    panel.installation.value = { available: false, reason }
    assert.equal(panel.canInstall('edge'), false)
    assert.equal(panel.canPrepare.value, false)
  }
  panel.dispose()
})

test('version ignores state until prepare replies, waits for matching new check and posts captured target only once', async () => {
  const preparation = deferred()
  const panel = versionPanel({ post: path => path === 'updates/prepare' ? preparation.promise : { code: 1, data: { id: 'task-1', status: 'pending' } } })
  panel.confirm()
  const installing = panel.install()
  assert.equal(panel.preparationAttempt.value, null)
  assert.deepEqual(plain(panel.pendingInstall.value).revision, versionIntent.revision)
  await panel.report({ installation: versionAvailable, executor: { id: 7, checked_at: versionRequestedAt, check_ok: true } })
  assert.equal(panel.taskPosts().length, 0, 'state cannot settle an unanswered prepare POST')
  preparation.resolve(panel.pendingReply())
  await installing
  panel.channels.value = [{ name: 'edge', revision: 'c'.repeat(40), digest: `sha256:${'d'.repeat(64)}`, status: 'update_available', installable: true }]
  panel.target.value = { revision: 'e'.repeat(40), digest: 'changed-dialog' }
  await panel.report({ installation: versionAvailable, executor: { id: 7, checked_at: new Date(versionNow - 1000).toISOString(), check_ok: true } })
  assert.equal(panel.taskPosts().length, 0, 'old successful check must not authorize this request')
  await panel.report({ executor: { id: 7, checked_at: versionRequestedAt, check_ok: true } })
  assert.deepEqual(panel.taskPosts().map(request => request.body), [versionIntent])
  assert.equal(panel.pendingInstall.value, null)
  assert.equal(panel.preparationAttempt.value, null)
  assert.equal(panel.preparing.value, false)
  assert.equal([...panel.timerDelays.values()].includes(90000), false)
  await panel.refresh()
  await panel.refresh()
  assert.equal(panel.taskPosts().length, 1)
  panel.dispose()
})

test('version check-only preparation works without an update and never creates a task', async () => {
  for (const fresh of [false, true]) {
    const panel = versionPanel({ post: () => ({ code: 1, data: { credential_id: 7, requested_at: fresh ? null : versionRequestedAt, installation: fresh ? versionAvailable : versionExpired, wake_status: fresh ? 'not_needed' : 'sent' } }) })
    panel.channels.value = []
    assert.equal(panel.canPrepare.value, true)
    assert.equal(panel.canInstall('edge'), false)
    await panel.prepareInstallation(null)
    assert.equal(panel.pendingInstall.value, null)
    if (!fresh) await panel.report({ installation: versionAvailable, executor: { id: 7, checked_at: versionRequestedAt, check_ok: true } })
    assert.equal(panel.preparing.value, false)
    assert.equal(panel.taskPosts().length, 0)
    panel.dispose()
  }
})

test('version fresh preparation creates a task once without waiting for a poll', async () => {
  const panel = versionPanel({ post: path => path === 'updates/prepare' ? { code: 1, data: { credential_id: 7, installation: versionAvailable, wake_status: 'not_needed' } } : { code: 1, data: { id: 'task-1', status: 'pending' } } })
  panel.confirm()
  await panel.install()
  await flush()
  assert.deepEqual(panel.taskPosts().map(request => request.body), [versionIntent])
  assert.equal(panel.preparing.value, false)
  assert.equal(panel.preparationAttempt.value, null)
  panel.dispose()
})

test('version wake failures keep one preparation pending for legacy run or poll instead of reposting', async () => {
  for (const wake_status of ['sent', 'unconfigured', 'failed', 'coalesced']) {
    const panel = versionPanel({ post: () => ({ code: 1, data: { credential_id: 7, requested_at: versionRequestedAt, installation: versionExpired, wake_status } }) })
    await panel.prepareInstallation(null)
    assert.equal(panel.preparing.value, true)
    if (wake_status === 'unconfigured') assert.match(panel.preparationHint.value, /周期 run、poll/)
    if (wake_status === 'failed') assert.match(panel.preparationHint.value, /唤醒未成功/)
    await panel.refresh()
    assert.equal(panel.posts.length, 1)
    assert.equal(panel.uncertain.value, false)
    panel.dispose()
  }
})

test('version completed failure, hard conditions, credential changes and competing task clear installation intent', async () => {
  const cases = [
    { installation: { available: false, reason: 'deployment_check_failed' }, executor: { id: 7, checked_at: versionRequestedAt, check_ok: false } },
    { installation: { available: false, reason: 'platform_unsupported' } },
    { installation: { available: false, reason: 'credential_expired' }, executor: null, preparation: { credential_id: 0 } },
    { installation: versionAvailable, executor: { id: 8, checked_at: versionRequestedAt, check_ok: true }, preparation: { credential_id: 8 } },
    { installation: versionAvailable, executor: { id: 7, checked_at: versionRequestedAt, check_ok: true }, task: { id: 'someone-else', status: 'pending' } },
    { installation: versionAvailable, executor: { id: 7, checked_at: versionRequestedAt, check_ok: false } },
  ]
  for (const patch of cases) {
    const panel = versionPanel()
    panel.confirm()
    await panel.install()
    await panel.report(patch)
    assert.equal(panel.preparing.value, false)
    assert.equal(panel.pendingInstall.value, null)
    assert.notEqual(panel.preparationError.value, '')
    assert.equal([...panel.timerDelays.values()].includes(90000), false)
    await panel.report({ installation: versionAvailable, preparation: { credential_id: 7 }, executor: { id: 7, checked_at: versionRequestedAt, check_ok: true }, task: null })
    assert.equal(panel.taskPosts().length, 0, 'late success must not restore a cleared intent')
    assert.equal(panel.uncertain.value, false)
    panel.dispose()
  }
})

test('version cancellation, deadline, disposal and account switch discard both late POST and late state success', async () => {
  for (const phase of ['awaiting-post', 'awaiting-check']) {
    for (const action of ['cancel', 'deadline', 'dispose', 'account']) {
      const request = deferred()
      const panel = versionPanel({ post: path => path === 'updates/prepare' ? request.promise : { code: 1, data: { id: 'unexpected', status: 'pending' } } })
      panel.confirm()
      const installing = panel.install()
      if (phase === 'awaiting-check') { request.resolve(panel.pendingReply()); await installing }
      if (action === 'cancel') panel.cancelPreparation()
      if (action === 'deadline') {
        panel.setNow(versionNow + 90000)
        const timer = [...panel.timerDelays].find(([, delay]) => delay === 90000)?.[0]
        assert.notEqual(timer, undefined)
        await panel.fireTimer(timer)
        assert.match(panel.preparationError.value, /90 秒/)
      }
      if (action === 'dispose') panel.dispose()
      if (action === 'account') { panel.account.value = '2'; await flush() }
      if (phase === 'awaiting-post') { request.resolve({ code: 1, data: { credential_id: 7, requested_at: versionRequestedAt, installation: versionAvailable, wake_status: 'not_needed' } }); await installing }
      await panel.report({ installation: versionAvailable, executor: { id: 7, checked_at: versionRequestedAt, check_ok: true } })
      assert.equal(panel.taskPosts().length, 0, `${phase}/${action} must never create a task`)
      assert.equal(panel.preparing.value, false)
      assert.equal(panel.preparationAttempt.value, null)
      assert.equal(panel.pendingInstall.value, null)
      assert.equal(panel.uncertain.value, false)
      assert.equal([...panel.timerDelays.values()].includes(90000), false)
      if (action !== 'dispose') panel.dispose()
    }
  }
})

test('version enforces elapsed 90 seconds even if the deadline callback has not run yet', async () => {
  for (const awaitingPost of [true, false]) {
    const request = deferred()
    const panel = versionPanel({ post: () => request.promise })
    panel.confirm()
    const installing = panel.install()
    if (!awaitingPost) { request.resolve(panel.pendingReply()); await installing }
    panel.setNow(versionNow + 90000)
    if (awaitingPost) { request.resolve({ code: 1, data: { credential_id: 7, installation: versionAvailable, wake_status: 'not_needed' } }); await installing }
    else await panel.report({ installation: versionAvailable, executor: { id: 7, checked_at: versionRequestedAt, check_ok: true } })
    assert.match(panel.preparationError.value, /90 秒/)
    assert.equal(panel.taskPosts().length, 0)
    assert.equal(panel.preparing.value, false)
    panel.dispose()
  }
})

test('version prepare rejection or transport failure is retryable and never marks task creation uncertain', async () => {
  for (const status of [0, 401, 403, 409, 412, 429, 500, 'throw']) {
    const panel = versionPanel({ post: () => { if (status === 'throw') throw new Error('connection lost'); return { code: 0, status, msg: `prepare failed ${status}`, data: { installation: versionExpired } } } })
    panel.confirm()
    await panel.install()
    assert.notEqual(panel.preparationError.value, '')
    assert.equal(panel.error.value, '')
    assert.equal(panel.uncertain.value, false)
    assert.equal(panel.preparing.value, false)
    assert.equal(panel.pendingInstall.value, null)
    assert.equal(panel.taskPosts().length, 0)
    assert.equal(panel.canInstall('edge'), true)
    await panel.refresh()
    assert.equal(panel.posts.length, 1, 'prepare must not automatically retry')
    panel.dispose()
  }
})

test('version lost task response remains uncertain across old completed state and never reposts', async () => {
  for (const thrown of [false, true]) {
    const oldTask = { id: 'previous', status: 'succeeded' }
    const panel = versionPanel({ state: { task: oldTask }, post: path => {
      if (path === 'updates/prepare') return { code: 1, data: { credential_id: 7, installation: versionAvailable, wake_status: 'not_needed' } }
      if (thrown) throw new Error('response lost')
      return { code: 0, status: 0, msg: 'response lost', data: null }
    } })
    panel.confirm()
    await panel.install()
    await flush()
    assert.equal(panel.uncertain.value, true)
    assert.match(panel.error.value, /任务创建结果尚未确认/)
    await panel.refresh()
    await panel.prepareInstallation(versionIntent)
    panel.confirm()
    await panel.install()
    assert.equal(panel.taskPosts().length, 1)
    assert.equal(panel.posts.filter(request => request.path === 'updates/prepare').length, 1)
    assert.equal(panel.uncertain.value, true, 'an older completed task cannot settle a lost creation response')
    await panel.report({ task: { id: 'new-task', status: 'pending' } })
    assert.equal(panel.uncertain.value, false)
    assert.equal(panel.taskPosts().length, 1)
    panel.dispose()
  }
})

test('version task target rejection keeps server message and requires new explicit confirmation', async () => {
  const panel = versionPanel({ post: path => path === 'updates/prepare' ? { code: 1, data: { credential_id: 7, installation: versionAvailable, wake_status: 'not_needed' } } : { code: 0, status: 409, msg: '目标已变化，请重新检查并确认', data: null } })
  panel.confirm()
  await panel.install()
  await flush()
  assert.deepEqual(panel.taskPosts().map(request => request.body), [versionIntent])
  assert.match(panel.rejection.value, /目标已变化/)
  assert.equal(panel.uncertain.value, false)
  assert.equal(panel.selected.value, '')
  await panel.install()
  await panel.refresh()
  assert.equal(panel.taskPosts().length, 1)
  panel.dispose()
})

test('version preparation uses the existing three-second local state poll and does not discover channels', async () => {
  const panel = versionPanel()
  panel.confirm()
  await panel.install()
  await panel.poll()
  assert.deepEqual(panel.gets.map(request => request.path), ['updates/state'])
  assert.deepEqual([...panel.timerDelays.values()].sort((a, b) => a - b), [3000, 90000])
  const timer = [...panel.timerDelays].find(([, delay]) => delay === 3000)[0]
  await panel.fireTimer(timer)
  assert.deepEqual(panel.gets.map(request => request.path), ['updates/state', 'updates/state'])
  assert.equal(panel.posts.length, 1)
  panel.dispose()
  assert.equal(panel.timers.size, 0)
})

test('version delegated administrator has no prepare or installation action', async () => {
  const panel = versionPanel({ io: { useAdminCapabilities: () => ({ isPrimaryAdmin: vue.ref(false), can: () => true }) } })
  assert.equal(panel.canPrepare.value, false)
  assert.equal(panel.canInstall('edge'), false)
  panel.confirm()
  await panel.install()
  await panel.prepareInstallation(null)
  assert.equal(panel.posts.length, 0)
  panel.dispose()
})

test('version credential rotation during an unanswered prepare cannot authorize a late ready response', async () => {
  const request = deferred()
  const panel = versionPanel({ post: () => request.promise })
  panel.confirm()
  const installing = panel.install()
  await panel.report({ executor: { id: 8, checked_at: versionRequestedAt, check_ok: true }, preparation: { credential_id: 8 }, installation: versionAvailable })
  assert.equal(panel.preparationAttempt.value, null)
  request.resolve({ code: 1, data: { credential_id: 7, installation: versionAvailable, wake_status: 'not_needed' } })
  await installing
  assert.equal(panel.pendingInstall.value, null)
  assert.equal(panel.preparing.value, false)
  assert.equal(panel.taskPosts().length, 0)
  assert.match(panel.preparationError.value, /凭据已变化/)
  panel.dispose()
})

test('version a cancelled prepare response cannot replace a newer check-only attempt', async () => {
  const old = deferred(), latest = deferred()
  let calls = 0
  const panel = versionPanel({ post: () => (++calls === 1 ? old.promise : latest.promise) })
  panel.confirm()
  const first = panel.install()
  panel.cancelPreparation()
  const second = panel.prepareInstallation(null)
  old.resolve({ code: 1, data: { credential_id: 7, installation: versionAvailable, wake_status: 'not_needed' } })
  await first
  assert.equal(panel.preparing.value, true)
  assert.equal(panel.pendingInstall.value, null)
  assert.equal(panel.preparationAttempt.value, null)
  latest.resolve(panel.pendingReply())
  await second
  assert.notEqual(panel.preparationAttempt.value, null)
  await panel.report({ installation: versionAvailable, executor: { id: 7, checked_at: versionRequestedAt, check_ok: true } })
  assert.equal(panel.preparing.value, false)
  assert.equal(panel.taskPosts().length, 0)
  panel.dispose()
})

test('version reopening restores state without any installation intent from the previous component', async () => {
  const first = versionPanel()
  first.confirm()
  await first.install()
  first.dispose()
  const reopened = versionPanel({ state: { installation: versionAvailable, executor: { id: 7, checked_at: versionRequestedAt, check_ok: true }, preparation: { credential_id: 7, requested_at: versionRequestedAt } } })
  await reopened.poll()
  assert.equal(reopened.pendingInstall.value, null)
  assert.equal(reopened.preparationAttempt.value, null)
  assert.equal(reopened.preparing.value, false)
  assert.equal(reopened.posts.length, 0)
  assert.equal(first.taskPosts().length, 0)
  reopened.dispose()
})

test('version preparation rejection preserves HTTP data installation reason without task uncertainty', async () => {
  for (const nested of [false, true]) {
    const installation = { available: false, reason: 'database_unsupported' }
    const data = { credential_id: 7, installation }
    const panel = versionPanel({ state: { installation }, post: () => ({ code: 0, status: 412, msg: '不能准备安装', data: nested ? { data } : data }) })
    panel.installation.value = { ...versionAvailable }
    panel.confirm()
    await panel.install()
    assert.match(panel.preparationError.value, /不能准备安装/)
    assert.match(panel.preparationError.value, /数据库不支持自动备份/)
    assert.equal(panel.installation.value.reason, 'database_unsupported')
    assert.equal(panel.canPrepare.value, false)
    assert.equal(panel.uncertain.value, false)
    assert.equal(panel.taskPosts().length, 0)
    panel.dispose()
  }
})

test('version null-data prepare rejection blocks repeat preparation and queues state behind an older ready response', async () => {
  for (const status of [412, 409]) {
    const oldState = deferred(), authoritativeState = deferred()
    let reads = 0
    const panel = versionPanel({
      state: { installation: { ...versionAvailable } },
      post: () => ({ code: 0, status, msg: '不能准备安装', data: null }),
      get: path => {
        assert.equal(path, 'updates/state')
        return ++reads === 1 ? oldState.promise : authoritativeState.promise
      },
    })
    const oldRefresh = panel.refresh()
    panel.confirm()
    await panel.install()
    assert.equal(reads, 1, 'authoritative refresh waits for the in-flight state request')
    assert.equal(panel.installation.value.reason, 'preparation_state_unknown')
    assert.equal(panel.canPrepare.value, false)
    assert.equal(panel.canInstall('edge'), false, 'the SFC installation button must be disabled immediately')
    await panel.prepareInstallation(null)
    assert.equal(panel.posts.length, 1, 'unknown state cannot authorize another prepare')
    oldState.resolve({ code: 1, data: { executor: { id: 7 }, installation: versionAvailable } })
    await flush()
    assert.equal(reads, 2, 'finally must issue the queued local state refresh')
    assert.equal(panel.installation.value.reason, 'preparation_state_unknown', 'late old-generation ready state is ignored')
    assert.equal(panel.canInstall('edge'), false)
    const installation = { available: false, reason: 'database_unsupported' }
    authoritativeState.resolve({ code: 1, data: { executor: { id: 7 }, installation, task: status === 409 ? { id: 'competing', status: 'pending' } : null } })
    await oldRefresh
    assert.equal(panel.installation.value.reason, 'database_unsupported')
    assert.equal(panel.canPrepare.value, false)
    assert.equal(panel.canInstall('edge'), false, 'authoritative hard rejection keeps the installation button disabled')
    assert.equal(panel.task.value?.id || null, status === 409 ? 'competing' : null)
    assert.equal(panel.uncertain.value, false)
    assert.equal(panel.pendingInstall.value, null)
    assert.equal(panel.taskPosts().length, 0)
    assert.equal(panel.timers.size, 0, 'rejection adds no polling timer')
    panel.dispose()
  }
})

test('version null-data rejection immediately reads local state and keeps unknown blocked if that read fails', async () => {
  for (const status of [412, 409]) {
    const stateReply = deferred()
    const panel = versionPanel({
      state: { installation: { ...versionAvailable } },
      post: () => ({ code: 0, status, data: null }),
      get: path => { assert.equal(path, 'updates/state'); return stateReply.promise },
    })
    const preparation = panel.prepareInstallation(null)
    await flush()
    assert.deepEqual(panel.gets.map(request => request.path), ['updates/state'])
    assert.equal(panel.installation.value.reason, 'preparation_state_unknown')
    assert.equal(panel.canPrepare.value, false)
    await panel.prepareInstallation(null)
    assert.equal(panel.posts.length, 1)
    stateReply.resolve({ code: 0, data: null })
    await preparation
    assert.equal(panel.installation.value.reason, 'preparation_state_unknown')
    assert.equal(panel.canInstall('edge'), false)
    assert.equal(panel.uncertain.value, false)
    assert.equal(panel.taskPosts().length, 0)
    assert.equal(panel.timers.size, 0)
    panel.dispose()
  }
})

test('version malformed preparation response never authorizes a task', async () => {
  const invalid = [
    null,
    { credential_id: 0, installation: versionAvailable, wake_status: 'not_needed' },
    { credential_id: 7, requested_at: 'invalid-time', installation: versionAvailable, wake_status: 'not_needed' },
    { credential_id: 7, installation: { available: 'true', reason: '' }, wake_status: 'not_needed' },
    { credential_id: 7, installation: versionAvailable, wake_status: 'invalid-status' },
    { credential_id: 7, installation: versionExpired, wake_status: 'sent' },
  ]
  for (const data of invalid) {
    const panel = versionPanel({ post: () => ({ code: 1, data }) })
    panel.confirm()
    await panel.install()
    assert.match(panel.preparationError.value, /响应无效/)
    assert.equal(panel.preparing.value, false)
    assert.equal(panel.pendingInstall.value, null)
    assert.equal(panel.uncertain.value, false)
    assert.equal(panel.taskPosts().length, 0)
    panel.dispose()
  }
})
