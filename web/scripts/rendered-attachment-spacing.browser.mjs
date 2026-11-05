// Uses the existing browser runtime; no server, download, or media requests.
// PLAYWRIGHT_MODULE=<installed playwright> CHROMIUM_PATH=<existing chromium>
// node scripts/rendered-attachment-spacing.browser.mjs
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { build } from 'esbuild'

const require = createRequire(import.meta.url)
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const webRoot = fileURLToPath(new URL('../', import.meta.url))
const [bundle, renderer, spacing, audioCss, vditorCss] = await Promise.all([
  build({
    stdin: {
      contents: `export * from './utils/rendered-attachment.ts'; export * from './utils/markdown-blank-lines.ts'; export { buildAttachmentAudioMarkdown, enhanceAttachmentAudioPlayers, destroyAttachmentAudioPlayers } from './utils/attachment-audio-player.ts'; export { enhanceNoteMusicLinks } from './utils/rendered-note-music.ts';`,
      resolveDir: webRoot,
      loader: 'ts',
    },
    bundle: true, write: false, platform: 'browser', format: 'iife', globalName: 'spacingFixture',
  }),
  readFile(new URL('../components/index/MarkdownRenderer.vue', import.meta.url), 'utf8'),
  readFile(new URL('../assets/css/markdown-line-spacing.css', import.meta.url), 'utf8'),
  readFile(new URL('../assets/css/attachment-audio-player.css', import.meta.url), 'utf8'),
  readFile(new URL('../node_modules/vditor/dist/index.css', import.meta.url), 'utf8'),
])
const browser = await chromium.launch({ headless: true, executablePath: process.env.CHROMIUM_PATH || undefined })
try {
  const page = await browser.newPage({ viewport: { width: 960, height: 900 } })
  await page.route('**/api/**', route => route.abort())
  await page.setContent('<main></main>')
  // Nuxt global styles load before the component styles. Keep that cascade here.
  await page.addStyleTag({ content: `${vditorCss}\n${audioCss}\n${spacing}\n${renderer.match(/<style[^>]*>([\s\S]*?)<\/style>/)?.[1] || ''}\nmain > section { width: 560px; font-size: 16px; }` })
  await page.addScriptTag({ path: fileURLToPath(new URL('../node_modules/vditor/dist/js/lute/lute.min.js', import.meta.url)) })
  await page.addScriptTag({ content: bundle.outputFiles[0].text })
  const results = await page.evaluate(() => {
    const fixture = globalThis.spacingFixture
    const lute = globalThis.Lute.New()
    lute.SetSoftBreak2HardBreak(true)
    const checks = []
    const check = (value, label) => { if (!value) throw new Error(label); checks.push(label) }
    const close = (actual, expected, label) => check(Math.abs(actual - expected) < 0.75, `${label}: ${actual} vs ${expected}`)
    const sources = {
      file: '[文件附件：报告.pdf](/api/files/report.pdf)',
      voice: '[音频附件：录音.webm](/api/audio/voice.webm)',
      music: '[♫ 本地音乐](/music/notes/track_42)',
    }
    const cardSelector = '.site-attachment-file, .site-attachment-audio, .note-music-marker'
    const render = (markdown, classes = 'markdown-preview vditor-reset') => {
      const root = document.createElement('section')
      root.className = classes
      root.innerHTML = lute.Md2HTML(fixture.encodeMarkdownExtraBlankLines(markdown))
      document.querySelector('main').append(root)
      fixture.markMarkdownPreservedBlankLineElements(root)
      fixture.applyAttachmentRenders(root, '/api')
      fixture.enhanceNoteMusicLinks(root, 1, async () => {}, () => {})
      fixture.normalizeMarkdownAttachmentParagraphs(root)
      fixture.enhanceAttachmentAudioPlayers(root)
      root.querySelectorAll('.site-attachment-audio').forEach(card => check(!!card.querySelector('audio') && !!card.querySelector('.site-attachment-audio__play'), 'recording cards mount actual audio controls'))
      return root
    }
    const cards = root => [...root.querySelectorAll(cardSelector)]
    const gaps = root => cards(root).slice(1).map((card, index) => card.getBoundingClientRect().top - cards(root)[index].getBoundingClientRect().bottom)
    const rows = root => [...root.querySelectorAll('.markdown-attachment-paragraph')]
    const lineHeight = 25.6

    for (const first of Object.keys(sources)) for (const second of Object.keys(sources)) {
      for (const breaks of [1, 2, 3, 4]) {
        const root = render(`${sources[first]}${'\n'.repeat(breaks)}${sources[second]}`)
        check(cards(root).length === 2, `${first}/${second}/${breaks}: both cards survive`)
        close(gaps(root)[0], 12 + Math.max(0, breaks - 1) * lineHeight, `${first}/${second}/${breaks}: exact source gap`)
        check(rows(root).length === (breaks === 1 ? 1 : 2), `${first}/${second}/${breaks}: paragraph boundaries survive`)
        const html = root.innerHTML
        fixture.normalizeMarkdownAttachmentParagraphs(root)
        check(root.innerHTML === html, `${first}/${second}/${breaks}: normalization is idempotent`)
        root.remove()
      }
    }

    for (const order of [['file', 'voice', 'music'], ['music', 'voice', 'file']]) {
      for (const separators of [[1, 2], [2, 1], [1, 3], [3, 1]]) {
        const root = render(sources[order[0]] + '\n'.repeat(separators[0]) + sources[order[1]] + '\n'.repeat(separators[1]) + sources[order[2]])
        gaps(root).forEach((gap, index) => close(gap, 12 + (separators[index] - 1) * lineHeight, `${order}/${separators}: gap stays in source position ${index}`))
        root.remove()
      }
    }

    for (const kind of Object.keys(sources)) {
      const root = render(`上文\n${sources[kind]}\n下文\n\n再下一段`)
      const block = rows(root)[0]
      check(block.children.length === 3, `${kind}: text/card/text produces exactly three rows`)
      check(block.children[0].textContent === '上文' && block.children[2].textContent.trim() === '下文', `${kind}: text stays in order`)
      check(!block.querySelector('br'), `${kind}: soft break does not leave an anonymous BR row`)
      close(root.lastElementChild.getBoundingClientRect().top - block.getBoundingClientRect().bottom, lineHeight, `${kind}: following prose keeps paragraph separator`)
      root.remove()
    }

    const blank = render(`${sources.voice}<br><br>${sources.file}`)
    check(blank.querySelectorAll('.markdown-attachment-line--blank').length === 1, 'two explicit BRs retain one empty row')
    close(gaps(blank)[0], 12 + lineHeight, 'explicit BR blank row has normal line height')
    blank.remove()

    const nested = render(`**上文\n${sources.voice}\n下文**`)
    check(rows(nested)[0].children.length === 3 && rows(nested)[0].querySelectorAll('strong').length === 3, 'nested inline formatting splits at BR without losing emphasis or order')
    nested.remove()

    // Bare audio uses a Markdown link until the renderer builds its card.
    const placeholder = fixture.buildAttachmentAudioMarkdown('/api/audio/voice.webm')
    const raw = render(`${placeholder}\n**后续加粗**\n${sources.file}\n尾行`)
    check(!!raw.querySelector('strong') && cards(raw).length === 2 && raw.textContent.includes('尾行'), 'bare audio conversion cannot swallow following Markdown lines')
    raw.remove()

    const tableSource = `| 附件 | 内容 |\n| --- | --- |\n| ${sources.voice}<br>${sources.file} | 文字<br>后文 |`
    const table = render(tableSource)
    const before = table.querySelector('table').outerHTML
    fixture.normalizeMarkdownAttachmentParagraphs(table)
    check(table.querySelector('table').outerHTML === before && !table.querySelector('table .markdown-attachment-line'), 'table cell breaks and structure remain untouched')
    table.remove()

    const code = render('```md\n' + sources.voice + '\n\n\n' + sources.file + '\n```\n\n`' + sources.file + '`')
    check(cards(code).length === 0 && code.querySelector('pre code').textContent.includes('\n\n\n'), 'code examples and their blank lines remain source text')
    code.remove()

    const container = document.createElement('section')
    container.innerHTML = '<div><p>\u00a0</p><span class="site-attachment-audio" data-site-audio-player></span></div>'
    fixture.markMarkdownPreservedBlankLineElements(container)
    check(!!container.querySelector('.site-attachment-audio') && container.querySelectorAll('.markdown-preserved-blank-line').length === 1, 'blank-line marking cannot consume an empty audio sibling')

    // The shared preview stylesheet uses the same block contract.
    const editor = document.createElement('div')
    editor.className = 'vditor-container'
    editor.innerHTML = '<div class="vditor-preview"></div>'
    document.body.append(editor)
    const preview = render(`${sources.file}\n\n${sources.voice}`, 'vditor-reset')
    editor.firstElementChild.append(preview)
    const previewRows = rows(preview)
    close(previewRows[1].getBoundingClientRect().top - previewRows[0].getBoundingClientRect().bottom, parseFloat(getComputedStyle(previewRows[1]).fontSize) * 1.6, 'editor preview preserves one blank line between attachment paragraphs')
    editor.remove()
    return checks
  })
  assert.ok(results.length > 150)
  console.log(`Rendered attachment spacing browser checks passed (${results.length} checks)`)
} finally {
  await browser.close()
}
