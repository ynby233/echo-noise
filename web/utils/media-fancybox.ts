import { zh_CN } from '@fancyapps/ui/l10n/Fancybox/zh_CN'
import { animateFancyboxHtml5VideoClose, prepareFancyboxHtml5VideoSlide } from './fancybox-video-close'

export const MEDIA_FANCYBOX_MAIN_CLASS = 'noise-media-fancybox'

const projectFancyboxL10n = {
  ...zh_CN,
  PREV: '上一张',
  NEXT: '下一张',
  CLOSE: '关闭',
  TOGGLE_ZOOM: '切换缩放',
  TOGGLEZOOM: '切换缩放',
  TOGGLE1TO1: '切换缩放',
  ITERATEZOOM: '切换缩放',
  TOGGLE_THUMBS: '切换缩略图',
  TOGGLE_SLIDESHOW: '切换幻灯片',
  TOGGLE_FULLSCREEN: '切换全屏',
  DOWNLOAD: '下载',
}

const tooltipLabelMap: Record<string, string> = {
  Close: '关闭',
  Next: '下一张',
  Previous: '上一张',
  Prev: '上一张',
  'Toggle zoom level': '切换缩放',
  '切换缩放级别': '切换缩放',
  'Toggle zoom': '切换缩放',
  'Toggle thumbnails': '切换缩略图',
  'Toggle slideshow': '切换幻灯片',
  'Toggle fullscreen': '切换全屏',
  Download: '下载',
  'Zoom in': '放大',
  'Zoom out': '缩小',
  Slideshow: '切换幻灯片',
  Fullscreen: '切换全屏',
  Thumbnails: '切换缩略图',
}

const actionLabelMap: Record<string, string> = {
  close: '关闭',
  next: '下一张',
  prev: '上一张',
  iterateZoom: '切换缩放',
  toggleZoom: '切换缩放',
  slideshow: '切换幻灯片',
  fullscreen: '切换全屏',
  thumbs: '切换缩略图',
  download: '下载',
}

const normalizeTooltipLabel = (raw: string | null | undefined) => {
  const value = String(raw || '').trim()
  if (!value) return ''
  return tooltipLabelMap[value] || value
}

const getButtonActionLabel = (button: HTMLElement) => {
  const raw = normalizeTooltipLabel(button.getAttribute('title') || button.getAttribute('aria-label') || button.dataset.tooltip)
  if (raw) return raw
  const action = button.dataset.panzoomAction || button.dataset.fancyboxToggle || button.dataset.fancyboxClose || button.dataset.fancyboxNext || button.dataset.fancyboxPrev || ''
  if (button.matches('[data-fancybox-close]')) return actionLabelMap.close
  if (button.matches('[data-fancybox-next]')) return actionLabelMap.next
  if (button.matches('[data-fancybox-prev]')) return actionLabelMap.prev
  if (button.matches('[data-fancybox-toggle-thumbs]')) return actionLabelMap.thumbs
  if (button.matches('[data-fancybox-toggle-fullscreen]')) return actionLabelMap.fullscreen
  return actionLabelMap[action] || ''
}

export const applyProjectFancyboxTooltips = (instance?: any) => {
  if (typeof document === 'undefined') return
  const roots: HTMLElement[] = []
  const container = instance?.container as HTMLElement | undefined
  if (container) roots.push(container)
  document.querySelectorAll<HTMLElement>(`.${MEDIA_FANCYBOX_MAIN_CLASS}.fancybox__container`).forEach((root) => {
    if (!roots.includes(root)) roots.push(root)
  })
  roots.forEach((root) => {
    root.querySelectorAll<HTMLElement>('.fancybox__toolbar .f-button, .fancybox__toolbar button, .f-button[title], button[title]').forEach((button) => {
      const label = getButtonActionLabel(button)
      if (!label) return
      button.classList.add('nw-tooltip-anchor', 'nw-tooltip-below')
      button.dataset.tooltip = label
      button.setAttribute('aria-label', label)
      button.removeAttribute('title')
    })
  })
}

const scheduleProjectFancyboxTooltips = (instance?: any) => {
  if (typeof window === 'undefined') return
  window.requestAnimationFrame(() => applyProjectFancyboxTooltips(instance))
  window.setTimeout(() => applyProjectFancyboxTooltips(instance), 80)
}

const composeHandlers = (...handlers: Array<((...args: any[]) => void) | undefined>) => (...args: any[]) => {
  handlers.forEach((handler) => handler?.(...args))
}

type MediaFancyboxOptionsInput = {
  startIndex?: number
  video?: boolean
  carouselInfinite?: boolean
  on?: Record<string, (...args: any[]) => void>
}

export const createMediaFancyboxOptions = (input: MediaFancyboxOptionsInput = {}) => {
  const { startIndex = 0, video = false, carouselInfinite = true, on = {} } = input
  const tooltipHandler = (instance: any) => scheduleProjectFancyboxTooltips(instance)
  const videoSlideHandler = (instance: any) => prepareFancyboxHtml5VideoSlide(instance)
  const options: Record<string, any> = {
    mainClass: MEDIA_FANCYBOX_MAIN_CLASS,
    animated: true,
    closeButton: false,
    startIndex,
    l10n: projectFancyboxL10n,
    Carousel: { infinite: carouselInfinite },
    Toolbar: {
      enabled: true,
      display: {
        left: ['infobar'],
        middle: [],
        right: ['iterateZoom', 'slideshow', 'fullscreen', 'thumbs', 'close'],
      },
    },
    Thumbs: {
      autoStart: true,
    },
    Images: {
      zoom: true,
    },
    Image: {
      zoom: true,
      click: true,
      wheel: 'slide',
    },
    compact: false,
    placeFocusBack: false,
    Hash: false,
    on: {
      ...on,
      ready: composeHandlers(tooltipHandler, on.ready),
      reveal: composeHandlers(tooltipHandler, videoSlideHandler, on.reveal),
      done: composeHandlers(tooltipHandler, videoSlideHandler, on.done),
      'Carousel.change': composeHandlers(tooltipHandler, videoSlideHandler, on['Carousel.change']),
    },
  }

  if (video) {
    options.Html = { videoAutoplay: false }
    options.on = {
      ...options.on,
      shouldClose: composeHandlers(animateFancyboxHtml5VideoClose, on.shouldClose),
      close: composeHandlers(animateFancyboxHtml5VideoClose, on.close),
    }
  }

  return options
}
