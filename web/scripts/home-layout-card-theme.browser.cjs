// Run against the generated homepage served at TEST_BASE_URL.
const assert = require('node:assert/strict')
const path = require('node:path')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

const content = [
  '[文件附件：报告.txt](/api/files/report.txt)',
  '[♫ EVA](/music/notes/track_42)',
  '[音频附件：录音.wav](/api/audio/voice.wav)',
].join('\n\n')

async function main() {
  const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined })
  try {
    const page = await browser.newPage({ viewport: { width: 1920, height: 1080 }, serviceWorkers: 'block' })
    const vditor = path.dirname(require.resolve('vditor/package.json'))
    await page.route('**/vditor@*/dist/**', route => {
      const resource = new URL(route.request().url()).pathname.split('/dist/').pop()
      return route.fulfill({ path: path.join(vditor, 'dist', resource) })
    })
    await page.route('**/api/**', route => {
      const pathname = new URL(route.request().url()).pathname
      if (pathname.endsWith('/setup/status')) return route.fulfill({ status: 404, json: {} })
      if (pathname.endsWith('/audio/voice.wav')) return route.fulfill({
        contentType: 'audio/wav',
        // Eight silent samples in a mono PCM WAV.
        body: Buffer.from('UklGRiwAAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQgAAACAgICAgICAgA==', 'base64'),
      })
      let data = {}
      if (pathname.endsWith('/frontend/config')) data = { frontendSettings: { homeLayoutDefault: 'single' } }
      if (pathname.endsWith('/messages/page')) data = { total: 1, items: [{ id: 1, content, username: 'fixture', user_id: 1, visibility: 'public', created_at: '2026-11-01T00:00:00Z' }] }
      return route.fulfill({ json: { code: 1, data } })
    })
    await page.addInitScript(() => localStorage.setItem('contentTheme', 'dark'))
    await page.goto(process.env.TEST_BASE_URL || 'http://127.0.0.1:3112/', { waitUntil: 'domcontentloaded' })
    const preview = page.locator('[data-msg-id="1"] .markdown-preview')
    const cards = preview.locator('.site-attachment-file, .note-music-marker, .site-attachment-audio')
    const colors = () => cards.evaluateAll(async elements => {
      // Read the final background after the cards' CSS transitions settle.
      await Promise.all(elements.flatMap(el => el.getAnimations().map(animation => animation.finished)))
      return elements.map(el => getComputedStyle(el).backgroundColor)
    })
    await preview.locator('.site-attachment-audio__play').waitFor()
    const dark = await colors()
    assert.equal(dark.length, 3)
    for (const [current, nextGrid] of [['单栏', 'grid-masonry'], ['瀑布流', 'grid-3']]) {
      await page.getByRole('button', { name: current, exact: true }).click()
      await page.locator(`.layout-container.${nextGrid}`).waitFor()
      assert.deepEqual(await colors(), dark, `${current}: card backgrounds changed`)
      assert.ok(await preview.evaluate(el => el.matches('.theme-dark.vditor-reset')), `${current}: preview lost its rendering classes`)
    }
    await page.getByRole('button', { name: '切换亮暗' }).click()
    await page.locator('[data-msg-id="1"] .markdown-preview.theme-light').waitFor()
    const light = await colors()
    for (let i = 0; i < dark.length; i++) assert.notEqual(light[i], dark[i], `card ${i} must respond to theme changes`)
    console.log('Homepage card backgrounds preserved across layout switches:', dark)
  } finally {
    await browser.close()
  }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
