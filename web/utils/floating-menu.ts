import type { Ref } from 'vue'

export type FloatingMenuPlacement = 'below' | 'above-right' | 'above-left'

export const clamp = (value: number, min: number, max: number) => Math.min(Math.max(value, min), Math.max(min, max))

export const getFixedCoordinateScale = () => {
  if (typeof window === 'undefined') return 1
  const zoom = Number.parseFloat(window.getComputedStyle(document.body).zoom || '1')
  return Number.isFinite(zoom) && zoom > 0 ? zoom : 1
}

export const getFixedViewport = (scale: number) => {
  const viewport = window.visualViewport
  const left = (viewport?.offsetLeft || 0) / scale
  const top = (viewport?.offsetTop || 0) / scale
  const width = (viewport?.width || window.innerWidth) / scale
  const height = (viewport?.height || window.innerHeight) / scale
  return { left, top, right: left + width, bottom: top + height }
}

export const getFixedRect = (element: HTMLElement, scale: number) => {
  const rect = element.getBoundingClientRect()
  const viewport = window.visualViewport
  const offsetLeft = viewport?.offsetLeft || 0
  const offsetTop = viewport?.offsetTop || 0
  return {
    left: (rect.left + offsetLeft) / scale,
    right: (rect.right + offsetLeft) / scale,
    top: (rect.top + offsetTop) / scale,
    bottom: (rect.bottom + offsetTop) / scale,
    width: rect.width / scale,
    height: rect.height / scale
  }
}

export const positionFloatingMenu = (
  trigger: HTMLElement | null,
  menu: HTMLElement | null,
  styleRef: Ref<Record<string, string>>,
  minWidth = 120,
  placement: FloatingMenuPlacement = 'below'
) => {
  if (!trigger || typeof window === 'undefined') return
  const scale = getFixedCoordinateScale()
  const rect = getFixedRect(trigger, scale)
  const viewport = getFixedViewport(scale)
  const menuWidth = Math.max(menu?.offsetWidth || minWidth, minWidth, rect.width)
  const menuHeight = menu?.offsetHeight || 180
  const pad = 8
  const gap = 4
  const minLeft = viewport.left + pad
  const maxLeft = Math.max(minLeft, viewport.right - menuWidth - pad)
  const idealLeft = placement === 'above-right'
    ? rect.right - menuWidth
    : placement === 'above-left'
      ? rect.right
      : rect.left + rect.width / 2 - menuWidth / 2
  const aboveTop = rect.top - menuHeight - gap
  const belowTop = rect.bottom + gap
  const minTop = viewport.top + pad
  const maxTop = Math.max(minTop, viewport.bottom - menuHeight - pad)
  const preferAbove = placement === 'above-right' || placement === 'above-left'
  const idealTop = preferAbove && aboveTop >= minTop ? aboveTop : belowTop
  styleRef.value = {
    position: 'fixed',
    left: `${clamp(idealLeft, minLeft, maxLeft)}px`,
    top: `${clamp(idealTop, minTop, maxTop)}px`,
    right: 'auto',
    bottom: 'auto',
    transform: 'none',
    minWidth: `${Math.max(minWidth, rect.width)}px`
  }
}

export const scheduleFloatingMenuPosition = (positioner: () => void) => {
  positioner()
  if (typeof window !== 'undefined') {
    window.requestAnimationFrame(() => {
      positioner()
      window.requestAnimationFrame(positioner)
    })
  }
}
