// Production-bundle browser verification with a local, synthetic API. No real
// accounts/data are touched. Routing is used only in failure cases, so measured
// cold/warm reads retain the browser's real HTTP cache behavior.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const http = require('node:http')
const zlib = require('node:zlib')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

const output = path.resolve(process.env.TEST_OUTPUT_ROOT || path.join(__dirname, '../.output/public'))
let servedOutput = output
const measureOnly = process.env.MEASURE_ONLY === '1'
const results = []
const record = row => { results.push(row); console.log('Passed:', row.label) }
const pixel = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j9XkAAAAASUVORK5CYII='
const content = `# R4 正文\n\n普通阅读 **内容**\n\n- [ ] 任务\n\n|甲|乙|\n|---|---|\n|1|2|\n\n![测试图片](${pixel})`
const chunks = fs.readdirSync(path.join(output, '_nuxt')).filter(name => name.endsWith('.js'))
const chunkFor = name => chunks.find(file => fs.readFileSync(path.join(output, '_nuxt', file), 'utf8').includes(`__name:"${name}"`))
const features = ['VditorEditor', 'Searchmode', 'InfoFeedList', 'UserNotificationCenter', 'BuiltinComments', 'ImageHostingUploader', 'AudioRecorder', 'StatusPanel']
const featureChunks = Object.fromEntries(features.map(name => [name, chunkFor(name)]))

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, 'http://local')
  const cookies = Object.fromEntries(String(req.headers.cookie || '').split(';').map(part => part.trim().split('=')))
  const loggedIn = cookies['r4-viewer'] !== 'guest'
  const json = (data, status = 200) => { res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }); res.end(JSON.stringify(data)) }
  if (url.pathname.startsWith('/api/')) {
    const p = url.pathname
    let data = {}
    if (p.endsWith('/setup/status')) return json({}, 404)
    if (p.endsWith('/user')) {
      if (!loggedIn) return json({ code: 0, msg: '未登录' }, 401)
      data = { id: 1, userid: 1, username: 'admin', is_admin: true }
    }
    if (p.endsWith('/frontend/config')) data = { frontendSettings: {
      feedEnabled: true, commentEnabled: true, musicEnabled: false, pwaEnabled: cookies['r4-sw'] === '1',
      announcementEnabled: false, homeLayoutDefault: cookies['r4-layout'] || 'masonry', latestGalleryEnabled: false,
    } }
    if (p.endsWith('/messages/page')) data = { total: 1, items: [{ id: 1, content: content + (cookies['r4-music'] === '1' ? '\n\nhttps://music.163.com/#/song?id=123\n\nhttps://music.163.com/#/song?id=456' : ''), username: 'admin', user_id: 1, visibility: 'public', can_interact: true, created_at: '2026-09-08T00:00:00Z' }] }
    if (p.endsWith('/guestbook/message')) data = { id: 2 }
    if (p.endsWith('/comments')) data = []
    if (p.endsWith('/comments/counts')) data = { 1: 0 }
    if (p.endsWith('/announcements/unread') || p.endsWith('/feed/items')) data = { items: [] }
    if (p.endsWith('/notifications')) data = { items: [], total: 0, unread_count: 0 }
    return json({ code: 1, data })
  }
  let file = path.resolve(servedOutput, '.' + decodeURIComponent(url.pathname))
  if (!file.startsWith(servedOutput + path.sep) && file !== servedOutput) return res.writeHead(403).end()
  if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) file = path.join(servedOutput, 'index.html')
  const mime = { '.js': 'text/javascript', '.css': 'text/css', '.html': 'text/html', '.json': 'application/json', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2' }
  const extension = path.extname(file)
  res.setHeader('Content-Type', mime[extension] || 'application/octet-stream')
  res.setHeader('Cache-Control', url.pathname.startsWith('/_nuxt/') ? 'public, max-age=31536000, immutable' : 'no-cache')
  let body = fs.readFileSync(file)
  if (/gzip/.test(req.headers['accept-encoding'] || '') && ['.js', '.css', '.html'].includes(extension)) {
    body = zlib.gzipSync(body); res.setHeader('Content-Encoding', 'gzip')
  }
  res.end(body)
})

;(async () => {
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  const baseURL = `http://127.0.0.1:${server.address().port}`
  const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, headless: true, args: ['--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream'] })
  const createPage = async ({ loggedIn = true, layout = 'masonry', savedLayout = true, serviceWorker = false, musicEmbed = false } = {}) => {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, serviceWorkers: serviceWorker ? 'allow' : 'block' })
    await context.addCookies([
      { name: 'r4-viewer', value: loggedIn ? 'admin' : 'guest', url: baseURL },
      { name: 'r4-layout', value: layout, url: baseURL },
      { name: 'r4-sw', value: serviceWorker ? '1' : '0', url: baseURL },
      { name: 'r4-music', value: musicEmbed ? '1' : '0', url: baseURL },
    ])
    await context.addInitScript(({ layout, savedLayout }) => {
      if (savedLayout) localStorage.setItem('homeLayoutDesktop', layout)
      window.__longTasks = []
      new PerformanceObserver(list => window.__longTasks.push(...list.getEntries().map(entry => ({ start: entry.startTime, duration: entry.duration })))).observe({ type: 'longtask', buffered: true })
    }, { layout, savedLayout })
    const page = await context.newPage()
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    const session = await context.newCDPSession(page)
    await session.send('Performance.enable')
    await session.send('Network.enable')
    const network = new Map()
    session.on('Network.requestWillBeSent', event => {
      if (event.type === 'Document') network.clear()
      // Chromium reports some modulepreload/prefetch requests as Other. Include
      // those assets as well; Resource Timing alone also hides cross-origin bytes.
      const pathname = new URL(event.request.url).pathname
      const type = /\.css$/.test(pathname) ? 'Stylesheet' : /\.m?js$/.test(pathname) ? 'Script' : event.type
      if (['Script', 'Stylesheet'].includes(type)) network.set(event.requestId, { url: event.request.url, type, transfer: 0 })
    })
    session.on('Network.loadingFinished', event => {
      if (network.has(event.requestId)) network.get(event.requestId).transfer = event.encodedDataLength
    })
    const snapshot = async label => {
      const metrics = Object.fromEntries((await session.send('Performance.getMetrics')).metrics.map(item => [item.name, item.value]))
      return { label, loggedIn, layout, ...await page.evaluate(() => ({
        resources: performance.getEntriesByType('resource').filter(entry => ['script', 'link'].includes(entry.initiatorType)).map(entry => ({ url: entry.name, transfer: entry.transferSize, decoded: entry.decodedBodySize })),
        longTasks: window.__longTasks, editor: !!document.querySelector('.vditor'),
        content: document.querySelector('.markdown-preview')?.textContent,
      })), networkResources: [...network.values()], scriptDuration: metrics.ScriptDuration, taskDuration: metrics.TaskDuration }
    }
    const open = async () => {
      const start = Date.now()
      await page.goto(baseURL)
      await page.waitForSelector('.markdown-preview h1', { timeout: 60000 })
      await page.waitForSelector('.page-loading-mask', { state: 'hidden' })
      const ready = Date.now() - start
      await page.waitForTimeout(500)
      return ready
    }
    return { context, page, errors, snapshot, open }
  }
  try {
    for (const loggedIn of [false, true]) {
      const run = await createPage({ loggedIn })
      const ready = await run.open()
      const cold = await run.snapshot('cold-reading'); cold.readyMs = ready; record(cold)
      if (!measureOnly) {
        assert.equal(cold.editor, false, 'hidden composer must not instantiate Vditor')
        for (const name of features) {
          assert(featureChunks[name], `production bundle for ${name} found`)
          assert(!cold.resources.some(item => item.url.endsWith('/' + featureChunks[name])), `${name} must not download before use`)
        }
        assert(!cold.resources.some(item => /jquery|APlayer|Meting|medium-zoom|bcrypt|fancybox/i.test(item.url)), 'unused external runtimes stay unloaded')
      }
      await run.page.reload(); await run.page.waitForSelector('.markdown-preview h1'); await run.page.waitForTimeout(500)
      record(await run.snapshot('warm-reading'))
      assert.deepEqual(run.errors, [], 'reading has no runtime exceptions')
      await run.context.close()
    }
    if (!measureOnly) {
      // The server's default can arrive after initial page setup. It must also
      // suppress the composer, even without a stored layout preference.
      const serverDefault = await createPage({ savedLayout: false })
      await serverDefault.open()
      assert.equal(await serverDefault.page.locator('.vditor').count(), 0)
      await serverDefault.context.close()

      const run = await createPage()
      const { page } = run
      await run.open()
      await page.locator('.markdown-preview [data-fancybox]').first().click()
      await page.waitForSelector('.fancybox__container')
      await page.keyboard.press('Escape'); await page.waitForSelector('.fancybox__container', { state: 'detached' })
      await page.getByRole('button', { name: '搜索', exact: true }).first().click()
      await page.getByPlaceholder('请输入关键词').fill('保留搜索词')
      await page.getByRole('button', { name: '取消', exact: true }).click()
      await page.getByRole('button', { name: '搜索', exact: true }).first().click()
      assert.equal(await page.getByPlaceholder('请输入关键词').inputValue(), '保留搜索词')
      await page.getByRole('button', { name: '取消', exact: true }).click()
      await page.getByRole('button', { name: '写笔记', exact: true }).click()
      const editor = page.locator('.vditor-ir [contenteditable="true"]').first()
      await editor.waitFor({ timeout: 60000 }); await editor.fill('保留未发布草稿')
      await editor.evaluate(element => { element.dataset.testInstance = 'original' })
      await page.getByRole('button', { name: '写笔记', exact: true }).click()
      await page.getByRole('button', { name: '写笔记', exact: true }).click()
      assert.equal(await editor.getAttribute('data-test-instance'), 'original')
      assert((await editor.textContent()).includes('保留未发布草稿'))
      await page.getByRole('button', { name: '图床上传', exact: true }).click()
      await page.waitForFunction(chunk => performance.getEntriesByType('resource').some(entry => entry.name.endsWith('/' + chunk)), featureChunks.ImageHostingUploader)
      await page.waitForSelector('.image-hosting-popup')
      await page.locator('.image-hosting-popup').getByRole('button', { name: '关闭', exact: true }).click()
      await page.getByRole('button', { name: '录音', exact: true }).click()
      await page.getByRole('dialog', { name: '录音', exact: true }).waitFor()
      await page.getByRole('dialog', { name: '录音', exact: true }).getByRole('button', { name: '取消', exact: true }).click()
      record(await run.snapshot('opened-editor-and-image-hosting'))
      // Close the uploader before leaving the page; there is no upload or publish.
      await page.reload(); await page.waitForSelector('.markdown-preview h1')
      await page.getByRole('button', { name: '通知', exact: true }).click()
      await page.waitForSelector('.notification-center')
      record(await run.snapshot('opened-notifications'))
      await page.getByRole('button', { name: '信息流', exact: true }).click()
      await page.waitForSelector('.feed-list-wrap')
      await page.getByRole('button', { name: '留言', exact: true }).click()
      await page.waitForSelector('.comment-board-wrap .builtin-comments')
      record(await run.snapshot('opened-feed-and-comments'))
      assert.deepEqual(run.errors, [], 'feature switching has no runtime exceptions')
      await run.context.close()

      const retry = await createPage()
      await retry.open()
      let attempts = 0
      const retryRequests = []
      await retry.page.route(url => url.pathname.endsWith(`/${featureChunks.Searchmode}`), route => {
        retryRequests.push(new URL(route.request().url()).pathname)
        return ++attempts === 1 ? route.abort('failed') : route.continue()
      })
      await retry.page.getByRole('button', { name: '搜索', exact: true }).first().click()
      await retry.page.getByRole('alert').filter({ hasText: '搜索加载失败' }).waitFor()
      await retry.page.getByRole('button', { name: '重试', exact: true }).click()
      await retry.page.getByPlaceholder('请输入关键词').waitFor()
      assert.deepEqual(retryRequests, [`/_nuxt/${featureChunks.Searchmode}`, `/_nuxt/__retry__/${featureChunks.Searchmode}`], 'retry requests the CSP-compatible recovery chunk')
      record({ label: 'search-download-retry', attempts, retryRequests })
      await retry.context.close()

      const cssRetry = await createPage()
      await cssRetry.open()
      let cssAttempts = 0
      await cssRetry.page.route('**/_nuxt/Searchmode*.css*', route => ++cssAttempts === 1 ? route.abort('failed') : route.continue())
      await cssRetry.page.getByRole('button', { name: '搜索', exact: true }).first().click()
      await cssRetry.page.getByRole('alert').filter({ hasText: '搜索加载失败' }).waitFor()
      await cssRetry.page.getByRole('button', { name: '重试', exact: true }).click()
      await cssRetry.page.getByPlaceholder('请输入关键词').waitFor()
      assert(cssAttempts >= 2, 'failed CSS is downloaded again')
      record({ label: 'stylesheet-download-retry', attempts: cssAttempts })
      await cssRetry.context.close()

      const music = await createPage({ musicEmbed: true })
      const musicAssets = []
      let playerAttempts = 0
      await music.page.route('**/aplayer@1.10.1/dist/APlayer.min.css', route => { musicAssets.push('css'); return route.fulfill({ contentType: 'text/css', body: '' }) })
      await music.page.route('**/aplayer@1.10.1/dist/APlayer.min.js', route => {
        musicAssets.push('player')
        return ++playerAttempts === 1 ? route.abort('failed') : route.fulfill({ contentType: 'text/javascript', body: 'window.APlayer = function() {}' })
      })
      await music.page.route('**/meting@2.0.1/dist/Meting.min.js', route => {
        musicAssets.push('meting')
        return route.fulfill({ contentType: 'text/javascript', body: 'if (!window.APlayer) throw new Error("APlayer must load first"); customElements.define("meting-js", class extends HTMLElement {})' })
      })
      await music.open()
      await music.page.locator('.music-load-retry').click()
      await music.page.waitForFunction(() => !!customElements.get('meting-js'))
      assert.deepEqual(musicAssets, ['css', 'player', 'player', 'meting'], 'music assets are shared and keep dependency order after retry')
      assert.deepEqual(music.errors, [])
      record({ label: 'music-assets-order-and-retry', assets: musicAssets })
      await music.context.close()

      const previewRetry = await createPage()
      let luteAttempts = 0
      let luteBlocked = true
      await previewRetry.page.route('**/lute/lute.min.js', route => { luteAttempts++; return luteBlocked ? route.abort('failed') : route.continue() })
      await previewRetry.page.goto(baseURL)
      await previewRetry.page.locator('.markdown-load-error').waitFor()
      assert((await previewRetry.page.locator('.markdown-preview').first().textContent()).includes('R4 正文'), 'failed preview keeps readable text')
      luteBlocked = false
      await previewRetry.page.locator('.markdown-load-error').getByRole('button', { name: '重试', exact: true }).click()
      await previewRetry.page.waitForSelector('.markdown-preview h1')
      record({ label: 'markdown-render-retry', attempts: luteAttempts })
      await previewRetry.context.close()

      const sw = await createPage({ serviceWorker: true })
      await sw.open()
      await sw.page.evaluate(() => navigator.serviceWorker.ready)
      const cachedInfo = await sw.page.evaluate(async () => {
        const names = await caches.keys()
        const rows = (await Promise.all(names.map(async name => {
          const cache = await caches.open(name)
          return Promise.all((await cache.keys()).map(async request => ({ url: request.url, bytes: (await (await cache.match(request)).arrayBuffer()).byteLength })))
        }))).flat()
        return { urls: rows.map(row => row.url), decodedBytes: rows.reduce((total, row) => total + row.bytes, 0) }
      })
      const cached = cachedInfo.urls
      assert(cached.some(url => url.includes(featureChunks.Searchmode)), 'SW still precaches unopened feature chunks')
      assert(!cached.some(url => url.includes('/__retry__/')), 'online-only recovery graph is not duplicated in the SW precache')
      assert.equal(await sw.page.locator('.vditor').count(), 0, 'SW precaching must not instantiate the editor')
      await sw.page.reload(); await sw.page.waitForSelector('.markdown-preview h1')
      await sw.context.setOffline(true)
      await sw.page.getByRole('button', { name: '搜索', exact: true }).first().click()
      await sw.page.getByPlaceholder('请输入关键词').waitFor()
      record({ label: 'service-worker-precache', cachedRequests: cached.length, decodedBytes: cachedInfo.decodedBytes, offlineSearch: true })
      await sw.context.close()

      if (process.env.TEST_PREVIOUS_OUTPUT) {
        servedOutput = path.resolve(process.env.TEST_PREVIOUS_OUTPUT)
        const update = await createPage({ serviceWorker: true })
        try {
          await update.open()
          await update.page.evaluate(() => navigator.serviceWorker.ready)
          // Returning installed clients are controlled by the previous worker.
          // Finish the initial installation before exercising a later release.
          await update.page.reload()
          await update.page.waitForSelector('.markdown-preview h1')
          servedOutput = output
          await update.page.evaluate(async () => (await navigator.serviceWorker.getRegistration()).update())
          await update.page.getByText('新版本已准备好', { exact: true }).waitFor({ timeout: 60000 })
          await update.page.getByRole('button', { name: '现在刷新', exact: true }).click()
          await update.page.waitForFunction(() => !document.querySelector('.vditor') && !!document.querySelector('.markdown-preview h1'), null, { timeout: 60000 })
          await update.page.getByRole('button', { name: '搜索', exact: true }).first().click()
          await update.page.getByPlaceholder('请输入关键词').waitFor()
          record({ label: 'service-worker-version-update', updatePrompt: true, newChunksLoaded: true })
        } finally { servedOutput = output; await update.context.close() }
      }

      const visible = await createPage({ layout: 'three' })
      await visible.open()
      await visible.page.waitForSelector('.vditor-ir [contenteditable="true"]', { timeout: 60000 })
      assert.equal(await visible.page.getByRole('button', { name: '写笔记', exact: true }).count(), 0, 'default visible editor needs no new click step')
      record(await visible.snapshot('default-visible-editor'))
      if (process.env.RESULT_FILE) {
        const artifactDir = path.dirname(process.env.RESULT_FILE)
        await visible.page.screenshot({ path: path.join(artifactDir, 'home-1440.png') })
        await visible.page.setViewportSize({ width: 390, height: 844 })
        await visible.page.waitForTimeout(500)
        await visible.page.screenshot({ path: path.join(artifactDir, 'home-390.png') })
        await visible.page.getByRole('button', { name: '展开工具栏', exact: true }).click()
        await visible.page.getByRole('button', { name: '切换亮暗', exact: true }).click()
        await visible.page.getByRole('button', { name: '收纳工具栏', exact: true }).click()
        await visible.page.waitForTimeout(300)
        await visible.page.screenshot({ path: path.join(artifactDir, 'home-390-dark.png') })
      }
      await visible.page.setViewportSize({ width: 1440, height: 1000 })
      await visible.page.getByRole('button', { name: '后台', exact: true }).first().click()
      await visible.page.waitForSelector('.admin-root')
      await visible.page.waitForTimeout(500)
      record(await visible.snapshot('opened-admin'))
      assert.deepEqual(visible.errors, [], 'default editor and admin entry have no runtime exceptions')
      await visible.context.close()
    }
    if (process.env.RESULT_FILE) fs.writeFileSync(process.env.RESULT_FILE, JSON.stringify({ featureChunks, results }, null, 2))
    console.log(JSON.stringify(results.map(({ resources, networkResources, longTasks, content, ...row }) => ({ ...row, ...(resources ? { requests: resources.length, transfer: resources.reduce((sum, item) => sum + item.transfer, 0), networkTransfer: networkResources.reduce((sum, item) => sum + item.transfer, 0), longTaskCount: longTasks.length } : {}) })), null, 2))
  } finally { await browser.close(); server.close() }
})().catch(error => { console.error(error); server.close(); process.exitCode = 1 })
