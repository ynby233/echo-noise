import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createJiti } from 'jiti'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const jiti = createJiti(import.meta.url)
const {
  MARKDOWN_BLANK_LINE_SENTINEL,
  MARKDOWN_PRESERVED_BLANK_LINE_CLASS,
  encodeMarkdownExtraBlankLines,
  isMarkdownBlankLineSentinel,
} = await jiti.import(join(webRoot, 'utils/markdown-blank-lines.ts'))

assert.equal(MARKDOWN_BLANK_LINE_SENTINEL, '\u00a0')
assert.equal(MARKDOWN_PRESERVED_BLANK_LINE_CLASS, 'markdown-preserved-blank-line')
assert.equal(isMarkdownBlankLineSentinel('\u00a0'), true)
assert.equal(isMarkdownBlankLineSentinel('\u200b\u00a0'), true)
assert.equal(isMarkdownBlankLineSentinel('x'), false)

assert.equal(
  encodeMarkdownExtraBlankLines('A\n\n\nB'),
  'A\n\n\u00a0\n\nB',
  'raw markdown with more than one blank separator should gain an explicit blank-line sentinel'
)

assert.equal(
  encodeMarkdownExtraBlankLines('A\n\n\u00a0\n\nB'),
  'A\n\n\u00a0\n\nB',
  'already encoded blank-line sentinels must stay idempotent'
)

assert.equal(
  encodeMarkdownExtraBlankLines('A\r\n\r\n\r\nB'),
  'A\n\n\u00a0\n\nB',
  'Windows newlines should normalize before blank-line encoding'
)

assert.equal(
  encodeMarkdownExtraBlankLines('```js\nA\n\n\nB\n```\n\n\nC'),
  '```js\nA\n\n\nB\n```\n\n\u00a0\n\nC',
  'blank lines inside fenced code blocks must remain code content'
)

assert.equal(
  encodeMarkdownExtraBlankLines('~~~\nA\n\n\nB\n~~~\n\n\nC'),
  '~~~\nA\n\n\nB\n~~~\n\n\u00a0\n\nC',
  'tilde fenced code blocks must be protected from blank-line sentinels'
)

const editor = await readFile(join(webRoot, 'components/index/VditorEditor.vue'), 'utf8')
const addForm = await readFile(join(webRoot, 'components/index/AddForm.vue'), 'utf8')
const renderer = await readFile(join(webRoot, 'components/index/MarkdownRenderer.vue'), 'utf8')
const announcement = await readFile(join(webRoot, 'components/widgets/AnnouncementBar.vue'), 'utf8')

assert.match(
  editor,
  /const\s+insertPreservedBlankLineBefore\s*=/,
  'pressing Enter at the start of a text block must create a real blank block before the text'
)

assert.match(
  editor,
  /const\s+PRESERVED_BLANK_LINE_DOM_ANCHOR\s*=\s*'\\u200b'/,
  'editor blank-line DOM anchors must be zero-width so the caret does not look shifted right'
)

assert.match(
  editor,
  /const\s+isCaretAtStartOfPlainBlock\s*=/,
  'leading Enter handling must be based on the caret position inside the real editor block'
)

assert.match(
  editor,
  /insertPreservedBlankLineBefore\(block\)[\s\S]{0,120}?placeCaretInPlainBlock\(block\)/,
  'plain leading Enter must move the line down while returning the caret to the moved line start'
)

assert.match(
  editor,
  /const\s+needsDomFallback\s*=\s*getEditorTables\(\)\.length\s*\|\|\s*hasEditorSoftBreakDom\(\)/,
  'preserved blank-line DOM must force DOM serialization instead of trusting Vditor getValue()'
)

assert.match(
  editor,
  /block\.classList\.contains\('vditor-preserved-blank-line'\)[\s\S]+?return\s+MARKDOWN_BLANK_LINE_SENTINEL[\s\S]+?rawText\.replace\(\/\\u00a0\/g,\s*' '\)/,
  'plain text block serialization must not save Vditor caret NBSP as normal text'
)

assert.match(
  editor,
  /const\s+serializePlainEditorLinesAsMarkdown\s*=[\s\S]+?appendPreservedBlankLine[\s\S]+?MARKDOWN_BLANK_LINE_SENTINEL/,
  'plain editor blank-line blocks must serialize as explicit sentinel paragraphs so preview keeps the same visual count'
)

assert.match(
  editor,
  /return\s+serializePlainEditorLinesAsMarkdown\(pieces\)/,
  'plain editor blocks must serialize through the line model, not paragraph-separated double lines'
)

assert.match(
  editor,
  /const\s+hasEditorPlainBlockDom\s*=/,
  'multi-line plain editor DOM must force DOM serialization instead of trusting Vditor paragraph markdown'
)

assert.match(
  editor,
  /const\s+handlePlainEditorBackspaceAtLineBoundary\s*=/,
  'preserved blank-line deletion must be handled as line-model editing instead of native paragraph merging'
)

assert.match(
  editor,
  /const\s+TABLE_CELL_BREAK_PLACEHOLDER\s*=\s*'%%NW_TABLE_BR%%'[\s\S]+?const\s+TABLE_CELL_BREAK_SOURCE_RE\s*=/,
  'table cell logical line breaks need a non-HTML placeholder for insertValue paths'
)

assert.match(
  editor,
  /const\s+encodePlainEditorPasteValue\s*=[\s\S]+?getMarkdownTableBlocks\(source\)[\s\S]+?block\.lines\.map\(\(line\)\s*=>\s*line\.replace\(TABLE_CELL_BREAK_RE,\s*TABLE_CELL_BREAK_PLACEHOLDER\)\)[\s\S]+?encodeMarkdownExtraBlankLines/,
  'pasting mixed prose and tables must preserve prose blank lines without letting table cell <br> content split the table'
)

assert.match(
  editor,
  /options\.blockBoundary\s*&&\s*output\s*&&\s*!output\.endsWith\('\\n'\)\)\s*output\s*\+=\s*'\\n'/,
  'pasted table blocks must keep a real block boundary so following prose is not absorbed as a table row'
)

assert.match(
  editor,
  /acceptNode\(node\)[\s\S]+?\(\?:<br\\s\*\\\/\?\\s\*>|\%\%NW_TABLE_BR\%\%\)/,
  'table break text-node replacement must scan both raw <br> text and paste placeholders'
)

assert.match(
  editor,
  /const\s+insertPlainEditorPastedTextWithBlankLines\s*=[\s\S]+?encodePlainEditorPasteValue\(pastedText\)[\s\S]+?materializeEditorPreservedBlankLineBlocks\(root\)/,
  'pasting text with multiple blank lines must normalize and materialize those blank lines before Vditor can collapse them'
)

assert.match(
  editor,
  /if\s*\(handlePlainEditorPasteWithBlankLines\(event\)\)\s*return/,
  'plain paste blank-line preservation must run before the generic blank-line input cleanup'
)

assert.match(
  editor,
  /document\.addEventListener\('paste',\s*onEditorPaste,\s*true\)[\s\S]+?root\.addEventListener\('paste',\s*onEditorPaste,\s*true\)/,
  'plain paste blank-line preservation must catch real clipboard paste events before Vditor handles them'
)

assert.match(
  editor,
  /const\s+clearLastEditorTableSelection\s*=\s*\(\)\s*=>\s*\{[\s\S]+?lastEditorTableSelectionRange\s*=\s*null[\s\S]+?lastEditorTableSelectionState\s*=\s*null[\s\S]+?\}/,
  'table-cell fallback selection must have a single cleanup path'
)

assert.match(
  editor,
  /const\s+clearLastEditorTableSelection\s*=\s*\(\)\s*=>\s*\{[\s\S]+?getEditorTableCellFromRange\(range\)[\s\S]+?selection\?\.removeAllRanges\(\)[\s\S]+?lastEditorTableSelectionRange\s*=\s*null/,
  'table-cell fallback cleanup must also drop a stale DOM selection left behind by the file picker'
)

assert.match(
  editor,
  /if\s*\(hasAttachmentMarker\(text\)\)\s*\{[\s\S]+?applyEditorTableCellSourceValue\(cell,\s*`\$\{current\}\$\{separator\}\$\{text\}`\)[\s\S]+?clearLastEditorTableSelection\(\)[\s\S]+?refreshAttachmentLinksFromEditor\(\)/,
  'attachment insertion into a table cell must consume the stored table target instead of trapping future uploads'
)

assert.match(
  editor,
  /const\s+insertAttachmentSourceValue\s*=[\s\S]+?vditorInstance\.setValue\(nextValue\)[\s\S]+?clearLastEditorTableSelection\(\)/,
  'attachment insertion outside tables must clear stale table fallback before appending to the editor body'
)

assert.doesNotMatch(
  editor,
  /querySelectorAll<HTMLElement>\('td,th'\)\.forEach\(\(cell\)\s*=>\s*renderAttachmentMarkersInEditableRoot\(cell\)\)/,
  'table cells must keep Vditor native attachment link nodes instead of being rewritten to plain custom anchors'
)

assert.match(
  editor,
  /const\s+editorTextToDomTableCellHtml\s*=[\s\S]+?escapeTableCellHtml\(line\)[\s\S]+?const\s+setEditorTableDomCellText\s*=[\s\S]+?cell\.innerHTML\s*=\s*editorTextToDomTableCellHtml\(value\)/,
  'live table-cell DOM mirrors must not reuse HTML-table source serialization that turns attachments into custom anchors'
)

assert.match(
  editor,
  /const\s+handleEditorTableBackspaceAtLineBoundary\s*=/,
  'table-cell Backspace at line boundaries must stay inside the current cell'
)

assert.match(
  editor,
  /const\s+openInlineEditorTableCellTextarea\s*=[\s\S]+?const\s+textarea\s*=\s*ensureInlineEditorTableTextarea\(\)[\s\S]+?inlineEditorTextareaCaretFromPoint\(textarea,\s*cell,\s*baseText,\s*event\)/,
  'inline table-cell clicks must enter the textarea editing model used for stable caret and IME behavior'
)

const tableMouseDownStart = editor.indexOf('const onEditorTableMouseDown =')
const tableMouseDownEnd = editor.indexOf('const onPlainEditorBlankAreaMouseDown =', tableMouseDownStart)
const tableMouseDownHandler = editor.slice(tableMouseDownStart, tableMouseDownEnd)

assert.match(
  tableMouseDownHandler,
  /openInlineEditorTableCellTextarea\(cell,\s*event\)/,
  'table-cell mousedown must open the inline textarea editor instead of relying on contenteditable caret placement'
)

assert.doesNotMatch(
  tableMouseDownHandler,
  /placeCaretInEditorTableCellVisualLine\(cell,\s*event\)/,
  'table-cell mousedown must not use the legacy contenteditable visual-line caret path'
)

assert.match(
  editor,
  /isInlineEditorTableTextareaEvent\(event\)[\s\S]{0,120}?event\.stopPropagation\(\)[\s\S]{0,80}?return true/,
  'native textarea beforeinput must bypass the Vditor table contenteditable input shim'
)

assert.match(
  editor,
  /targetLine\s*>=\s*renderedLineCount[\s\S]{0,260}?targetLine\s*-\s*renderedLineCount\s*\+\s*1/,
  'clicking a visual blank row must create only virtual textarea newlines before real input commits them'
)

assert.match(
  editor,
  /INLINE_TABLE_CELL_BOTTOM_EDGE_SHIELD_MIN_PX/,
  'inline table caret mapping must reserve a bottom shield so clicks cannot land in the cell border seam'
)

assert.match(
  editor,
  /const\s+ensureInlineEditorTableTextareaBottomShield\s*=[\s\S]+?editor-inline-table-cell-bottom-shield[\s\S]+?const\s+positionInlineEditorTableTextareaBottomShield\s*=[\s\S]+?metrics\.shieldTop[\s\S]+?metrics\.shieldHeight/,
  'inline table editing must place a real pointer shield over the bottom seam instead of relying on native textarea hit testing'
)

assert.match(
  editor,
  /const\s+stopInlineEditorTableTextareaBottomShieldEvent\s*=[\s\S]+?event\.preventDefault\(\)[\s\S]+?event\.stopPropagation\(\)/,
  'the bottom shield must intercept pointer events before the textarea can move the caret into the seam'
)

assert.match(
  editor,
  /const\s+inlineEditorTextareaLineFromPoint\s*=[\s\S]+?Math\.min\(metrics\.maxLine,\s*rawLine\)/,
  'initial table-cell clicks must clamp vertical caret placement to fully available visual lines'
)

assert.match(
  editor,
  /if\s*\(!baseText\s*&&\s*targetLine\s*<=\s*0\)\s*return\s*\{\s*value:\s*'',\s*offset:\s*0\s*\}/,
  'clicking the first visual line of an empty table cell must place the textarea caret at offset 0'
)

assert.match(
  editor,
  /inlineEditorTableTextareaState\s*=\s*\{[\s\S]{0,900}?dirty:\s*false[\s\S]{0,900}?textarea\.value\s*=\s*caret\.value/,
  'opening the inline table textarea may add virtual blank rows but must not mark the cell dirty'
)

assert.match(
  editor,
  /const\s+positionInlineEditorTableTextarea\s*=[\s\S]+?getFixedCoordinateScale\(\)[\s\S]+?getFixedRect\(cell,\s*scale\)/,
  'inline table textarea positioning must use the same scaled fixed-coordinate model as the existing floating controls'
)

assert.match(
  editor,
  /const\s+captureInlineEditorTextareaStyle\s*=[\s\S]+?fontFamily:\s*style\.fontFamily[\s\S]+?fontSize:\s*style\.fontSize[\s\S]+?lineHeight:\s*style\.lineHeight[\s\S]+?padding:\s*style\.padding/,
  'inline table textarea must snapshot the original cell typography before the cell is hidden'
)

assert.match(
  editor,
  /const\s+resizeInlineEditorTableTextareaToContent\s*=[\s\S]+?updateInlineEditorTableTextareaCellLayoutMirror\(\)[\s\S]+?textarea\.scrollHeight[\s\S]+?state\.cell\.style\.height\s*=\s*`\$\{requiredHeight\}px`[\s\S]+?positionInlineEditorTableTextarea\(\{\s*fitContent:\s*false\s*\}\)/,
  'inline table textarea must resize from the live textarea content and keep the hidden table cell as a layout mirror'
)

assert.match(
  editor,
  /const\s+onInlineEditorTableTextareaInput\s*=\s*\(\)\s*=>\s*\{[\s\S]{0,140}?inlineEditorTableTextareaState\.dirty\s*=\s*true[\s\S]{0,100}?scheduleInlineEditorTableTextareaSync\(\)/,
  'inline table textarea input must only mark pending text instead of forcing full editor emission on every keystroke'
)

const inlineTextareaScheduleStart = editor.indexOf('const scheduleInlineEditorTableTextareaSync =')
const inlineTextareaScheduleEnd = editor.indexOf('const closeInlineEditorTableTextarea =', inlineTextareaScheduleStart)
const inlineTextareaScheduleHandler = editor.slice(inlineTextareaScheduleStart, inlineTextareaScheduleEnd)

assert.match(
  inlineTextareaScheduleHandler,
  /resizeInlineEditorTableTextareaToContent\(\)/,
  'inline table textarea input must resize the overlay and edited cell instead of leaving the editor clipped'
)

assert.doesNotMatch(
  inlineTextareaScheduleHandler,
  /syncInlineEditorTableTextareaToCell|emitEditorValue|vditorInstance\.setValue/,
  'inline table textarea input must not force a full editor/source commit on every keystroke'
)

const inlineTextareaMirrorStart = editor.indexOf('const updateInlineEditorTableTextareaCellLayoutMirror =')
const inlineTextareaMirrorEnd = editor.indexOf('const resizeInlineEditorTableTextareaToContent =', inlineTextareaMirrorStart)
const inlineTextareaMirrorHandler = editor.slice(inlineTextareaMirrorStart, inlineTextareaMirrorEnd)

assert.match(
  inlineTextareaMirrorHandler,
  /restoreInlineEditorTableTextareaCellLayout\(state\)[\s\S]+?setEditorTableDomCellText\(state\.cell,\s*value,\s*\/\\n\$\/\.test\(value\)\)[\s\S]+?markEditorTableCellSourceDirty\(state\.cell,\s*value\)/,
  'inline table textarea input must update the hidden cell DOM as a layout mirror so Backspace can shrink row height immediately'
)

assert.doesNotMatch(
  inlineTextareaMirrorHandler,
  /emitEditorValue|vditorInstance\.setValue|syncInlineEditorTableTextareaToCell/,
  'the live layout mirror must stay local and not trigger a full editor rerender while typing'
)

assert.match(
  editor,
  /cell\.style\.color\s*=\s*'transparent'[\s\S]{0,120}?cell\.style\.caretColor\s*=\s*'transparent'/,
  'inline table editing must hide the underlying cell with inline styles so textarea text is never doubled'
)

assert.match(
  editor,
  /const\s+editorStyle\s*=\s*captureInlineEditorTextareaStyle\(cell\)[\s\S]{0,700}?editorStyle,/,
  'inline table textarea must preserve the original cell text color before hiding the cell'
)

assert.match(
  editor,
  /textarea\.style\.background\s*=\s*'transparent'[\s\S]{0,260}?applyInlineEditorTextareaCellBoxStyle\(textarea,\s*state\)/,
  'inline table textarea must not cover the cell with an opaque block and must render with the original text color'
)

assert.match(
  editor,
  /const\s+applyInlineEditorTextareaCellBoxStyle\s*=[\s\S]+?borderTopWidth[\s\S]+?borderRightWidth[\s\S]+?borderBottomWidth[\s\S]+?borderLeftWidth/,
  'inline table textarea text origin must include cell borders so editing does not visually twitch by one border pixel'
)

assert.match(
  editor,
  /textarea\.style\.background\s*=\s*'transparent'[\s\S]{0,260}?applyInlineEditorTextareaCellBoxStyle\(textarea,\s*state\)/,
  'inline table textarea positioning must use the border-compensated cell text box style'
)

assert.match(
  editor,
  /textarea\.style\.visibility\s*=\s*'hidden'[\s\S]{0,260}?positionInlineEditorTableTextarea\(\)[\s\S]{0,260}?cell\.classList\.add\('editor-inline-table-cell-editing'\)[\s\S]{0,180}?textarea\.style\.visibility\s*=\s*'visible'/,
  'inline table cell text must be hidden only after the textarea is positioned to avoid enter-edit flicker'
)

assert.match(
  editor,
  /if\s*\(textarea\s*&&\s*state\?\.dirty\)\s*syncInlineEditorTableTextareaToCell\(\{\s*reposition:\s*false\s*\}\)/,
  'inline table textarea must commit pending text when editing closes'
)

const tableExpandStart = editor.indexOf('const openHoveredTableExpand =')
const tableExpandEnd = editor.indexOf('const replaceTableBreakTextNodes =', tableExpandStart)
const tableExpandHandler = editor.slice(tableExpandStart, tableExpandEnd)

assert.match(
  tableExpandHandler,
  /closeInlineEditorTableTextarea\(\)[\s\S]{0,180}?flushPendingEditorTableCellSourceSync\(\)/,
  'opening the expanded table must first close and commit the inline table textarea before reading rows'
)

assert.match(
  editor,
  /const\s+normalizeEditorTableCellTextEdges\s*=[\s\S]+?stripOuterTableCellHorizontalPadding[\s\S]+?text\.includes\('\\n'\)[\s\S]+?return text/,
  'table-cell source normalization must preserve leading and trailing logical blank lines instead of trimming them away'
)

assert.match(
  editor,
  /const\s+tableCellSourceToEditorText\s*=[\s\S]{0,260}?return\s+normalizeEditorTableCellTextEdges\(text\)/,
  'expanded markdown table cells must preserve leading and trailing blank lines when opened for editing'
)

assert.match(
  editor,
  /const\s+htmlTableCellToEditorText\s*=[\s\S]+?startsWithBreak[\s\S]+?endsWithBreak[\s\S]+?if\s*\(text\s*&&\s*startsWithBreak[\s\S]+?if\s*\(text\s*&&\s*endsWithBreak[\s\S]+?return\s+text/,
  'expanded rendered table cells must preserve leading and trailing blank lines when opened for editing'
)

assert.match(
  editor,
  /const\s+mergeRenderedTableCellEdgeBreaks\s*=[\s\S]+?countEdgeLineBreaks[\s\S]+?const\s+mergeRenderedTableEdgeBreaks\s*=/,
  'expanded table source rows must be merged with rendered edge breaks so source drift cannot erase visual blank lines'
)

assert.match(
  tableExpandHandler,
  /const\s+renderedRows\s*=\s*editableRowsFromRenderedTable\(table\)[\s\S]+?mergeRenderedTableEdgeBreaks\(editableRowsFromTableBlock\(block\),\s*renderedRows\)/,
  'opening expanded tables must retain rendered leading/trailing cell blank lines even when Vditor source has drifted'
)

assert.match(
  editor,
  /const\s+inlineEditorTextareaEdgeGuard[^=]*=[\s\S]+?INLINE_TABLE_CELL_EDGE_GUARD_PX[\s\S]+?const\s+inlineEditorTextareaGuardedClientY[^=]*=[\s\S]+?Math\.max\(minY,\s*Math\.min\(maxY,\s*event\.clientY\)\)/,
  'inline table caret mapping must exclude the cell edge band from valid caret hit testing'
)

assert.match(
  editor,
  /const\s+inlineEditorTextareaLineFromPoint\s*=[\s\S]+?inlineEditorTextareaGuardedClientY\(event,\s*metrics\)[\s\S]+?rawLine/,
  'initial and subsequent table-cell clicks must use guarded Y coordinates instead of raw border-edge clicks'
)

assert.doesNotMatch(
  editor,
  /const\s+canMaterializeEditorTableVisualBlankLine\s*=/,
  'table-cell mouse clicks must not revive the old contenteditable blank-line materializer'
)

assert.match(
  editor,
  /const\s+createEditorTableCaretAnchorNode\s*=\s*\(\)\s*=>\s*document\.createTextNode\(TABLE_CELL_CARET_ANCHOR\)/,
  'table-cell Enter must use a zero-width caret anchor so the visible caret moves to the new line'
)

assert.match(
  editor,
  /placeCaretAtEditorTableCellTextOffset[\s\S]+?getEditorEditableFromNode\(cell\)\?\.focus\(\{\s*preventScroll:\s*true\s*\}\)/,
  'programmatic table caret placement must focus the editable root so real keyboard and IME input target the editor'
)

assert.match(
  editor,
  /const\s+placeCaretInPlainEditorVisualBlankLine\s*=/,
  'clicks below the last editor line must map to the clicked visual blank line'
)

assert.match(
  editor,
  /if\s*\(clearEditorTableEmptyPlaceholder\(compositionCell\)\)\s*placeCaretAtStartOfEditorTableCell\(compositionCell\)/,
  'IME composition in empty table cells must clear placeholder spaces before text is committed'
)

assert.match(
  editor,
  /const\s+shouldSuppressEditorTableCompositionCommitArtifact\s*=/,
  'IME candidate commit keys must not leak a literal Space or Enter into table cells'
)

assert.match(
  editor,
  /commitEditorTableCellDomEdit\(cell,\s*\{[\s\S]{0,180}?stabilize:\s*!editorTableCompositionActive[\s\S]{0,120}?\}\)/,
  'table-cell IME input must not stabilize the live cell DOM before compositionend'
)

assert.doesNotMatch(
  editor,
  /renderAttachments:\s*!editorTableCompositionActive/,
  'table-cell IME input must not request custom attachment rerendering that bypasses Vditor native link nodes'
)

assert.match(
  editor,
  /const\s+scheduleRestoreEditorTableCompositionCaret\s*=/,
  'table-cell IME composition must restore caret by the logical text offset instead of falling to the cell end'
)

assert.doesNotMatch(
  editor,
  /if\s*\(caretCell\)\s*placeCaretAtEndOfEditorTableCell\(caretCell\)/,
  'table composition must not force the caret to the end of the cell'
)

assert.match(
  editor,
  /if\s*\(insertEditorLeadingBlankLine\(event\)\)\s*return/,
  'plain Enter handling must run before the blank-line Enter fallback'
)

assert.match(
  addForm,
  /encodeMarkdownExtraBlankLines\(stripFullImageAttachmentsMarker\(content\)\)\.trim\(\)/,
  'published content must pass through the shared blank-line encoder'
)

assert.match(
  addForm,
  /encodeMarkdownExtraBlankLines\(stripFullImageAttachmentsMarker\(rawValue\)\)/,
  'composer preview must pass through the shared blank-line encoder'
)

assert.match(
  addForm,
  /markMarkdownPreservedBlankLineElements\(root\)/,
  'composer preview must materialize encoded blank-line sentinels into visible blank-line blocks'
)

assert.match(
  renderer,
  /encodeMarkdownExtraBlankLines\(stripFullImageAttachmentsMarker\(markdown\s*\?\?\s*''\)\)/,
  'published markdown rendering must pass through the shared blank-line encoder'
)

assert.match(
  renderer,
  /markMarkdownPreservedBlankLineElements\(previewElement\.value\)/,
  'published markdown rendering must materialize encoded blank-line sentinels into visible blank-line blocks'
)

assert.match(
  editor,
  /materializeEditorPreservedBlankLineBlocks\(root\)/,
  'editor DOM must materialize encoded blank-line sentinels so edit and preview views stay visually consistent'
)

assert.match(
  editor,
  /markMarkdownPreservedBlankLineElements\(holder\)/,
  'Vditor preview transform must materialize encoded blank-line sentinels'
)

assert.match(
  editor,
  /const\s+syncEditorDomToVditorValueForPreview\s*=[\s\S]+?getEditorVisibleDomTableSafeValue\(\)[\s\S]+?vditorInstance\.setValue\(nextValue\)/,
  'Vditor toolbar preview must sync the DOM-preserved editor value before rendering preview'
)

assert.match(
  editor,
  /isPreviewItem\(item\)[\s\S]{0,120}?syncEditorDomToVditorValueForPreview\(\)/,
  'clicking the Vditor preview toolbar button must run the safe preview sync'
)

assert.match(
  announcement,
  /:deep\(\.markdown-preview p\)[^{]*\{[^}]*white-space:\s*nowrap\s*!important/,
  'announcement markdown must remain single-line even when normal markdown preserves user line breaks'
)

console.log('vditor blank-line preservation tests passed')
