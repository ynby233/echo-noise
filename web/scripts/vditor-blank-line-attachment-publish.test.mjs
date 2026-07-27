import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createJiti } from 'jiti'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const jiti = createJiti(import.meta.url)
const {
  MARKDOWN_BLANK_LINE_SENTINEL,
  isMarkdownBlankLineSentinel,
  serializeMarkdownEditorBlocks,
} = await jiti.import(join(webRoot, 'utils/markdown-blank-lines.ts'))

const editorSource = await readFile(join(webRoot, 'components/index/VditorEditor.vue'), 'utf8')

// 根因回归：在编辑器空白区域点击会把当前块标记为空行占位块；
// 若随后仅上传附件就发布，DOM 序列化必须依据块内真实内容，而不能只凭标记类判空，
// 否则附件 Markdown 会被静默丢弃，发布时被误判为“请输入内容或上传附件”。

const blockTextFn = editorSource.match(/const serializePlainEditorBlockText = \(block: Element\) => \{[\s\S]*?\n\}/)
assert.ok(blockTextFn, '必须能定位 serializePlainEditorBlockText')
const blockTextSource = blockTextFn[0]

assert.match(
  blockTextSource,
  /const isBlankLineDom = isMarkdownBlankLineSentinel\(rawText\)/,
  '空行判定必须先根据块内真实文本内容计算'
)
assert.match(
  blockTextSource,
  /if \(isBlankLineDom && block\.classList\.contains\(PLAIN_EMPTY_LINE_CLASS\)\) return ''/,
  '仅当块内确实为空时，空行占位类才可序列化为空字符串'
)
assert.match(
  blockTextSource,
  /if \(isBlankLineDom && \(block\.classList\.contains\('vditor-preserved-blank-line'\)/,
  '仅当块内确实为空时，保留空行类才可序列化为空行哨兵'
)
assert.doesNotMatch(
  blockTextSource,
  /\n  if \(block\.classList\.contains\(PLAIN_EMPTY_LINE_CLASS\)\) return ''/,
  '不得仅凭标记类判空，否则空行块内的附件会被丢弃'
)

// 插入附件后必须清理已失效的空行标记类，避免 DOM 状态与真实内容长期不一致。
assert.match(
  editorSource,
  /const clearStalePlainBlankLineMarkers = \(\) => \{/,
  '必须提供失效空行标记清理逻辑'
)
const insertFn = editorSource.match(/const insertNormalizedEditorValue = \(nextValue: string\) => \{[\s\S]*?\n\}/)
assert.ok(insertFn, '必须能定位 insertNormalizedEditorValue')
assert.match(
  insertFn[0],
  /clearStalePlainBlankLineMarkers\(\)/,
  '插入附件后必须清理失效的空行标记类'
)

// 契约层面：附件所在行必须作为真实内容参与序列化，而不是被折叠成空行。
const attachmentLine = `[文件附件：note.txt](/api/files/note.txt)`
assert.equal(isMarkdownBlankLineSentinel(attachmentLine), false, '附件行不得被判定为空行')
assert.equal(
  serializeMarkdownEditorBlocks([attachmentLine]),
  attachmentLine,
  '仅含附件的单块内容必须原样保留，发布内容不能为空'
)
assert.equal(
  serializeMarkdownEditorBlocks([MARKDOWN_BLANK_LINE_SENTINEL, attachmentLine]),
  `${MARKDOWN_BLANK_LINE_SENTINEL}\n\n${attachmentLine}`,
  '空白区域点击产生的空行块之后，附件行仍必须保留'
)

console.log('vditor blank-line attachment publish tests passed')
