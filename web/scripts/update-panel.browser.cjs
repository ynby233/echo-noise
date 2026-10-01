// Actual production Vue/NUxt chunks + an isolated, stateful API. No real data.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const http = require('node:http')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const output = path.resolve(process.env.TEST_OUTPUT_ROOT || path.join(__dirname, '../.output/public'))
const revision = '2'.repeat(40), digest = 'sha256:' + 'b'.repeat(64)
let task = null, offline = false, lostPost = false, posts = 0, follow = 'stable', reason = '', channelStatus = 'update_available', role = 1, credential = null
let rejectPost = 0, delayPost = false, completeDelayedPost = null, stateReads = 0
const state = () => ({ task: role === 1 ? task : task && { id: task.id, status: task.status, channel: task.channel }, installation: { available: !reason, reason }, executor: role === 1 ? credential : null, installed: { version: 'v1.0.0', ...(role === 1 ? { revision: '1'.repeat(40), build_identity: '111111111111' } : {}) } })
const server = http.createServer(async (req, res) => {
  const p = new URL(req.url, 'http://local').pathname
  const json = (data, status = 200, msg = '') => res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }).end(JSON.stringify({ code: status < 300 ? 1 : 0, data, msg }))
  if (p.startsWith('/api/')) {
    if (p.endsWith('/setup/status')) return json({}, 404)
    if (p === '/api/updates/tasks' && req.method === 'POST') {
      posts++
      const chunks = []; for await (const chunk of req) chunks.push(chunk)
      const body = JSON.parse(Buffer.concat(chunks).toString())
      assert.equal(body.revision, revision); assert.equal(body.digest, digest)
      if (rejectPost) return json(null, rejectPost, '渠道或安装条件已变化，请重新检查并确认')
      if (delayPost) {
        completeDelayedPost = () => { task = { id: 'delayed-task-id', channel: body.channel, status: 'pending', target_revision: revision, target_digest: digest } }
        res.destroy(); return
      }
      task = { id: 'server-task-id', channel: body.channel, status: 'pending', target_revision: revision, target_digest: digest }
      if (lostPost) { offline = true; res.writeHead(201, { 'Content-Type': 'application/json' }); res.write('{"code":'); setImmediate(() => res.destroy()); return }
      return json(task, 201)
    }
    if (p === '/api/updates/state' || p === '/api/updates') {
      if (offline) return json({}, 503)
      if (p.endsWith('/state')) { stateReads++; return json(state()) }
      return json({ ...state(), follow_channel: follow, instance_id: role === 1 ? 'a'.repeat(32) : '', report: { channels: ['stable', 'edge'].map(name => ({ name, status: name === 'stable' ? 'no_release' : channelStatus, installable: name === 'edge' && channelStatus === 'update_available', version: name === 'edge' && role === 1 ? revision.slice(0, 12) : '', ...(role === 1 ? { revision, digest } : {}) })), latest_source: { status: 'source_skipped' } } })
    }
    if (p === '/api/updates/channel') { const chunks = []; for await (const c of req) chunks.push(c); follow = JSON.parse(Buffer.concat(chunks).toString()).channel; return json({ channel: follow }) }
    if (p === '/api/updates/executor/credential') { credential = { last_seen_at: new Date().toISOString() }; return json({ token: 'enu_' + 'a'.repeat(64), credential }, 201) }
    if (p.endsWith('/user')) return json({ id: role, userid: role, username: 'fixture', is_admin: true })
    if (p.endsWith('/authorization/me')) return json({ capabilities: ['version.view'] })
    if (p.endsWith('/frontend/config')) return json({ frontendSettings: { pwaEnabled: false, musicEnabled: false, announcementEnabled: false, latestGalleryEnabled: false, homeLayoutDefault: 'masonry' } })
    if (p.endsWith('/messages/page')) return json({ items: [], total: 0 })
    if (p.endsWith('/version/runtime')) return json({ isContainer: true })
    if (p.endsWith('/status')) return json({ users: [{ id: role, username: 'fixture', is_admin: true }] })
    return json({})
  }
  let file = path.resolve(output, '.' + decodeURIComponent(p))
  if (file !== output && !file.startsWith(output + path.sep)) return res.writeHead(403).end()
  if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) file = path.join(output, 'index.html')
  const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml', '.woff2': 'font/woff2' }
  res.writeHead(200, { 'Content-Type': mime[path.extname(file)] || 'application/octet-stream' }).end(fs.readFileSync(file))
})
;(async () => {
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  const base = `http://127.0.0.1:${server.address().port}`
  const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, headless: true })
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, serviceWorkers: 'block' })
  const draft = { content: 'ordinary draft [文件附件：这是很长很长的附件名称.pdf](/api/files/safe.pdf)', visibility: 'public' }
  await context.addInitScript(d => { localStorage.setItem('addform_draft_v1', JSON.stringify(d)); localStorage.setItem('userStore', JSON.stringify({ user: { userid: 1, id: 1, username: 'fixture', is_admin: true }, isLogin: true, token: 'fixture' })) }, draft)
  const page = await context.newPage()
  const errors = []; page.on('pageerror', e => errors.push(e.message))
  const open = async () => { await page.goto(base + '/status#version-section'); await page.locator('#version-section').waitFor() }
  const section = page.locator('#version-section')
  const edge = section.locator('[data-channel="edge"]')
  const install = edge.getByRole('button', { name: '安装测试版', exact: true })
  const text = async value => { await section.getByText(value, { exact: false }).first().waitFor() }
  try {
    const scenario = process.env.UPDATE_PANEL_SCENARIO
    if (!scenario || scenario === 'rejection') {
      for (const status of [409, 412]) {
        task = null; rejectPost = status
        await open(); await page.waitForFunction(() => !document.querySelector('[data-channel="edge"] button:last-child').disabled)
        await install.click(); await section.getByRole('checkbox').check()
        await section.getByRole('button', { name: '确认创建任务', exact: true }).click()
        await page.waitForFunction(() => document.querySelector('#version-section').textContent.includes('渠道或安装条件已变化，请重新检查并确认'), null, { timeout: 3000 })
        await page.waitForFunction(() => !document.querySelector('[data-channel="edge"] button:last-child').disabled)
        assert(await install.isEnabled(), `explicit ${status} must not leave an unknown creation locked`)
        assert.equal(task, null)
      }
      rejectPost = 0
      console.log('Passed: explicit 409/412 rejection remains recoverable without reload')
    }
    if (!scenario || scenario === 'delayed') {
      task = { id: 'old-completed-task', status: 'failed', channel: 'edge' }; delayPost = true
      await open(); await page.waitForFunction(() => !document.querySelector('[data-channel="edge"] button:last-child').disabled)
      await text('old-completed-task')
      await install.click(); await section.getByRole('checkbox').check()
      const before = stateReads
      await section.getByRole('button', { name: '确认创建任务', exact: true }).click()
      for (let attempt = 0; attempt < 100 && (!completeDelayedPost || stateReads <= before); attempt++) await page.waitForTimeout(50)
      assert(completeDelayedPost && stateReads > before, 'lost POST is followed by a state query')
      await text('任务创建结果尚未确认')
      const afterFailure = stateReads
      for (let attempt = 0; attempt < 100 && stateReads <= afterFailure; attempt++) await page.waitForTimeout(50)
      assert(stateReads > afterFailure, 'another poll still sees the old result')
      assert(await install.isDisabled(), 'old terminal task cannot confirm a still processing POST')
      const beforePosts = posts
      completeDelayedPost(); delayPost = false
      await text('delayed-task-id'); await text('等待执行器')
      assert.equal(posts, beforePosts, 'recovery only queries the original request')
      assert(await install.isDisabled())
      console.log('Passed: old result does not unlock unknown POST; original new task restores')
    }
    if (scenario) return
    task = null; posts = 0
    await open(); await text('已安装提交'); await text('尚无正式版')
    for (const [stateReason, expected] of [['executor_unconfigured', '执行器未配置'], ['executor_offline', '执行器离线'], ['executor_upgrade_required', '脚本过旧'], ['database_unsupported', '数据库不支持'], ['deployment_check_failed', '备份检查失败'], ['platform_unsupported', '当前架构不支持']]) {
      reason = stateReason; await section.getByRole('button', { name: '检查更新', exact: true }).click(); await text(expected); assert(await install.isDisabled())
    }
    reason = ''; await section.getByRole('button', { name: '检查更新', exact: true }).click(); await install.waitFor(); await page.waitForFunction(() => !document.querySelector('[data-channel="edge"] button:last-child').disabled)
    await section.getByRole('button', { name: '创建凭据', exact: true }).click(); await section.getByRole('textbox', { name: '一次性执行器凭据' }).waitFor()
    await page.reload(); await section.waitFor(); assert.equal(await section.getByRole('textbox', { name: '一次性执行器凭据' }).count(), 0)
    await install.click(); await text('检测到当前浏览器存在草稿'); await section.getByRole('checkbox').check()
    lostPost = true
    await section.getByRole('button', { name: '确认创建任务', exact: true }).evaluate(el => { el.click(); el.click() })
    await text('任务创建结果尚未确认'); assert.equal(posts, 1); assert(await install.isDisabled())
    offline = false
    await section.getByRole('button', { name: '重试连接', exact: true }).click(); await text('等待执行器'); assert.equal(posts, 1)
    for (const status of ['claimed', 'downloading', 'stopping', 'backing_up', 'replacing', 'verifying', 'needs_attention']) {
      task.status = status; await section.getByRole('button', { name: '检查更新', exact: true }).click(); assert(await install.isDisabled())
    }
    await page.reload(); await section.waitFor(); await text('需要人工核对与结案'); assert.equal(posts, 1)
    // A separate browser context has no saved task ID, and still finds attention.
    const other = await browser.newContext({ serviceWorkers: 'block' })
    const otherPage = await other.newPage(); await otherPage.goto(base + '/status#version-section'); await otherPage.getByText('需要人工核对与结案', { exact: false }).waitFor(); await other.close()
    for (const width of [320, 390, 768, 1440]) {
      await page.setViewportSize({ width, height: 1000 })
      for (const dark of [false, true]) {
        await page.evaluate(d => localStorage.setItem('adminTheme', d ? 'dark' : 'light'), dark)
        await page.reload(); await section.waitFor(); await text('需要人工核对与结案')
        assert.equal(await page.locator('.admin-root').getAttribute('data-admin-theme'), dark ? 'dark' : 'light')
        const overflow = await section.evaluate(el => el.scrollWidth > el.clientWidth + 1)
        assert.equal(overflow, false, `version panel overflow at ${width}, dark=${dark}`)
      }
    }
    task.status = 'failed'; await page.reload(); await section.waitFor(); await text('更新失败。请查看宿主日志'); assert.equal(posts, 1)
    task.status = 'succeeded'; await page.reload(); await section.waitFor(); await text('执行器已确认目标镜像'); assert.equal(posts, 1)
    assert.deepEqual(JSON.parse(await page.evaluate(() => localStorage.getItem('addform_draft_v1'))), draft)
    task = null
    for (const status of ['current', 'channel_behind', 'diverged', 'unknown_history', 'check_failed', 'unsupported']) {
      channelStatus = status; await section.getByRole('button', { name: '检查更新', exact: true }).click(); assert(await install.isDisabled())
    }
    role = 2; await page.reload(); await section.waitFor(); await text('正式版'); assert.equal(await section.getByRole('button', { name: '安装测试版', exact: true }).count(), 0); assert.equal(await section.getByText('已安装提交', { exact: false }).count(), 0); assert(!((await section.textContent()).includes(revision.slice(0, 12))))
    assert.deepEqual(errors, [])
    console.log('U5 production browser passed: channels/capability/credential/lost POST/reconnect/reopen/attention/success/drafts/privacy/320-1440 themes')
  } finally { await browser.close(); await new Promise(resolve => server.close(resolve)) }
})().catch(e => { console.error(e); process.exitCode = 1; server.close() })
