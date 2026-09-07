const assert = require('node:assert/strict')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

;(async () => {
  const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, headless: true })
  try {
    const page = await browser.newPage({ viewport: { width: 1400, height: 1100 } })
    let total = 45
    const requests = []
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    await page.route('**/api/**', async route => {
      const path = new URL(route.request().url()).pathname
      let data = {}
      if (path.endsWith('/setup/status')) return route.fulfill({ status: 404, json: {} })
      if (path.endsWith('/user')) data = { userid: 1, username: 'admin', is_admin: true }
      if (path.endsWith('/frontend/config')) data = {
        frontendSettings: { enableGithubCard: true, feedEnabled: true, feedLimit: 100, musicEnabled: false, pwaEnabled: false, announcementEnabled: false, homeLayoutDefault: 'three' },
      }
      if (path.endsWith('/messages/page')) {
        const query = route.request().postDataJSON()
        requests.push(query)
        data = {
          total,
          items: Array.from({ length: Math.max(0, Math.min(15, total - (query.page - 1) * 15)) }, (_, i) => ({
            id: (query.page - 1) * 15 + i + 1,
            content: `第${query.page}页 https://github.com/AlkaidLab/foundation-sunshine`,
            username: 'admin', user_id: 1, visibility: 'public', created_at: '2026-09-08T00:00:00Z',
          })),
        }
      }
      if (path.endsWith('/feed/items') || path.endsWith('/feed/refresh')) data = {
        items: Array.from({ length: 40 }, (_, i) => ({
          title: `Feed${i}`, content: `feed row ${i}`, description: `feed row ${i}`,
          link: `https://example.com/${i}`, timestamp: 1788880000 - i,
          publishedAt: '2026-09-08T00:00:00Z', type: 'rss', source: 'test',
        })),
      }
      if (path.endsWith('/announcements/unread')) data = { items: [] }
      await route.fulfill({ json: { code: 1, data } })
    })
    await page.route('https://api.github.com/repos/**', route => route.fulfill({ json: { full_name: 'AlkaidLab/foundation-sunshine' } }))
    await page.goto(process.env.TEST_BASE_URL || 'http://127.0.0.1:3112/')
    await page.waitForSelector('.content-container[data-msg-id]')
    await page.waitForSelector('.content-container.file-attachment-shadow-open .github-card-loaded')
    const overflow = await page.locator('.content-container[data-msg-id]').first().evaluate(element => {
      return [element, element.closest('.message-list-item'), element.querySelector('.overflow-y-hidden'), element.querySelector('.markdown-preview')]
        .map(node => getComputedStyle(node).overflow)
    })
    assert(overflow.every(value => value === 'visible'), `GitHub card shadows must remain visible: ${overflow}`)
    for (const tab of ['最新', '个人', '信息流']) {
      await page.getByRole('button', { name: tab, exact: true }).first().click()
      await page.waitForTimeout(1200)
      await page.getByRole('button', { name: '下一页', exact: true }).last().click()
      await page.waitForTimeout(1000)
      await page.reload()
      await page.waitForTimeout(2200)
      const saved = await page.evaluate(() => JSON.parse(sessionStorage.getItem('home-page-position')))
      assert.equal(saved.page, 2)
      assert.equal((await page.locator('.hero-tab.active').first().innerText()).trim(), tab)
      if (tab !== '信息流') {
        assert.equal(requests.at(-1).page, 2)
        assert(await page.locator('.content-container[data-msg-id="16"]').count())
      } else {
        assert((await page.locator('.feed-card').first().innerText()).includes('12'))
      }
      console.log(`${tab}: page 2 restored after browser reload`)
    }
    await page.getByRole('button', { name: '最新', exact: true }).first().click()
    await page.waitForTimeout(1200)
    await page.getByRole('button', { name: '下一页', exact: true }).last().click()
    await page.waitForTimeout(1000)
    total = 15
    await page.reload()
    await page.waitForSelector('.content-container[data-msg-id="1"]')
    assert.equal(requests.at(-1).page, 1, 'removed pages fall back to the last available page')
    assert.deepEqual(errors, [], 'no browser runtime errors')
    console.log('Home pagination and GitHub shadow browser checks passed')
  } finally {
    await browser.close()
  }
})().catch(error => { console.error(error); process.exitCode = 1 })
