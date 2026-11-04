import type { CommonTagTrigger } from './common-tag-picker'

export type CommonTagEditor = HTMLElement | HTMLTextAreaElement

export interface CommonTagSelection {
  editor: CommonTagEditor
  kind: 'textarea' | 'contenteditable'
  source: string
  start: number
  end: number
  before: string
  after: string
  prefix: string
  range?: Range
}
const blockNames: Record<string, true> = {
  P: true, DIV: true, LI: true, TD: true, TH: true, H1: true, H2: true,
  H3: true, H4: true, H5: true, H6: true, PRE: true, BLOCKQUOTE: true
}
const isTextarea = (editor: CommonTagEditor): editor is HTMLTextAreaElement => editor.tagName === 'TEXTAREA'

// Range.toString() omits <br> and paragraph boundaries. Include them when finding
// the active #prefix so a tag can never consume text from a previous visual line.
const textWithBreaks = (node: Node): string => {
  if (node.nodeType === 3) return node.textContent || ''
  if (node.nodeType === 1 && (node as Element).tagName === 'BR') return '\n'
  const text = Array.from(node.childNodes).map(textWithBreaks).join('')
  return node.nodeType === 1 && blockNames[(node as Element).tagName] ? `\n${text}` : text
}

const textRange = (editor: HTMLElement, start: number, end: number): Range => {
  const doc = editor.ownerDocument
  const range = doc.createRange()
  const walker = doc.createTreeWalker(editor, 4 /* SHOW_TEXT */)
  let offset = 0
  let foundStart = false
  let node: Node | null
  range.selectNodeContents(editor)
  range.collapse(false)
  while ((node = walker.nextNode())) {
    const length = node.textContent?.length || 0
    if (!foundStart && (start < offset + length || (start === offset + length && start === (editor.textContent?.length || 0)))) {
      range.setStart(node, Math.max(0, start - offset))
      foundStart = true
    }
    if (foundStart && end <= offset + length) {
      range.setEnd(node, Math.max(0, end - offset))
      return range
    }
    offset += length
  }
  return range
}

export const captureCommonTagSelection = (editor: CommonTagEditor | null): CommonTagSelection | null => {
  if (!editor || !editor.isConnected) return null
  if (isTextarea(editor)) {
    const start = editor.selectionStart
    const end = editor.selectionEnd
    return {
      editor, kind: 'textarea', source: editor.value, start, end,
      before: editor.value.slice(0, start), after: editor.value.slice(end),
      prefix: editor.value.slice(0, start)
    }
  }
  const selection = editor.ownerDocument.defaultView?.getSelection()
  if (!selection?.rangeCount) return null
  const range = selection.getRangeAt(0)
  if (!editor.contains(range.startContainer) || !editor.contains(range.endContainer)) return null
  const before = range.cloneRange()
  before.selectNodeContents(editor)
  before.setEnd(range.startContainer, range.startOffset)
  const after = range.cloneRange()
  after.selectNodeContents(editor)
  after.setStart(range.endContainer, range.endOffset)
  return {
    editor, kind: 'contenteditable', source: editor.textContent || '',
    start: before.toString().length, end: before.toString().length + range.toString().length,
    before: before.toString(), after: after.toString(),
    prefix: textWithBreaks(before.cloneContents()), range: range.cloneRange()
  }
}

export const narrowCommonTagSelection = (selection: CommonTagSelection, trigger: CommonTagTrigger): CommonTagSelection => {
  const length = trigger.end - trigger.start
  const start = selection.start - length
  let range: Range | undefined
  if (selection.kind === 'contenteditable') {
    const startRange = textRange(selection.editor, start, selection.end)
    range = selection.range?.cloneRange() || startRange
    range.setStart(startRange.startContainer, startRange.startOffset)
  }
  return {
    ...selection, start, before: selection.before.slice(0, -length),
    range
  }
}

export const restoreCommonTagRange = (selection: CommonTagSelection): Range | null => {
  const { editor } = selection
  if (!editor.isConnected || selection.kind !== 'contenteditable' || editor.textContent !== selection.source) return null
  if (selection.range && editor.contains(selection.range.startContainer) && editor.contains(selection.range.endContainer)) {
    const before = selection.range.cloneRange()
    before.selectNodeContents(editor)
    before.setEnd(selection.range.startContainer, selection.range.startOffset)
    if (before.toString().length === selection.start && selection.range.toString() === selection.source.slice(selection.start, selection.end)) {
      return selection.range.cloneRange()
    }
  }
  return textRange(editor, selection.start, selection.end)
}

export const insertCommonTagText = (
  selection: CommonTagSelection,
  text: string,
  insertText?: (text: string) => void
): boolean => {
  const { editor } = selection
  if (!editor.isConnected) return false
  if (isTextarea(editor)) {
    if (editor.disabled || editor.readOnly || editor.value !== selection.source) return false
    editor.focus({ preventScroll: true })
    editor.setSelectionRange(selection.start, selection.end)
    editor.setRangeText(text, selection.start, selection.end, 'end')
    editor.dispatchEvent(new Event('input', { bubbles: true }))
    return true
  }
  const range = restoreCommonTagRange(selection)
  if (!range) return false
  editor.focus({ preventScroll: true })
  const domSelection = editor.ownerDocument.defaultView?.getSelection()
  if (!domSelection) return false
  domSelection.removeAllRanges()
  domSelection.addRange(range)
  if (insertText) {
    // Vditor owns rendering, history and model updates; never mutate its DOM here.
    insertText(text)
  } else {
    range.deleteContents()
    const node = editor.ownerDocument.createTextNode(text)
    range.insertNode(node)
    range.setStartAfter(node)
    range.collapse(true)
    domSelection.removeAllRanges()
    domSelection.addRange(range)
    editor.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: text }))
  }
  return true
}

export const commonTagEventInScope = (target: EventTarget | null, editor: CommonTagEditor | null, menu: HTMLElement | null): boolean => {
  if (!target || typeof (target as Node).nodeType !== 'number') return false
  return !!(editor?.contains(target as Node) || menu?.contains(target as Node))
}

export const commonTagCaretRect = (selection: CommonTagSelection): DOMRect => {
  const { editor } = selection
  if (!isTextarea(editor)) {
    const range = restoreCommonTagRange(selection)
    range?.collapse(false)
    const rect = range?.getBoundingClientRect()
    if (rect && (rect.width || rect.height)) return rect
    const container = range?.endContainer
    const element = container?.nodeType === 1 ? container as Element : container?.parentElement
    return element?.getBoundingClientRect() || editor.getBoundingClientRect()
  }
  const doc = editor.ownerDocument
  const mirror = doc.createElement('div')
  const marker = doc.createElement('span')
  const rect = editor.getBoundingClientRect()
  const computed = doc.defaultView!.getComputedStyle(editor)
  for (const property of [
    'box-sizing', 'font-family', 'font-size', 'font-weight', 'font-style', 'line-height',
    'letter-spacing', 'text-transform', 'text-indent', 'text-align', 'word-spacing', 'tab-size',
    'padding-top', 'padding-right', 'padding-bottom', 'padding-left',
    'border-top-width', 'border-right-width', 'border-bottom-width', 'border-left-width', 'border-style'
  ]) mirror.style.setProperty(property, computed.getPropertyValue(property))
  Object.assign(mirror.style, {
    position: 'fixed', left: `${rect.left}px`, top: `${rect.top}px`,
    width: `${rect.width}px`, height: `${rect.height}px`, visibility: 'hidden',
    overflow: 'hidden', whiteSpace: 'pre-wrap', overflowWrap: 'break-word', pointerEvents: 'none'
  })
  mirror.setAttribute('aria-hidden', 'true')
  mirror.textContent = editor.value.slice(0, selection.end)
  marker.textContent = '\u200b'
  mirror.appendChild(marker)
  doc.body.appendChild(mirror)
  try {
    mirror.scrollTop = editor.scrollTop
    mirror.scrollLeft = editor.scrollLeft
    return marker.getBoundingClientRect()
  } finally {
    mirror.remove()
  }
}
