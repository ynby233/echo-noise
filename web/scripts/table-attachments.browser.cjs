// Run against the local production build. Synthetic API and isolated browser
// storage keep this check independent of real accounts, uploads and notes.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const http = require('node:http')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

const output = path.resolve(process.env.TEST_OUTPUT_ROOT || path.join(__dirname, '../.output/public'))
assert.ok(fs.existsSync(path.join(output, 'index.html')), 'Run npm run generate first')
const longFile = '[文件附件：这是一个长度超过二十四个字符的长文件名称用于回归测试报告.pdf](/api/files/one.pdf)'
const otherFile = '[文件附件：这是一个长度超过二十四个字符的长文件名称用于另一个测试报告.pdf](/api/files/two.pdf)'
const longAudio = '[音频附件：另一个长度超过二十四个字符的长音频名称用于顺序回归测试.webm](/api/audio/long.webm)'
const cases = [
  ['long-name', longFile],
  ['same-truncated-name-and-mixed', `前${longFile}中${otherFile}后${longAudio}末[文件附件：short.txt](/api/files/short.txt)`],
  ['same-name-different-url', '[文件附件：same.txt](/api/files/one.txt)中[文件附件：same.txt](/api/files/two.txt)'],
  ['edge-and-inner-breaks', `<br />前${longFile}<br /><br />后${longAudio}<br />`],
  ['image-audio-file', `前[图片附件：image.png](/api/images/image.png)中${longAudio}后${longFile}`],
]
const server = http.createServer((req, res) => {
  const pathname = new URL(req.url, 'http://local').pathname
  const file = path.resolve(output, '.' + decodeURIComponent(pathname === '/' ? '/index.html' : pathname))
  if (!file.startsWith(output + path.sep)) return res.writeHead(403).end()
  if (!fs.existsSync(file) || !fs.statSync(file).isFile()) return res.writeHead(404).end()
  res.setHeader('Content-Type', { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml' }[path.extname(file)] || 'application/octet-stream')
  res.end(fs.readFileSync(file))
})

;(async () => {
  let browser
  try {
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
    browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, headless: true })
    for (const [name, cell] of cases) {
      const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, serviceWorkers: 'block' })
      try {
        const content = `| 类型 | 内容 |\n| --- | --- |\n| 附件 | ${cell} |`
        await context.addInitScript(content => {
          localStorage.setItem('homeLayoutDesktop', 'masonry')
          localStorage.setItem('addform_draft_v1', JSON.stringify({ content, visibility: 'public' }))
        }, content)
        const page = await context.newPage()
        const errors = []
        page.on('pageerror', error => errors.push(error.message))
        await page.route('**/api/**', route => {
          const pathname = new URL(route.request().url()).pathname
          if (pathname.endsWith('/setup/status')) return route.fulfill({ status: 404, json: {} })
          let data = {}
          if (pathname.endsWith('/user')) data = { id: 1, userid: 1, username: 'fixture', is_admin: true }
          if (pathname.endsWith('/frontend/config')) data = { frontendSettings: { announcementEnabled: false, pwaEnabled: false, musicEnabled: false, homeLayoutDefault: 'masonry', commentEnabled: true } }
          if (pathname.endsWith('/messages/page')) data = { total: 0, items: [] }
          if (pathname.endsWith('/announcements/unread')) data = { items: [] }
          if (pathname.endsWith('/comments')) data = []
          return route.fulfill({ json: { code: 1, data } })
        })
        // Use the installed Vditor resources, avoiding CDN availability as a
        // prerequisite for a table serialization test.
        const vditorRoot = path.dirname(require.resolve('vditor/package.json'))
        await page.route('**/vditor@*/**', route => {
          const relative = new URL(route.request().url()).pathname.split(/vditor@[^/]+\//)[1]
          const file = path.resolve(vditorRoot, relative || '')
          if (!file.startsWith(vditorRoot + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) return route.abort()
          return route.fulfill({ path: file })
        })
        await page.goto(`http://127.0.0.1:${server.address().port}`)
        await page.getByRole('button', { name: '写笔记', exact: true }).click()
        const editor = page.locator('.vditor-ir [contenteditable="true"]').first()
        await editor.locator('table').waitFor()
        const open = async () => {
          await editor.locator('td').first().hover()
          await page.getByRole('button', { name: '放大显示该表格', exact: true }).click()
          return page.getByRole('dialog', { name: '放大显示表格', exact: true })
        }
        const closeAndRead = async (dialog, marker) => {
          await page.getByRole('button', { name: '关闭放大表格', exact: true }).click()
          await dialog.waitFor({ state: 'detached' })
          await page.waitForFunction(marker => JSON.parse(localStorage.addform_draft_v1).content.includes(marker), marker)
          return page.evaluate(() => JSON.parse(localStorage.addform_draft_v1).content.trim())
        }
        for (let cycle = 0; cycle < 3; cycle++) {
          const dialog = await open()
          const marker = `修改${cycle}`
          await dialog.locator('[data-expanded-row="1"][data-expanded-cell="0"]').fill(marker)
          assert.equal(await closeAndRead(dialog, marker), content.replace('| 附件 |', `| ${marker} |`), `${name}, cycle ${cycle}: saved source changed`)
        }
        // Deliberate replacement must not resurrect any old attachment source.
        for (let cycle = 0; cycle < 2; cycle++) {
          const dialog = await open()
          const marker = `纯文本${cycle}`
          await dialog.locator('[data-expanded-row="1"][data-expanded-cell="1"]').fill(marker)
          assert.equal(await closeAndRead(dialog, marker), `| 类型 | 内容 |\n| --- | --- |\n| 修改2 | ${marker} |`, `${name}: removed attachment returned`)
        }
        assert.deepEqual(errors, [], `${name}: browser errors`)
        console.log(`Passed: ${name} (3 edits, 2 replacements)`)
      } finally {
        await context.close()
      }
    }
  } finally {
    await browser?.close()
    await new Promise(resolve => server.close(resolve))
  }
})().catch(error => { console.error(error); process.exitCode = 1 })
