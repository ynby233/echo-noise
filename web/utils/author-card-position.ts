export type AuthorCardRect = { left: number; top: number; right: number; bottom: number }
export type AuthorCardSize = { width: number; height: number }
export type AuthorCardPlacement = 'right' | 'left' | 'below' | 'above'

const clamp = (value: number, min: number, max: number) => Math.min(Math.max(value, min), Math.max(min, max))

// All inputs are getBoundingClientRect/visualViewport pixels, before converting
// to the body's fixed-position CSS coordinates (which may inherit CSS zoom).
export const resolveAuthorCardPosition = (
  anchor: AuthorCardRect,
  size: AuthorCardSize,
  viewport: AuthorCardRect,
  narrow: boolean,
  gap = 8,
  padding = 8
) => {
  const minLeft = viewport.left + padding
  const minTop = viewport.top + padding
  const maxRight = viewport.right - padding
  const maxBottom = viewport.bottom - padding
  const rightSpace = maxRight - anchor.right - gap
  const leftSpace = anchor.left - gap - minLeft
  const belowSpace = maxBottom - anchor.bottom - gap
  const aboveSpace = anchor.top - gap - minTop
  const placement: AuthorCardPlacement = narrow
    ? (belowSpace >= size.height || belowSpace >= aboveSpace ? 'below' : 'above')
    : (rightSpace >= size.width || rightSpace >= leftSpace ? 'right' : 'left')
  const left = placement === 'right' ? anchor.right + gap
    : placement === 'left' ? anchor.left - gap - size.width
      : anchor.left + (anchor.right - anchor.left - size.width) / 2
  const top = placement === 'below' ? anchor.bottom + gap
    : placement === 'above' ? anchor.top - gap - size.height
      : anchor.top + (anchor.bottom - anchor.top - size.height) / 2
  return {
    left: clamp(left, minLeft, maxRight - size.width),
    top: clamp(top, minTop, maxBottom - size.height),
    placement
  }
}

export const toAuthorCardFixedPoint = (
  point: { left: number; top: number },
  origin: { left: number; top: number },
  scale: AuthorCardSize
) => ({ left: (point.left - origin.left) / scale.width, top: (point.top - origin.top) / scale.height })

export const positionAuthorCard = (anchor: HTMLElement, card: HTMLElement, probe: HTMLElement, narrow: boolean) => {
  if (!anchor.isConnected || !anchor.getClientRects().length) return false
  const visual = window.visualViewport
  const viewport = {
    left: visual?.offsetLeft || 0,
    top: visual?.offsetTop || 0,
    right: (visual?.offsetLeft || 0) + (visual?.width || window.innerWidth),
    bottom: (visual?.offsetTop || 0) + (visual?.height || window.innerHeight)
  }
  const rect = anchor.getBoundingClientRect()
  if (rect.right <= viewport.left || rect.left >= viewport.right || rect.bottom <= viewport.top || rect.top >= viewport.bottom) return false
  // A nested scroller can hide an avatar while its rect is still inside the
  // window. Do not leave a detached card floating over unrelated notes.
  const visible = { ...viewport }
  for (let parent = anchor.parentElement; parent && parent !== document.body; parent = parent.parentElement) {
    const style = window.getComputedStyle(parent)
    const bounds = parent.getBoundingClientRect()
    if (/(auto|scroll|hidden|clip)/.test(style.overflowX)) {
      visible.left = Math.max(visible.left, bounds.left)
      visible.right = Math.min(visible.right, bounds.right)
    }
    if (/(auto|scroll|hidden|clip)/.test(style.overflowY)) {
      visible.top = Math.max(visible.top, bounds.top)
      visible.bottom = Math.min(visible.bottom, bounds.bottom)
    }
  }
  if (rect.right <= visible.left || rect.left >= visible.right || rect.bottom <= visible.top || rect.top >= visible.bottom) return false
  // A fixed 100px probe shares the card's containing block. Measuring its
  // origin and scale handles body zoom without assuming a browser's fixed
  // origin during visualViewport panning or pinch zoom.
  const origin = probe.getBoundingClientRect()
  const scale = { width: origin.width / 100, height: origin.height / 100 }
  if (!(scale.width > 0 && scale.height > 0)) return false
  card.style.maxWidth = `${Math.max(1, viewport.right - viewport.left - 16) / scale.width}px`
  card.style.maxHeight = `${Math.max(1, viewport.bottom - viewport.top - 16) / scale.height}px`
  const size = card.getBoundingClientRect()
  const position = resolveAuthorCardPosition(rect, size, viewport, narrow)
  const fixed = toAuthorCardFixedPoint(position, origin, scale)
  card.style.left = `${fixed.left}px`
  card.style.top = `${fixed.top}px`
  card.style.visibility = 'visible'
  card.dataset.placement = position.placement
  return true
}
