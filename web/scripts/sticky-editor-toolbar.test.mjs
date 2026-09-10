import assert from 'node:assert/strict'
import { createJiti } from 'jiti'

const { createStickyEditorToolbar } = await createJiti(import.meta.url).import('../utils/sticky-editor-toolbar.ts')

const toolbar = { offsetHeight: 42, nextSibling: null, style: {} }
const content = {}
let placeholder = null
const root = {
  offsetHeight: 600,
  style: {},
  classList: { contains: () => false },
  getBoundingClientRect: () => ({ top: -20, bottom: 580, left: 100, width: 720 }),
  querySelector: selector => selector === '.vditor-toolbar' ? toolbar : selector === '.vditor-content' ? content : null,
  insertBefore: element => { placeholder = element },
}
const container = { matches: () => false, querySelector: selector => selector === '.vditor' ? root : null }
const windowListeners = new Map()
globalThis.document = {
  createElement: () => ({ style: {}, removed: false, remove() { this.removed = true } }),
  querySelectorAll: () => [],
}
globalThis.window = {
  innerWidth: 1280,
  addEventListener: (name, listener) => windowListeners.set(name, listener),
  removeEventListener: name => windowListeners.delete(name),
}
const frames = new Map()
let nextFrame = 0
globalThis.requestAnimationFrame = callback => { frames.set(++nextFrame, callback); return nextFrame }
globalThis.cancelAnimationFrame = frame => frames.delete(frame)
const observers = []
class Observer {
  observed = []
  disconnected = false
  constructor(callback) { this.callback = callback; observers.push(this) }
  observe(target) { this.observed.push(target) }
  disconnect() { this.disconnected = true }
}
globalThis.ResizeObserver = Observer
globalThis.MutationObserver = Observer

let readyCount = 0
const sticky = createStickyEditorToolbar({ container: () => container, onReady: () => { readyCount += 1 } })
sticky.mount()
assert.equal(readyCount, 1)
assert.equal(sticky.element(), toolbar)
assert.equal(toolbar.style.position, 'absolute', 'toolbar must stay anchored to the editor while its top is above the viewport')
assert.equal(toolbar.style.top, '0px')
assert.equal(toolbar.style.left, '0px')
assert.equal(toolbar.style.width, '100%')
assert.equal(placeholder.style.height, '42px')
observers[0].callback()
observers[0].callback()
assert.equal(frames.size, 1, 'resize notifications coalesce into one owned frame')
sticky.dispose()
assert.equal(frames.size, 0, 'dispose cancels pending resize work')
assert.equal(sticky.element(), null)
assert.equal(placeholder.removed, true)
assert.equal(windowListeners.size, 0)
assert.equal(toolbar.style.position, '')
root.matches = selector => selector === '.vditor'
const direct = createStickyEditorToolbar({ container: () => root })
direct.mount()
assert.equal(direct.element(), toolbar, 'Vditor may use the supplied container itself as its root')
direct.dispose()

console.log('sticky editor toolbar lifecycle tests passed')
