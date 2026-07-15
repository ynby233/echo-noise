import assert from 'node:assert/strict'
import { resolveTableTrackResize, resolveTableTrailingScrollReserve } from '../utils/table-resize-session.ts'

const fractionalBoundary = {
  startPointer: 560,
  startBoundary: 559.6,
  startSize: 316.8,
  minSize: 48,
}

assert.deepEqual(
  resolveTableTrackResize(fractionalBoundary, 560, false),
  { active: false, size: 316.8 },
  'pointer-down jitter at the same client coordinate must not mutate table geometry'
)

assert.deepEqual(
  resolveTableTrackResize(fractionalBoundary, 560.5, false),
  { active: false, size: 316.8 },
  'sub-threshold movement must not turn a long press into a resize'
)

const moved = resolveTableTrackResize(fractionalBoundary, 600, false)
assert.equal(moved.active, true, 'a real drag must activate the resize session')
assert.ok(Math.abs(moved.size - 357.2) < 1e-9, 'active resizing must target the pointer from the actual fractional border coordinate')

const returned = resolveTableTrackResize(fractionalBoundary, 560, true)
assert.equal(returned.active, true, 'an activated resize session must stay active')
assert.ok(Math.abs(returned.size - 317.2) < 1e-9, 'returning to the initial pointer coordinate must keep the border under the pointer')

assert.deepEqual(
  resolveTableTrackResize({ ...fractionalBoundary, startBoundary: 600, startSize: 48 }, 520, true),
  { active: true, size: 48 },
  'minimum track size must clamp the resolved size'
)

assert.equal(resolveTableTrailingScrollReserve(0, 78, 58), 20, 'shrinking a column must preserve the removed width as trailing scroll range')
assert.equal(resolveTableTrailingScrollReserve(20, 78, 88), 10, 'growing a column must consume an existing trailing scroll reserve')
assert.equal(resolveTableTrailingScrollReserve(20, 78, 108), 0, 'growth beyond the reserve must expand the real scroll range')

console.log('table resize session tests passed')
