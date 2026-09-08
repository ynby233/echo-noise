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
  let timerID = 0
  const events = new EventTarget()
  const account = options.account || vue.ref('1')
  const drafts = options.drafts || new Map()
  const props = vue.reactive({ theme: { text: 'light', mutedText: 'muted' }, adminShellCardClass: ['light'], adminPanelCardClass: ['light'], adminSubtleCardClass: ['light'] })
  const scope = vue.effectScope()
  const sandbox = vm.createContext({
    ...vue, console, exports: {}, Event, AbortSignal, URL, Date, Intl,
    defineProps: () => props,
    defineEmits: () => () => {},
    useToast: () => ({ add() {} }),
    useRuntimeConfig: () => ({ public: { baseApi: '/api' } }),
    useAdminCapabilities: () => ({ isPrimaryAdmin: vue.ref(true), can: () => true }),
    inject: key => key.description === 'admin drafts' ? drafts : account,
    onMounted: fn => hooks.mounted.push(fn),
    onActivated: fn => hooks.activated.push(fn),
    onDeactivated: fn => hooks.deactivated.push(fn),
    onUnmounted: fn => hooks.unmounted.push(fn),
    window: Object.assign(events, {
      setTimeout: fn => { timers.set(++timerID, fn); return timerID },
      clearTimeout: id => timers.delete(id),
    }),
    setInterval: fn => { timers.set(++timerID, fn); return timerID },
    clearInterval: id => timers.delete(id),
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
    scope.run(() => vm.runInContext(code, sandbox, { filename }))
  }
  run(readFileSync(new URL('config-draft.ts', directory), 'utf8'), 'config-draft.ts')
  sandbox.useConfigDraft = sandbox.exports.useConfigDraft
  const source = readFileSync(new URL(`${name}.vue`, directory), 'utf8').split('<script setup lang="ts">')[1].split('</script>')[0]
  // A block avoids colliding with the composable module's local variables.
  run(`{\n${source}\nglobalThis.subject = { ${expose} }\n}`, `${name}.vue`)
  return {
    ...sandbox.subject, props, timers, drafts, account,
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

test('music custom CDN draft and preset selection survive eviction together', async () => {
  const drafts = new Map()
  const io = { loadFrontendSettings: async () => ({ frontendSettings: {} }) }
  const first = component('MusicSection', 'form, cdnPreset, load', { drafts, io })
  await first.load()
  first.cdnPreset.value = 'jsdelivr'
  await flush()
  const expected = first.form.musicJsCdnURL
  assert.match(expected, /npm\/netease-mini-player@2\.0\.4/)
  first.dispose()
  const next = component('MusicSection', 'form, cdnPreset, load', { drafts, io })
  await next.load()
  await flush()
  assert.equal(next.cdnPreset.value, 'jsdelivr')
  assert.equal(next.form.musicJsCdnURL, expected)
  next.dispose()
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
