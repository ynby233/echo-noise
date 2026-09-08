// Native module-cache regression: failures several levels down, retries while
// still offline, shared live bindings, and an already-running state singleton.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const http = require('node:http')
const path = require('node:path')
const ts = require('typescript')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const compile = file => ts.transpileModule(fs.readFileSync(path.join(__dirname, '../utils', file), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const sources = {
  '/loader.js': compile('retryable-module.ts').replace("'./module-recovery'", "'/recovery.js'"),
  '/recovery.js': compile('module-recovery.ts').replace("'es-module-lexer/js'", "'/lexer.js'"),
  '/lexer.js': fs.readFileSync(require.resolve('es-module-lexer/js'), 'utf8'),
  '/_nuxt/state.js': 'globalThis.stateExecutions = (globalThis.stateExecutions || 0) + 1; export const state = { draft: "keep this draft" };',
  '/_nuxt/leaf.js': 'export let count = 0; export const increment = () => ++count;',
  '/_nuxt/middle.js': 'export { count, increment } from "./leaf.js";',
  '/_nuxt/entry.js': 'import { state } from "./state.js"; import * as counter from "./middle.js"; export default { state, counter, base: import.meta.url, later: () => import("./later.js") };',
  '/_nuxt/other.js': 'import { state } from "./state.js"; import * as counter from "./middle.js"; export default { state, counter };',
  '/_nuxt/later.js': 'export default "dynamic import retained";',
}
let offline = true
const requests = []
const server = http.createServer((req, res) => {
  requests.push({ url: req.url, offline })
  if (req.url === '/_nuxt/leaf.js' && offline) { res.writeHead(503).end(); return }
  if (sources[req.url]) { res.writeHead(200, { 'Content-Type': 'text/javascript' }).end(sources[req.url]); return }
  res.writeHead(200, { 'Content-Type': 'text/html' }).end('<!doctype html><title>Module recovery fixture</title>')
})
;(async () => {
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, headless: true })
  try {
    const page = await browser.newPage()
    await page.goto(`http://127.0.0.1:${server.address().port}`)
    const initial = await page.evaluate(async () => {
      window.originalState = (await import('/_nuxt/state.js')).state
      const { createRetryableModule } = await import('/loader.js')
      window.loadFeature = createRetryableModule(() => import('/_nuxt/entry.js'))
      window.loadOther = createRetryableModule(() => import('/_nuxt/other.js'))
      return (await Promise.allSettled([loadFeature(), loadOther()])).map(result => result.status)
    })
    assert.deepEqual(initial, ['rejected', 'rejected'])
    const stillOffline = await page.evaluate(async () => {
      try { await loadFeature(); return 'unexpected success' } catch { return 'rejected' }
    })
    assert.equal(stillOffline, 'rejected', 'another failed retry must remain retryable')
    offline = false
    const result = await page.evaluate(async () => {
      const [first, same] = await Promise.all([loadFeature(), loadFeature()])
      const other = await loadOther()
      first.default.counter.increment()
      return {
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
      sameResult: true, sameState: true, sameCounter: true, liveCount: 1,
      stateExecutions: 1, draft: 'keep this draft', originalBase: true, dynamic: 'dynamic import retained',
    })
    assert.equal(requests.filter(request => request.url === '/_nuxt/leaf.js' && !request.offline).length, 1, 'repaired dependency is shared by both parents')
    console.log(JSON.stringify({ passed: true, result, requests }, null, 2))
  } finally { await browser.close(); server.close() }
})().catch(error => { console.error(error); server.close(); process.exitCode = 1 })
