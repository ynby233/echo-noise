import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const editorPath = fileURLToPath(new URL('../components/index/VditorEditor.vue', import.meta.url))
const rendererPath = fileURLToPath(new URL('../components/index/MarkdownRenderer.vue', import.meta.url))
const [editor, renderer] = await Promise.all([
  readFile(editorPath, 'utf8'),
  readFile(rendererPath, 'utf8'),
])

// 编辑器侧：放大表格中的附件标记必须允许列宽缩小并对超长标记做省略截断。
assert.match(
  editor,
  /\.editor-table-expand-attachment-tag \{[\s\S]*?max-width: 100%;[\s\S]*?overflow: hidden;[\s\S]*?text-overflow: ellipsis;[\s\S]*?white-space: nowrap;[\s\S]*?\}/,
  '附件标记必须可截断：列宽缩小时超长标记后半省略，而不是撑宽列宽'
)
assert.doesNotMatch(
  editor,
  /\.editor-table-expand-attachment-tag \{[\s\S]*?overflow: visible;[\s\S]*?\}/,
  '附件标记不得再使用 overflow: visible 溢出撑宽单元格'
)
assert.match(
  editor,
  /\.editor-table-expand-cell \{[\s\S]*?min-width: 0;[\s\S]*?\}/,
  '放大表格单元格必须允许收缩，附件标记才能真正被列宽截断'
)
assert.match(
  editor,
  /\.editor-table-expand-attachments \{[\s\S]*?min-width: 0;[\s\S]*?\}/,
  '附件容器必须允许收缩，避免其撑住列宽'
)

// 渲染器侧：发布后的放大表格预览，含附件占位块的列必须像文字一样获得自然宽度下限，避免卡片被压扁。
assert.match(
  renderer,
  /const RENDERED_TABLE_ATTACHMENT_CARD_WIDTH = \d+/,
  '必须为文件卡/音频卡定义列宽自然下限常量'
)
assert.match(
  renderer,
  /const RENDERED_TABLE_ATTACHMENT_VIDEO_WIDTH = \d+/,
  '必须为视频附件定义列宽自然下限常量'
)
assert.match(
  renderer,
  /const RENDERED_TABLE_ATTACHMENT_IMAGE_WIDTH = \d+/,
  '必须为图片附件定义列宽自然下限常量'
)
assert.match(
  renderer,
  /const estimateRenderedTableCellAttachmentWidth = \(cell: HTMLTableCellElement \| undefined\) => \{[\s\S]*?noise-attachment-file[\s\S]*?noise-attachment-audio[\s\S]*?RENDERED_TABLE_ATTACHMENT_CARD_WIDTH[\s\S]*?noise-attachment-render--video[\s\S]*?RENDERED_TABLE_ATTACHMENT_VIDEO_WIDTH[\s\S]*?noise-attachment-image[\s\S]*?RENDERED_TABLE_ATTACHMENT_IMAGE_WIDTH[\s\S]*?\}/,
  '附件块宽度探测必须覆盖文件卡/音频、视频、图片三类占位块'
)
assert.match(
  renderer,
  /const attachmentWidth = estimateRenderedTableCellAttachmentWidth\(cell\)[\s\S]*?Math\.max\(max, textWidth, attachmentWidth\)/,
  '列自然宽度必须在文字估算基础上并入附件块宽度下限，两者取最大'
)

console.log('table attachment column width checks passed')