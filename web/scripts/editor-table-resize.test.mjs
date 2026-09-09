import assert from 'node:assert/strict'
import { createJiti } from 'jiti'

const { createEditorTableResize } = await createJiti(import.meta.url).import('../utils/editor-table-resize.ts')
const listeners = new Map()
const classes = new Set()
globalThis.window = {
  addEventListener: (name, listener) => listeners.set(name, listener),
  removeEventListener: name => listeners.delete(name),
}
globalThis.document = { body: { classList: {
  add: name => classes.add(name),
  remove: (...names) => names.forEach(name => classes.delete(name)),
} } }
globalThis.HTMLElement = class {}
const cell = { style: {} }
const column = { style: {} }
const table = { style: { marginRight: '30px' }, rows: [{ style: {}, cells: [cell] }], querySelectorAll: () => [column] }
const rowHeights = { value: [50] }
const columnWidths = { value: [100] }
let measurements = 0
let updates = 0
const resize = createEditorTableResize({ table: () => table, rowHeights, columnWidths, onColumnResizeEnd: () => measurements++, onResize: () => updates++ })
const pointer = x => ({ clientX: x, clientY: x, preventDefault() {}, stopPropagation() {} })
resize.start({ type: 'column', index: 0, startPointer: 50, startSize: 100, minSize: 48, scale: 2 }, {})
assert.equal(columnWidths.value[0], 100, 'pointer down preserves authored width')
listeners.get('pointermove')(pointer(101))
assert.equal(columnWidths.value[0], 100, 'sub-threshold movement does not resize')
listeners.get('pointermove')(pointer(121))
assert.equal(columnWidths.value[0], 110.5)
assert.equal(cell.style.width, '110.5px')
assert.equal(column.style.width, '110.5px')
assert.equal(table.style.marginRight, '19.5px')
assert.equal(measurements, 0, 'dragging does not measure row content')
listeners.get('pointerup')()
assert.equal(measurements, 1)
assert.equal(listeners.size, 0)
assert.equal(classes.size, 0)
resize.start({ type: 'row', index: 0, startPointer: 50, startSize: 50, minSize: 38 }, {})
listeners.get('pointermove')(pointer(10))
assert.equal(rowHeights.value[0], 38)
assert.equal(cell.style.height, '38px')
const updatesBeforeDispose = updates
resize.dispose()
resize.dispose()
assert.equal(updates, updatesBeforeDispose, 'disposal must not enqueue new layout work')
assert.equal(listeners.size, 0)
assert.equal(classes.size, 0)
assert.equal(resize.active.value, null)
console.log('editor table resize event and disposal tests passed')
