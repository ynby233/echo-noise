import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const editorPath = fileURLToPath(new URL('../components/index/VditorEditor.vue', import.meta.url))
const rendererPath = fileURLToPath(new URL('../components/index/MarkdownRenderer.vue', import.meta.url))
const globalCssPath = fileURLToPath(new URL('../assets/css/tailwind.css', import.meta.url))
const [editor, renderer, globalCss] = await Promise.all([
  readFile(editorPath, 'utf8'),
  readFile(rendererPath, 'utf8'),
  readFile(globalCssPath, 'utf8'),
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
    `${name} row drag must preserve the native row-resize cursor under the pointer`
  )
  assert.match(
    source,
    new RegExp(`body\\.is-resizing-${bodyPrefix}-column,\\s*body\\.is-resizing-${bodyPrefix}-column \\*[\\s\\S]*?cursor:\\s*var\\(--table-column-resize-cursor\\)\\s*!important`),
    `${name} column drag must preserve the native column-resize cursor under the pointer`
  )
}

assert.match(
  globalCss,
  /--table-row-resize-cursor:\s*row-resize;/,
  'row resizing must use the platform-native DPI-aware cursor'
)
assert.match(
  globalCss,
  /--table-column-resize-cursor:\s*col-resize;/,
  'column resizing must use the platform-native DPI-aware cursor'
)

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
  /startPointer:\s*event\.clientY[\s\S]*?startPointer:\s*event\.clientX/,
  'editor drag math must start from the exact pointer-down coordinates'
)
assert.match(
  sourceBetween(editor, 'const startExpandedTableRowResize', 'const startExpandedTableColumnResize'),
  /startBoundary:[\s\S]*?getBoundingClientRect\(\)[\s\S]*?startSize:\s*expandedTableRowHeight\(rowIndex\)/,
  'editor row dragging must anchor to the rendered border but resize from the authored track height without adding collapsed-border thickness twice'
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
assert.doesNotMatch(
  editor,
  /freezeExpandedTableResizeLayout|previousManualRowHeights|previousManualColumnWidths/,
  'editor pointer-down must not rewrite unrelated tracks or create a whole-table freeze snapshot'
)
assert.match(
  editor,
  /resolveTableTrackResize\([\s\S]*?drag\.active\s*=\s*resolved\.active/,
  'editor dragging must use the shared actual-boundary resize contract'
)
assert.match(
  editorResizeMove,
  /resolveTableTrailingScrollReserve\(drag\.startTrailingScrollReserve,\s*drag\.startSize,\s*nextHeight\)[\s\S]*?resolveTableTrailingScrollReserve\(drag\.startTrailingScrollReserve,\s*drag\.startSize,\s*nextWidth\)/,
  'editor resizing must preserve trailing scroll range for both rows and columns'
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
assert.doesNotMatch(
  renderer,
  /freezeRenderedTableResizeLayout|previousManualRowHeights|previousManualColumnWidths/,
  'published pointer-down must not rewrite unrelated tracks or create a whole-table freeze snapshot'
)
assert.match(
  renderer,
  /resolveTableTrackResize\([\s\S]*?drag\.active\s*=\s*resolved\.active/,
  'published dragging must use the shared actual-boundary resize contract'
)
assert.match(
  publishedResizeMove,
  /resolveTableTrailingScrollReserve\(drag\.startTrailingScrollReserve,\s*drag\.startSize,\s*nextHeight\)[\s\S]*?resolveTableTrailingScrollReserve\(drag\.startTrailingScrollReserve,\s*drag\.startSize,\s*nextWidth\)/,
  'published resizing must preserve trailing scroll range for both rows and columns'
)
assert.match(renderer, /drag\?\.type\s*===\s*'column'\s*&&\s*drag\.active/, 'published release-time row measurement must run only after an actual column resize')
assert.match(
  renderer,
  /startPointer:\s*event\.clientY[\s\S]*?startPointer:\s*event\.clientX/,
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
