import assert from 'node:assert/strict'
import { createJiti } from 'jiti'

const { createRenderedTaskListEnhancer } = await createJiti(import.meta.url).import('../utils/rendered-task-list.ts')

const item = () => {
  const classes = new Set()
  return { classList: { add: value => classes.add(value), toggle: (value, enabled) => enabled ? classes.add(value) : classes.delete(value) }, classes }
}
const input = checked => {
  const parent = item()
  const attributes = new Map()
  return {
    checked,
    defaultChecked: checked,
    disabled: false,
    tabIndex: 0,
    dataset: {},
    style: {},
    parent,
    closest(selector) { return selector === 'li' ? parent : selector === 'input[type="checkbox"]' ? this : null },
    setAttribute: (name, value) => attributes.set(name, value),
    removeAttribute: name => attributes.delete(name),
    attributes,
    onclick: null,
    onchange: null,
  }
}

const inputs = [input(false), input(true)]
const listeners = new Map()
const root = {
  dataset: {},
  querySelectorAll: () => inputs,
  contains: candidate => inputs.includes(candidate),
  addEventListener: (name, listener) => listeners.set(name, listener),
  removeEventListener: name => listeners.delete(name),
}
let content = '- [ ] first\n- [x] second'
const persisted = []
const enhancer = createRenderedTaskListEnhancer({
  root: () => root,
  content: () => content,
  setContent: next => { content = next },
  editable: () => true,
  messageId: () => 42,
  persist: async (messageId, next) => { persisted.push([messageId, next]); return true },
})

enhancer.mount()
await new Promise(resolve => setTimeout(resolve, 0))
assert.equal(root.dataset.taskListEditable, 'true')
assert.deepEqual(inputs.map(value => value.checked), [false, true])
inputs[0].checked = true
await listeners.get('change')({ target: inputs[0], stopPropagation() {}, preventDefault() {} })
assert.equal(content, '- [x] first\n- [x] second')
assert.deepEqual(persisted, [[42, content]])

enhancer.dispose()
assert.equal(inputs[0].onclick, null)
assert.equal(inputs[0].onchange, null)
assert.equal(listeners.size, 0)

let rejectStale
content = '- [ ] first\n- [x] second'
const staleEnhancer = createRenderedTaskListEnhancer({
  root: () => root,
  content: () => content,
  setContent: next => { content = next },
  editable: () => true,
  messageId: () => 42,
  persist: () => new Promise((resolve, reject) => { rejectStale = reject }),
})
staleEnhancer.mount()
await new Promise(resolve => setTimeout(resolve, 0))
inputs[0].checked = true
const staleChange = listeners.get('change')({ target: inputs[0], stopPropagation() {}, preventDefault() {} })
staleEnhancer.dispose()
content = '- [ ] replacement'
rejectStale(new Error('late failure'))
await staleChange
assert.equal(content, '- [ ] replacement', 'a disposed enhancer must not restore stale content after async failure')

console.log('rendered task-list lifecycle tests passed')
