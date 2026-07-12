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
