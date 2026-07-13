import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import {
  insertEditorValueFallback,
  replaceTableSourceLine,
  resolveTableAttachmentTarget,
} from '../utils/vditor-table-attachment.ts'

const attachment = '[文件附件：report.pdf](/api/files/report.pdf)'
const calls = []
const route = insertEditorValueFallback({
  insertMD: (value) => calls.push(['insertMD', value]),
  insertValue: (value) => calls.push(['insertValue', value]),
}, attachment, true)

assert.equal(route, 'attachment-markdown', '附件回退必须使用 Markdown 插入路径')
assert.deepEqual(calls, [['insertMD', attachment]], '附件 Markdown 绝不能进入 Vditor.insertValue')
calls.length = 0
const plainRoute = insertEditorValueFallback({
  insertMD: (value) => calls.push(['insertMD', value]),
  insertValue: (value) => calls.push(['insertValue', value]),
}, '普通文本', false)
assert.equal(plainRoute, 'value')
assert.deepEqual(calls, [['insertValue', '普通文本']], '非附件插入必须保留 Vditor 原有路径')

const editable = { connected: true }
const target = { editable, tableIndex: 2, rowIndex: 3, cellIndex: 4 }
const resolved = resolveTableAttachmentTarget(
  target,
  (candidate) => candidate.connected,
  (candidate) => `${candidate.tableIndex}:${candidate.rowIndex}:${candidate.cellIndex}`
)
assert.equal(resolved, '2:3:4', '已捕获的表格目标必须由上传生命周期管理，不能因耗时而过期')
assert.equal(
  resolveTableAttachmentTarget(target, () => false, () => 'unexpected'),
  null,
  '编辑器 DOM 已销毁时不能复用捕获目标'
)

const sourceLines = Array.from({ length: 20_000 }, (_, index) => `row-${index}`)
const changedLine = '| cell | [文件附件：report.pdf](/api/files/report.pdf) |'
const nextLines = replaceTableSourceLine(sourceLines, 12_345, changedLine)
assert.ok(nextLines, '合法表格行必须能更新')
assert.equal(nextLines[12_345], changedLine)
assert.equal(sourceLines[12_345], 'row-12345', '单行更新不能修改输入源')
assert.equal(
  nextLines.reduce((count, line, index) => count + Number(line !== sourceLines[index]), 0),
  1,
  'Markdown 表格附件插入只能重写目标行'
)

const editorPath = fileURLToPath(new URL('../components/index/VditorEditor.vue', import.meta.url))
const addFormPath = fileURLToPath(new URL('../components/index/AddForm.vue', import.meta.url))
const [editor, addForm] = await Promise.all([
  readFile(editorPath, 'utf8'),
  readFile(addFormPath, 'utf8'),
])

assert.match(editor, /insertEditorValueFallback\(vditorInstance, nextValue, hasAttachmentMarker\(nextValue\)\)/, '组件必须使用已测试的插入路由')
assert.doesNotMatch(editor, /EDITOR_TABLE_ATTACHMENT_INSERT_TARGET_TTL_MS|expiresAt:\s*Date\.now\(\)\s*\+\s*EDITOR_TABLE_ATTACHMENT/, '附件目标不能保留时间失效逻辑')
assert.doesNotMatch(editor, /\[DEBUG-|debugRawInsertValue|debugSafeInsertValue/, '诊断代码必须清理')
const targetCaptureStart = editor.indexOf('const prepareEditorAttachmentInsertionTarget =')
const targetCaptureEnd = editor.indexOf('const consumePreparedEditorTableAttachmentCell =', targetCaptureStart)
const targetCapture = editor.slice(targetCaptureStart, targetCaptureEnd)
assert.ok(targetCaptureStart >= 0 && targetCaptureEnd > targetCaptureStart, '应能定位附件目标捕获逻辑')
assert.doesNotMatch(targetCapture, /setEditorTableDomCellText|markEditorTableCellSourceDirty|syncInlineEditorTableTextareaToCell/, '捕获附件目标必须是只读操作，不能重写或提交活动单元格')
const inlineCommitStart = editor.indexOf('const syncInlineEditorTableTextareaToCell =')
const inlineCommitEnd = editor.indexOf('const scheduleInlineEditorTableTextareaSync =', inlineCommitStart)
const inlineCommit = editor.slice(inlineCommitStart, inlineCommitEnd)
assert.ok(inlineCommitStart >= 0 && inlineCommitEnd > inlineCommitStart, '应能定位内联单元格提交逻辑')
assert.match(inlineCommit, /markEditorTableCellSourceDirty\(state\.cell, value\)/, '关闭内联编辑器时必须保留待提交源码')
assert.doesNotMatch(inlineCommit, /setEditorTableDomCellText/, '关闭内联编辑器不能再次重写已经实时镜像的单元格 DOM')
const tableInsertStart = editor.indexOf('const insertAttachmentIntoTableCellWithoutVditorReset =')
const tableInsertEnd = editor.indexOf('const insertValueIntoCurrentTableCell =', tableInsertStart)
const tableInsert = editor.slice(tableInsertStart, tableInsertEnd)
assert.doesNotMatch(tableInsert, /hasUnsafeMarkdownTableStructure\(/, '已知表格源只能在 emitKnownEditorSourceValue 中校验一次')
assert.match(addForm, /@cancel="clearEditorAttachmentInsertTarget"/, '文件选择器取消时必须显式清理捕获目标')

for (const handler of ['handleImageHostingSuccess', 'addAttachment', 'handleAudioUploaded']) {
  const start = addForm.indexOf(`const ${handler} =`)
  assert.ok(start >= 0, `应能找到 ${handler}`)
  const nextHandler = addForm.indexOf('\nconst ', start + 8)
  const source = addForm.slice(start, nextHandler >= 0 ? nextHandler : undefined)
  assert.doesNotMatch(source, /syncContentFromEditor\(\)/, `${handler} 上传成功后不能同步全量读取 Vditor`)
}

console.log('vditor table attachment regression tests passed')
