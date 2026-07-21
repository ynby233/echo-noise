import assert from 'node:assert/strict'
import { applyTableTrackSize, resolveTableTrackResize, resolveTableTrailingScrollReserve } from '../utils/table-resize-session.ts'

const fractionalSession = {
  startPointer: 562.4,
  startSize: 316.8,
  minSize: 48,
}

assert.deepEqual(
  resolveTableTrackResize(fractionalSession, 562.4, false),
  { active: false, size: 316.8 },
  'pointer-down jitter at the same client coordinate must not mutate table geometry'
)

assert.deepEqual(
  resolveTableTrackResize(fractionalSession, 562.9, false),
  { active: false, size: 316.8 },
  'sub-threshold movement must not turn a long press into a resize'
)

const firstMoved = resolveTableTrackResize(fractionalSession, 563.4, false)
assert.equal(firstMoved.active, true, 'the first real pointer movement must activate the resize session')
assert.ok(
  Math.abs(firstMoved.size - 317.8) < 1e-9,
  'the first active movement must change the track only by the pointer delta without snapping to the pointer-down offset'
)

const moved = resolveTableTrackResize(fractionalSession, 602.4, true)
assert.equal(moved.active, true, 'a real drag must activate the resize session')
assert.ok(Math.abs(moved.size - 356.8) < 1e-9, 'active resizing must preserve the exact pointer displacement')

const returned = resolveTableTrackResize(fractionalSession, 562.4, true)
assert.equal(returned.active, true, 'an activated resize session must stay active')
assert.ok(Math.abs(returned.size - 316.8) < 1e-9, 'returning to the initial pointer coordinate must restore the exact initial size')

assert.deepEqual(
  resolveTableTrackResize({ ...fractionalSession, startSize: 48 }, 520, true),
  { active: true, size: 48 },
  'minimum track size must clamp the resolved size'
)

const makeCell = () => ({ style: {} })
const columns = [{ style: {} }, { style: {} }]
const table = {
  rows: [
    { style: {}, cells: [makeCell(), makeCell()] },
    { style: {}, cells: [makeCell(), makeCell()] },
  ],
  querySelectorAll: (selector) => selector === 'colgroup col' ? columns : [],
}

applyTableTrackSize(table, 'row', 1, 72.5)
assert.equal(table.rows[1].style.height, '72.5px', 'row writes must update the real row synchronously')
assert.deepEqual(
  table.rows[1].cells.map((cell) => cell.style.height),
  ['72.5px', '72.5px'],
  'row writes must keep every cell border on the same synchronous track'
)

applyTableTrackSize(table, 'column', 1, 133.25)
assert.equal(columns[1].style.width, '133.25px', 'column writes must update the real col synchronously')
assert.deepEqual(
  table.rows.map((row) => row.cells[1].style.width),
  ['133.25px', '133.25px'],
  'column writes must keep every cell border on the same synchronous track'
)

assert.equal(resolveTableTrailingScrollReserve(0, 78, 58), 20, 'shrinking a column must preserve the removed width as trailing scroll range')
assert.equal(resolveTableTrailingScrollReserve(20, 78, 88), 10, 'growing a column must consume an existing trailing scroll reserve')
assert.equal(resolveTableTrailingScrollReserve(20, 78, 108), 0, 'growth beyond the reserve must expand the real scroll range')

// Regression: body { zoom } must not break 1:1 pointer tracking.
// getBoundingClientRect()/clientX are post-zoom (visual) px, while style.width is pre-zoom (CSS) px.
// Call sites normalize by dividing visual px by the zoom scale, so the rendered edge must track the pointer 1:1.
for (const zoom of [1, 1.1, 1.25, 0.9090909091]) {
  const startVisualSize = 150.7
  const startVisualPointer = 264.7
  const pointerVisualDelta = 40
  const session = {
    startPointer: startVisualPointer / zoom,
    startSize: startVisualSize / zoom,
    minSize: 48,
    scale: zoom,
  }
  const movedPointerVisual = startVisualPointer + pointerVisualDelta
  const resolved = resolveTableTrackResize(session, movedPointerVisual / zoom, true)
  const renderedSize = resolved.size * zoom
  const renderedDelta = renderedSize - startVisualSize
  assert.ok(
    Math.abs(renderedDelta - pointerVisualDelta) < 1e-6,
    `under body zoom ${zoom} the rendered resize edge must follow the pointer 1:1 (got ${renderedDelta} for a ${pointerVisualDelta}px drag)`
  )
}

console.log('table resize session tests passed')
