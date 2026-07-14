import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const editorPath = fileURLToPath(new URL('../components/index/VditorEditor.vue', import.meta.url))
const rendererPath = fileURLToPath(new URL('../components/index/MarkdownRenderer.vue', import.meta.url))
const [editor, renderer] = await Promise.all([
  readFile(editorPath, 'utf8'),
  readFile(rendererPath, 'utf8'),
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
    new RegExp(`\\.${prefix}-row-resize-handle\\s*\\{[\\s\\S]*?bottom:\\s*-1px;[\\s\\S]*?height:\\s*2px;[\\s\\S]*?cursor:\\s*row-resize`),
    `${name} row hit target must be the same two-pixel line centered on the real border`
  )
  assert.match(
    source,
    new RegExp(`\\.${prefix}-column-resize-handle\\s*\\{[\\s\\S]*?right:\\s*-1px;[\\s\\S]*?width:\\s*2px;[\\s\\S]*?cursor:\\s*col-resize`),
    `${name} column hit target must be the same two-pixel line centered on the real border`
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
    new RegExp(`body\\.is-resizing-${bodyPrefix}-row,\\s*body\\.is-resizing-${bodyPrefix}-row \\*[\\s\\S]*?cursor:\\s*row-resize\\s*!important`),
    `${name} row drag cursor must override text cursors under the pointer`
  )
  assert.match(
    source,
    new RegExp(`body\\.is-resizing-${bodyPrefix}-column,\\s*body\\.is-resizing-${bodyPrefix}-column \\*[\\s\\S]*?cursor:\\s*col-resize\\s*!important`),
    `${name} column drag cursor must override text cursors under the pointer`
  )
}

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
assert.match(
  renderer,
  /syncRenderedTableExpandLayout\(\{\s*rebuildHandles:\s*false\s*\}\)/,
  'published dragging must preserve the active handle line instead of rebuilding every handle on each pointer move'
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

console.log('expanded table resize geometry tests passed')
