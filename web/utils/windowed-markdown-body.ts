// This deliberately windows only static, settled text markup. Media, tables,
// editors and unknown widgets retain their original DOM and lifecycle.
const STATIC_TAGS: Record<string, true> = {
  P: true, BR: true, HR: true, H1: true, H2: true, H3: true, H4: true,
  H5: true, H6: true, BLOCKQUOTE: true, UL: true, OL: true, LI: true,
  STRONG: true, EM: true, DEL: true, S: true, B: true, I: true, U: true,
  PRE: true, CODE: true, SPAN: true, SUP: true, SUB: true, MARK: true,
  SMALL: true, A: true, BUTTON: true,
}

export function canReleaseMarkdownBody(root: HTMLElement): boolean {
  const doc = root.ownerDocument
  if (!root.isConnected || root.isContentEditable || root.scrollTop || root.scrollLeft) return false
  // Focus elsewhere in the same card may belong to an editor or an action in
  // progress. Selection can span this body with both endpoints outside it.
  const card = root.closest('.message-item, .content-container, .feed-item-card') || root
  if (doc.activeElement && card.contains(doc.activeElement)) return false
  if (card.querySelector('[contenteditable="true"], textarea, [role="dialog"]')) return false
  const selection = doc.getSelection()
  if (selection && !selection.isCollapsed) {
    for (let index = 0; index < selection.rangeCount; index++) {
      if (selection.getRangeAt(index).intersectsNode(root)) return false
    }
  }
  if (doc.fullscreenElement || doc.querySelector('.fancybox__container, .rendered-table-expand-overlay')) return false
  for (const node of root.querySelectorAll<HTMLElement>('*')) {
    if (!STATIC_TAGS[node.tagName]) return false
    if (node.isContentEditable || node.scrollTop || node.scrollLeft) return false
    // Anchor destinations must stay addressable even while far off screen.
    // Unknown attributes/widgets and externally managed links stay mounted.
    if (node.hasAttribute('id') || node.hasAttribute('name') || node.hasAttribute('contenteditable')
      || node.hasAttribute('data-fancybox') || node.hasAttribute('data-attachment-kind')
      || node.hasAttribute('tabindex') || node.hasAttribute('role')
      || node.classList.contains('github-card')) return false
    if (node.tagName === 'BUTTON' && !node.classList.contains('clickable-tag')) return false
    if (Array.from(node.attributes).some(attribute => /^on/i.test(attribute.name))) return false
  }
  return root.hasChildNodes()
}

// IO callbacks are asynchronous. Visible bodies must begin normal rendering on
// mount, without first committing a raw-Markdown placeholder to list layout.
export function isMarkdownBodyNearViewport(root: HTMLElement): boolean {
  const win = root.ownerDocument.defaultView
  if (!win) return true
  let top = 0
  let bottom = win.innerHeight
  for (let parent = root.parentElement; parent; parent = parent.parentElement) {
    if (!/(auto|scroll)/.test(win.getComputedStyle(parent).overflowY) || !parent.clientHeight) continue
    const rect = parent.getBoundingClientRect()
    top = Math.max(top, rect.top + parent.clientTop)
    bottom = Math.min(bottom, rect.top + parent.clientTop + parent.clientHeight)
  }
  const rect = root.getBoundingClientRect()
  return rect.bottom >= top - 1200 && rect.top <= bottom + 1200
}

type BodyOptions = {
  canRelease: () => boolean
  onRelease: () => void
  onRestore: () => void
  onNearChange?: (visible: boolean) => void
}

type ObservedBody = {
  near: (visible: boolean) => void
  far: (visible: boolean) => void
  resized: () => void
  restore: () => void
  refresh: () => void
}

type ObserverHub = {
  add: (root: HTMLElement, body: ObservedBody) => void
  remove: (root: HTMLElement) => void
  canRelease: () => boolean
}

const hubs = new WeakMap<Document, ObserverHub>()

function getObserverHub(doc: Document): ObserverHub | null {
  const existing = hubs.get(doc)
  if (existing) return existing
  const win = doc.defaultView
  // Without both observers keep the normal renderer, including on old clients.
  if (!win?.IntersectionObserver || !win.ResizeObserver) return null
  const bodies = new Map<Element, ObservedBody>()
  let printing = false
  const nearObserver = new win.IntersectionObserver(entries => {
    for (const entry of entries) bodies.get(entry.target)?.near(entry.isIntersecting)
  }, { rootMargin: '1200px 0px' })
  const farObserver = new win.IntersectionObserver(entries => {
    for (const entry of entries) bodies.get(entry.target)?.far(entry.isIntersecting)
  }, { rootMargin: '2400px 0px' })
  const resizeObserver = new win.ResizeObserver(entries => {
    for (const entry of entries) bodies.get(entry.target)?.resized()
  })
  const restoreAll = () => { for (const body of bodies.values()) body.restore() }
  const refreshAll = () => { for (const body of bodies.values()) body.refresh() }
  // Text also stays in dormant bodies for browser-menu find and accessibility.
  // Shortcut find restores rich markup; print locks it for the whole session.
  const onFind = (event: KeyboardEvent) => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'f') restoreAll()
  }
  const beforePrint = () => { printing = true; restoreAll() }
  const afterPrint = () => { printing = false; refreshAll() }
  const onFontsLoaded = () => {
    restoreAll()
    refreshAll()
  }
  win.addEventListener('beforeprint', beforePrint)
  win.addEventListener('afterprint', afterPrint)
  doc.addEventListener('keydown', onFind)
  doc.fonts?.addEventListener('loadingdone', onFontsLoaded)
  const hub: ObserverHub = {
    canRelease: () => !printing,
    add(root, body) {
      bodies.set(root, body)
      nearObserver.observe(root)
      farObserver.observe(root)
      resizeObserver.observe(root)
    },
    remove(root) {
      nearObserver.unobserve(root)
      farObserver.unobserve(root)
      resizeObserver.unobserve(root)
      bodies.delete(root)
      if (bodies.size) return
      nearObserver.disconnect()
      farObserver.disconnect()
      resizeObserver.disconnect()
      win.removeEventListener('beforeprint', beforePrint)
      win.removeEventListener('afterprint', afterPrint)
      doc.removeEventListener('keydown', onFind)
      doc.fonts?.removeEventListener('loadingdone', onFontsLoaded)
      hubs.delete(doc)
    },
  }
  hubs.set(doc, hub)
  return hub
}

export function createWindowedMarkdownBody(root: HTMLElement, options: BodyOptions) {
  let disposed = false
  let near: boolean | null = null
  let far: boolean | null = null
  let snapshot: { html: string; width: number; styles: string[] } | null = null
  const properties = ['height', 'min-height', 'max-height', 'box-sizing', 'overflow', 'white-space']
  const hub = getObserverHub(root.ownerDocument)

  const restore = () => {
    if (disposed || !snapshot) return
    const saved = snapshot
    snapshot = null
    // Restore synchronously before releasing the measured placeholder. This
    // avoids async Markdown parsing moving the scroll target on return.
    root.innerHTML = saved.html
    properties.forEach((property, index) => {
      const [value, priority] = saved.styles[index].split('\u0000')
      if (value) root.style.setProperty(property, value, priority)
      else root.style.removeProperty(property)
    })
    delete root.dataset.markdownDormant
    options.onRestore()
  }

  const refresh = () => {
    if (disposed || !hub || snapshot || near !== false || far !== false) return
    if (!hub.canRelease()) return
    if (!options.canRelease() || !canReleaseMarkdownBody(root)) return
    // A content-visibility ancestor may report an intrinsic placeholder size.
    // Only measure boxes whose actual contents participate in layout.
    if (root.checkVisibility && !root.checkVisibility({ contentVisibilityAuto: true })) return
    const rect = root.getBoundingClientRect()
    if (rect.height <= 0 || rect.width <= 0) return
    const saved = {
      html: root.innerHTML,
      width: rect.width,
      styles: properties.map(property => `${root.style.getPropertyValue(property)}\u0000${root.style.getPropertyPriority(property)}`),
    }
    const text = root.innerText || root.textContent || ''
    options.onRelease()
    snapshot = saved
    root.style.setProperty('box-sizing', 'border-box', 'important')
    for (const property of ['height', 'min-height', 'max-height']) {
      root.style.setProperty(property, `${rect.height}px`, 'important')
    }
    root.style.setProperty('overflow', 'hidden', 'important')
    root.style.setProperty('white-space', 'pre-wrap', 'important')
    root.textContent = text
    root.dataset.markdownDormant = 'true'
  }

  hub?.add(root, {
    near(visible) {
      near = visible
      if (visible) restore()
      else refresh()
      options.onNearChange?.(visible)
    },
    far(visible) {
      far = visible
      if (!visible) refresh()
    },
    resized() {
      if (snapshot && Math.abs(root.getBoundingClientRect().width - snapshot.width) > 0.5) {
        restore()
        // Responsive column changes invalidate height. Remeasure the restored
        // markup at the new width before releasing it again.
        refresh()
      }
    },
    restore,
    refresh,
  })

  return {
    supported: !!hub,
    restore,
    refresh,
    dispose(restoreContent = true) {
      if (disposed) return
      if (restoreContent) restore()
      disposed = true
      snapshot = null
      hub?.remove(root)
    },
  }
}
