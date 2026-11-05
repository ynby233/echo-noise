import { parseNoteMusicTrackID, type NoteMusicPlayer } from './note-music'

export const EDITOR_NOTE_MUSIC_HINT = '点击试听'
const LINK_SELECTOR = 'a, [data-type="a"]'
const CODE_SELECTOR = 'code, [data-type="code-block"], [data-type="code-inline"], .vditor-ir__marker--pre, textarea'
const EDITABLE_SELECTOR = '.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset'

// Read the native link on every pass. Keeping its children and href intact lets
// Lute retain the same Markdown through mode changes, undo and clipboard edits.
export function editorNoteMusicInfo(node: HTMLElement) {
  if (node.closest(CODE_SELECTOR)) return null
  const ir = node.matches('[data-type="a"]')
  const href = ir
    ? node.querySelector('.vditor-ir__marker--link')?.textContent?.trim() || ''
    : node.getAttribute('href') || node.getAttribute('data-note-music-href') || ''
  if (!parseNoteMusicTrackID(href)) return null
  const label = ir ? node.querySelector('.vditor-ir__link')?.textContent || '' : node.textContent || ''
  // createNoteMusicMarkdown emits plain labels; escape Markdown delimiters too
  // so hand-edited labels round-trip when the editor uses its text fallback.
  const escaped = label.replace(/([\\\[\]])/g, '\\$1')
  return { href, label, source: `[${escaped}](${href})` }
}

export function enhanceEditorNoteMusic(root: HTMLElement, preview = false) {
  root.querySelectorAll<HTMLElement>(LINK_SELECTOR).forEach(node => {
    const info = editorNoteMusicInfo(node)
    if (!info) {
      if (node.matches('.editor-note-music-link, .editor-note-music-node')) {
        node.classList.remove('editor-note-music-link', 'editor-note-music-node')
        node.querySelector('.vditor-ir__link')?.classList.remove('editor-note-music-link')
        for (const attribute of ['contenteditable', 'tabindex', 'role', 'data-note-music-hint', 'aria-label', 'draggable', 'data-note-music-href']) node.removeAttribute(attribute)
      }
      return
    }
    const isPreview = preview || !!node.closest('.vditor-preview')
    const ir = node.matches('[data-type="a"]')
    node.classList.add(ir ? 'editor-note-music-node' : 'editor-note-music-link')
    node.classList.remove('vditor-ir__node--expand')
    node.setAttribute('contenteditable', 'false')
    node.setAttribute('tabindex', '0')
    node.setAttribute('role', 'button')
    node.setAttribute('data-note-music-hint', EDITOR_NOTE_MUSIC_HINT)
    node.setAttribute('aria-label', `${info.label}，${EDITOR_NOTE_MUSIC_HINT}`)
    node.setAttribute('draggable', 'false')
    if (ir) node.querySelector('.vditor-ir__link')?.classList.add('editor-note-music-link')
    if (isPreview) {
      // Vditor's preview is detached before insertion: removing href also keeps
      // middle click and the browser's link context menu from opening a draft.
      node.setAttribute('data-note-music-href', info.href)
      node.removeAttribute('href')
    }
  })
}

// Only call this on serialization clones, never the live editing DOM.
export function replaceEditorNoteMusicWithSource(root: HTMLElement) {
  root.querySelectorAll<HTMLElement>(LINK_SELECTOR).forEach(node => {
    const info = editorNoteMusicInfo(node)
    if (info) node.replaceWith(root.ownerDocument.createTextNode(info.source))
  })
}

export function bindEditorNoteMusicInteractions(root: HTMLElement, playDraftMusic?: NoteMusicPlayer) {
  const pending = new WeakSet<HTMLElement>()
  const doc = root.ownerDocument
  const hit = (event: Event) => {
    const target = event.target instanceof Element ? event.target : null
    const node = target?.closest<HTMLElement>(LINK_SELECTOR) || null
    return node && root.contains(node) && editorNoteMusicInfo(node) ? node : null
  }
  const stop = (event: Event) => {
    event.preventDefault()
    event.stopImmediatePropagation()
  }
  const select = (node: HTMLElement) => {
    const editable = node.closest<HTMLElement>(EDITABLE_SELECTOR)
    if (!editable) return
    editable.focus({ preventScroll: true })
    const range = doc.createRange()
    range.setStartAfter(node)
    range.collapse(true)
    const selection = doc.defaultView?.getSelection()
    selection?.removeAllRanges()
    selection?.addRange(range)
  }
  const selectedMarker = () => {
    const selection = doc.defaultView?.getSelection()
    if (!selection?.rangeCount) return null
    const range = selection.getRangeAt(0)
    if (range.startContainer !== range.endContainer || range.endOffset !== range.startOffset + 1) return null
    const node = range.startContainer.childNodes[range.startOffset]
    return node instanceof HTMLElement && root.contains(node) && editorNoteMusicInfo(node) ? node : null
  }
  const pointer = (event: Event) => {
    if (!hit(event)) return
    // Vditor must not expand the IR URL on press.
    event.stopImmediatePropagation()
  }
  const activate = (event: Event) => {
    const node = hit(event)
    if (!node) return
    stop(event)
    select(node)
    if (!playDraftMusic || pending.has(node)) return
    pending.add(node)
    node.setAttribute('aria-busy', 'true')
    void playDraftMusic({ trackID: parseNoteMusicTrackID(editorNoteMusicInfo(node)!.href), messageID: 0, draft: true })
      .catch(() => {})
      .finally(() => { pending.delete(node); node.removeAttribute('aria-busy') })
  }
  const keydown = (event: KeyboardEvent) => {
    const focused = hit(event)
    if (event.key === 'Enter' || event.key === ' ') {
      if (focused) activate(event)
      return
    }
    const node = focused || selectedMarker()
    if (!node) return
    if (event.key !== 'Backspace' && event.key !== 'Delete') return
    const editable = node.closest<HTMLElement>(EDITABLE_SELECTOR)
    if (!editable) return
    editable.focus({ preventScroll: true })
    const range = doc.createRange()
    range.selectNode(node)
    const selection = doc.defaultView?.getSelection()
    selection?.removeAllRanges()
    selection?.addRange(range)
    // Native editing commands participate in the browser/Vditor undo history.
    // If unavailable, allow the real Delete/Backspace event to edit the range.
    if (doc.execCommand('delete')) stop(event)
  }
  root.addEventListener('pointerdown', pointer, true)
  root.addEventListener('mousedown', pointer, true)
  root.addEventListener('click', activate, true)
  root.addEventListener('auxclick', activate, true)
  root.addEventListener('keydown', keydown, true)
  return () => {
    root.removeEventListener('pointerdown', pointer, true)
    root.removeEventListener('mousedown', pointer, true)
    root.removeEventListener('click', activate, true)
    root.removeEventListener('auxclick', activate, true)
    root.removeEventListener('keydown', keydown, true)
  }
}
