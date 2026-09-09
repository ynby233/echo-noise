import assert from 'node:assert/strict'
import { createJiti } from 'jiti'

const { createRenderedTableEnhancer } = await createJiti(import.meta.url).import('../utils/rendered-table-enhancer.ts')

const makeClassList = () => {
  const values = new Set()
  return { add: (...items) => items.forEach(item => values.add(item)), values }
}
const parent = { insertBefore(wrapper) { wrapper.parentElement = parent } }
const table = {
  tHead: null,
  tBodies: [],
  parentElement: parent,
  classList: makeClassList(),
  wrapper: null,
  closest: selector => selector === '.site-table-scroll' ? table.wrapper : null,
  querySelectorAll: () => [],
}
const makeButton = () => {
  const listeners = new Map()
  return {
    className: '', dataset: {}, removed: false,
    setAttribute() {},
    addEventListener: (name, listener) => listeners.set(name, listener),
    removeEventListener: name => listeners.delete(name),
    remove() { this.removed = true },
    listeners,
  }
}
const documentStub = {
  createElement(tag) {
    if (tag === 'button') return makeButton()
    if (tag === 'div') {
      return {
        className: '', dataset: {}, children: [], parentElement: null,
        querySelector(selector) { return selector === '.site-rendered-table-expand-button' ? this.children.find(child => String(child.className || '').includes('site-rendered-table-expand-button')) || null : null },
        appendChild(child) { this.children.push(child); if (child === table) table.wrapper = this },
      }
    }
    throw new Error(`unexpected element ${tag}`)
  },
}
globalThis.document = documentStub

let opened = null
const root = { querySelectorAll: selector => selector === 'table' ? [table] : [] }
const enhancer = createRenderedTableEnhancer({ root: () => root, open: value => { opened = value } })
enhancer.mount()
assert.ok(table.wrapper, 'mount must wrap a table inside the supplied root')
assert.equal(table.wrapper.children.length, 2, 'wrapper contains the table and one expand button')
enhancer.update()
assert.equal(table.wrapper.children.length, 2, 'update must be idempotent')
const button = table.wrapper.children[1]
button.listeners.get('click')({ preventDefault() {}, stopPropagation() {} })
assert.equal(opened, table)
enhancer.dispose()
assert.equal(button.removed, true)
assert.equal(button.listeners.size, 0)

console.log('rendered table enhancer lifecycle tests passed')
