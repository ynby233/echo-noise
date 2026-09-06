import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire, stripTypeScriptTypes } from 'node:module'
import { execFileSync } from 'node:child_process'
import vm from 'node:vm'
import { createJiti } from 'jiti'

const require = createRequire(import.meta.url)
require('../node_modules/vditor/dist/js/lute/lute.min.js')
const { serializeMarkdownEditorBlocks, encodeMarkdownExtraBlankLines, MARKDOWN_BLANK_LINE_SENTINEL } = await createJiti(import.meta.url).import('../utils/markdown-blank-lines.ts')
const source = process.argv.includes('--head')
  ? execFileSync('git', ['show', 'HEAD:web/components/index/VditorEditor.vue'], { encoding: 'utf8' })
  : readFileSync(new URL('../components/index/VditorEditor.vue', import.meta.url), 'utf8')
class Element {
  constructor(tag, html, text) { this.tag = tag; this.outerHTML = html; this.textContent = text }
  matches(selector) { return selector.split(',').some(s => s.trim() === this.tag) }
  querySelector() { return null }
}
class HTMLTableElement extends Element {}
const lute = globalThis.Lute.New()
const context = vm.createContext({
  Element, HTMLTableElement, serializeMarkdownEditorBlocks, encodeMarkdownExtraBlankLines, MARKDOWN_BLANK_LINE_SENTINEL,
  normalizeAttachmentSourceText: value => value,
  vditorInstance: { vditor: { currentMode: 'ir', lute } },
})
const start = source.indexOf('const serializePlainEditorDomAsMarkdown =')
const end = source.indexOf('const getEditorDomContentFallback =', start)
vm.runInContext(stripTypeScriptTypes(source.slice(start, end) + '\nglobalThis.serialize = serializePlainEditorDomAsMarkdown; globalThis.serializeWithTables = serializeEditorDomAsMarkdown;'), context)
const text = value => ({ textContent: value })
for (const [tag, html, expected] of [
  ['ul', '<ul data-block="0"><li class="vditor-task"><input type="checkbox"> 待办</li><li class="vditor-task"><input type="checkbox" checked> 完成</li></ul>', /\* \[ \] +待办\n\* \[X\] +完成/i],
  ['ol', '<ol data-block="0"><li>第一项</li><li>第二项</li></ol>', /1\. 第一项\n[12]\. 第二项/],
  ['ul', '<ul data-block="0"><li>第一项</li><li>第二项</li></ul>', /\* 第一项\n\* 第二项/],
  ['blockquote', '<blockquote data-block="0"><p data-block="0">引用</p></blockquote>', /> 引用/],
]) {
  for (const serialize of [context.serialize, context.serializeWithTables]) {
    const result = serialize({ childNodes: [text('上文'), new Element(tag, html, '待办完成'), text('下文')] })
    assert.match(result, expected, `${tag} must retain Markdown structure between text blocks`)
    assert.match(result, /^上文\n\n/, 'formatted block must not join the preceding text')
    assert.match(result, /\n\n下文$/, 'following text must not become part of the formatted block')
  }
}
assert.equal(context.serialize({ childNodes: [text('第一行'), text(''), text('第二行')] }), '第一行\n第二行')
console.log('editor formatted block serialization tests passed')
