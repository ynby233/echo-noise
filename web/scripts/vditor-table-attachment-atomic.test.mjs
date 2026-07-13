import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import {
  getFixedEditorClipInsets,
  insertTableCellAtomicValue,
  resolveTableAttachmentTarget,
} from '../utils/vditor-table-attachment.ts'

const attachmentA = '[文件附件：a.pdf](/api/files/a.pdf)'
const attachmentB = '[文件附件：b.pdf](/api/files/b.pdf)'
const attachmentC = '[音频附件：c.webm](/api/files/c.webm)'

assert.deepEqual(
  insertTableCellAtomicValue('alpha omega', attachmentA, 5),
  {
    value: `alpha ${attachmentA} omega`,
    caretOffset: 6 + attachmentA.length,
  },
  'an attachment must be inserted at the captured caret, not appended to the cell'
)

assert.deepEqual(
  insertTableCellAtomicValue('omega', attachmentA, 0),
  { value: `${attachmentA} omega`, caretOffset: attachmentA.length },
  'insertion at the start must preserve the following text'
)

assert.deepEqual(
  insertTableCellAtomicValue('alpha', attachmentA, 999),
  { value: `alpha ${attachmentA}`, caretOffset: 6 + attachmentA.length },
  'out-of-range offsets must clamp to the end without losing content'
)

assert.deepEqual(
  insertTableCellAtomicValue(attachmentA, attachmentB, attachmentA.length),
  { value: `${attachmentA} ${attachmentB}`, caretOffset: attachmentA.length + 1 + attachmentB.length },
  'adjacent atomic attachments must retain a stable token boundary'
)

const mixedAttachmentValue = `left ${attachmentA} middle ${attachmentB} right`
const mixedAttachmentOffset = `left ${attachmentA} `.length
assert.deepEqual(
  insertTableCellAtomicValue(mixedAttachmentValue, attachmentC, mixedAttachmentOffset),
  {
    value: `left ${attachmentA} ${attachmentC} middle ${attachmentB} right`,
    caretOffset: mixedAttachmentOffset + attachmentC.length,
  },
  'inserting among mixed text and multiple atoms must preserve the captured order and caret boundary'
)

assert.deepEqual(
  getFixedEditorClipInsets(
    { left: 100, top: 20, right: 300, bottom: 120 },
    { left: 160, top: 0, right: 260, bottom: 100 }
  ),
  { top: 0, right: 40, bottom: 20, left: 60, visible: true },
  'a fixed cell editor must be clipped to the visible table viewport'
)

assert.deepEqual(
  getFixedEditorClipInsets(
    { left: 100, top: 20, right: 140, bottom: 60 },
    { left: 160, top: 0, right: 260, bottom: 100 }
  ),
  { top: 0, right: 0, bottom: 0, left: 40, visible: false },
  'a fully scrolled-out cell editor must not remain visible'
)

const editable = { connected: true }
const target = { editable, tableIndex: 0, rowIndex: 1, cellIndex: 2, offset: 17 }
assert.deepEqual(
  resolveTableAttachmentTarget(target, (candidate) => candidate.connected, (stored) => ({
    cell: `${stored.tableIndex}:${stored.rowIndex}:${stored.cellIndex}`,
    offset: stored.offset,
  })),
  { cell: '0:1:2', offset: 17 },
  'the upload lifecycle must preserve the captured source offset with the cell identity'
)

const editorPath = fileURLToPath(new URL('../components/index/VditorEditor.vue', import.meta.url))
const addFormPath = fileURLToPath(new URL('../components/index/AddForm.vue', import.meta.url))
const audioRecorderPath = fileURLToPath(new URL('../components/index/AudioRecorder.vue', import.meta.url))
const [editor, addForm, audioRecorder] = await Promise.all([
  readFile(editorPath, 'utf8'),
  readFile(addFormPath, 'utf8'),
  readFile(audioRecorderPath, 'utf8'),
])

assert.match(
  editor,
  /cell\.querySelector\('\.editor-table-attachment-marker'\)[\s\S]{0,300}?openInlineEditorTableAtomicEditor\(cell,\s*event\)/,
  'cells containing attachments must use the structured atomic editor rather than a source textarea'
)

assert.match(
  editor,
  /const\s+openInlineEditorTableAtomicEditor\s*=[\s\S]+?editor\.innerHTML\s*=\s*cell\.innerHTML[\s\S]+?placeCaretAtInlineEditorTableAtomicOffset\(offset\)/,
  'the structured editor must preserve text and attachment nodes while mapping the click to a stable source offset'
)

assert.match(
  editor,
  /const\s+onInlineEditorTableAtomicMarkerClick\s*=[\s\S]+?data-attachment-source[\s\S]+?dispatchEvent\(new MouseEvent\('click'/,
  'clicking an atomic marker in the structured editor must retain the existing preview interaction'
)

assert.doesNotMatch(
  editor,
  /targetLine\s*>=\s*renderedLineCount|editor-inline-table-cell-bottom-shield|INLINE_TABLE_CELL_BOTTOM_EDGE_SHIELD_MIN_PX/,
  'pointer placement must not manufacture line breaks or rely on a bottom-edge patch element'
)

const markerCss = editor.match(/\.vditor-container \.editor-table-attachment-marker \{[\s\S]*?\n\}/)?.[0] || ''
assert.match(markerCss, /max-width:\s*100%/)
assert.match(markerCss, /white-space:\s*normal/)
assert.match(markerCss, /overflow-wrap:\s*anywhere/)
assert.match(markerCss, /word-break:\s*break-word/)

const atomicMarkerCss = editor.match(/\.editor-inline-table-cell-atomic-editor \.editor-table-attachment-marker \{[\s\S]*?\n\}/)?.[0] || ''
assert.match(atomicMarkerCss, /max-width:\s*100%/)
assert.match(atomicMarkerCss, /white-space:\s*normal/)
assert.match(atomicMarkerCss, /overflow-wrap:\s*anywhere/)
assert.match(atomicMarkerCss, /word-break:\s*break-word/)
assert.doesNotMatch(
  editor,
  /:global\(\.editor-inline-table-cell/,
  'the component uses an unscoped style block, so body-mounted editors must use valid global selectors directly'
)

assert.match(
  editor,
  /pendingEditorTableAttachmentInsertionTarget\s*=\s*\{[\s\S]{0,320}?offset:\s*getEditorTableCellInsertionOffset\(cell\)/,
  'upload preparation must capture the caret offset together with the table cell'
)

assert.match(
  audioRecorder,
  /@pointerdown="prepareRecordingInsertTarget"[\s\S]+?emit\('prepare-insert'\)/,
  'recording must capture the insertion target before focus leaves the editor'
)
assert.match(audioRecorder, /emit\('insert-cancelled'\)/, 'recording cancellation and failures must clear the captured target')
assert.match(
  addForm,
  /<AudioRecorder[\s\S]+?@prepare-insert="prepareEditorAttachmentInsert"[\s\S]+?@insert-cancelled="clearEditorAttachmentInsertTarget"/,
  'AddForm must connect the recording lifecycle to the shared attachment target lifecycle'
)

console.log('vditor table attachment atomic-model tests passed')
