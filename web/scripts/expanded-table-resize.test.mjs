import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const editorPath = fileURLToPath(new URL('../components/index/VditorEditor.vue', import.meta.url))
const rendererPath = fileURLToPath(new URL('../components/index/MarkdownRenderer.vue', import.meta.url))
const [editor, renderer] = await Promise.all([
  readFile(editorPath, 'utf8'),
  readFile(rendererPath, 'utf8'),
])

for (const [name, source, prefix, bodyPrefix] of [
  ['editor', editor, 'editor-table-expand', 'expanded-table'],
  ['published', renderer, 'rendered-table-expand', 'rendered-table'],
]) {
  assert.match(
    source,
    new RegExp(`\\.${prefix}-row-resize-handle\\.is-table-edge::after\\s*\\{[\\s\\S]*?top:\\s*100%`),
    `${name} bottom-edge guide must be centered on the visible table border while its hit target stays inside`
  )
  assert.match(
    source,
    new RegExp(`\\.${prefix}-column-resize-handle\\.is-table-edge::after\\s*\\{[\\s\\S]*?left:\\s*100%`),
    `${name} right-edge guide must be centered on the visible table border while its hit target stays inside`
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
  /startClient:\s*cell\?\.getBoundingClientRect\(\)\.bottom[\s\S]*?startClient:\s*cell\?\.getBoundingClientRect\(\)\.right/,
  'editor drag math must start from the real cell border rather than the interior pointer hit position'
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
  /startClient:\s*cellElement\.getBoundingClientRect\(\)\.bottom[\s\S]*?startClient:\s*cellElement\.getBoundingClientRect\(\)\.right/,
  'published drag math must start from the real cell border rather than the interior pointer hit position'
)

console.log('expanded table resize geometry tests passed')
