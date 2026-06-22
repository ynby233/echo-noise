type FancyboxLike = {
  getSlide?: () => any
  container?: HTMLElement | null
  options?: Record<string, any>
  __noiseVideoCloseAnimated?: boolean
}

const validRect = (rect: DOMRect | null | undefined) => !!rect && rect.width > 1 && rect.height > 1

const isImageSource = (src: string | null | undefined) => {
  const value = String(src || '').trim()
  return !!value && (/^(data:image|blob:)/i.test(value) || /\.(png|jpe?g|gif|webp|bmp|svg)(?:[?#].*)?$/i.test(value))
}

export const normalizeSameSiteMediaSource = (src: string) => {
  const value = String(src || '').trim()
  if (!value || typeof window === 'undefined') return value
  try {
    const url = new URL(value, window.location.href)
    if (/^\/api\/(?:images|video)\//.test(url.pathname) && url.origin !== window.location.origin) {
      return `${window.location.origin}${url.pathname}${url.search}${url.hash}`
    }
  } catch {}
  return value
}

const captureVideoFrame = (video: HTMLVideoElement | null) => {
  if (!video || !video.videoWidth || !video.videoHeight || video.readyState < 2) return ''
  try {
    const canvas = document.createElement('canvas')
    canvas.width = video.videoWidth
    canvas.height = video.videoHeight
    const ctx = canvas.getContext('2d')
    if (!ctx) return ''
    ctx.drawImage(video, 0, 0, canvas.width, canvas.height)
    return canvas.toDataURL('image/jpeg', 0.86)
  } catch {
    return ''
  }
}

const getSlideVideoElement = (slide: any) => {
  const root = (slide?.contentEl || slide?.el) as HTMLElement | null
  if (root instanceof HTMLVideoElement) return root
  return root?.querySelector?.('video') as HTMLVideoElement | null
}

export const getVideoElementSource = (video: HTMLVideoElement) => {
  return normalizeSameSiteMediaSource(video.currentSrc || video.getAttribute('src') || video.querySelector('source')?.getAttribute('src') || '')
}

const getSlideImageFallback = (slide: any, video: HTMLVideoElement | null) => {
  const candidates = [
    video?.poster,
    slide?.poster,
    slide?.thumbElSrc,
    slide?.thumbSrc,
    slide?.triggerEl?.dataset?.thumbSrc,
    slide?.triggerEl?.dataset?.poster,
    slide?.thumbEl?.currentSrc,
    slide?.thumbEl?.src
  ]
  return candidates.find(isImageSource) || ''
}

const getSlideContentElement = (slide: any) => {
  const video = getSlideVideoElement(slide)
  return video || (slide?.contentEl as HTMLElement | null) || (slide?.el?.querySelector?.('.fancybox__content') as HTMLElement | null)
}

const getSlideHideElements = (slide: any, contentEl: HTMLElement | null) => {
  const slideEl = slide?.el as HTMLElement | null
  const descendants = slideEl
    ? Array.from(slideEl.querySelectorAll('.fancybox__content, .f-html5video, .fancybox__html5video, video')) as HTMLElement[]
    : []
  return [
    contentEl,
    contentEl?.closest?.('.fancybox__content') as HTMLElement | null,
    slide?.contentEl as HTMLElement | null,
    ...descendants,
    slideEl
  ].filter((item, index, list): item is HTMLElement => !!item && list.indexOf(item) === index)
}

const getTriggerElement = (instance: FancyboxLike, slide: any) => {
  return (slide?.triggerEl || slide?.thumbEl || instance?.options?.triggerEl || null) as HTMLElement | null
}

const containRect = (sourceRect: DOMRect, targetRect: DOMRect) => {
  const sourceRatio = sourceRect.width / sourceRect.height
  const targetRatio = targetRect.width / targetRect.height
  if (!Number.isFinite(sourceRatio) || sourceRatio <= 0 || !Number.isFinite(targetRatio) || targetRatio <= 0) return targetRect

  let width = targetRect.width
  let height = targetRect.height
  if (sourceRatio > targetRatio) {
    height = width / sourceRatio
  } else {
    width = height * sourceRatio
  }

  return {
    left: targetRect.left + (targetRect.width - width) / 2,
    top: targetRect.top + (targetRect.height - height) / 2,
    width,
    height
  }
}

export const animateFancyboxHtml5VideoClose = (instance: FancyboxLike) => {
  if (typeof document === 'undefined') return
  if (instance.__noiseVideoCloseAnimated) return
  const slide = instance?.getSlide?.()
  if (!slide || slide.type !== 'html5video') return

  const contentEl = getSlideContentElement(slide)
  const targetEl = getTriggerElement(instance, slide)
  const startRect = contentEl?.getBoundingClientRect?.()
  const targetRect = targetEl?.getBoundingClientRect?.()
  if (!contentEl || !validRect(startRect) || !validRect(targetRect)) return

  const video = getSlideVideoElement(slide)
  const frameSrc = captureVideoFrame(video) || getSlideImageFallback(slide, video)
  if (!frameSrc) return
  instance.__noiseVideoCloseAnimated = true

  const overlay = document.createElement('img')
  overlay.src = frameSrc
  overlay.alt = ''
  overlay.decoding = 'async'

  Object.assign(overlay.style, {
    position: 'fixed',
    left: `${startRect!.left}px`,
    top: `${startRect!.top}px`,
    width: `${startRect!.width}px`,
    height: `${startRect!.height}px`,
    objectFit: 'contain',
    objectPosition: 'center center',
    pointerEvents: 'none',
    zIndex: '10002',
    transformOrigin: 'top left',
    transition: 'transform 280ms cubic-bezier(0.22, 1, 0.36, 1), opacity 280ms cubic-bezier(0.22, 1, 0.36, 1)',
    willChange: 'transform, opacity',
    opacity: '1'
  })

  document.body.appendChild(overlay)

  const finalRect = containRect(startRect!, targetRect!)
  const scaleX = finalRect.width / startRect!.width
  const scaleY = finalRect.height / startRect!.height
  const translateX = finalRect.left - startRect!.left
  const translateY = finalRect.top - startRect!.top

  const hideEls = getSlideHideElements(slide, contentEl)
  const previousVisibility = hideEls.map((item) => item.style.visibility || '')
  const previousOpacity = hideEls.map((item) => item.style.opacity || '')
  const previousPointerEvents = hideEls.map((item) => item.style.pointerEvents || '')
  let cleaned = false
  const cleanup = () => {
    if (cleaned) return
    cleaned = true
    overlay.remove()
    hideEls.forEach((item, index) => {
      item.style.visibility = previousVisibility[index] || ''
      item.style.opacity = previousOpacity[index] || ''
      item.style.pointerEvents = previousPointerEvents[index] || ''
    })
  }

  hideEls.forEach((item) => {
    item.style.visibility = 'hidden'
    item.style.opacity = '0'
    item.style.pointerEvents = 'none'
  })

  const runAnimation = () => {
    if (cleaned) return
    requestAnimationFrame(() => {
      overlay.style.transform = `translate3d(${translateX}px, ${translateY}px, 0) scale(${scaleX}, ${scaleY})`
      overlay.style.opacity = '0'
    })
    overlay.addEventListener('transitionend', cleanup, { once: true })
    window.setTimeout(cleanup, 360)
  }

  const decode = typeof overlay.decode === 'function'
    ? overlay.decode()
    : Promise.resolve()
  decode.then(runAnimation).catch(cleanup)

  if (!overlay.complete) {
    overlay.addEventListener('error', cleanup, { once: true })
  }
}

export const ensureFancyboxVideoThumbnail = (video: HTMLVideoElement, target: HTMLElement = video) => {
  const src = getVideoElementSource(video)
  if (!src) return
  const listenersBound = target.dataset.fancyboxVideoThumbBound === 'true'
  const updateOpenThumb = (thumb: string) => {
    const slide = video.closest('.fancybox__slide')
    const container = slide?.closest('.fancybox__container') || document
    const selectedThumb = container.querySelector('.f-thumbs__slide.is-nav-selected img, .f-thumbs__slide.is-selected img') as HTMLImageElement | null
    if (selectedThumb && selectedThumb.src !== thumb) selectedThumb.src = thumb
  }
  const apply = (thumb: string, force = false) => {
    if (!isImageSource(thumb)) return
    if (!force && target.dataset.thumbSrc === thumb) return
    target.dataset.thumbSrc = thumb
    target.dataset.poster = thumb
    video.setAttribute('poster', thumb)
    updateOpenThumb(thumb)
  }
  const poster = video.getAttribute('poster') || ''
  if (poster) {
    apply(poster)
  }
  let lastCapturedAt = -1
  const draw = () => {
    const frame = captureVideoFrame(video)
    if (frame) {
      lastCapturedAt = video.currentTime || 0
      apply(frame, true)
    }
  }
  const scheduleDraw = (force = false) => {
    const currentAt = video.currentTime || 0
    if (!force && target.dataset.thumbSrc && Math.abs(currentAt - lastCapturedAt) < 0.35) return
    window.requestAnimationFrame(draw)
  }
  if (video.readyState >= 2) {
    scheduleDraw(!target.dataset.thumbSrc || video.currentTime > 0)
  }
  if (listenersBound) return
  target.dataset.fancyboxVideoThumbBound = 'true'
  const cleanup = () => {
    video.removeEventListener('loadeddata', onReady)
    video.removeEventListener('canplay', onReady)
  }
  const onReady = () => {
    cleanup()
    scheduleDraw(true)
  }
  video.addEventListener('loadeddata', onReady, { once: true })
  video.addEventListener('canplay', onReady, { once: true })
  video.addEventListener('play', () => scheduleDraw(true))
  video.addEventListener('pause', () => scheduleDraw(true))
  video.addEventListener('seeked', () => scheduleDraw(true))
  video.addEventListener('timeupdate', () => scheduleDraw(false))
}
