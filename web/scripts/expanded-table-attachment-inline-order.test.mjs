import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const editorPath = fileURLToPath(new URL('../components/index/VditorEditor.vue', import.meta.url))
const editor = await readFile(editorPath, 'utf8')

const sliceBetween = (source, startMarker, endMarker) => {
  const start = source.indexOf(startMarker)
  assert.notEqual(start, -1, `missing start marker: ${startMarker}`)
  const end = source.indexOf(endMarker, start + startMarker.length)
  assert.notEqual(end, -1, `missing end marker: ${endMarker}`)
  return source.slice(start, end)
}

const expandDialogTemplate = sliceBetween(editor, '<section class="editor-table-expand-dialog"', '</Teleport>')

// The expanded dialog must edit each cell as a single inline-rendered surface.
// A textarea cannot host inline attachment markers, so it forced the old split
// rendering that relocated every attachment to the bottom of the cell.
assert.doesNotMatch(
  expandDialogTemplate,
  /<textarea/,
  'expanded table cells must not fall back to a textarea, which cannot render inline attachment markers'
)
assert.match(
  expandDialogTemplate,
  /class="editor-table-expand-cell-editor"[\s\S]*?:contenteditable="expandedTableEditable \? 'true' : 'false'"/,
  'expanded table cells must edit through a contenteditable surface gated by expandedTableEditable'
)

// The dedicated attachment list below the cell text is what visually moved
// attachments to the cell bottom. It must be gone entirely.
for (const removed of [
  'editor-table-expand-attachments',
  'editor-table-expand-attachment-tag',
  'mergeExpandedCellEditorText',
]) {
  assert.doesNotMatch(
    editor,
    new RegExp(removed.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')),
    `${removed} belongs to the split render that reordered attachments and must not come back`
  )
}

// Expanded rendering has to reuse the same cell serializer as the inline editor,
// otherwise the two views can drift apart again.
const expandedCellHtmlFn = sliceBetween(editor, 'const expandedTableCellEditorHtml =', 'const expandedTableCellEditorElements =')
assert.match(
  expandedCellHtmlFn,
  /editorTextToDomTableCellHtml\(value\)/,
  'expanded cell HTML must be produced by editorTextToDomTableCellHtml, the same helper the inline editor uses'
)
assert.doesNotMatch(
  expandedCellHtmlFn,
  /stripAttachmentMarkersFromEditorText/,
  'expanded cell HTML must not strip attachment markers out of the cell text'
)
assert.match(
  editor,
  /const setEditorTableDomCellText = \([\s\S]*?editorTextToDomTableCellHtml\(value\)/,
  'the inline editor must keep using editorTextToDomTableCellHtml so both views share one renderer'
)

// Editing inside the dialog must serialize the DOM back to cell text in place,
// preserving the original text/attachment/text ordering.
const commitFn = sliceBetween(editor, 'const commitExpandedTableCellEditor =', 'const expandedTableBaseColumnWidths =')
assert.match(
  commitFn,
  /row\[cellIndex\] = editorTableContentTextFromElement\(editor\)/,
  'committing an expanded cell must serialize the live DOM order back into the row'
)
assert.doesNotMatch(
  commitFn,
  /join\('\\n'\)/,
  'committing an expanded cell must not rebuild the value by appending attachments after the text'
)

// HTML is written only while the dialog opens, so typing never resets the caret.
assert.match(
  editor,
  /const registerExpandedTableCellEditor = \([\s\S]*?if \(el\.dataset\.expandedRendered === '1'\) return/,
  'expanded cell HTML must be written once per open, not on every input'
)

// The row-height probe must measure the contenteditable surface, since that is
// now what wraps inline markers.
assert.match(
  editor,
  /const measureExpandedTableCellContentHeight = \([\s\S]*?probe\.className = 'editor-table-expand-cell-editor'[\s\S]*?probe\.innerHTML = editor\.innerHTML/,
  'auto row height must be measured against a clone of the contenteditable cell editor'
)

// Inline markers wrap with the column instead of being clipped to one line,
// which is what keeps the expanded view identical to the inline view.
assert.match(
  editor,
  /\.editor-table-expand-cell-editor \.editor-table-attachment-marker \{[\s\S]*?display: inline;[\s\S]*?white-space: normal;[\s\S]*?\}/,
  'expanded attachment markers must render inline and wrap like ordinary text'
)
assert.match(
  editor,
  /\.editor-table-expand-cell-editor \{[\s\S]*?white-space: pre-wrap;[\s\S]*?\}/,
  'the expanded cell editor must preserve authored line breaks'
)

// Attachment marker labels must stay within a bounded length in BOTH views, with
// the file extension kept as the tail, while the markdown source stays intact.
assert.match(
  editor,
  /const ATTACHMENT_MARKER_MAX_NAME_LENGTH = \d+/,
  'a single bound must define the marker label length for every view'
)
assert.match(
  editor,
  /const truncateAttachmentDisplayName = \(name: string\) => \{[\s\S]*?ATTACHMENT_MARKER_ELLIPSIS \+ extension/,
  'long attachment names must be middle-truncated so the extension remains the tail'
)
assert.match(
  editor,
  /const editorAttachmentInfoToTableMarkerHtml = \([\s\S]*?escapeTableCellHtml\(attachmentMarkerDisplayTitle\(info\)\)/,
  'the HTML marker builder must render the truncated label'
)
assert.match(
  editor,
  /const createEditorTableAttachmentMarkerElement = \([\s\S]*?marker\.textContent = attachmentMarkerDisplayTitle\(info\)/,
  'the DOM marker builder must render the same truncated label as the HTML builder'
)
assert.match(
  editor,
  /const createEditorTableAttachmentMarkerElement = \([\s\S]*?marker\.title = info\.title/,
  'the untruncated name must stay reachable as a native tooltip'
)
assert.match(
  editor,
  /const editorAttachmentInfoToTableMarkerHtml = \([\s\S]*?title="\$\{safeFullTitle\}"/,
  'the HTML marker must also expose the untruncated name as a tooltip'
)
assert.match(
  editor,
  /const editorAttachmentInfoToTableMarkerHtml = \([\s\S]*?data-attachment-source="\$\{safeSource\}"/,
  'truncation must never touch data-attachment-source, which is what serialization reads back'
)
assert.match(
  editor,
  /const replaceAttachmentNodesWithSourceText = \([\s\S]*?attachmentInfoToMarkdownSource\(info\)/,
  'serialization must rebuild the full markdown source, not the truncated label'
)

// Column width estimation must measure what the cell shows. Stripping markers to
// an empty string collapsed marker-only columns to the minimum width.
assert.match(
  editor,
  /const estimateTableLineWidth = \([\s\S]*?attachmentMarkersToDisplayTitleText\(/,
  'column width estimation must account for the rendered marker label'
)
assert.doesNotMatch(
  editor,
  /const estimateTableLineWidth = \([\s\S]*?stripAttachmentMarkersFromEditorText\(/,
  'column width estimation must not treat an attachment marker as zero-width text'
)

console.log('expanded table inline attachment order checks passed')