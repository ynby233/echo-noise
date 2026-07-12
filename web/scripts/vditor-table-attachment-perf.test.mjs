import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const sourcePath = fileURLToPath(new URL('../components/index/VditorEditor.vue', import.meta.url))
const source = await readFile(sourcePath, 'utf8')

const insertion = source.match(
  /const insertAttachmentIntoTableCellWithoutVditorReset[\s\S]*?\n}\n\nconst insertValueIntoCurrentTableCell/
)
assert.ok(insertion, '应能找到表格附件插入路径')

const insertionSource = insertion[0]
assert.match(
  insertionSource,
  /refreshAttachmentLinksInTableCellFromEditor\(cell\)/,
  '表格附件插入必须只刷新目标单元格'
)
assert.doesNotMatch(
  insertionSource,
  /refreshAttachmentLinksFromEditor\(\)/,
  '表格附件插入不得触发全编辑器附件扫描'
)
assert.doesNotMatch(
  insertionSource,
  /setTimeout\(\(\) => refreshAttachmentLinksFromEditor\(\), 0\)/,
  '表格附件插入不得重复排入全编辑器附件扫描'
)

const inputStart = source.indexOf('input: (content: string) => {')
const inputEnd = source.indexOf('preview:', inputStart)
assert.ok(inputStart >= 0 && inputEnd > inputStart, '应能找到 Vditor input 回调')
const input = source.slice(inputStart, inputEnd)
assert.doesNotMatch(
  input,
  /window\.setTimeout\(emitSafeValue, (48|160)\)/,
  '表格输入不得安排多轮延迟全量序列化'
)

console.log('vditor table attachment performance regression: passed')
