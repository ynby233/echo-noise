import assert from 'node:assert/strict'
import {
  commonTagInsertion, commonTagKeyAction, filterCommonTags, findCommonTagTrigger,
  moveCommonTagIndex, normalizeCommonTags, replaceCommonTagRange
} from '../utils/common-tag-picker.ts'
import {
  captureCommonTagSelection, commonTagEventInScope, insertCommonTagText,
  narrowCommonTagSelection, restoreCommonTagRange
} from '../utils/common-tag-picker-dom.ts'

assert.deepEqual(normalizeCommonTags([' 工作 ', '#生活', '工作', '', 'bad tag', 'bad.tag', '项目/计划']), ['工作', '生活'])
assert.deepEqual(filterCommonTags(['工作', '工作记录', '生活', 'Vue'], '工'), ['工作', '工作记录'])
assert.deepEqual(filterCommonTags(['Vue', '生活'], 'vU'), ['Vue'])
assert.deepEqual(filterCommonTags(['工作'], '不存在'), [])
assert.deepEqual(findCommonTagTrigger('前文 #'), { start: 3, end: 4, query: '' })
assert.deepEqual(findCommonTagTrigger('前文 #工 后文', 5), { start: 3, end: 5, query: '工' })
for (const text of ['#工作 ', '#工\n下一行', '#工。', '普通内容', '#工\n']) assert.equal(findCommonTagTrigger(text), null)
assert.equal(findCommonTagTrigger('#项目/计'), null)
assert.equal(commonTagInsertion('工作', '', ''), '#工作 ')
assert.equal(commonTagInsertion('工作', '前文', '后文'), ' #工作 ')
assert.equal(commonTagInsertion('工作', '前文 ', ' 后文'), '#工作')
assert.deepEqual(replaceCommonTagRange('前文 #工 后文', 3, 5, '工作'), {
  value: '前文 #工作 后文', caret: 6, text: '#工作', start: 3, end: 5
})
assert.equal(replaceCommonTagRange('前文 #工余文', 3, 5, '工作').value, '前文 #工作 余文')
assert.equal(replaceCommonTagRange('前文 选中文字 后文', 3, 8, '生活').value, '前文 #生活 后文')
assert.equal(replaceCommonTagRange('', 0, 0, '工作').value, '#工作 ')

const key = (name, overrides = {}) => ({
  key: name, shiftKey: false, ctrlKey: false, metaKey: false, altKey: false,
  isComposing: false, keyCode: 0, ...overrides
})
for (const [event, expected] of [
  [key('Tab'), 'next'], [key('Tab', { shiftKey: true }), 'previous'],
  [key('ArrowDown'), 'next'], [key('ArrowUp'), 'previous'],
  [key('Enter'), 'select'], [key('Escape'), 'close']
]) {
  assert.equal(commonTagKeyAction(event, true, true), expected)
  assert.equal(commonTagKeyAction(event, false, true), null, 'closed menus must not consume editor keys')
  assert.equal(commonTagKeyAction(event, true, false), null, 'other editors and controls must retain their keys')
  assert.equal(commonTagKeyAction(event, true, true, true), null, 'composition must retain every key, including Escape')
  assert.equal(commonTagKeyAction({ ...event, isComposing: true }, true, true), null)
  assert.equal(commonTagKeyAction({ ...event, keyCode: 229 }, true, true), null)
}
assert.equal(commonTagKeyAction(key('Enter', { ctrlKey: true }), true, true), null)
assert.equal(moveCommonTagIndex(0, 3, -1), 2)
assert.equal(moveCommonTagIndex(2, 3, 1), 0)
assert.equal(moveCommonTagIndex(0, 0, 1), -1)

class Textarea extends EventTarget {
  tagName = 'TEXTAREA'
  nodeType = 1
  isConnected = true
  disabled = false
  readOnly = false
  constructor(value, caret) {
    super()
    this.value = value
    this.selectionStart = this.selectionEnd = caret
  }
  focus() { this.focused = true }
  contains(node) { return node === this }
  setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end }
  setRangeText(text, start, end) {
    this.value = this.value.slice(0, start) + text + this.value.slice(end)
    this.selectionStart = this.selectionEnd = start + text.length
  }
}
const textarea = new Textarea('前文 #工 后文', 5)
let model = textarea.value
let inputCount = 0
textarea.addEventListener('input', event => { model = textarea.value; inputCount++; assert.equal(event.bubbles, true) })
const caret = captureCommonTagSelection(textarea)
const target = narrowCommonTagSelection(caret, findCommonTagTrigger(caret.prefix))
assert.equal(insertCommonTagText(target, commonTagInsertion('工作', target.before, target.after), () => assert.fail('textarea must not call Vditor')), true)
assert.equal(model, '前文 #工作 后文', 'a real input event must update the v-model consumer')
assert.equal(textarea.selectionStart, 6)
assert.equal(textarea.selectionEnd, 6)
assert.equal(inputCount, 1)
assert.equal(insertCommonTagText(target, '#生活 '), false, 'a stale draft must never be overwritten')
textarea.isConnected = false
assert.equal(insertCommonTagText(captureCommonTagSelection(new Textarea('test', 0)), '#工作 '), true)
assert.equal(insertCommonTagText(target, '#工作 '), false)
const readonly = new Textarea('不变', 1)
readonly.readOnly = true
assert.equal(insertCommonTagText(captureCommonTagSelection(readonly), '#工作 '), false)

// Small DOM range double: checks the public Vditor insertion path, including
// prefixes split across text nodes and ranges surviving a render with new nodes.
const root = {
  tagName: 'PRE', nodeType: 1, isConnected: true,
  nodes: [{ nodeType: 3, textContent: '前文 #' }, { nodeType: 3, textContent: '工 后文' }],
  get textContent() { return this.nodes.map(node => node.textContent).join('') },
  contains(node) { return node === this || this.nodes.includes(node) },
  focus() { this.focused = true }
}
const offsetOf = (node, offset) => node === root ? (offset ? root.textContent.length : 0)
  : root.nodes.slice(0, root.nodes.indexOf(node)).reduce((sum, item) => sum + item.textContent.length, 0) + offset
class TestRange {
  constructor() { this.selectNodeContents(root) }
  selectNodeContents() { this.startContainer = this.endContainer = root; this.startOffset = 0; this.endOffset = 1 }
  setStart(node, offset) { this.startContainer = node; this.startOffset = offset }
  setEnd(node, offset) { this.endContainer = node; this.endOffset = offset }
  cloneRange() { return Object.assign(new TestRange(), this) }
  collapse(toStart) {
    if (toStart) this.setEnd(this.startContainer, this.startOffset)
    else this.setStart(this.endContainer, this.endOffset)
  }
  toString() { return root.textContent.slice(offsetOf(this.startContainer, this.startOffset), offsetOf(this.endContainer, this.endOffset)) }
  cloneContents() { return { nodeType: 11, childNodes: [{ nodeType: 3, textContent: this.toString() }] } }
}
let current = new TestRange()
current.setStart(root.nodes[1], 1)
current.collapse(true)
const domSelection = { rangeCount: 1, getRangeAt: () => current, removeAllRanges() {}, addRange(range) { current = range } }
root.ownerDocument = {
  defaultView: { getSelection: () => domSelection },
  createRange: () => new TestRange(),
  createTreeWalker: () => { let index = 0; return { nextNode: () => root.nodes[index++] || null } }
}
const domCaret = captureCommonTagSelection(root)
const domTarget = narrowCommonTagSelection(domCaret, findCommonTagTrigger(domCaret.prefix))
let inserted
assert.equal(insertCommonTagText(domTarget, commonTagInsertion('工作', domTarget.before, domTarget.after), text => {
  assert.equal(current.toString(), '#工', 'restore exactly the typed prefix before calling insertValue')
  inserted = text
}), true)
assert.equal(inserted, '#工作')
assert.equal(root.textContent, '前文 #工 后文', 'Vditor owns its DOM changes')
root.nodes = [{ nodeType: 3, textContent: '前文 #工 后文' }]
assert.equal(restoreCommonTagRange(domTarget).toString(), '#工', 'restore offsets after Vditor replaces text nodes')
root.nodes[0].textContent = '已切换草稿'
assert.equal(restoreCommonTagRange(domTarget), null)

const firstParagraphText = { nodeType: 3, textContent: '第一段' }
const secondParagraphText = { nodeType: 3, textContent: '第二段' }
root.nodes = [firstParagraphText, secondParagraphText]
current = new TestRange()
current.setStart(secondParagraphText, 0)
current.collapse(true)
const paragraphStart = captureCommonTagSelection(root)
assert.equal(paragraphStart.start, firstParagraphText.textContent.length)
assert.equal(restoreCommonTagRange(paragraphStart).startContainer, secondParagraphText,
  'the second paragraph start must not become the first paragraph end at the same flat text offset')
assert.equal(insertCommonTagText(paragraphStart, '#工作 ', () => {
  assert.equal(current.startContainer, secondParagraphText)
  assert.equal(current.startOffset, 0)
}), true)

const emptyParagraph = { nodeType: 1, tagName: 'P', textContent: '', childNodes: [] }
root.nodes = [firstParagraphText, emptyParagraph]
current = new TestRange()
current.setStart(emptyParagraph, 0)
current.collapse(true)
const emptyParagraphCaret = captureCommonTagSelection(root)
assert.equal(restoreCommonTagRange(emptyParagraphCaret).startContainer, emptyParagraph,
  'a toolbar insertion in an empty paragraph must retain that actual DOM boundary')
assert.equal(insertCommonTagText(emptyParagraphCaret, '#生活 ', () => {
  assert.equal(current.startContainer, emptyParagraph)
  assert.equal(current.startOffset, 0)
}), true)

const menu = { contains: node => node === option }
const option = { nodeType: 1 }
assert.equal(commonTagEventInScope(textarea, textarea, menu), true)
assert.equal(commonTagEventInScope(option, textarea, menu), true)
assert.equal(commonTagEventInScope(new Textarea('', 0), textarea, menu), false)
assert.equal(commonTagEventInScope(null, textarea, menu), false)
assert.equal(commonTagEventInScope({}, textarea, menu), false)

console.log('common tag picker behavior tests passed')
