// Run after `npm run generate`. Real production chunks, native browser imports,
// and a synthetic API; fault targets come from this build's manifest.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const http = require('node:http')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const web = path.resolve(__dirname, '..')
const output = path.resolve(process.env.TEST_OUTPUT_ROOT || path.join(web, '.output/public'))
const manifest = JSON.parse(fs.readFileSync(path.join(web, '.nuxt/dist/server/client.manifest.json'), 'utf8'))
const preview = manifest['utils/markdown-preview-runtime.ts']
const dependency = manifest[preview.imports[0]].file
const editor = manifest['components/index/VditorEditor.vue'].file
const search = manifest['components/index/Searchmode.vue'].file
const results = []
const server = http.createServer((req, res) => {
  const pathname = new URL(req.url, 'http://local').pathname
  if (pathname.startsWith('/api/')) {
    let data = {}
    if (pathname.endsWith('/setup/status')) { res.writeHead(404).end('{}'); return }
    if (pathname.endsWith('/user')) data = { id: 1, userid: 1, username: 'fixture', is_admin: true }
    if (pathname.endsWith('/frontend/config')) data = { frontendSettings: { announcementEnabled: false, pwaEnabled: false, musicEnabled: false, homeLayoutDefault: 'masonry', latestGalleryEnabled: false } }
    if (pathname.endsWith('/messages/page')) data = { total: 2, items: [1, 2].map(id => ({ id, content: `# Retry note ${id}\n\nBody **content**`, username: 'fixture', user_id: 1, visibility: 'public', can_interact: true, created_at: '2026-09-09T00:00:00Z' })) }
    if (pathname.endsWith('/comments')) data = []
    if (pathname.endsWith('/announcements/unread')) data = { items: [] }
    res.writeHead(200, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }).end(JSON.stringify({ code: 1, data }))
    return
  }
  let file = path.resolve(output, '.' + decodeURIComponent(pathname))
  if (file !== output && !file.startsWith(output + path.sep)) { res.writeHead(403).end(); return }
  if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) {
    if (pathname.startsWith('/_nuxt/')) { res.writeHead(404).end(); return }
    file = path.join(output, 'index.html')
  }
  const mime = { '.js': 'text/javascript', '.css': 'text/css', '.html': 'text/html', '.json': 'application/json', '.svg': 'image/svg+xml' }
  res.writeHead(200, { 'Content-Type': mime[path.extname(file)] || 'application/octet-stream' }).end(fs.readFileSync(file))
})

;(async () => {
  // A stale fault target must fail the test before any success can be reported.
  for (const file of [preview.file, dependency, editor, search]) assert(fs.existsSync(path.join(output, '_nuxt', file)), `current build contains ${file}`)
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  const base = `http://127.0.0.1:${server.address().port}`
  const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, headless: true })
  try {
    for (const mode of ['preview-dependency', 'concurrent-preview-css', 'concurrent-editor-css', 'shared-preview', 'search-retry-keeps-draft']) {
      const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, serviceWorkers: 'block' })
      await context.addInitScript(() => localStorage.setItem('homeLayoutDesktop', 'masonry'))
      const page = await context.newPage()
      page.setDefaultTimeout(15000)
      const row = { mode, requests: [], errors: [] }
      page.on('pageerror', error => row.errors.push(error.message))
      let blocked = true
      let release
      let delayStarted = false
      let markStarted
      const started = new Promise(resolve => { markStarted = resolve })
      const waitForStart = async () => {
        let timer
        try { await Promise.race([started, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('target module was not requested within 15 seconds')), 15000) })]) }
        finally { clearTimeout(timer) }
      }
      const delay = new Promise(resolve => { release = resolve })
      if (mode === 'preview-dependency') await page.route(`**/_nuxt/${dependency}*`, route => {
        row.requests.push({ url: route.request().url(), blocked })
        return blocked ? route.abort('failed') : route.continue()
      })
      if (mode.includes('css')) await page.route('**/_nuxt/Searchmode*.css*', route => {
        row.requests.push({ url: route.request().url(), blocked })
        return blocked ? route.abort('failed') : route.continue()
      })
      if (mode === 'search-retry-keeps-draft') await page.route(`**/_nuxt/${search}*`, route => {
        row.requests.push({ url: route.request().url(), blocked })
        return blocked ? route.abort('failed') : route.continue()
      })
      if (mode.includes('css') || mode === 'shared-preview') {
        const delayed = mode === 'concurrent-editor-css' ? editor : preview.file
        await page.route(`**/_nuxt/${delayed}`, async route => {
          delayStarted = true
          markStarted()
          row.requests.push({ url: route.request().url(), delayed: true })
          await delay
          await route.continue()
        })
      }
      try {
        await page.goto(base, { waitUntil: 'domcontentloaded' })
        await page.locator('.page-loading-mask').waitFor({ state: 'hidden' })
        if (mode === 'preview-dependency') {
          await page.locator('.markdown-load-error').first().waitFor()
          blocked = false
          await page.locator('.markdown-load-error button').first().click()
          await page.locator('.markdown-preview h1').first().waitFor({ timeout: 5000 })
          await page.locator('.markdown-load-error button').click()
          await page.waitForFunction(() => document.querySelectorAll('.markdown-preview h1').length === 2)
          assert.equal(await page.locator('.markdown-load-error').count(), 0, 'both consumers can reuse the recovered preview dependency')
          assert(row.requests.some(request => !request.blocked), 'the actual failed dependency is fetched after network recovery')
        } else if (mode === 'search-retry-keeps-draft') {
          await page.getByRole('button', { name: '写笔记', exact: true }).click()
          const input = page.locator('.vditor-ir [contenteditable="true"]').first()
          await input.fill('Keep the existing draft through module recovery')
          await input.evaluate(element => { window.originalEditor = element })
          await page.locator('.floating-sidebar button[aria-label="搜索"]').click()
          await page.getByRole('alert').filter({ hasText: '搜索加载失败' }).waitFor()
          blocked = false
          await page.getByRole('alert').getByRole('button', { name: '重试', exact: true }).click()
          await page.getByPlaceholder('请输入关键词').waitFor()
          assert(await input.evaluate(element => element === window.originalEditor), 'the mounted editor retains its DOM identity')
          assert.match(await input.innerText(), /Keep the existing draft/)
          assert(row.requests.some(request => !request.blocked), 'search retry really fetched the failed chunk')
        } else if (mode === 'shared-preview') {
          await waitForStart()
          assert(delayStarted)
          release()
          await page.waitForFunction(() => document.querySelectorAll('.markdown-preview h1').length === 2)
          assert.equal(row.requests.length, 1, 'two concurrent preview consumers share the module request')
        } else {
          if (mode === 'concurrent-editor-css') await page.getByRole('button', { name: '写笔记', exact: true }).click()
          await waitForStart()
          assert(delayStarted, 'the unrelated feature really is still downloading')
          await page.locator('.floating-sidebar button[aria-label="搜索"]').click()
          await page.getByRole('alert').filter({ hasText: '搜索加载失败' }).waitFor()
          release()
          const unrelated = mode === 'concurrent-editor-css' ? '.vditor-ir [contenteditable="true"]' : '.markdown-preview h1'
          await page.locator(unrelated).first().waitFor({ timeout: 5000 })
          assert.equal(await page.locator('.markdown-load-error').count(), 0, 'search CSS failure must not fail healthy Markdown')
          blocked = false
          await page.getByRole('alert').filter({ hasText: '搜索加载失败' }).getByRole('button', { name: '重试', exact: true }).click()
          await page.getByPlaceholder('请输入关键词').waitFor()
        }
        assert.deepEqual(row.errors, [], 'handled loading failures do not become uncaught page exceptions')
        row.passed = true
      } catch (error) { row.passed = false; row.failure = error.message }
      finally { release(); await context.close() }
      results.push(row)
      console.log(JSON.stringify(row))
    }
    if (process.env.RESULT_FILE) fs.writeFileSync(process.env.RESULT_FILE, JSON.stringify(results, null, 2))
    assert(results.every(row => row.passed), 'production loading failure regressions must all pass')
  } finally { await browser.close(); server.close() }
})().catch(error => { console.error(error); server.close(); process.exitCode = 1 })
