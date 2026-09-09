// CSP-compatible recovery fixture: alternate same-origin modules preserve the
// successful app singleton while failed static branches use fresh module URLs.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const http = require('node:http')
const path = require('node:path')
const ts = require('typescript')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const compile = file => ts.transpileModule(fs.readFileSync(path.join(__dirname, '../utils', file), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const securityHeaders = fs.readFileSync(path.join(__dirname, '../../internal/middleware/security_headers.go'), 'utf8')
const contentSecurityPolicy = securityHeaders.match(/const siteContentSecurityPolicy = "([^"]+)"/)?.[1]
assert(contentSecurityPolicy, 'application Content-Security-Policy is readable by the recovery test')
const sources = {
  '/loader.js': compile('retryable-module.ts').replace("'./module-recovery'", "'/recovery.js'"),
  '/recovery.js': compile('module-recovery.ts'),
  '/_nuxt/state.js': 'globalThis.stateExecutions = (globalThis.stateExecutions || 0) + 1; export const state = { draft: "keep this draft" };',
  '/_nuxt/leaf.js': 'export let count = 0; export const increment = () => ++count;',
  '/_nuxt/middle.js': 'export { count, increment } from "./leaf.js";',
  '/_nuxt/entry.js': 'import { state } from "./state.js"; import * as counter from "./middle.js"; export default { state, counter };',
  '/_nuxt/other.js': 'import { state } from "./state.js"; import * as counter from "./middle.js"; export default { state, counter };',
  '/_nuxt/__retry__/leaf.js': 'export let count = 0; export const increment = () => ++count;',
  '/_nuxt/__retry__/middle.js': 'export { count, increment } from "./leaf.js";',
  '/_nuxt/__retry__/entry.js': 'import { state } from "../state.js"; import * as counter from "./middle.js"; export default { state, counter, base: new URL("../entry.js", import.meta.url).href, later: () => import("../later.js") };',
  '/_nuxt/__retry__/other.js': 'import { state } from "../state.js"; import * as counter from "./middle.js"; export default { state, counter };',
  '/_nuxt/later.js': 'export default "dynamic import retained";',
}
const requests = []
let blockOriginalLeaf = true
const server = http.createServer((req, res) => {
  requests.push(req.url)
  res.setHeader('Content-Security-Policy', contentSecurityPolicy)
  res.setHeader('X-Content-Type-Options', 'nosniff')
  if (req.url === '/unblock-original-leaf') { blockOriginalLeaf = false; res.writeHead(204).end(); return }
  if (req.url === '/_nuxt/leaf.js' && blockOriginalLeaf) { res.writeHead(503).end(); return }
  if (sources[req.url]) { res.writeHead(200, { 'Content-Type': 'text/javascript' }).end(sources[req.url]); return }
  res.writeHead(200, { 'Content-Type': 'text/html' }).end('<!doctype html><title>Module recovery fixture</title>')
})
;(async () => {
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, headless: true })
  try {
    const page = await browser.newPage()
    const errors = []
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    await page.goto(`http://127.0.0.1:${server.address().port}`)
    const result = await page.evaluate(async () => {
      window.originalState = (await import('/_nuxt/state.js')).state
      const { createRetryableModule } = await import('/loader.js')
      const loadFeature = createRetryableModule(() => import('/_nuxt/entry.js'))
      const loadOther = createRetryableModule(() => import('/_nuxt/other.js'))
      const failed = (await Promise.allSettled([loadFeature(), loadOther()])).map(item => item.status)
      await fetch('/unblock-original-leaf')
      const [first, same] = await Promise.all([loadFeature(), loadFeature()])
      const other = await loadOther()
      first.default.counter.increment()
      return {
        failed,
        sameResult: first === same,
        sameState: first.default.state === originalState && other.default.state === originalState,
        sameCounter: first.default.counter === other.default.counter,
        liveCount: other.default.counter.count,
        stateExecutions,
        draft: first.default.state.draft,
        originalBase: first.default.base === new URL('/_nuxt/entry.js', location.href).href,
        dynamic: (await first.default.later()).default,
      }
    })
    assert.deepEqual(result, {
      failed: ['rejected', 'rejected'], sameResult: true, sameState: true,
      sameCounter: true, liveCount: 1, stateExecutions: 1,
      draft: 'keep this draft', originalBase: true, dynamic: 'dynamic import retained',
    })
    assert.equal(requests.filter(url => url === '/_nuxt/__retry__/leaf.js').length, 1)
    assert(!errors.some(message => message.includes('Content Security Policy')), 'recovery must comply with the application CSP')
    console.log(JSON.stringify({ passed: true, result, requests }, null, 2))
  } finally { await browser.close(); server.close() }
})().catch(error => { console.error(error); server.close(); process.exitCode = 1 })
