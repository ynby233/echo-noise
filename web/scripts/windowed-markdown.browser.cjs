// Real MarkdownRenderer SFC + windowing utility in Chromium. Preview parsing and
// unrelated media/network enhancers are fixtures; no backend or downloads needed.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const http = require('node:http')
const { build } = require('esbuild')
const { parse, compileScript } = require('@vue/compiler-sfc')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright-core')
const web = path.resolve(__dirname, '..')
const source = fs.readFileSync(path.join(web, 'components/index/MarkdownRenderer.vue'), 'utf8')
const { descriptor } = parse(source)
const css = descriptor.styles.map(style => style.content).join('\n')
const mocks = {
  '#imports': 'export const useRuntimeConfig = () => ({ public: { baseApi: "/api" } })',
  '~/store/message': 'export const useMessageStore = () => ({ updateMessage: async () => {} })',
  '~/utils/vditor-preview': `export const loadVditorPreview = async () => ({ preview: async (root, content, options) => {
    const key = root.closest('article').dataset.note;
    window.parses[key] = (window.parses[key] || 0) + 1;
    root.classList.add('vditor-reset');
    root.innerHTML = content;
    await options.after();
  } })`,
  '~/utils/github-card': `export const enhanceGitHubCards = root => root.querySelector('[data-hold-github]')
    ? new Promise(resolve => { window.resolveGithub = resolve }) : Promise.resolve()`,
  '~/utils/rendered-task-list': 'export const createRenderedTaskListEnhancer = () => ({ mount() {}, update() {}, dispose() {} })',
  '~/utils/rendered-table-dialog': `import { ref } from 'vue'; export const createRenderedTableDialog = () => ({
    mount() {}, update() {}, dispose() {}, close() {}, body: ref(null), visible: ref(false), closing: ref(false), html: ref(''), dark: ref(false)
  })`,
}
// Stub only independent enhancers; the component lifecycle and safety policy run
// unchanged. This test does not claim to validate third-party media playback.
for (const match of source.matchAll(/import\s*\{([^}]+)\}\s*from\s*['"](~\/[^'"]+)['"]/g)) {
  const [, names, module] = match
  if (mocks[module] || module.endsWith('/windowed-markdown-body') || module.endsWith('/meting-player')) continue
  mocks[module] = names.split(',').map(name => {
    name = name.trim()
    if (name === 'enhanceGitHubCards' || name === 'enhanceMetingPlayers') return `export const ${name} = async () => {}`
    if (name === 'encodeMarkdownExtraBlankLines' || name === 'resolveManagedAttachmentURL' || name === 'normalizeMediaPreviewUrl') return `export const ${name} = (...args) => args.at(-1)`
    if (name.startsWith('is')) return `export const ${name} = () => false`
    return `export const ${name} = () => {}`
  }).join('\n')
}
const entry = `
import { createApp, h, reactive } from 'vue'
import MarkdownRenderer from './components/index/MarkdownRenderer.vue'
window.parses = {}
window.renderEvents = {}
window.fixture = reactive({ active: true, bounded: true, notes: Array.from({ length: 60 }, (_, i) =>
  Array.from({ length: 12 }, (_, j) => '<p>note-' + i + '-paragraph-' + j + ' <strong>完整正文</strong> ' + '长正文用于确认返回滚动后的文本与高度。'.repeat(4) + '</p>').join('')
) })
const state = window.fixture
window.app = createApp({ setup: () => () => state.active ? state.notes.map((content, index) =>
  h('article', { class: 'message-item', 'data-note': index }, [
    h('header', [h('span', '作者 ' + index), h('button', { class: 'subscription' }, '订阅')]),
    h(MarkdownRenderer, { content, boundedRendering: state.bounded, messageId: index + 1,
      onRendered: () => { window.renderEvents[index] = (window.renderEvents[index] || 0) + 1 } }),
  ])
) : null }).mount('#app')
`

async function main() {
  const script = compileScript(descriptor, { id: 'windowed-markdown-fixture', inlineTemplate: true })
  const result = await build({ stdin: { contents: entry, resolveDir: web, loader: 'js' }, bundle: true, write: false, format: 'iife', platform: 'browser', define: { 'process.env.NODE_ENV': '"test"' }, plugins: [{ name: 'windowed-markdown-fixture', setup(builder) {
    builder.onResolve({ filter: /^(~\/|#imports)/ }, args => mocks[args.path]
      ? { path: args.path, namespace: 'fixture' }
      : { path: path.join(web, args.path.slice(2) + '.ts') })
    builder.onLoad({ filter: /.*/, namespace: 'fixture' }, args => ({ contents: mocks[args.path], loader: 'js', resolveDir: web }))
    builder.onLoad({ filter: /MarkdownRenderer\.vue$/ }, () => ({ contents: script.content, loader: 'ts', resolveDir: web }))
  } }] })
  const server = http.createServer((request, response) => {
    if (request.url === '/fixture.js') return response.writeHead(200, { 'Content-Type': 'text/javascript' }).end(result.outputFiles[0].text)
    response.writeHead(200, { 'Content-Type': 'text/html' }).end(`<!doctype html><html><head><style>
      body{margin:0;font:16px sans-serif}article{width:600px;max-width:90vw;margin:20px auto;padding:12px;border:1px solid #ccc}p{margin:1em 0}header{display:flex;gap:12px}
      ${css}
    </style></head><body><div id="app"></div><script src="/fixture.js"></script></body></html>`)
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  let browser
  try {
    browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, headless: true })
    const page = await browser.newPage({ viewport: { width: 1000, height: 700 } })
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    await page.addInitScript(() => {
      window.observers = { intersection: 0, resize: 0 }
      window.holdIntersection = true
      window.heldIntersections = []
      for (const [name, key] of [['IntersectionObserver', 'intersection'], ['ResizeObserver', 'resize']]) {
        const Original = window[name]
        window[name] = class extends Original {
          constructor(callback, options) {
            super((...args) => {
              if (name === 'IntersectionObserver' && window.holdIntersection) window.heldIntersections.push(() => callback(...args))
              else callback(...args)
            }, options)
            this.live = true; window.observers[key]++
          }
          disconnect() { if (this.live) { this.live = false; window.observers[key]-- }; super.disconnect() }
        }
      }
    })
    await page.goto(`http://127.0.0.1:${server.address().port}`)
    await page.waitForFunction(() => window.parses[0] && document.querySelector('[data-note="59"] [data-markdown-pending]'))
    assert.equal(await page.locator('[data-note="0"] [data-markdown-pending]').count(), 0, 'visible body parses before the first IO callback')
    assert.equal(await page.evaluate(() => window.parses[59] || 0), 0, 'first offscreen body must not parse')
    assert.match(await page.locator('[data-note="59"] .markdown-preview').textContent(), /note-59-paragraph-11/, 'complete pending content remains readable')
    assert.deepEqual(await page.evaluate(() => window.observers), { intersection: 2, resize: 1 }, 'all sixty bodies share observers')
    await page.evaluate(() => {
      window.holdIntersection = false
      window.heldIntersections.splice(0).forEach(deliver => deliver())
    })
    const initial = await page.locator('[data-note="0"] .markdown-preview').evaluate(root => ({ html: root.innerHTML, text: root.textContent, height: root.getBoundingClientRect().height }))
    const firstParses = await page.evaluate(() => window.parses[0])
    const scrollTo = async index => {
      await page.locator(`[data-note="${index}"]`).evaluate(root => root.scrollIntoView({ block: 'start' }))
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(() => requestAnimationFrame(resolve)))))
    }
    await scrollTo(30)
    await page.waitForFunction(() => document.querySelector('[data-note="0"] [data-markdown-dormant]'))
    const sleeping = await page.locator('[data-note="0"] .markdown-preview').evaluate(root => ({ height: root.getBoundingClientRect().height, children: root.childElementCount, text: root.textContent }))
    assert.equal(sleeping.children, 0, 'rich descendants are released')
    assert.ok(Math.abs(sleeping.height - initial.height) < 1, 'measured height survives release')
    assert.match(sleeping.text, /note-0-paragraph-11/, 'dormant body retains complete text')
    assert.equal(await page.locator('.subscription').count(), 60, 'author/subscription shell stays mounted')
    await scrollTo(0)
    await page.waitForFunction(() => !document.querySelector('[data-note="0"] [data-markdown-dormant]'))
    assert.equal(await page.locator('[data-note="0"] .markdown-preview').innerHTML(), initial.html)
    assert.equal(await page.evaluate(() => window.parses[0]), firstParses, 'return restores synchronously without parsing again')

    // Resize invalidates a dormant height; release must use the new width.
    await scrollTo(30)
    await page.waitForFunction(() => document.querySelector('[data-note="0"] [data-markdown-dormant]'))
    await page.setViewportSize({ width: 470, height: 700 })
    await page.waitForFunction(old => document.querySelector('[data-note="0"] .markdown-preview').getBoundingClientRect().height > old, initial.height)
    const resizedHeight = await page.locator('[data-note="0"] .markdown-preview').evaluate(root => root.getBoundingClientRect().height)
    await scrollTo(0)
    assert.ok(Math.abs(await page.locator('[data-note="0"] .markdown-preview').evaluate(root => root.getBoundingClientRect().height) - resizedHeight) < 1)

    // Active focus and cross-body selection are retained, then ordinary bodies
    // can be released on a later viewport transition.
    await page.locator('[data-note="0"] .subscription').focus()
    await scrollTo(30)
    assert.equal(await page.locator('[data-note="0"] [data-markdown-dormant]').count(), 0, 'focused card must remain intact')
    await page.evaluate(() => document.activeElement.blur())
    await scrollTo(0)
    await page.evaluate(() => {
      const range = document.createRange()
      range.setStartBefore(document.querySelector('[data-note="0"] header'))
      range.setEndAfter(document.querySelector('[data-note="1"] header'))
      const selection = getSelection(); selection.removeAllRanges(); selection.addRange(range)
    })
    await scrollTo(30)
    assert.equal(await page.locator('[data-note="0"] [data-markdown-dormant]').count(), 0, 'selection crossing the body must prevent release')
    await page.evaluate(() => getSelection().removeAllRanges())

    // Unknown and stateful descendants keep node identity, including paused
    // players, because recreating them can lose playback or layout state.
    for (const markup of ['<audio controls></audio>', '<video controls></video>', '<iframe src="about:blank"></iframe>', '<img width="120" height="80" alt="image">', '<table><tr><td>table</td></tr></table>', '<textarea>draft</textarea>', '<div contenteditable="true">editing</div>', '<p id="anchor-destination">anchor</p>']) {
      await scrollTo(0)
      const previousParses = await page.evaluate(markup => {
        const count = window.parses[0]
        window.fixture.notes[0] = markup
        return count
      }, markup)
      await page.waitForFunction(count => window.parses[0] > count, previousParses)
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
      await page.evaluate(() => { window.protectedNode = document.querySelector('[data-note="0"] .markdown-preview').firstElementChild })
      await scrollTo(30)
      assert.equal(await page.evaluate(() => document.querySelector('[data-note="0"] .markdown-preview').firstElementChild === window.protectedNode), true, markup)
    }
    await scrollTo(0)
    await page.evaluate(() => { window.fixture.notes[0] = '<p>print-complete</p>' })
    await page.waitForFunction(() => document.querySelector('[data-note="0"] .markdown-preview').textContent === 'print-complete')
    await scrollTo(30)
    await page.waitForFunction(() => document.querySelector('[data-note="0"] [data-markdown-dormant]'))
    await page.evaluate(() => dispatchEvent(new Event('beforeprint')))
    assert.equal(await page.locator('[data-markdown-dormant]').count(), 0)
    await scrollTo(59)
    assert.equal(await page.locator('[data-markdown-dormant]').count(), 0, 'printing prevents observer-triggered release')
    await page.evaluate(() => dispatchEvent(new Event('afterprint')))

    // Exercise the real Meting loader while its network request is unresolved.
    // Replacing a far-away body must invalidate both old enhancement continuations.
    const musicRoutes = []
    let notifyMusicRequest
    let musicRequested = new Promise(resolve => { notifyMusicRequest = resolve })
    await page.route('https://cdn.jsdelivr.net/**', route => { musicRoutes.push(route); notifyMusicRequest() })
    await scrollTo(0)
    await page.evaluate(() => {
      window.fixture.notes[0] = '<meting-js server="netease" id="1"></meting-js><span data-hold-github="true">old-card</span>'
    })
    await musicRequested
    await page.waitForFunction(() => typeof window.resolveGithub === 'function')
    await scrollTo(30)
    await page.evaluate(() => { window.fixture.notes[0] = 'new complete distant body' })
    await page.waitForFunction(() => document.querySelector('[data-note="0"] [data-markdown-pending]')?.textContent === 'new complete distant body')
    const newBodyEvents = await page.evaluate(() => window.renderEvents[0])
    await page.evaluate(async () => { window.resolveGithub(); await Promise.resolve() })
    assert.equal(await page.evaluate(() => window.renderEvents[0]), newBodyEvents, 'old GitHub completion cannot emit for the replacement body')
    await musicRoutes.shift().abort('failed')
    await page.waitForFunction(() => !document.querySelector('link[href*="APlayer.min.css"]'))
    assert.equal(await page.locator('[data-note="0"] .music-load-retry').count(), 0, 'old Meting failure cannot append retry to a replacement body')
    assert.equal(await page.locator('[data-note="0"] .markdown-preview').textContent(), 'new complete distant body')
    musicRequested = new Promise(resolve => { notifyMusicRequest = resolve })
    await scrollTo(0)
    await page.evaluate(() => { window.fixture.notes[0] = '<meting-js server="netease" id="2"></meting-js>' })
    await musicRequested
    await musicRoutes.shift().abort('failed')
    await page.waitForFunction(() => document.querySelector('[data-note="0"] .music-load-retry'))

    // Ordinary pagination eagerly renders all bodies and removes the pool.
    await page.evaluate(() => { window.fixture.bounded = false })
    await page.waitForFunction(() => document.querySelectorAll('[data-markdown-pending], [data-markdown-dormant]').length === 0)
    assert.ok(await page.evaluate(() => window.parses[59] > 0))
    assert.deepEqual(await page.evaluate(() => window.observers), { intersection: 0, resize: 0 })
    await page.evaluate(() => { window.fixture.active = false })
    await page.waitForFunction(() => document.querySelectorAll('.markdown-preview').length === 0)
    assert.deepEqual(errors, [])
    console.log('windowed markdown browser behavior passed')
  } finally {
    await browser?.close()
    await new Promise(resolve => server.close(resolve))
  }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
