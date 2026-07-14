import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const editorPath = fileURLToPath(new URL('../components/index/VditorEditor.vue', import.meta.url))
const rendererPath = fileURLToPath(new URL('../components/index/MarkdownRenderer.vue', import.meta.url))
const globalCssPath = fileURLToPath(new URL('../assets/css/tailwind.css', import.meta.url))
const rowCursorPath = fileURLToPath(new URL('../public/cursors/table-row-resize.svg', import.meta.url))
const columnCursorPath = fileURLToPath(new URL('../public/cursors/table-column-resize.svg', import.meta.url))
const readOptionalFile = async (path) => {
  try {
    return await readFile(path, 'utf8')
  } catch (error) {
    if (error?.code === 'ENOENT') return ''
    throw error
  }
}
const [editor, renderer, globalCss, rowCursor, columnCursor] = await Promise.all([
  readFile(editorPath, 'utf8'),
  readFile(rendererPath, 'utf8'),
  readFile(globalCssPath, 'utf8'),
  readOptionalFile(rowCursorPath),
  readOptionalFile(columnCursorPath),
])

const sourceBetween = (source, startMarker, endMarker) => {
  const start = source.indexOf(startMarker)
  const end = source.indexOf(endMarker, start + startMarker.length)
  assert.notEqual(start, -1, `missing start marker: ${startMarker}`)
  assert.notEqual(end, -1, `missing end marker: ${endMarker}`)
  return source.slice(start, end)
}

for (const [name, source, prefix, bodyPrefix] of [
  ['editor', editor, 'editor-table-expand', 'expanded-table'],
  ['published', renderer, 'rendered-table-expand', 'rendered-table'],
]) {
  assert.match(
    source,
    new RegExp(`\\.${prefix}-row-resize-handle\\s*\\{[\\s\\S]*?bottom:\\s*-0\\.5px;[\\s\\S]*?height:\\s*1px;[\\s\\S]*?cursor:\\s*var\\(--table-row-resize-cursor\\)`),
    `${name} row hit target must be the exact one-pixel border line`
  )
  assert.match(
    source,
    new RegExp(`\\.${prefix}-column-resize-handle\\s*\\{[\\s\\S]*?right:\\s*-0\\.5px;[\\s\\S]*?width:\\s*1px;[\\s\\S]*?cursor:\\s*var\\(--table-column-resize-cursor\\)`),
    `${name} column hit target must be the exact one-pixel border line`
  )
  assert.doesNotMatch(
    source,
    new RegExp(`\\.${prefix}-(?:row|column)-resize-handle\\.is-table-edge(?:\\s*|::after\\s*)\\{`),
    `${name} outer borders must use the same centered geometry as internal borders`
  )
  assert.match(
    source,
    new RegExp(`\\.${prefix}-row-resize-handle::after\\s*\\{[\\s\\S]*?inset:\\s*0;`),
    `${name} visible row guide must exactly cover its hit target`
  )
  assert.match(
    source,
    new RegExp(`\\.${prefix}-column-resize-handle::after\\s*\\{[\\s\\S]*?inset:\\s*0;`),
    `${name} visible column guide must exactly cover its hit target`
  )
  assert.match(
    source,
    new RegExp(`\\.${prefix}-row-resize-handle\\.is-resizing::after[\\s\\S]*?\\.${prefix}-column-resize-handle\\.is-resizing::after`),
    `${name} resizing highlight must be scoped to the active row or column line`
  )
  assert.doesNotMatch(
    source,
    new RegExp(`body\\.is-resizing-${bodyPrefix}-row \\.${prefix}-row-resize-handle::after|body\\.is-resizing-${bodyPrefix}-column \\.${prefix}-column-resize-handle::after`),
    `${name} drag state must not highlight every row or column handle`
  )
  assert.match(
    source,
    new RegExp(`body\\.is-resizing-${bodyPrefix}-row,\\s*body\\.is-resizing-${bodyPrefix}-row \\*[\\s\\S]*?cursor:\\s*var\\(--table-row-resize-cursor\\)\\s*!important`),
    `${name} row drag must preserve the same explicit cursor hotspot under the pointer`
  )
  assert.match(
    source,
    new RegExp(`body\\.is-resizing-${bodyPrefix}-column,\\s*body\\.is-resizing-${bodyPrefix}-column \\*[\\s\\S]*?cursor:\\s*var\\(--table-column-resize-cursor\\)\\s*!important`),
    `${name} column drag must preserve the same explicit cursor hotspot under the pointer`
  )
}

assert.match(
  globalCss,
  /--table-row-resize-cursor:\s*url\(['"]\/cursors\/table-row-resize\.svg['"]\)\s*16\s+16,\s*row-resize;/,
  'row resize cursor must declare a platform-independent hotspot at the artwork center'
)
assert.match(
  globalCss,
  /--table-column-resize-cursor:\s*url\(['"]\/cursors\/table-column-resize\.svg['"]\)\s*16\s+16,\s*col-resize;/,
  'column resize cursor must declare a platform-independent hotspot at the artwork center'
)
assert.match(rowCursor, /viewBox="0 0 32 32"/, 'row cursor artwork must use the 32px coordinate system matched by its 16px hotspot')
assert.match(rowCursor, /d="M8 16H24"/, 'row cursor artwork must visibly cross its hotspot on the horizontal border axis')
assert.match(columnCursor, /viewBox="0 0 32 32"/, 'column cursor artwork must use the 32px coordinate system matched by its 16px hotspot')
assert.match(columnCursor, /d="M16 8V24"/, 'column cursor artwork must visibly cross its hotspot on the vertical border axis')

assert.match(
  editor,
  /expandedTableActiveResize\.value\s*=\s*\{\s*type:\s*drag\.type,\s*index:\s*drag\.index\s*\}/,
  'editor resize session must expose exactly one active row or column index to the template'
)
assert.match(
  editor,
  /'is-resizing':\s*expandedTableActiveResize\?\.type\s*===\s*'row'[\s\S]*?expandedTableActiveResize\.index\s*===\s*rowIndex[\s\S]*?'is-resizing':\s*expandedTableActiveResize\?\.type\s*===\s*'column'[\s\S]*?expandedTableActiveResize\.index\s*===\s*cellIndex/,
  'editor must highlight every segment of only the active row or column line'
)
assert.match(
  editor,
  /startClient:\s*event\.clientY[\s\S]*?startClient:\s*event\.clientX/,
  'editor drag math must start from the exact pointer-down coordinates'
)
assert.doesNotMatch(
  sourceBetween(editor, 'const startExpandedTableResize', 'const startExpandedTableRowResize'),
  /onExpandedTableResizeMove\(event\)/,
  'editor pointer-down must not resize before the first real pointer move'
)
const editorResizeMove = sourceBetween(editor, 'const onExpandedTableResizeMove', 'const startExpandedTableResize')
assert.doesNotMatch(
  editorResizeMove,
  /Math\.ceil|scheduleMeasureExpandedTableAutoRowHeights/,
  'editor pointer moves must preserve sub-pixel pointer geometry and defer content measurement until drag end'
)
assert.match(
  editor,
  /const freezeExpandedTableResizeLayout[\s\S]*?getBoundingClientRect\(\)\.width[\s\S]*?getBoundingClientRect\(\)\.height/,
  'editor pointer-down must freeze the actual rendered columns and rows before dragging'
)
assert.match(
  editor,
  /previousManualRowHeights[\s\S]*?previousManualColumnWidths[\s\S]*?resizedRowHeight[\s\S]*?resizedColumnWidth/,
  'editor drag snapshots must stay transient and commit only the resized track'
)
assert.match(
  renderer,
  /dataset\.resizeIndex\s*=\s*String\(rowIndex\)[\s\S]*?dataset\.resizeIndex\s*=\s*String\(cellIndex\)/,
  'published resize handles must identify the row or column line they belong to'
)
assert.match(
  renderer,
  /querySelectorAll\(`\.\$\{handleClass\}\[data-resize-index="\$\{drag\.index\}"\]`\)[\s\S]*?classList\.add\('is-resizing'\)/,
  'published resize session must highlight all segments belonging to only the active line index'
)
const publishedResizeMove = sourceBetween(renderer, 'const onRenderedTableResizeMove', 'const startRenderedTableResize')
assert.doesNotMatch(
  publishedResizeMove,
  /syncRenderedTableExpandLayout|measureRenderedTableAutoRowHeights|applyAdaptiveRenderedTableColumns|applyRenderedTableRowHeights|Math\.ceil/,
  'published pointer moves must update only the target track without any full-table measurement or rounding'
)
assert.match(
  publishedResizeMove,
  /table\.rows\[drag\.index\][\s\S]*?row\.style\.height/,
  'published row pointer moves must directly update only the target row'
)
assert.match(
  publishedResizeMove,
  /querySelectorAll<HTMLTableColElement>\('colgroup col'\)[\s\S]*?column\.style\.width/,
  'published column pointer moves must directly update only the target col'
)
assert.match(
  renderer,
  /const freezeRenderedTableResizeLayout[\s\S]*?getBoundingClientRect\(\)\.width[\s\S]*?getBoundingClientRect\(\)\.height/,
  'published pointer-down must freeze the actual rendered columns and rows before dragging'
)
assert.match(
  renderer,
  /startBoundary[\s\S]*?desiredBoundary[\s\S]*?correction/,
  'published drag math must use the target border coordinate as the source of truth'
)
assert.match(
  renderer,
  /previousManualRowHeights[\s\S]*?previousManualColumnWidths[\s\S]*?resizedRowHeight[\s\S]*?resizedColumnWidth/,
  'published drag snapshots must stay transient and commit only the resized track'
)
assert.match(
  renderer,
  /startClient:\s*event\.clientY[\s\S]*?startClient:\s*event\.clientX/,
  'published drag math must start from the exact pointer-down coordinates'
)
assert.doesNotMatch(
  sourceBetween(renderer, 'const startRenderedTableResize', 'const ensureRenderedTableResizeHandles'),
  /onRenderedTableResizeMove\(event\)/,
  'published pointer-down must not resize before the first real pointer move'
)

for (const [name, source, keyframePrefix] of [
  ['editor', editor, 'editorTableDialog'],
  ['published', renderer, 'renderedTableDialog'],
]) {
  const openAnimation = sourceBetween(source, `@keyframes ${keyframePrefix}In`, `@keyframes ${keyframePrefix}Out`)
  assert.doesNotMatch(openAnimation, /transform\s*:/, `${name} draggable dialog must not animate its coordinate system while becoming interactive`)
}

console.log('expanded table resize geometry tests passed')
