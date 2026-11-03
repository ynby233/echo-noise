import assert from 'node:assert/strict'
import { positionAuthorCard, resolveAuthorCardPosition, toAuthorCardFixedPoint } from '../utils/author-card-position.ts'

const viewport = { left: 0, top: 0, right: 1000, bottom: 800 }
const size = { width: 300, height: 120 }
const rect = (left, top, width = 36, height = 36) => ({ left, top, right: left + width, bottom: top + height, width, height })

assert.deepEqual(resolveAuthorCardPosition(rect(100, 200), size, viewport, false),
  { left: 144, top: 158, placement: 'right' }, 'desktop cards must anchor to the active avatar on the right')
assert.deepEqual(resolveAuthorCardPosition(rect(900, 200), size, viewport, false),
  { left: 592, top: 158, placement: 'left' }, 'a right-edge avatar must flip to the left')
assert.deepEqual(resolveAuthorCardPosition(rect(200, 200), size, { ...viewport, right: 500 }, false),
  { left: 192, top: 158, placement: 'right' }, 'when neither side fits, use the roomier side and clamp')

const mobile = { left: 0, top: 0, right: 390, bottom: 844 }
assert.deepEqual(resolveAuthorCardPosition(rect(15, 100), size, mobile, true),
  { left: 8, top: 144, placement: 'below' }, 'narrow screens should use below and clamp the left edge')
assert.deepEqual(resolveAuthorCardPosition(rect(340, 780), size, mobile, true),
  { left: 82, top: 652, placement: 'above' }, 'a narrow-screen bottom corner must flip above and clamp right')
assert.deepEqual(resolveAuthorCardPosition(rect(100, 0), size, viewport, false),
  { left: 144, top: 8, placement: 'right' }, 'vertical centering must respect the top viewport edge')
assert.deepEqual(resolveAuthorCardPosition(rect(100, 764), size, viewport, false),
  { left: 144, top: 672, placement: 'right' }, 'vertical centering must respect the bottom viewport edge')

const panned = { left: 150, top: 200, right: 550, bottom: 500 }
assert.deepEqual(resolveAuthorCardPosition(rect(480, 400), { width: 260, height: 180 }, panned, true),
  { left: 282, top: 212, placement: 'above' }, 'visual viewport panning and reduced keyboard height must bound both axes')

for (const zoom of [1, 1.1, 1.25, 0.9]) {
  const origin = { left: -17.25, top: 9.5 }
  for (const anchor of [rect(30, 60), rect(600, 330), rect(950, 720)]) {
    const position = resolveAuthorCardPosition(anchor, size, viewport, false)
    const fixed = toAuthorCardFixedPoint(position, origin, { width: zoom, height: zoom })
    assert.ok(Math.abs(fixed.left * zoom + origin.left - position.left) < 1e-8, `zoom ${zoom}: rendered left must match visual coordinates`)
    assert.ok(Math.abs(fixed.top * zoom + origin.top - position.top) < 1e-8, `zoom ${zoom}: rendered top must match visual coordinates`)
  }
}

// Exercise the DOM adapter as well as geometry: it must measure AFTER applying
// viewport limits, remeasure late profile data, and use each current anchor.
const previousWindow = globalThis.window
const previousDocument = globalThis.document
try {
  const body = {}
  globalThis.document = { body }
  let zoom = 1.1
  let origin = { left: 0, top: 0 }
  const visualViewport = { offsetLeft: 0, offsetTop: 0, width: 1000, height: 800 }
  globalThis.window = {
    visualViewport, innerWidth: 1920, innerHeight: 1080,
    getComputedStyle: element => element.overflow || { overflowX: 'visible', overflowY: 'visible' }
  }
  let activeRect = rect(100, 200)
  const anchor = { isConnected: true, parentElement: body, getClientRects: () => [activeRect], getBoundingClientRect: () => activeRect }
  const probe = { getBoundingClientRect: () => ({ ...origin, width: 100 * zoom, height: 100 * zoom }) }
  let contentHeight = 120
  const card = {
    style: {}, dataset: {},
    getBoundingClientRect() {
      return {
        width: Math.min(300, Number.parseFloat(this.style.maxWidth)) * zoom,
        height: Math.min(contentHeight, Number.parseFloat(this.style.maxHeight)) * zoom
      }
    }
  }
  const rendered = () => ({ left: Number.parseFloat(card.style.left) * zoom + origin.left, top: Number.parseFloat(card.style.top) * zoom + origin.top })
  assert.equal(positionAuthorCard(anchor, card, probe, false), true)
  assert.ok(Math.abs(rendered().left - activeRect.right - 8) < 1e-8, 'body zoom 1.1 must preserve the visual avatar gap')
  assert.equal(card.dataset.placement, 'right')

  activeRect = rect(900, 650)
  positionAuthorCard(anchor, card, probe, false)
  assert.equal(card.dataset.placement, 'left', 'switching avatars must not reuse the earlier anchor')
  assert.ok(Math.abs(rendered().left + card.getBoundingClientRect().width - activeRect.left + 8) < 1e-8)
  contentHeight = 700
  positionAuthorCard(anchor, card, probe, false)
  assert.ok(Math.abs(rendered().top + card.getBoundingClientRect().height - 792) < 1e-8, 'late profile expansion must remeasure and remain inside the bottom edge')

  // A small visual viewport is different from window.innerWidth. The card
  // must shrink before measuring, even while body zoom is still enabled.
  visualViewport.offsetLeft = 150
  visualViewport.offsetTop = 200
  visualViewport.width = 240
  visualViewport.height = 200
  activeRect = rect(320, 310)
  origin = { left: -22, top: 13 }
  positionAuthorCard(anchor, card, probe, true)
  const actual = rendered()
  const measured = card.getBoundingClientRect()
  assert.ok(Math.abs(actual.left - 158) < 1e-8)
  assert.ok(Math.abs(actual.top - 208) < 1e-8)
  assert.ok(Math.abs(measured.width - 224) < 1e-8, 'max width must account for zoom and visual viewport width')
  assert.ok(Math.abs(measured.height - 184) < 1e-8, 'oversized content must scroll within the measured viewport height')

  anchor.parentElement = {
    parentElement: body, overflow: { overflowX: 'hidden', overflowY: 'auto' },
    getBoundingClientRect: () => rect(150, 200, 240, 80)
  }
  assert.equal(positionAuthorCard(anchor, card, probe, true), false, 'nested scroll clipping must close a card whose avatar has left the scroller')
  anchor.parentElement = body
  anchor.isConnected = false
  assert.equal(positionAuthorCard(anchor, card, probe, true), false, 'removed anchors must not leave an orphan card')
} finally {
  if (previousWindow === undefined) delete globalThis.window
  else globalThis.window = previousWindow
  if (previousDocument === undefined) delete globalThis.document
  else globalThis.document = previousDocument
}

console.log('author card position tests passed')
