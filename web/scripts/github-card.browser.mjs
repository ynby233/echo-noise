// Run with PLAYWRIGHT_MODULE and CHROMIUM_PATH pointing to the existing browser test runtime.
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'

const require = createRequire(import.meta.url)
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, headless: true })
try {
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } })
  await page.setContent('<main></main>')
  await page.addScriptTag({ path: fileURLToPath(new URL('../node_modules/vditor/dist/js/lute/lute.min.js', import.meta.url)) })
  const source = await readFile(new URL('../utils/github-card.ts', import.meta.url), 'utf8')
  const renderer = await readFile(new URL('../components/index/MarkdownRenderer.vue', import.meta.url), 'utf8')
  const css = renderer.split('<style>')[1].split('</style>')[0]
  await page.addStyleTag({content: css + '\n.vditor-reset svg {width:auto;height:auto} '})
  const js = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
  const result = await page.evaluate(async js => {
    const { enhanceGitHubCards } = await import('data:text/javascript;charset=utf-8,' + encodeURIComponent(js))
    const url = 'https://github.com/AlkaidLab/foundation-sunshine'
    const lute = window.Lute.New()
    const inputs = [url, `正文前 ${url} 正文后`, `| 仓库 | 混排 |\n| --- | --- |\n| ${url} | 你好<br>${url}<br>结尾 |`,
      `<table><tr><td>${url}</td><td>你好 ${url}<br>结尾</td></tr></table>`,
      `普通文字 [项目](${url}) 后文\n\n| 仓库 |\n| --- |\n| [项目](${url}) |`,
      `\`${url}\`\n\n\`\`\`text\n${url}\n\`\`\``, `![image](${url}/raw/main/image.png)`]
    let calls = 0
    let resolveResponse
    const response = new Promise(resolve => { resolveResponse = resolve })
    window.fetch = async () => { calls++; return response }
    const roots = inputs.map(markdown => {
      const root = document.createElement('section')
      root.innerHTML = lute.Md2HTML(markdown)
      document.querySelector('main').append(root)
      return root
    })
    roots.forEach(root => {root.className = 'markdown-preview vditor-reset'})
    const tasks = roots.map(root => enhanceGitHubCards(root))
    const before = roots.map(root => ({ cards: root.querySelectorAll('.github-card').length, links: root.querySelectorAll('.github-card-title').length }))
    const clone = roots[2].cloneNode(true)
    document.querySelector('main').append(clone)
    tasks.push(enhanceGitHubCards(clone))
    resolveResponse({ ok: true, json: async () => ({ full_name: '<img src=x onerror=alert(1)>' }) })
    await Promise.all(tasks)
    await enhanceGitHubCards(roots[2])
    const results = roots.map(root => ({ cards: root.querySelectorAll('.github-card').length, loaded: root.querySelectorAll('.github-card-loaded').length, tableCards: root.querySelectorAll('td .github-card').length, text: root.textContent, unsafeImages: root.querySelectorAll('.github-card-title img').length }))
    window.fetch = async () => { throw Error('offline') }
    const failure = document.createElement('section')
    failure.innerHTML = lute.Md2HTML('https://github.com/example/unavailable')
    document.body.append(failure)
    await enhanceGitHubCards(failure)
    const retryClone = failure.cloneNode(true)
    window.fetch = async () => ({ ok: true, json: async () => ({ full_name: 'example/unavailable' }) })
    await enhanceGitHubCards(retryClone)
    const badgeWidths = [...document.querySelectorAll('.github-card .gh-badge')].map(e=>e.getBoundingClientRect().width)
    return { badgeWidths, before, results, calls, cloneLoaded: clone.querySelectorAll('.github-card-loaded').length,
      failedLink: failure.querySelector('.github-card-title')?.href, failedLoading: failure.querySelectorAll('.github-card-loading').length,
      retryLoaded: retryClone.querySelectorAll('.github-card-loaded').length }
  }, js)
  assert.deepEqual(result.before.map(r => r.cards), [1, 1, 2, 2, 2, 0, 0])
  assert.deepEqual(result.before.map(r => r.links), [1, 1, 2, 2, 2, 0, 0], 'usable cards exist before the API responds')
  assert.deepEqual(result.results.map(r => r.loaded), [1, 1, 2, 2, 2, 0, 0])
  assert.deepEqual(result.results.map(r => r.tableCards), [0, 0, 2, 2, 1, 0, 0])
  assert(result.badgeWidths.every(width=>width===20), 'Vditor SVG styles must not enlarge GitHub badge')
  assert.equal(result.calls, 1, 'duplicate cards share one request')
  assert.equal(result.cloneLoaded, 2, 'expanded table cloned during loading finishes independently')
  assert.equal(result.results.reduce((n, r) => n + r.unsafeImages, 0), 0, 'API text cannot inject HTML')
  assert.match(result.results[1].text, /正文前[\s\S]*正文后/)
  assert.match(result.results[2].text, /你好[\s\S]*结尾/)
  assert.doesNotMatch(result.results[5].text, /github-card|Loading/)
  assert.equal(result.failedLink, 'https://github.com/example/unavailable')
  assert.equal(result.failedLoading, 0)
  assert.equal(result.retryLoaded, 1, 'failed requests are not cached permanently')
  console.log('GitHub cards: Markdown/HTML tables, prose, mixed content, duplicate instances, code/image exclusion, pending clones, failure and retry passed')
} finally { await browser.close() }
