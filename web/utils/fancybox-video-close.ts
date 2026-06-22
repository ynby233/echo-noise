type FancyboxLike = {
  getSlide?: () => any
  container?: HTMLElement | null
  options?: Record<string, any>
}

const validRect = (rect: DOMRect | null | undefined) => !!rect && rect.width > 1 && rect.height > 1

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
  return root?.querySelector?.('video') as HTMLVideoElement | null
}

export const getVideoElementSource = (video: HTMLVideoElement) => {
  return video.currentSrc || video.getAttribute('src') || video.querySelector('source')?.getAttribute('src') || ''
}

const getSlideContentElement = (slide: any) => {
  const video = getSlideVideoElement(slide)
  return video || (slide?.contentEl as HTMLElement | null) || (slide?.el?.querySelector?.('.fancybox__content') as HTMLElement | null)
}

const getTriggerElement = (instance: FancyboxLike, slide: any) => {
  return (slide?.triggerEl || slide?.thumbEl || instance?.options?.triggerEl || null) as HTMLElement | null
}

export const animateFancyboxHtml5VideoClose = (instance: FancyboxLike) => {
  if (typeof document === 'undefined') return
  const slide = instance?.getSlide?.()
  if (!slide || slide.type !== 'html5video') return

  const contentEl = getSlideContentElement(slide)
  const targetEl = getTriggerElement(instance, slide)
  const startRect = contentEl?.getBoundingClientRect?.()
  const targetRect = targetEl?.getBoundingClientRect?.()
  if (!contentEl || !validRect(startRect) || !validRect(targetRect)) return

  const video = getSlideVideoElement(slide)
  const frameSrc = captureVideoFrame(video) || video?.poster || slide.poster || slide.thumbElSrc || slide.thumbSrc || ''
  const overlay = document.createElement(frameSrc ? 'img' : 'div') as HTMLElement
  if (overlay instanceof HTMLImageElement) {
    overlay.src = frameSrc
    overlay.alt = ''
    overlay.decoding = 'async'
  } else {
    overlay.style.background = '#000'
  }

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
    transition: 'transform 260ms cubic-bezier(0.22, 1, 0.36, 1), opacity 260ms ease',
    willChange: 'transform, opacity',
    opacity: '1'
  })

  const previousVisibility = contentEl.style.visibility
  contentEl.style.visibility = 'hidden'
  document.body.appendChild(overlay)

  const scaleX = targetRect!.width / startRect!.width
  const scaleY = targetRect!.height / startRect!.height
  const translateX = targetRect!.left - startRect!.left
  const translateY = targetRect!.top - startRect!.top

  requestAnimationFrame(() => {
    overlay.style.transform = `translate3d(${translateX}px, ${translateY}px, 0) scale(${scaleX}, ${scaleY})`
    overlay.style.opacity = '0.12'
  })

  window.setTimeout(() => {
    overlay.remove()
    contentEl.style.visibility = previousVisibility
  }, 320)
}

export const ensureFancyboxVideoThumbnail = (video: HTMLVideoElement, target: HTMLElement = video) => {
  const src = getVideoElementSource(video)
  if (!src || target.dataset.thumbSrc) return
  const apply = (thumb: string) => {
    if (!thumb || target.dataset.thumbSrc) return
    target.dataset.thumbSrc = thumb
    target.dataset.poster = thumb
    video.setAttribute('poster', video.getAttribute('poster') || thumb)
  }
  const poster = video.getAttribute('poster') || ''
  if (poster) {
    apply(poster)
    return
  }
  const draw = () => {
    const frame = captureVideoFrame(video)
    if (frame) apply(frame)
  }
  if (video.readyState >= 2) {
    draw()
    return
  }
  const cleanup = () => {
    video.removeEventListener('loadeddata', onReady)
    video.removeEventListener('canplay', onReady)
    video.removeEventListener('seeked', onReady)
  }
  const onReady = () => {
    cleanup()
    draw()
  }
  video.addEventListener('loadeddata', onReady, { once: true })
  video.addEventListener('canplay', onReady, { once: true })
  video.addEventListener('seeked', onReady, { once: true })
}
