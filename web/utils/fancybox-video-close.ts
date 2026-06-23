type FancyboxLike = {
  getSlide?: () => any
  close?: () => void
  container?: HTMLElement | null
  options?: Record<string, any>
  __noiseVideoCloseAnimated?: boolean
  __noiseVideoClosePending?: boolean
  __noiseVideoCloseRetrying?: boolean
}

const firstFrameCache = new Map<string, Promise<string>>()

const validRect = (rect: DOMRect | null | undefined) => !!rect && rect.width > 1 && rect.height > 1

const isImageSource = (src: string | null | undefined) => {
  const value = String(src || '').trim()
  return !!value && (/^(data:image|blob:)/i.test(value) || /\.(png|jpe?g|gif|webp|bmp|svg)(?:[?#].*)?$/i.test(value))
}

export const normalizeMediaPreviewUrl = (src: string) => {
  const value = String(src || '').trim()
  if (!value || typeof window === 'undefined') return value
  try {
    const url = new URL(value, window.location.href)
    const isApiMediaPath = url.pathname.startsWith('/api/images/') || url.pathname.startsWith('/api/video/')
    if (url.origin !== window.location.origin && isApiMediaPath) {
      return `${window.location.origin}${url.pathname}${url.search}${url.hash}`
    }
    return url.href
  } catch {
    return value
  }
}

const captureVideoFrame = (video: HTMLVideoElement | null) => {
  if (!video || !video.videoWidth || !video.videoHeight || video.readyState < 2) return ''
  try {
    const maxSize = 1280
    const scale = Math.min(1, maxSize / Math.max(video.videoWidth, video.videoHeight))
    const canvas = document.createElement('canvas')
    canvas.width = Math.max(1, Math.round(video.videoWidth * scale))
    canvas.height = Math.max(1, Math.round(video.videoHeight * scale))
    const ctx = canvas.getContext('2d')
    if (!ctx) return ''
    ctx.drawImage(video, 0, 0, canvas.width, canvas.height)
    return canvas.toDataURL('image/jpeg', 0.86)
  } catch {
    return ''
  }
}

export const captureVideoFirstFrameFromSource = (source: string, timeoutMs = 5200) => {
  const src = normalizeMediaPreviewUrl(source)
  if (!src || typeof document === 'undefined' || typeof window === 'undefined') return Promise.resolve('')
  const cached = firstFrameCache.get(src)
  if (cached) return cached

  const promise = new Promise<string>((resolve) => {
    const video = document.createElement('video')
    let finished = false
    let soughtToStart = false
    const finish = (thumb: string) => {
      if (finished) return
      finished = true
      cleanup()
      resolve(isImageSource(thumb) ? thumb : '')
    }
    const cleanup = () => {
      window.clearTimeout(timer)
      video.removeEventListener('loadedmetadata', onLoadedMetadata)
      video.removeEventListener('loadeddata', onReady)
      video.removeEventListener('canplay', onReady)
      video.removeEventListener('seeked', onReady)
      video.removeEventListener('error', onError)
      video.removeAttribute('src')
      try { video.load() } catch {}
    }
    const drawFrame = () => {
      const thumb = captureVideoFrame(video)
      if (thumb) finish(thumb)
    }
    const onReady = () => drawFrame()
    const onLoadedMetadata = () => {
      try {
        if (!soughtToStart && Number.isFinite(video.duration) && video.duration > 0) {
          soughtToStart = true
          video.currentTime = 0
        }
      } catch {}
      drawFrame()
    }
    const onError = () => finish('')
    const timer = window.setTimeout(() => finish(captureVideoFrame(video)), timeoutMs)
    video.crossOrigin = 'anonymous'
    video.muted = true
    video.playsInline = true
    video.preload = 'auto'
    video.addEventListener('loadedmetadata', onLoadedMetadata)
    video.addEventListener('loadeddata', onReady)
    video.addEventListener('canplay', onReady)
    video.addEventListener('seeked', onReady)
    video.addEventListener('error', onError)
    video.src = src
    try { video.load() } catch { finish('') }
  })

  firstFrameCache.set(src, promise)
  promise.then((thumb) => {
    if (!thumb) firstFrameCache.delete(src)
  }).catch(() => firstFrameCache.delete(src))
  return promise
}

const getSlideVideoElement = (slide: any) => {
  const root = (slide?.contentEl || slide?.el) as HTMLElement | null
  if (root instanceof HTMLVideoElement) return root
  return root?.querySelector?.('video') as HTMLVideoElement | null
}

export const getVideoElementSource = (video: HTMLVideoElement) => {
  return video.currentSrc || video.getAttribute('src') || video.querySelector('source')?.getAttribute('src') || ''
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

const applyVideoFrameFallback = (slide: any, video: HTMLVideoElement | null, thumb: string) => {
  if (!isImageSource(thumb)) return
  if (video && !video.getAttribute('poster')) video.setAttribute('poster', thumb)
  if (slide) {
    slide.poster = thumb
    slide.thumbSrc = thumb
    slide.thumbElSrc = thumb
  }
  const trigger = slide?.triggerEl as HTMLElement | undefined
  if (trigger?.dataset) {
    trigger.dataset.poster = thumb
    trigger.dataset.thumbSrc = thumb
  }
}

const getSlideContentElement = (slide: any) => {
  const video = getSlideVideoElement(slide)
  return video || (slide?.contentEl as HTMLElement | null) || (slide?.el?.querySelector?.('.fancybox__content') as HTMLElement | null)
}

const getSlideHideElements = (slide: any, contentEl: HTMLElement | null) => {
  return [
    contentEl,
    contentEl?.closest?.('.fancybox__content') as HTMLElement | null,
    slide?.contentEl as HTMLElement | null,
    slide?.el as HTMLElement | null
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

const waitForVideoFrameThenClose = (instance: FancyboxLike, slide: any, video: HTMLVideoElement | null, event?: Event) => {
  if (event?.type === 'shouldClose') event.preventDefault()
  if (!video || event?.type !== 'shouldClose' || instance.__noiseVideoClosePending) return
  instance.__noiseVideoClosePending = true
  let done = false
  const source = normalizeMediaPreviewUrl(getVideoElementSource(video) || slide?.src || slide?.triggerEl?.dataset?.src || '')
  const closeWithoutFrame = () => {
    instance.__noiseVideoCloseAnimated = true
    requestAnimationFrame(() => {
      instance.close?.()
    })
  }
  const finish = (thumb: string) => {
    if (done) return
    done = true
    window.clearTimeout(timer)
    video.removeEventListener('loadeddata', onReady)
    video.removeEventListener('canplay', onReady)
    video.removeEventListener('seeked', onReady)
    instance.__noiseVideoClosePending = false
    if (!thumb) {
      closeWithoutFrame()
      return
    }
    applyVideoFrameFallback(slide, video, thumb)
    instance.__noiseVideoCloseRetrying = true
    requestAnimationFrame(() => {
      try { instance.close?.() } finally { instance.__noiseVideoCloseRetrying = false }
    })
  }
  const onReady = () => {
    const thumb = captureVideoFrame(video)
    if (thumb) finish(thumb)
  }
  const timer = window.setTimeout(() => {
    finish(captureVideoFrame(video) || getSlideImageFallback(slide, video))
  }, source ? 5600 : 1800)
  video.addEventListener('loadeddata', onReady)
  video.addEventListener('canplay', onReady)
  video.addEventListener('seeked', onReady)
  if (source) captureVideoFirstFrameFromSource(source).then(finish).catch(() => finish(captureVideoFrame(video) || getSlideImageFallback(slide, video)))
  try { video.load?.() } catch {}
  onReady()
}

export const animateFancyboxHtml5VideoClose = (instance: FancyboxLike, event?: Event) => {
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
  if (!frameSrc) return waitForVideoFrameThenClose(instance, slide, video, event)
  applyVideoFrameFallback(slide, video, frameSrc)
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
  hideEls.forEach((item) => {
    item.style.visibility = 'hidden'
    item.style.opacity = '0'
  })
  let cleaned = false
  const cleanup = () => {
    if (cleaned) return
    cleaned = true
    overlay.remove()
    hideEls.forEach((item, index) => {
      item.style.visibility = previousVisibility[index] || ''
      item.style.opacity = previousOpacity[index] || ''
    })
  }

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
  const src = normalizeMediaPreviewUrl(getVideoElementSource(video))
  if (!src) return
  const apply = (thumb: string) => {
    if (!isImageSource(thumb)) return
    target.dataset.thumbSrc = thumb
    target.dataset.poster = thumb
    video.setAttribute('poster', video.getAttribute('poster') || thumb)
  }
  const poster = video.getAttribute('poster') || target.dataset.poster || target.dataset.thumbSrc || ''
  if (poster) apply(poster)
  const refresh = () => {
    const frame = captureVideoFrame(video)
    if (frame) apply(frame)
  }
  if (video.readyState >= 2) refresh()
  if (target.dataset.noiseVideoThumbBound === 'true') return
  target.dataset.noiseVideoThumbBound = 'true'
  const onReady = () => refresh()
  video.preload = 'auto'
  video.addEventListener('loadeddata', onReady)
  video.addEventListener('canplay', onReady)
  captureVideoFirstFrameFromSource(src).then(apply).catch(() => {})
  try { video.load?.() } catch {}
}
