export type EditorAttachmentInsertApi = {
  insertMD: (value: string) => void
  insertValue: (value: string) => void
}

export const insertEditorValueFallback = (
  api: EditorAttachmentInsertApi,
  value: string,
  isAttachment: boolean
) => {
  if (isAttachment) {
    api.insertMD(value)
    return 'attachment-markdown'
  }
  api.insertValue(value)
  return 'value'
}

export type TableAttachmentTarget<TEditable> = {
  editable: TEditable
  tableIndex: number
  rowIndex: number
  cellIndex: number
  offset: number
}

export const resolveTableAttachmentTarget = <TEditable, TCell>(
  target: TableAttachmentTarget<TEditable> | null,
  isEditableAttached: (editable: TEditable) => boolean,
  resolveCell: (target: TableAttachmentTarget<TEditable>) => TCell | null
) => {
  if (!target || !isEditableAttached(target.editable)) return null
  return resolveCell(target)
}

export const replaceTableSourceLine = (
  sourceLines: readonly string[],
  lineIndex: number,
  nextLine: string
) => {
  if (lineIndex < 0 || lineIndex >= sourceLines.length) return null
  const lines = [...sourceLines]
  lines[lineIndex] = nextLine
  return lines
}

export const insertTableCellAtomicValue = (
  currentValue: string,
  insertedValue: string,
  requestedOffset: number
) => {
  const current = String(currentValue || '')
  const inserted = String(insertedValue || '').trim()
  const offset = Math.max(0, Math.min(current.length, Number.isFinite(requestedOffset) ? requestedOffset : current.length))
  if (!inserted) return { value: current, caretOffset: offset }

  const before = current.slice(0, offset)
  const after = current.slice(offset)
  const leadingBoundary = before && !/\s$/.test(before) ? ' ' : ''
  const trailingBoundary = after && !/^\s/.test(after) ? ' ' : ''
  return {
    value: `${before}${leadingBoundary}${inserted}${trailingBoundary}${after}`,
    caretOffset: before.length + leadingBoundary.length + inserted.length,
  }
}

export type FixedEditorRect = {
  left: number
  top: number
  right: number
  bottom: number
}

export const getFixedEditorClipInsets = (rect: FixedEditorRect, clippingRect: FixedEditorRect) => {
  const width = Math.max(0, rect.right - rect.left)
  const height = Math.max(0, rect.bottom - rect.top)
  const top = Math.min(height, Math.max(0, clippingRect.top - rect.top))
  const right = Math.min(width, Math.max(0, rect.right - clippingRect.right))
  const bottom = Math.min(height, Math.max(0, rect.bottom - clippingRect.bottom))
  const left = Math.min(width, Math.max(0, clippingRect.left - rect.left))
  return {
    top,
    right,
    bottom,
    left,
    visible: width - left - right > 0 && height - top - bottom > 0,
  }
}
