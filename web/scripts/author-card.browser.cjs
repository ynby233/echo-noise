// Real Vue SFC and scoped CSS, isolated from the backend. Uses installed tools only.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const http = require('node:http')
const { build } = require('esbuild')
const { parse, compileScript, compileStyle } = require('@vue/compiler-sfc')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright-core')
const web = path.resolve(__dirname, '..')
const styles = []
const entry = `
import { createApp, h, reactive } from 'vue'
import { createPinia } from 'pinia'
import AuthorCard from './components/index/AuthorCard.vue'
const state = window.fixture = reactive({ columns: 1, dark: false, layoutKey: 'single', description: '欢迎访问', count: 5, active: true })
window.fixtureUser = reactive({ isLogin: false, user: null, token: '' })
window.subscriptionCalls = []
window.fixtureSubscribed = false
const avatar = 'data:image/svg+xml,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#3b82f6"/><text x="12" y="27" fill="white">J</text></svg>')
createApp({ setup: () => () => state.active ? h('div', { class: 'frame', style: { gridTemplateColumns: 'repeat(' + state.columns + ', minmax(0,1fr))' } }, Array.from({ length: state.columns }, (_, index) => h('article', { class: 'message-list-item', 'data-note': index }, [
  h('header', [h(AuthorCard, { avatar, profileAvatar: avatar, authorId: 8, username: '作者' + index, description: state.description, count: state.count, dark: state.dark, layoutKey: state.layoutKey }), h('span', '作者' + index)]),
  h('p', '用于验证作者浮卡定位和遮挡的笔记内容。'), h('div', { class: 'note-cover' }, '笔记叠层')
]))) : null }).use(createPinia()).mount('#app')
`
async function bundle() {
  let count = 0
  const result = await build({ stdin: { contents: entry, resolveDir: web, loader: 'js' }, bundle: true, write: false, format: 'iife', platform: 'browser', define: { 'process.env.NODE_ENV': '"test"' }, plugins: [{ name: 'author-card-fixture', setup(b) {
    b.onResolve({ filter: /^~\/(store\/user|utils\/api)$/ }, args => ({ path: args.path, namespace: 'subscription-fixture' }))
    b.onLoad({ filter: /.*/, namespace: 'subscription-fixture' }, args => ({ contents: args.path.endsWith('/user')
      ? 'export const useUserStore = () => window.fixtureUser'
      : `const request = (method, url) => { window.subscriptionCalls.push({ method, url }); if (method !== 'GET') window.fixtureSubscribed = method === 'PUT'; return Promise.resolve({ code: 1, data: { author_id: 8, subscribed: window.fixtureSubscribed } }); }; export const getRequest = url => request('GET', url); export const putRequest = url => request('PUT', url); export const deleteRequest = url => request('DELETE', url);`, loader: 'js', resolveDir: web }))
    b.onResolve({ filter: /^~\// }, args => ({ path: path.join(web, args.path.slice(2) + '.ts') }))
    b.onLoad({ filter: /\.vue$/ }, args => {
      const { descriptor, errors } = parse(fs.readFileSync(args.path, 'utf8'), { filename: args.path })
      assert.equal(errors.length, 0)
      const scope = `data-v-author-fixture-${++count}`
      const script = compileScript(descriptor, { id: scope, genDefaultAs: '__component', inlineTemplate: true, templateOptions: { compilerOptions: { scopeId: scope } } })
      for (const style of descriptor.styles) {
        const css = compileStyle({ source: style.content, filename: args.path, id: scope, scoped: style.scoped })
        assert.equal(css.errors.length, 0)
        styles.push(css.code)
      }
      return { contents: script.content + `\n__component.__scopeId = '${scope}'; export default __component`, loader: 'ts', resolveDir: path.dirname(args.path) }
    })
  } }] })
  return result.outputFiles[0].text
}
async function main() {
  const javascript = await bundle()
  const server = http.createServer((request, response) => {
    if (request.url === '/fixture.js') return response.writeHead(200, { 'Content-Type': 'text/javascript' }).end(javascript)
    response.writeHead(200, { 'Content-Type': 'text/html' }).end(`<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><style>
      *{box-sizing:border-box}body{margin:0;font:14px sans-serif;background:#cbd5e1}.spacer{height:60px}.scroll{height:560px;overflow:auto;position:relative;z-index:10;transform:translate3d(0,0,0)}.frame{display:grid;gap:18px;margin:0 24px;position:relative;transform:translateZ(0)}.message-list-item{min-width:0;min-height:260px;padding:16px;border-radius:16px;background:#fff;position:relative;z-index:1;overflow:hidden;transform:translateZ(0)}header{display:flex;align-items:center;gap:8px}.note-cover{position:absolute;top:60px;left:0;right:0;bottom:0;background:#e2e8f0;z-index:2147483647}.dark .message-list-item{background:#1e293b;color:white}.dark .note-cover{background:#334155}#modal{display:none;position:fixed;inset:0;background:#0008;z-index:50}.tail{height:1100px}@media(min-width:1920px){body{zoom:1.1}}
      ${styles.join('\n')}
      </style></head><body><div class="spacer"></div><div class="scroll"><div id="app"></div><div class="tail"></div></div><div class="tail"></div><div id="modal">模态遮罩</div><script src="/fixture.js"></script></body></html>`)
  })
  let browser
  try {
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
    browser = await chromium.launch({ headless: true, executablePath: process.env.CHROMIUM_PATH || undefined })
    const page = await browser.newPage({ viewport: { width: 1280, height: 850 } })
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(`http://127.0.0.1:${server.address().port}/`)
    const card = page.locator('.author-card')
    const trigger = index => page.locator('.author-card-trigger').nth(index)
    const bounds = async index => page.evaluate(index => {
      const a = document.querySelectorAll('.author-card-trigger')[index].getBoundingClientRect()
      const element = document.querySelector('.author-card'), c = element.getBoundingClientRect()
      return { anchor: { left: a.left, top: a.top, right: a.right, bottom: a.bottom }, card: { left: c.left, top: c.top, right: c.right, bottom: c.bottom }, placement: element.dataset.placement, width: innerWidth, height: innerHeight, parent: element.parentElement.tagName }
    }, index)
    async function assertPlacement(index) {
      await card.waitFor({ state: 'visible' })
      await page.waitForFunction(() => document.querySelector('.author-card')?.dataset.placement)
      const value = await bounds(index), a = value.anchor, c = value.card
      assert.equal(value.parent, 'BODY', 'float must escape note stacking and clipping')
      assert(c.left >= 6 && c.top >= 6 && c.right <= value.width - 6 && c.bottom <= value.height - 6, JSON.stringify(value))
      if (value.placement === 'right') assert(Math.abs(c.left - a.right - 8) < 2, JSON.stringify(value))
      if (value.placement === 'left') assert(Math.abs(a.left - c.right - 8) < 2, JSON.stringify(value))
      if (value.placement === 'below') assert(Math.abs(c.top - a.bottom - 8) < 2, JSON.stringify(value))
      if (value.placement === 'above') assert(Math.abs(a.top - c.bottom - 8) < 2, JSON.stringify(value))
      assert(await card.evaluate(element => { const r = element.getBoundingClientRect(); return element.contains(document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)) }), 'card must render above notes')
      return value
    }
    const results = []
    for (const width of [375, 768, 1280, 1920]) {
      await page.setViewportSize({ width, height: 850 })
      assert.equal(await page.evaluate(() => Number(getComputedStyle(document.body).zoom)), width >= 1920 ? 1.1 : 1, 'fixture must apply the actual wide-screen zoom')
      for (const columns of width < 640 ? [1] : [1, 2, 4]) {
        for (const dark of [false, true]) {
          await page.evaluate(({ columns, dark }) => { Object.assign(window.fixture, { columns, dark, layoutKey: `${columns}-${dark}`, description: '欢迎访问', count: 5 }); document.documentElement.classList.toggle('dark', dark) }, { columns, dark })
          await trigger(columns - 1).click()
          const value = await assertPlacement(columns - 1)
          results.push({ width, columns, dark, placement: value.placement })
          assert.equal(await card.evaluate(element => getComputedStyle(element).color), dark ? 'rgb(255, 255, 255)' : 'rgb(17, 24, 39)')
          assert.equal(await card.locator('.author-card-subscribe').count(), 0, 'guest has no subscription control')
          assert(await card.evaluate(element => { const name = element.querySelector('strong').getBoundingClientRect(); const count = element.querySelector('.author-card-count').getBoundingClientRect(); return Math.abs(name.top - count.top) < 5 }), 'note count remains beside the username')
          await page.keyboard.press('Escape')
          await card.waitFor({ state: 'detached' })
        }
      }
    }
    assert.equal(await page.evaluate(() => window.subscriptionCalls.length), 0, 'guest must not request restricted subscription APIs')
    await page.setViewportSize({ width: 375, height: 850 })
    await page.evaluate(() => { Object.assign(window.fixtureUser, { isLogin: true, user: { userid: 3 }, token: 'member' }); Object.assign(window.fixture, { columns: 2, layoutKey: 'subscriptions' }) })
    await trigger(0).click()
    await card.locator('.author-card-subscribe:not(:disabled)').waitFor()
    await card.locator('.author-card-subscribe').click()
    await page.waitForFunction(() => document.querySelector('.author-card-subscribe')?.getAttribute('aria-pressed') === 'true')
    assert(await card.locator('.author-card-subscribe').evaluate(element => element.classList.contains('nw-action-btn--primary')))
    await trigger(1).click()
    await card.waitFor({ state: 'visible' })
    assert.equal(await card.locator('.author-card-subscribe').getAttribute('aria-pressed'), 'true', 'same author cards share status')
    assert.equal(await page.evaluate(() => window.subscriptionCalls.filter(item => item.method === 'GET').length), 1)
    await card.locator('.author-card-subscribe').click()
    await page.waitForFunction(() => document.querySelector('.author-card-subscribe')?.getAttribute('aria-pressed') === 'false')
    await page.evaluate(() => { window.fixtureUser.user = { userid: 8 } })
    await page.waitForFunction(() => !document.querySelector('.author-card-subscribe'))
    await page.evaluate(() => Object.assign(window.fixtureUser, { isLogin: false, user: null, token: '' }))
    await page.keyboard.press('Escape')
    // Hover may cross the avatar/card gap without dismissing, then closes on exit.
    await page.setViewportSize({ width: 1280, height: 850 })
    await page.evaluate(() => Object.assign(window.fixture, { columns: 1, dark: false, layoutKey: 'hover' }))
    await trigger(0).hover()
    await assertPlacement(0)
    await card.hover()
    await page.waitForTimeout(280)
    assert.equal(await card.count(), 1)
    await page.mouse.move(1250, 800)
    await card.waitFor({ state: 'detached' })
    // Async profile size, nested scroll, ancestor shift, viewport resize and body zoom.
    await trigger(0).click()
    await page.evaluate(() => { window.fixture.description = '较长的作者简介。'.repeat(30); window.fixture.count = 99 })
    await assertPlacement(0)
    await page.evaluate(() => { document.querySelector('.scroll').scrollTop = 35 })
    await page.waitForTimeout(80)
    await assertPlacement(0)
    await page.evaluate(() => { document.querySelector('.frame').style.marginTop = '100px' })
    await page.waitForTimeout(80)
    await assertPlacement(0)
    await page.setViewportSize({ width: 1920, height: 850 })
    await page.waitForTimeout(80)
    await assertPlacement(0)
    await page.evaluate(() => { window.fixture.layoutKey = 'layout-changed' })
    await card.waitFor({ state: 'detached' })
    // Bottom edge on a narrow display chooses above instead of crossing viewport.
    await page.setViewportSize({ width: 375, height: 560 })
    await page.evaluate(() => { window.fixture.description = '欢迎访问'; document.querySelector('.scroll').scrollTop = 0; document.querySelector('.frame').style.marginTop = '360px' })
    await trigger(0).click()
    assert.equal((await assertPlacement(0)).placement, 'above')
    await page.locator('.spacer').click({ position: { x: 4, y: 4 } })
    await card.waitFor({ state: 'detached' })
    // Keyboard access, modal stacking and unmount cleanup.
    await trigger(0).focus()
    await page.keyboard.press('ArrowDown')
    await assertPlacement(0)
    assert(await card.evaluate(element => document.activeElement === element), 'keyboard opening must focus the teleported card')
    await page.evaluate(() => { window.fixture.description = '键盘滚动测试。'.repeat(300) })
    await card.evaluate(element => { element.style.maxHeight = '120px' })
    await page.keyboard.press('PageDown')
    await page.waitForFunction(() => document.querySelector('.author-card')?.scrollTop > 0)
    await page.keyboard.press('Escape')
    await card.waitFor({ state: 'detached' })
    assert(await trigger(0).evaluate(element => document.activeElement === element), 'Escape must return keyboard focus to the avatar')
    await page.evaluate(() => { window.fixture.description = '欢迎访问' })
    await page.keyboard.press('ArrowDown')
    await assertPlacement(0)
    await page.evaluate(() => { document.querySelector('#modal').style.display = 'block' })
    assert(await card.evaluate(element => { const r = element.getBoundingClientRect(); return document.elementFromPoint(r.left + 10, r.top + 10)?.id === 'modal' }), 'modal must remain above author card')
    await page.evaluate(() => { document.querySelector('#modal').style.display = 'none'; window.fixture.active = false })
    await card.waitFor({ state: 'detached' })
    await page.evaluate(() => dispatchEvent(new Event('resize')))
    assert.deepEqual(errors, [], 'no browser runtime errors')
    if (process.env.AUTHOR_CARD_ARTIFACT_DIR) {
      fs.mkdirSync(process.env.AUTHOR_CARD_ARTIFACT_DIR, { recursive: true })
      fs.writeFileSync(path.join(process.env.AUTHOR_CARD_ARTIFACT_DIR, 'author-card-browser-results.json'), JSON.stringify({ cases: results, interactions: 'passed', errors }, null, 2))
    }
    console.log(`Author card browser checks passed (${results.length} layout/theme/viewport cases, hover, profile resize, nested scroll, ancestor shift, zoom, edge flip, keyboard, modal stacking, unmount)`)
  } finally {
    if (browser) await browser.close()
    await new Promise(resolve => server.close(resolve))
  }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
