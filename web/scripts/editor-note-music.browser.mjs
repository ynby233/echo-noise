import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createRequire, stripTypeScriptTypes } from 'node:module'
import { fileURLToPath } from 'node:url'

// Run after loading the project's environment. Uses the already installed
// Playwright/browser cache; no dev server, downloads or playback requests.
// node scripts/editor-note-music.browser.mjs
const require = createRequire(import.meta.url)
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const [music, editorMusic, editor, component] = await Promise.all([
  readFile(new URL('../utils/note-music.ts', import.meta.url), 'utf8'),
  readFile(new URL('../utils/editor-note-music.ts', import.meta.url), 'utf8'),
  readFile(new URL('../utils/editor-dom-session.ts', import.meta.url), 'utf8'),
  readFile(new URL('../components/index/VditorEditor.vue', import.meta.url), 'utf8'),
])
const plainSerializer = editor.slice(editor.indexOf('const serializePlainEditorBlockText ='), editor.indexOf('const serializePlainEditorDomAsMarkdown ='))
const source = stripTypeScriptTypes([
  music.replace(/^export /gm, ''),
  editorMusic.replace(/^import[^\r\n]*\r?\n/gm, '').replace(/^export /gm, ''),
  'const MARKDOWN_BLANK_LINE_SENTINEL = "fixture-blank"; const PLAIN_EMPTY_LINE_CLASS = "fixture-empty";',
  'const isMarkdownBlankLineSentinel = () => false; const normalizeAttachmentSourceText = value => value;',
  plainSerializer,
  'globalThis.musicFixture = { createNoteMusicMarkdown, enhanceEditorNoteMusic, editorNoteMusicInfo, replaceEditorNoteMusicWithSource, bindEditorNoteMusicInteractions, serializePlainEditorBlockText };',
].join('\n'))
const browser = await chromium.launch({ headless: true, executablePath: process.env.CHROMIUM_PATH || undefined })
try {
  const page = await browser.newPage()
  page.on('pageerror', error => console.error(error.message))
  await page.setContent('<main class="vditor-container" id="root"></main>')
  await page.addStyleTag({ content: component.match(/<style[^>]*>([\s\S]*?)<\/style>/)?.[1] || '' })
  await page.addScriptTag({ path: fileURLToPath(new URL('../node_modules/vditor/dist/js/lute/lute.min.js', import.meta.url)) })
  await page.addScriptTag({ content: source })
  const results = await page.evaluate(() => {
    const { createNoteMusicMarkdown, enhanceEditorNoteMusic, editorNoteMusicInfo, replaceEditorNoteMusicWithSource, bindEditorNoteMusicInteractions, serializePlainEditorBlockText } = globalThis.musicFixture
    const root = document.querySelector('#root')
    const lute = globalThis.Lute.New()
    const markdown = createNoteMusicMarkdown({ trackID: 'local-track_42', title: '音乐 标题', artist: '歌手' })
    const checks = []
    const check = (condition, name) => { if (!condition) throw new Error(name); checks.push(name) }
    for (const mode of ['ir', 'wysiwyg']) {
      root.innerHTML = `<div class="vditor-${mode}"><pre class="vditor-reset" contenteditable="true"></pre></div>`
      const editable = root.querySelector('pre')
      const fromMd = value => mode === 'ir' ? lute.Md2VditorIRDOM(value) : lute.Md2VditorDOM(value)
      const toMd = () => (mode === 'ir' ? lute.VditorIRDOM2Md(editable.innerHTML) : lute.VditorDOM2Md(editable.innerHTML)).trim()
      editable.innerHTML = fromMd(`上文 ${markdown} 下文\n\n移动目标\n\n[普通链接](https://example.com)\n\n\`${markdown}\`\n\n\`\`\`text\n${markdown}\n\`\`\``)
      const original = toMd()
      const played = []
      const dispose = bindEditorNoteMusicInteractions(root, async request => { played.push(request) })
      enhanceEditorNoteMusic(root)
      enhanceEditorNoteMusic(root)
      const marker = root.querySelector('.editor-note-music-node, a.editor-note-music-link')
      check(!!marker && marker.getAttribute('contenteditable') === 'false', `${mode}: atomic music marker`)
      check(toMd() === original, `${mode}: enhancement preserves native Lute/getValue output including ordinary links and code`)
      check(marker.getAttribute('data-note-music-hint') === '点击试听', `${mode}: draft playback hint`)
      check(root.querySelectorAll('.editor-note-music-node, a.editor-note-music-link').length === 1, `${mode}: code examples are not enhanced`)
      if (mode === 'ir') {
        marker.classList.add('vditor-ir__node--expand')
        check([...marker.querySelectorAll('.vditor-ir__marker')].every(node => getComputedStyle(node).display === 'none'), 'ir: URL and syntax remain hidden when Vditor expands the focused node')
      }
      const block = marker.closest('p[data-block], div[data-block]')
      check(serializePlainEditorBlockText(block).includes(markdown), `${mode}: actual plain-block fallback retains full music Markdown`)
      const clone = editable.cloneNode(true)
      replaceEditorNoteMusicWithSource(clone)
      check(clone.textContent.includes(markdown), `${mode}: table/text serialization clones retain the track ID`)
      for (const type of ['click', 'auxclick']) {
        const event = new MouseEvent(type, { bubbles: true, cancelable: true, button: type === 'auxclick' ? 1 : 0 })
        marker.dispatchEvent(event)
        check(event.defaultPrevented, `${mode}: ${type} cannot navigate`)
      }
      check(played.length === 1 && played[0].trackID === 'local-track_42' && played[0].draft === true && played[0].messageID === 0, `${mode}: clicking a draft uses temporary preview without a published note ID`)
      const caret = window.getSelection().getRangeAt(0)
      check(caret.collapsed && caret.startContainer === marker.parentNode && caret.startOffset === [...marker.parentNode.childNodes].indexOf(marker) + 1, `${mode}: click leaves the caret after the marker for continued typing`)
      for (const key of ['Enter', ' ']) {
        const typing = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
        editable.dispatchEvent(typing)
        check(!typing.defaultPrevented, `${mode}: ${JSON.stringify(key)} from editable remains available for typing`)
      }
      const enter = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
      marker.dispatchEvent(enter)
      check(enter.defaultPrevented, `${mode}: keyboard activation cannot navigate`)
      let ordinaryWasPrevented = true
      const ordinary = [...editable.querySelectorAll('a, [data-type="a"]')].find(node => node.textContent.includes('普通链接'))
      ordinary.addEventListener('click', event => { ordinaryWasPrevented = event.defaultPrevented; event.preventDefault() }, { once: true })
      ordinary.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
      check(!ordinaryWasPrevented, `${mode}: ordinary link events are left to Vditor`)

      check(marker.getAttribute('draggable') === 'false', `${mode}: browser dragging cannot split a music marker into raw link fragments`)
      marker.focus()
      const remove = new KeyboardEvent('keydown', { key: 'Delete', bubbles: true, cancelable: true })
      marker.dispatchEvent(remove)
      check(remove.defaultPrevented && !marker.isConnected, `${mode}: focused marker deletes atomically`)
      check(!serializePlainEditorBlockText(block).includes('/music/notes/'), `${mode}: deletion is reflected in outgoing Markdown`)
      check(toMd().includes('https://example.com') && toMd().includes('local-track_42'), `${mode}: ordinary link and code samples survive marker deletion`)
      check(document.execCommand('undo'), `${mode}: native deletion can be undone`)
      check(serializePlainEditorBlockText(block).includes(markdown), `${mode}: undo restores complete music source`)
      dispose()
    }
    root.innerHTML = '<div class="vditor-preview"><a href="/music/notes/local-track_42">♫ 预览音乐</a><a href="https://example.com">普通</a><code><a href="/music/notes/code">代码示例</a></code></div>'
    const dispose = bindEditorNoteMusicInteractions(root)
    enhanceEditorNoteMusic(root, true)
    enhanceEditorNoteMusic(root)
    const preview = root.querySelector('.editor-note-music-link')
    check(!preview.hasAttribute('href') && preview.getAttribute('draggable') === 'false', 'built-in preview has no navigable or draggable URL')
    check(editorNoteMusicInfo(preview).href === '/music/notes/local-track_42', 'preview retains the storage marker separately')
    check(root.querySelector('code a').getAttribute('href') === '/music/notes/code', 'code-contained links are untouched')
    const click = new MouseEvent('click', { bubbles: true, cancelable: true })
    preview.dispatchEvent(click)
    check(click.defaultPrevented, 'built-in preview activation is blocked')
    dispose()
    let afterDispose = true
    preview.addEventListener('click', event => { afterDispose = event.defaultPrevented })
    preview.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    check(!afterDispose, 'disposing removes the music event handlers')
    return checks
  })
  assert.ok(results.length >= 30)
  console.log(`Editor note music browser behavior checks passed (${results.length} checks)`)
} finally {
  await browser.close()
}
