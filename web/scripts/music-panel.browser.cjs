// Source SFC fixture: real MusicSection/workbench/cover and scoped CSS; native UI adapters.
// Requires existing dependencies only. Inject PLAYWRIGHT_MODULE, CHROMIUM_PATH and
// PLAYWRIGHT_BROWSERS_PATH from the D-drive environment; never downloads a browser.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const http = require('node:http')
const { build } = require('esbuild')
const { parse, compileScript, compileStyle } = require('@vue/compiler-sfc')
const web = path.resolve(__dirname, '..')
const envRoot = 'D:/Agent Orchestrator/environments/echo-noise'
process.env.PLAYWRIGHT_BROWSERS_PATH ||= path.join(envRoot, 'cache/playwright')
const playwrightModule = process.env.PLAYWRIGHT_MODULE
assert(playwrightModule && path.isAbsolute(playwrightModule) && /^[dD]:[\\/]/.test(playwrightModule), 'Inject absolute D-drive PLAYWRIGHT_MODULE')
if (process.env.CHROMIUM_PATH) assert(/^[dD]:[\\/]/.test(process.env.CHROMIUM_PATH), 'CHROMIUM_PATH must be on D drive')
assert(/^[dD]:[\\/]/.test(process.env.PLAYWRIGHT_BROWSERS_PATH), 'Browser cache must be on D drive')
const { chromium } = require(playwrightModule)
const id = n => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`
const maliciousTitle = '<img src=x onerror="window.tagExecuted=true"> 中文 & 标签'
const tracks = Array.from({ length: 55 }, (_, i) => ({ trackID: id(i + 1), title: i === 1 ? maliciousTitle : `歌曲 ${String(i + 1).padStart(2, '0')} ${'长标题'.repeat(15)}`, artist: i === 1 ? '<script>bad()</script>' : '艺术家', album: '专辑 & 标签', durationMS: 3000, format: 'flac', coverURL: i === 2 ? 'https://evil.invalid/x.png' : `/fixture/api/music/library/${id(i + 1)}/cover`, cueTrackNumber: i === 1 ? 2 : 0, lyricsAvailable: i % 2 === 0, available: i !== 3 }))
let config = { version: 7, frontendSettings: { musicSource: 'local', musicEnabled: true }, scanIntervalMinutes: 60, playlist: [tracks[0]], scan: { state: 'succeeded', invalidCUE: 2, metadataFailed: 1, nextAutoScanAt: '0001-01-01T00:00:00Z' }, rootReadable: true, toolsReady: true, counts: { total: 55, available: 54, unavailable: 1 } }
const calls = [], writes = []
let rejectSave = false, readFailure = false
const css = []
const entry = `
import { createApp, h, reactive, ref, provide, KeepAlive } from 'vue'
import MusicSection from './components/admin/sections/MusicSection.vue'
import { adminDraftsKey, adminDraftAccountKey } from './components/admin/sections/config-draft'
const permissions = window.fixture = reactive({ view: true, manage: true, active: true })
const account = ref('fixture-a'); const drafts = new Map()
window.fixtureAccount = account; window.fixtureDrafts = drafts
const app = createApp({ setup() { provide(adminDraftsKey, drafts); provide(adminDraftAccountKey, account); return () => h(KeepAlive, null, { default: () => permissions.active ? h(MusicSection, { theme: { mutedText: 'muted' }, adminPanelCardClass: 'card' }) : null }) } })
app.component('UButton', { props: ['disabled', 'loading'], setup: (p, { slots }) => () => h('button', { disabled: p.disabled || p.loading }, slots.default?.()) })
app.component('UInput', { props: ['modelValue', 'disabled', 'placeholder'], emits: ['update:modelValue'], setup: (p, { emit }) => () => h('input', { value: p.modelValue, disabled: p.disabled, placeholder: p.placeholder, onInput: e => emit('update:modelValue', e.target.value) }) })
app.component('USelect', { props: ['modelValue', 'options', 'disabled'], emits: ['update:modelValue'], setup: (p, { emit }) => () => h('select', { value: p.modelValue, disabled: p.disabled, onChange: e => emit('update:modelValue', p.options.find(o => String(o.value) === e.target.value)?.value) }, p.options.map(o => h('option', { value: o.value }, o.label))) })
app.component('UToggle', { props: ['modelValue', 'disabled'], emits: ['update:modelValue'], setup: (p, { emit }) => () => h('input', { type: 'checkbox', checked: p.modelValue, disabled: p.disabled, onChange: e => emit('update:modelValue', e.target.checked) }) })
app.component('AdminModuleHeader', { props: ['title', 'description'], setup: (p, { slots }) => () => h('header', [h('h2', p.title), h('p', p.description), slots.actions?.()]) })
app.mount('#app')
`
async function bundle() {
  let sfcID = 0
  const result = await build({ stdin: { contents: entry, resolveDir: web, loader: 'js' }, bundle: true, write: false, platform: 'browser', format: 'iife', define: { 'process.env.NODE_ENV': '"test"' }, plugins: [{ name: 'music-fixture', setup(b) {
    b.onResolve({ filter: /^(#imports|~\/composables\/useAdminCapabilities)$/ }, args => ({ path: args.path, namespace: 'fixture' }))
    b.onLoad({ filter: /.*/, namespace: 'fixture' }, args => ({ contents: args.path === '#imports' ? `export const useRuntimeConfig = () => ({ public: { baseApi: '/fixture/api/' } })` : `export const useAdminCapabilities = () => ({ can: key => key === 'music.view' ? window.fixture.view : window.fixture.manage, refreshCapabilities: async () => { window.fixture.view = false; window.fixture.manage = false } })`, loader: 'js' }))
    b.onLoad({ filter: /\.vue$/ }, args => {
      const { descriptor, errors } = parse(fs.readFileSync(args.path, 'utf8'), { filename: args.path }); assert.equal(errors.length, 0)
      const scope = `data-v-fixture-${++sfcID}`
      const script = compileScript(descriptor, { id: scope, genDefaultAs: '__component', inlineTemplate: true, templateOptions: { compilerOptions: { scopeId: scope } } })
      for (const style of descriptor.styles) { const compiled = compileStyle({ source: style.content, filename: args.path, id: scope, scoped: style.scoped }); assert.equal(compiled.errors.length, 0); css.push(compiled.code) }
      return { contents: script.content + `\n__component.__scopeId = '${scope}'; export default __component`, loader: 'ts', resolveDir: path.dirname(args.path) }
    })
  } }] })
  return result.outputFiles[0].text
}
async function main() {
  const javascript = await bundle()
  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url, 'http://fixture'); calls.push({ method: req.method, path: url.pathname, q: url.searchParams.get('q') })
    const json = (data, status = 200) => res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }).end(JSON.stringify({ code: status < 300 ? 1 : 0, data }))
    if (url.pathname === '/fixture.js') return res.writeHead(200, { 'Content-Type': 'text/javascript' }).end(javascript)
    if (url.pathname === '/fixture.css') return res.writeHead(200, { 'Content-Type': 'text/css' }).end(css.join('\n'))
    if (url.pathname.endsWith('/cover')) return res.writeHead(404).end()
    if (url.pathname.endsWith('/music/config')) {
      if (req.method === 'PUT') {
        const chunks = []; for await (const chunk of req) chunks.push(chunk)
        const body = JSON.parse(Buffer.concat(chunks).toString()); writes.push(body)
        if (rejectSave || body.version !== config.version) return json(null, 409)
        config = { ...config, version: config.version + 1, frontendSettings: body.frontendSettings, scanIntervalMinutes: body.scanIntervalMinutes, playlist: body.trackIDs.map(trackID => tracks.find(t => t.trackID === trackID)) }
      }
      return readFailure ? json(null, 503) : json(config)
    }
    if (url.pathname.endsWith('/refresh-status')) return json({ state: 'succeeded', runID: 'fixture-scan' })
    if (url.pathname.endsWith('/refresh')) return json({ scan: { state: 'running', runID: 'fixture-scan' }, started: true }, 202)
    if (url.pathname.endsWith('/library')) {
      if (readFailure) return json(null, 503)
      const q = url.searchParams.get('q') || '', page = Number(url.searchParams.get('page') || 1)
      let filtered = tracks.filter(t => [t.title, t.artist, t.album].some(s => s.includes(q)))
      const format = url.searchParams.get('format'), lyrics = url.searchParams.get('lyrics'), availability = url.searchParams.get('availability'), selected = url.searchParams.get('selected')
      filtered = filtered.filter(t => (!format || t.format === format) && (lyrics === 'all' || t.lyricsAvailable === (lyrics === 'yes')) && (availability === 'all' || t.available === (availability === 'available')) && (selected === 'all' || config.playlist.some(p => p.trackID === t.trackID) === (selected === 'yes')))
      return json({ items: filtered.slice((page - 1) * 25, page * 25), total: filtered.length, page, pageSize: 25 })
    }
    if (url.pathname.startsWith('/fixture/api/')) return json(null, 404)
    res.writeHead(200, { 'Content-Type': 'text/html' }).end('<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><style>body{margin:16px;font:14px sans-serif}*{box-sizing:border-box}input,select{max-width:100%;min-width:0}button{white-space:normal}.admin-labeled-field{display:flex;flex-direction:column;gap:4px}.admin-fields-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.admin-form-section{padding:12px;border:1px solid #bbb}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style><link rel="stylesheet" href="/fixture.css"><div id="app"></div><script src="/fixture.js"></script>')
  })
  let browser
  try {
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
    browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, headless: true })
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, serviceWorkers: 'block' })
    const page = await context.newPage(); const errors = [], external = []
    page.on('pageerror', error => errors.push(error.message))
    await page.route('**/*', route => { const url = new URL(route.request().url()); if (url.hostname !== '127.0.0.1') { external.push(url.href); return route.abort() } return route.continue() })
    await page.goto(`http://127.0.0.1:${server.address().port}/`)
    const section = page.locator('#site-music-section'), button = name => section.locator(`[data-testid="${name}"]`)
    const row = n => section.locator(`[data-track-id="${id(n)}"]`)
    await row(2).waitFor(); await button('music-save').waitFor({ state: 'visible' })
    await page.waitForFunction(() => !document.querySelector('[data-testid="music-save"]').disabled)
    assert((await section.innerText()).includes('CUE 无法解析或引用音源缺失 2 份'))
    assert((await section.innerText()).includes('元数据读取失败 1 首'))
    assert((await section.innerText()).includes('下次自动刷新：暂无'), 'unset timestamp must not render year 1')
    assert.equal(await row(2).locator('.music-track-title').textContent(), maliciousTitle)
    assert.equal(await row(2).locator('script, .music-track-details img').count(), 0)
    assert.equal(await page.evaluate(() => Boolean(window.tagExecuted)), false)
    assert((await row(2).textContent()).includes('CUE · 第 2 轨'))
    assert.equal(await row(3).locator('img').count(), 0, 'external cover is rejected')
    await row(2).getByRole('checkbox').check(); await button('music-add-selected').click()
    await section.locator(`[data-draft-track-id="${id(2)}"] [data-testid="music-move-up"]`).click()
    assert.deepEqual(await section.locator('[data-draft-track-id]').evaluateAll(nodes => nodes.map(n => n.dataset.draftTrackId)), [id(2), id(1)])
    const businessCalls = () => calls.filter(call => /\/music\/(config|library(?:\/refresh(?:-status)?)?)$/.test(call.path)).length
    const beforeCancel = businessCalls()
    await button('music-reload').click(); await button('music-cancel-reload').click()
    assert.equal(businessCalls(), beforeCancel, 'cancel reload performs no music configuration or library request')
    config.version = 9; rejectSave = true
    await button('music-save').evaluate(node => { node.click(); node.click() })
    await section.getByText('草稿已保留', { exact: false }).waitFor()
    assert.equal(writes.length, 1); assert.equal(writes[0].version, 7)
    await page.evaluate(() => window.dispatchEvent(new Event('frontend-config-updated')))
    await page.waitForFunction(() => !document.querySelector('[data-testid="music-reload"]').disabled)
    assert.equal(writes.length, 1); assert.equal(await section.locator('[data-draft-track-id]').count(), 2)
    await button('music-reload').click(); await button('music-confirm-reload').click()
    await page.waitForFunction(() => !document.querySelector('[data-testid="music-save"]').disabled)
    rejectSave = false
    await row(2).locator('[data-testid="music-add-track"]').click(); await button('music-save').click()
    await section.getByText('保存成功', { exact: false }).waitFor()
    assert.equal(writes.at(-1).version, 9); assert.deepEqual(writes.at(-1).trackIDs, [id(1), id(2)])
    await section.locator(`[data-draft-track-id="${id(2)}"] [data-testid="music-remove"]`).click()
    config.version = 11
    await page.evaluate(() => { window.fixture.active = false })
    await section.waitFor({ state: 'detached' })
    await page.evaluate(() => { window.fixture.active = true })
    await section.waitFor(); await page.waitForFunction(() => !document.querySelector('[data-testid="music-save"]').disabled)
    assert.equal(await section.locator('[data-draft-track-id]').count(), 1, 'KeepAlive preserves unsaved removal')
    await button('music-save').click(); await section.getByText('草稿已保留', { exact: false }).waitFor()
    assert.equal(writes.at(-1).version, 10, 'reactivation never rebases an edited expected version')
    await page.evaluate(() => { window.fixture.manage = false })
    assert(await button('music-save').isDisabled()); assert(await button('music-refresh').isDisabled())
    await button('music-next-page').click(); await row(26).waitFor()
    await button('music-search').fill('歌曲 55'); await row(55).waitFor()
    assert.equal(await section.locator('[data-track-id]').count(), 1)
    assert(calls.some(call => call.q === '歌曲 55'), 'view-only searches the whole server library')
    await button('music-search').fill(''); await row(1).waitFor()
    for (const width of [1440, 768, 390]) {
      await page.setViewportSize({ width, height: 1000 })
      assert(await section.evaluate(node => node.scrollWidth <= node.clientWidth + 1), `section overflow at ${width}`)
      const layout = await section.evaluate(node => {
        const rect = selector => node.querySelector(selector).getBoundingClientRect()
        const select = rect('.music-interval select'), refresh = rect('[data-testid="music-refresh"]')
        const library = rect('.music-library'), playlist = rect('.music-playlist')
        const list = node.querySelector('.music-library-list')
        return {
          controlCenterDelta: Math.abs(select.y + select.height / 2 - refresh.y - refresh.height / 2),
          controlBottomDelta: Math.abs(select.bottom - refresh.bottom),
          statusHeight: rect('.music-status').height,
          libraryHeight: library.height, playlistHeight: playlist.height,
          listHeight: list.getBoundingClientRect().height, listScrollHeight: list.scrollHeight,
          listMaxHeight: parseFloat(getComputedStyle(list).maxHeight)
        }
      })
      assert(width < 480 ? layout.controlBottomDelta < 2 : layout.controlCenterDelta < 2, `refresh controls misaligned at ${width}: ${JSON.stringify(layout)}`)
      assert(layout.listHeight <= layout.listMaxHeight + 2, `library must have bounded height at ${width}`)
      assert(layout.listScrollHeight > layout.listHeight, `full candidate page must scroll at ${width}`)
      if (width >= 1024) {
        assert(layout.statusHeight < 200, `desktop status remains unnecessarily tall: ${layout.statusHeight}`)
        assert(layout.playlistHeight < layout.libraryHeight, 'short playlist must not stretch to the candidate column height')
      }
    }
    readFailure = true; await page.evaluate(() => window.dispatchEvent(new Event('frontend-config-updated')))
    await section.getByText('音乐服务暂不可用，请重试', { exact: false }).first().waitFor(); readFailure = false
    await section.getByRole('button', { name: '重试', exact: true }).first().click()
    await page.waitForFunction(() => !document.querySelector('[data-testid="music-reload"]').disabled)
    await page.evaluate(() => { window.fixture.manage = true })
    await button('music-refresh').click()
    await section.getByText('扫描中', { exact: true }).waitFor()
    await page.evaluate(() => { window.fixture.view = false })
    await page.waitForFunction(() => document.querySelectorAll('[data-track-id],[data-draft-track-id],#site-music-section img').length === 0 && window.fixtureDrafts.size === 0)
    const afterRevocation = calls.length
    await page.waitForTimeout(2200)
    assert.equal(calls.length, afterRevocation, 'revocation stops polling and reads')
    assert.deepEqual(external, []); assert.deepEqual(errors, [])
    console.log('Music source SFC fixture assertions passed: tags/CUE/cover/order/conflict/KeepAlive/view-only/revocation/widths')
  } finally { if (browser) await browser.close(); await new Promise(resolve => server.close(resolve)) }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
