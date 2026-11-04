// Real CommonTagPicker SFC under the containing blocks used by the composer.
// Run with existing Playwright/Chromium, or --serve for a browser-tool session.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const http = require('node:http')
const { build } = require('esbuild')
const { parse, compileScript } = require('@vue/compiler-sfc')
const web = path.resolve(__dirname, '..')
const source = fs.readFileSync(path.join(web, 'components/index/CommonTagPicker.vue'), 'utf8')
const { descriptor } = parse(source)
const entry = `
import { createApp, h, ref } from 'vue'
import CommonTagPicker from './components/index/CommonTagPicker.vue'
const editor = ref(null)
const state = window.fixture = { kind: 'editable', dialog: false }
window.mountFixture = (kind = 'editable', dialog = false) => {
  window.fixtureApp?.unmount()
  document.querySelector('dialog').close()
  const host = document.querySelector(dialog ? '#modal-app' : '#app')
  const app = createApp({ setup: () => () => h('section', { class: 'composer' }, [
    kind === 'textarea'
      ? h('textarea', { ref: editor, id: 'editor', rows: 5 })
      : h('div', { ref: editor, id: 'editor', contenteditable: true }),
    h('div', { class: 'toolbar' }, [h(CommonTagPicker, {
      tags: ['生活', '工作', '阅读'], getEditor: () => editor.value
    })])
  ]) })
  app.component('UIcon', { render: () => h('span', '#') })
  app.mount(host)
  window.fixtureApp = app
  if (dialog) document.querySelector('dialog').showModal()
}
window.mountFixture()
`
async function startServer(port = 0) {
  const script = compileScript(descriptor, { id: 'tag-picker-fixture', inlineTemplate: true })
  const bundle = await build({ stdin: { contents: entry, resolveDir: web }, bundle: true, write: false,
    format: 'iife', platform: 'browser', define: { 'process.env.NODE_ENV': '"test"' },
    plugins: [{ name: 'tag-picker-fixture', setup(builder) {
      builder.onResolve({ filter: /^~\// }, args => ({ path: path.join(web, args.path.slice(2) + '.ts') }))
      builder.onLoad({ filter: /CommonTagPicker\.vue$/ }, () => ({ contents: script.content, loader: 'ts', resolveDir: web }))
    } }] })
  const server = http.createServer((request, response) => {
    if (request.url === '/fixture.js') return response.writeHead(200, { 'Content-Type': 'text/javascript' }).end(bundle.outputFiles[0].text)
    response.writeHead(200, { 'Content-Type': 'text/html' }).end(`<!doctype html><html><head><style>
      body{margin:0;font:16px sans-serif;min-height:1400px;--nw-floating-bg:white;--nw-floating-text:#222;--nw-floating-border:#bbb;--nw-floating-selected-bg:#fed;--nw-floating-selected-border:#f90}
      .composer{margin:160px 80px 0;padding:14px;width:460px;max-width:calc(100vw - 100px);background:#eee;transform:translateZ(0)}
      #editor{display:block;box-sizing:border-box;width:100%;min-height:130px;border:1px solid #999;padding:10px;white-space:pre-wrap;line-height:24px;font:16px/24px sans-serif}
      .toolbar{margin-top:12px;padding:8px;backdrop-filter:blur(6px);background:#fffa}
      .common-tag-trigger{padding:8px 14px}
      dialog{padding:15px;max-width:95vw;transform:translateZ(0)}dialog .composer{margin:0;width:400px}
      ${descriptor.styles.map(style => style.content).join('\n')}
    </style></head><body><div id="app"></div><dialog><div id="modal-app"></div></dialog><script src="/fixture.js"></script></body></html>`)
  })
  await new Promise(resolve => server.listen(port, '127.0.0.1', resolve))
  return server
}
async function verify(page) {
  for (const kind of ['editable', 'textarea']) for (const zoom of [1, 1.1, .9]) for (const dialog of [false, true]) {
    await page.evaluate(({ kind, zoom, dialog }) => { document.body.style.zoom = String(zoom); window.scrollTo(0, 0); window.mountFixture(kind, dialog) }, { kind, zoom, dialog })
    await page.waitForSelector('#editor')
    await page.click('#editor')
    await page.keyboard.type('#')
    await page.waitForFunction(() => document.querySelector('.common-tag-menu')?.matches(':popover-open'))
    const caret = await page.evaluate(() => {
      const menu = document.querySelector('.common-tag-menu').getBoundingClientRect()
      const element = document.querySelector('#editor')
      const editor = element.getBoundingClientRect()
      const style = getComputedStyle(element)
      const zoom = parseFloat(getComputedStyle(document.body).zoom) || 1
      const canvas = document.createElement('canvas').getContext('2d')
      canvas.font = style.font
      const left = editor.left + (parseFloat(style.paddingLeft) + parseFloat(style.borderLeftWidth) + canvas.measureText('#').width) * zoom
      const top = editor.top + (parseFloat(style.paddingTop) + parseFloat(style.borderTopWidth)) * zoom
      const bottom = top + parseFloat(style.lineHeight) * zoom
      return { dx: Math.abs(menu.left - left), dy: Math.min(Math.abs(menu.bottom - top), Math.abs(menu.top - bottom)), menu: menu.toJSON(), editor: editor.toJSON() }
    })
    assert.ok(caret.dx < 3 && caret.dy < 12, `${kind}, zoom ${zoom}, dialog ${dialog}: caret menu misplaced ${JSON.stringify(caret)}`)
    await page.keyboard.press('Tab')
    await page.keyboard.press('Enter')
    assert.match(await page.$eval('#editor', el => el.value ?? el.textContent), /^#工作\s*$/)
    await page.click('.common-tag-trigger')
    await page.waitForFunction(() => document.querySelector('.common-tag-menu')?.matches(':popover-open'))
    const checkButton = async () => page.evaluate(() => {
      const menu = document.querySelector('.common-tag-menu').getBoundingClientRect()
      const button = document.querySelector('.common-tag-trigger').getBoundingClientRect()
      return { dx: Math.abs(menu.left - button.left), gap: Math.min(Math.abs(menu.bottom - button.top), Math.abs(menu.top - button.bottom)) }
    })
    let button = await checkButton()
    assert.ok(button.dx < 3 && button.gap < 12, `button menu misplaced: ${JSON.stringify({ kind, zoom, dialog, button })}`)
    if (!dialog) {
      await page.evaluate(() => window.scrollBy(0, 40))
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
      button = await checkButton()
      assert.ok(button.dx < 3 && button.gap < 12, `scroll lost button anchor: ${JSON.stringify(button)}`)
    }
    await page.click('.common-tag-option')
    assert.match(await page.$eval('#editor', el => el.value ?? el.textContent), /#生活/)
    assert.equal(await page.$('.common-tag-menu'), null)
    console.log(`Passed tag picker: ${kind}, zoom ${zoom}, dialog ${dialog}`)
  }
}
module.exports = { startServer, verify }
if (require.main === module) (async () => {
  const server = await startServer(process.argv.includes('--serve') ? 15325 : 0)
  if (process.argv.includes('--serve')) { console.log('Tag picker fixture ready on 15325'); return }
  let browser
  try {
    const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright-core')
    browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, headless: true })
    const page = await browser.newPage({ viewport: { width: 1100, height: 850 } })
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(`http://127.0.0.1:${server.address().port}`)
    await verify(page)
    assert.deepEqual(errors, [])
  } finally { await browser?.close(); await new Promise(resolve => server.close(resolve)) }
})().catch(error => { console.error(error); process.exitCode = 1 })
