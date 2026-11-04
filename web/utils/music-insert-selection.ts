// Keep the editor's DOM selection while a modal owns focus. Restoring a detached
// range is unsafe: callers must cancel the insertion when restore returns false.
export type MusicInsertSelection = {
  root: HTMLElement
  editable: HTMLElement
  range: Range
}

export const captureMusicInsertSelection = (root: HTMLElement | null): MusicInsertSelection | null => {
  if (!root || typeof window === 'undefined') return null
  const selection = window.getSelection()
  const current = selection?.rangeCount ? selection.getRangeAt(0) : null
  const node = current?.commonAncestorContainer
  const element = node instanceof Element ? node : node?.parentElement
  const editable = element?.closest<HTMLElement>('[contenteditable="true"]')
  if (current && editable && root.contains(editable) && editable.contains(current.commonAncestorContainer)) {
    return { root, editable, range: current.cloneRange() }
  }
  // A toolbar opened from the keyboard may have no remaining DOM selection.
  const fallback = Array.from(root.querySelectorAll<HTMLElement>('[contenteditable="true"]'))
    .find(candidate => candidate.getClientRects().length > 0)
  if (!fallback) return null
  const range = document.createRange()
  range.selectNodeContents(fallback)
  range.collapse(false)
  return { root, editable: fallback, range }
}

export const restoreMusicInsertSelection = (target: MusicInsertSelection): boolean => {
  const { root, editable, range } = target
  if (!root.isConnected || !root.contains(editable) || !editable.contains(range.commonAncestorContainer)) return false
  const selection = window.getSelection()
  if (!selection) return false
  editable.focus({ preventScroll: true })
  selection.removeAllRanges()
  selection.addRange(range)
  return true
}
