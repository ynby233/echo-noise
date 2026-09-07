import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createRequire } from 'node:module'
import { parse, compileScript } from '@vue/compiler-sfc'
import { transformSync } from 'esbuild'
import { computed, createRenderer, h, nextTick, reactive } from 'vue'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const component = await readFile(join(root, 'components/comments/BuiltinComments.vue'), 'utf8')
const manager = await readFile(join(root, 'components/admin/CommentManager.vue'), 'utf8')
const panel = await readFile(join(root, 'components/index/StatusPanel.vue'), 'utf8')

assert.match(component, /useAdminCapabilities/, 'thread UI must consume delegated-admin capabilities')
assert.match(component, /can\(['"]comments\.edit['"]\)/, 'cross-author body editing must require comments.edit')
assert.match(component, /can\(['"]comments\.trash['"]\)/, 'cross-author trashing must require comments.trash')
assert.match(component, /can\(['"]comments\.change_visibility['"]\)/, 'cross-author visibility editing must use its independent capability')
assert.match(component, /v-if="canChangeCommentVisibility\(c\)"/, 'root visibility control must be hidden without its capability')
assert.match(component, /v-if="canChangeCommentVisibility\(child\)"/, 'reply visibility control must be hidden without its capability')
assert.match(manager, /v-if="!recycleBin && canTrash && row\.can_trash"/, 'management trash action must be capability and target controlled')
assert.match(manager, /v-if="!recycleBin && canEdit && row\.can_edit"/, 'management edit action must be capability and target controlled')
assert.match(manager, /v-if="!recycleBin && canChangeVisibility && row\.can_change_visibility"/, 'management visibility action must be independently controlled')
assert.match(manager, /v-if="recycleBin && canRestore/, 'restore action must be capability controlled')
assert.match(manager, /v-if="recycleBin && canDeletePermanently && row\.can_permanently_delete"/, 'permanent delete action must be capability and target controlled')
assert.match(
  manager,
  /<AdminSelectionBar\b[^>]*\bv-if="canSelectForBatch"/,
  'batch-selection bar must be hidden when the administrator has no action capability for the current manager'
)
assert.match(
  manager,
  /<input\s+v-if="canSelectForBatch"\s+v-model="selected"\s+type="checkbox"/,
  'row selection checkboxes must be hidden when no batch action is authorized'
)
assert.match(
  manager,
  /const\s+canSelectForBatch\s*=\s*computed\(\(\)\s*=>\s*props\.recycleBin\s*\?\s*\(canRestore\.value\s*\|\|\s*canDeletePermanently\.value\)\s*:\s*canTrash\.value\)/,
  'batch selection must follow the actionable capability set for the active or recycle-bin view'
)
assert.match(panel, /canSection\('comment-recycle-bin'\)/, 'comment recycle-bin panel must be section-gated')
assert.doesNotMatch(component, /can\(['"]comments\.delete['"]\)/, 'retired comments.delete capability must not remain')

for (const target of ['c', 'child']) {
  assert.match(component, new RegExp(`<button\\s+v-if="canEditComment\\(${target}\\)"[^>]*>编辑<\\/button>`))
  assert.match(component, new RegExp(`<button\\s+v-if="canDeleteComment\\(${target}\\)"[^>]*>删除<\\/button>`))
}

assert.match(component, /comment\?\.can_interact\s*===\s*true/, 'reply controls must consume the server interaction decision')
// Mount the real selection component with Vue's renderer: verify reactive state and
// emitted actions without a browser, CSS assertions, or a replacement selection model.
const selectionSource = await readFile(join(root, 'components/admin/AdminSelectionBar.vue'), 'utf8')
const { descriptor } = parse(selectionSource)
const compiled = compileScript(descriptor, { id: 'selection-behavior-test', inlineTemplate: true })
const module = { exports: {} }
new Function('require', 'module', 'exports', 'computed', transformSync(compiled.content, { loader: 'ts', format: 'cjs' }).code)(
  createRequire(import.meta.url), module, module.exports, computed
)
const SelectionBar = module.exports.default
const node = (type, text = '') => ({ type, text, props: {}, children: [], parent: null })
const renderer = createRenderer({
  createElement: node,
  createText: text => node('#text', text),
  createComment: text => node('#comment', text),
  setText: (target, text) => { target.text = text },
  setElementText: (target, text) => { target.text = text; target.children = [] },
  patchProp: (target, key, previous, value) => { target.props[key] = value },
  insert: (child, parent, anchor = null) => {
    if (child.parent) child.parent.children.splice(child.parent.children.indexOf(child), 1)
    const at = anchor ? parent.children.indexOf(anchor) : -1
    parent.children.splice(at < 0 ? parent.children.length : at, 0, child)
    child.parent = parent
  },
  remove: child => { child.parent?.children.splice(child.parent.children.indexOf(child), 1) },
  parentNode: child => child.parent,
  nextSibling: child => child.parent?.children[child.parent.children.indexOf(child) + 1] || null,
})
const state = reactive({ selected: 0, total: 3, disabled: false, allSelected: undefined })
const events = []
const app = renderer.createApp({
  render: () => h(SelectionBar, {
    ...state,
    scopeLabel: '全选当前页',
    'onSelect-all': checked => { events.push(checked); state.selected = checked ? state.total : 0 },
    onClear: () => { events.push('clear'); state.selected = 0 },
  }),
})
app.component('UButton', { render() { return h('button', this.$attrs, this.$slots.default?.()) } })
const tree = node('root')
const find = (target, type) => target.type === type ? target : target.children.map(child => find(child, type)).find(Boolean)
app.mount(tree)
const checkbox = () => find(tree, 'input')
const clear = () => find(tree, 'button')
assert.equal(checkbox().props.checked, false)
assert.equal(clear().props.disabled, true, 'empty selection must disable clearing')
state.selected = 1
await nextTick()
assert.equal(checkbox().props.indeterminate, true, 'partial selection must render an indeterminate checkbox')
checkbox().props.onChange({ target: { checked: true } })
await nextTick()
assert.equal(state.selected, 3, 'select-all must emit true to the parent')
assert.equal(checkbox().props.checked, true)
assert.equal(checkbox().props.indeterminate, false)
checkbox().props.onChange({ target: { checked: false } })
await nextTick()
assert.equal(state.selected, 0, 'unchecking select-all must emit false to the parent')
state.selected = 1
await nextTick()
clear().props.onClick()
await nextTick()
assert.equal(state.selected, 0, 'clear must notify the parent and reset the selection')
assert.deepEqual(events, [true, false, 'clear'])
state.selected = 1
state.allSelected = true
await nextTick()
assert.equal(checkbox().props.checked, true, 'explicit scope completion must override a count mismatch')
state.disabled = true
await nextTick()
assert.equal(checkbox().props.disabled, true)
assert.equal(clear().props.disabled, true, 'busy state must disable both selection controls')
state.disabled = false
state.total = 0
state.selected = 0
state.allSelected = undefined
await nextTick()
assert.equal(checkbox().props.disabled, true, 'an empty scope must not offer select-all')
app.unmount()

console.log('comment capability visibility and shared selection behavior tests passed')
