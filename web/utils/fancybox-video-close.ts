type FancyboxLike = {
  getSlide?: () => any
  close?: () => void
  container?: HTMLElement | null
  options?: Record<string, any>
  __noiseVideoCloseAnimated?: boolean
  __noiseVideoClosePending?: boolean
  __noiseVideoCloseRetrying?: boolean
}

type VideoPlaybackState = {
  firstFrame?: string
  currentFrame?: string
  currentTime?: number
  duration?: number
  hasPlayback?: boolean
  updatedAt?: number
}

type VideoStateSyncOptions = {
  originVideo?: HTMLVideoElement | null
  syncTime?: boolean
}

type VideoSurfaceRegisterOptions = {
  pauseOtherVideos?: boolean
  source?: string
}

const firstFrameCache = new Map<string, Promise<string>>()
const videoSurfaceRegistry = new Map<string, { videos: Set<HTMLVideoElement>; targets: Set<HTMLElement> }>()
const VIDEO_PLAYBACK_MEMORY_KEY = 'noise-video-playback-state:v1'
const PLAYBACK_PROGRESS_THRESHOLD = 0.15

const getVideoMemoryKey = (source: string) => {
  const normalized = normalizeMediaPreviewUrl(source)
  if (!normalized || typeof window === 'undefined') return normalized
  try {
    const url = new URL(normalized, window.location.href)
    if (url.origin === window.location.origin) return `${url.pathname}${url.search}`
    return url.href
  } catch {
    return normalized
  }
}

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

const getPlaybackMemory = (): Record<string, VideoPlaybackState> => {
  if (typeof localStorage === 'undefined') return {}
  try {
    const parsed = JSON.parse(localStorage.getItem(VIDEO_PLAYBACK_MEMORY_KEY) || '{}')
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

const setPlaybackMemory = (memory: Record<string, VideoPlaybackState>) => {
  if (typeof localStorage === 'undefined') return
  try { localStorage.setItem(VIDEO_PLAYBACK_MEMORY_KEY, JSON.stringify(memory)) } catch {}
}

const getVideoState = (source: string): VideoPlaybackState => {
  const key = getVideoMemoryKey(source)
  if (!key) return {}
  const memory = getPlaybackMemory()
  const legacyKey = normalizeMediaPreviewUrl(source)
  return memory[key] || (legacyKey && legacyKey !== key ? memory[legacyKey] : undefined) || {}
}

const updateVideoState = (source: string, patch: VideoPlaybackState, options: VideoStateSyncOptions = {}) => {
  const key = getVideoMemoryKey(source)
  if (!key) return
  const memory = getPlaybackMemory()
  const legacyKey = normalizeMediaPreviewUrl(source)
  const previous = {
    ...(legacyKey && legacyKey !== key ? (memory[legacyKey] || {}) : {}),
    ...(memory[key] || {})
  }
  const nextState = {
    ...previous,
    ...patch,
    updatedAt: Date.now()
  }
  memory[key] = nextState
  if (legacyKey && legacyKey !== key) delete memory[legacyKey]
  setPlaybackMemory(memory)
  syncRegisteredVideoSurfaces(key, nextState, options)
}

export const clearVideoPlaybackMemory = () => {
  videoSurfaceRegistry.clear()
  if (typeof localStorage === 'undefined') return
  try { localStorage.removeItem(VIDEO_PLAYBACK_MEMORY_KEY) } catch {}
}

const hasRememberedPlayback = (state: VideoPlaybackState | null | undefined) => !!state?.hasPlayback || Number(state?.currentTime || 0) > PLAYBACK_PROGRESS_THRESHOLD

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
    if (thumb) updateVideoState(src, { firstFrame: thumb }, { syncTime: false })
    else firstFrameCache.delete(src)
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

const recordVideoProgress = (video: HTMLVideoElement, source = getVideoElementSource(video), captureFrame = false) => {
  const src = normalizeMediaPreviewUrl(source)
  if (!src) return
  const currentTime = Number(video.currentTime || 0)
  const duration = Number(video.duration || 0)
  const hasPlayback = currentTime > PLAYBACK_PROGRESS_THRESHOLD || !video.paused
  const frame = captureFrame && hasPlayback ? captureVideoFrame(video) : ''
  updateVideoState(src, {
    currentTime: Number.isFinite(currentTime) ? currentTime : 0,
    duration: Number.isFinite(duration) ? duration : 0,
    hasPlayback,
    ...(frame ? { currentFrame: frame } : {})
  }, { originVideo: video, syncTime: true })
}

const applyStoredFrameToVideo = (video: HTMLVideoElement, target: HTMLElement, state: VideoPlaybackState) => {
  const thumb = hasRememberedPlayback(state) ? (state.currentFrame || state.firstFrame || '') : (state.firstFrame || '')
  if (!isImageSource(thumb)) return
  target.dataset.thumbSrc = thumb
  target.dataset.poster = thumb
  video.setAttribute('poster', thumb)
}

const restoreStoredVideoTime = (video: HTMLVideoElement, state: VideoPlaybackState) => {
  const time = Number(state.currentTime || 0)
  if (!Number.isFinite(time) || time <= PLAYBACK_PROGRESS_THRESHOLD) return
  const duration = Number(video.duration || state.duration || 0)
  const safeTime = duration > 0 ? Math.min(time, Math.max(0, duration - 0.1)) : time
  try {
    if (Math.abs(Number(video.currentTime || 0) - safeTime) > 0.25) video.currentTime = safeTime
  } catch {}
}

const getStateFrame = (state: VideoPlaybackState) => {
  return hasRememberedPlayback(state) ? (state.currentFrame || state.firstFrame || '') : (state.firstFrame || '')
}

export const getVideoPlaybackFrameForSource = (source: string) => getStateFrame(getVideoState(source))

const applyFrameToTarget = (target: HTMLElement | null | undefined, thumb: string) => {
  if (!target || !isImageSource(thumb)) return
  target.dataset.thumbSrc = thumb
  target.dataset.poster = thumb
  if (target instanceof HTMLImageElement) target.src = thumb
  const img = target.querySelector?.('img') as HTMLImageElement | null
  if (img) img.src = thumb
}

const syncVideoTimeWhenReady = (video: HTMLVideoElement, state: VideoPlaybackState) => {
  if (video.readyState >= 1) {
    restoreStoredVideoTime(video, state)
    return
  }
  video.addEventListener('loadedmetadata', () => restoreStoredVideoTime(video, getVideoState(video.dataset.noiseVideoPlaybackSource || getVideoElementSource(video))), { once: true })
}

const getRegisteredSurfaces = (source: string) => {
  const src = getVideoMemoryKey(source)
  if (!src) return null
  let entry = videoSurfaceRegistry.get(src)
  if (!entry) {
    entry = { videos: new Set<HTMLVideoElement>(), targets: new Set<HTMLElement>() }
    videoSurfaceRegistry.set(src, entry)
  }
  return { src, entry }
}

const syncRegisteredVideoSurfaces = (source: string, state = getVideoState(source), options: VideoStateSyncOptions = {}) => {
  const src = getVideoMemoryKey(source)
  if (!src || typeof document === 'undefined') return
  const entry = videoSurfaceRegistry.get(src)
  const frame = getStateFrame(state)
  entry?.videos.forEach((video) => {
    if (!document.contains(video)) {
      entry.videos.delete(video)
      return
    }
    if (isImageSource(frame) && (video !== options.originVideo || video.paused)) video.setAttribute('poster', frame)
    if (options.syncTime && video !== options.originVideo && video.paused) syncVideoTimeWhenReady(video, state)
  })
  entry?.targets.forEach((target) => {
    if (!document.contains(target)) {
      entry.targets.delete(target)
      return
    }
    applyFrameToTarget(target, frame)
  })
}

const pauseOtherRegisteredVideos = (source: string, currentVideo: HTMLVideoElement | null) => {
  const src = getVideoMemoryKey(source)
  if (!src) return
  const entry = videoSurfaceRegistry.get(src)
  entry?.videos.forEach((video) => {
    if (!video || video === currentVideo || video.paused) return
    recordVideoProgress(video, src, true)
    try { video.pause() } catch {}
    recordVideoProgress(video, src, true)
  })
}

const registerVideoSurface = (source: string, video: HTMLVideoElement | null, targets: Array<HTMLElement | null | undefined>, options: VideoSurfaceRegisterOptions = {}) => {
  const registered = getRegisteredSurfaces(source)
  if (!registered) return
  const { src, entry } = registered
  if (video) {
    entry.videos.add(video)
    video.dataset.noiseVideoPlaybackSource = src
  }
  targets.forEach((target) => {
    if (!target) return
    entry.targets.add(target)
    target.dataset.noiseVideoPlaybackSource = src
  })
  if (options.pauseOtherVideos) pauseOtherRegisteredVideos(src, video)
  syncRegisteredVideoSurfaces(src, getVideoState(src), { syncTime: true })
}

const bindVideoPlaybackState = (video: HTMLVideoElement, target: HTMLElement, source: string, extraTargets: HTMLElement[] = [], options: VideoSurfaceRegisterOptions = {}) => {
  registerVideoSurface(source, video, [target, ...extraTargets], options)
  video.dataset.noiseVideoPlaybackSource = getVideoMemoryKey(source) || source
  if (video.dataset.noiseVideoPlaybackBound === 'true') return
  video.dataset.noiseVideoPlaybackBound = 'true'
  const getBoundSource = () => video.dataset.noiseVideoPlaybackSource || source
  const record = () => recordVideoProgress(video, getBoundSource(), false)
  const recordWithFrame = () => recordVideoProgress(video, getBoundSource(), true)
  let lastFrameCaptureAt = 0
  const recordThrottledFrame = () => {
    record()
    const now = Date.now()
    if (now - lastFrameCaptureAt < 1200) return
    lastFrameCaptureAt = now
    recordWithFrame()
  }
  video.addEventListener('play', record)
  video.addEventListener('timeupdate', recordThrottledFrame)
  video.addEventListener('pause', recordWithFrame)
  video.addEventListener('seeked', recordWithFrame)
  video.addEventListener('ended', recordWithFrame)
  video.addEventListener('loadedmetadata', () => {
    const state = getVideoState(getBoundSource())
    applyStoredFrameToVideo(video, target, state)
    restoreStoredVideoTime(video, state)
  })
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

const applyVideoFrameFallback = (slide: any, video: HTMLVideoElement | null, thumb: string, source?: string) => {
  if (!isImageSource(thumb)) return
  if (video) video.setAttribute('poster', thumb)
  if (slide) {
    slide.poster = thumb
    slide.thumbSrc = thumb
    slide.thumbElSrc = thumb
  }
  const trigger = slide?.triggerEl as HTMLElement | undefined
  ;[trigger, slide?.thumbEl, slide?.el?.querySelector?.('.f-thumbs__slide__img')].forEach((item) => applyFrameToTarget(item as HTMLElement | null | undefined, thumb))
  if (source) updateVideoState(source, { currentFrame: thumb }, { originVideo: video, syncTime: false })
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

const getCloseFrameSource = (slide: any, video: HTMLVideoElement | null) => normalizeMediaPreviewUrl(
  (video ? getVideoElementSource(video) : '') || slide?.src || slide?.triggerEl?.dataset?.src || ''
)

const hasLivePlayback = (video: HTMLVideoElement | null) => {
  if (!video) return false
  return Number(video.currentTime || 0) > PLAYBACK_PROGRESS_THRESHOLD || !video.paused
}

const getVideoCloseFrame = (slide: any, video: HTMLVideoElement | null, source: string) => {
  const state = getVideoState(source)
  const rememberedPlayback = hasRememberedPlayback(state)
  const videoTime = Number(video?.currentTime || 0)
  const rememberedTime = Number(state.currentTime || 0)
  const canCaptureLiveFrame = hasLivePlayback(video) && (
    !rememberedPlayback ||
    rememberedTime <= PLAYBACK_PROGRESS_THRESHOLD ||
    Math.abs(videoTime - rememberedTime) < 0.75 ||
    videoTime > rememberedTime
  )
  if (canCaptureLiveFrame) {
    const frame = captureVideoFrame(video)
    if (frame) {
      if (video) recordVideoProgress(video, source, true)
      return frame
    }
  }
  if (rememberedPlayback && isImageSource(state.currentFrame)) return state.currentFrame || ''
  return state.firstFrame || getSlideImageFallback(slide, video)
}

const waitForVideoFrameThenClose = (instance: FancyboxLike, slide: any, video: HTMLVideoElement | null, event?: Event) => {
  if (event?.type === 'shouldClose') event.preventDefault()
  if (!video || event?.type !== 'shouldClose' || instance.__noiseVideoClosePending) return
  instance.__noiseVideoClosePending = true
  let done = false
  const source = getCloseFrameSource(slide, video)
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
    applyVideoFrameFallback(slide, video, thumb, source)
    instance.__noiseVideoCloseRetrying = true
    requestAnimationFrame(() => {
      try { instance.close?.() } finally { instance.__noiseVideoCloseRetrying = false }
    })
  }
  const onReady = () => {
    if (!hasLivePlayback(video) && !hasRememberedPlayback(getVideoState(source))) return
    const thumb = captureVideoFrame(video)
    if (thumb) {
      if (source) recordVideoProgress(video, source, true)
      finish(thumb)
    }
  }
  const finishWithBestFrame = (fallback = '') => {
    const frame = getVideoCloseFrame(slide, video, source)
    finish(frame || fallback)
  }
  const timer = window.setTimeout(() => {
    finishWithBestFrame()
  }, source ? 5600 : 1800)
  video.addEventListener('loadeddata', onReady)
  video.addEventListener('canplay', onReady)
  video.addEventListener('seeked', onReady)
  if (source) {
    captureVideoFirstFrameFromSource(source)
      .then((thumb) => {
        if (!hasLivePlayback(video) && !hasRememberedPlayback(getVideoState(source))) finish(thumb)
      })
      .catch(() => finishWithBestFrame())
  }
  if (video.readyState < 1 && !hasRememberedPlayback(getVideoState(source))) {
    try { video.load?.() } catch {}
  }
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
  const source = getCloseFrameSource(slide, video)
  const frameSrc = getVideoCloseFrame(slide, video, source)
  if (!frameSrc) return waitForVideoFrameThenClose(instance, slide, video, event)
  applyVideoFrameFallback(slide, video, frameSrc, source)
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

export const prepareFancyboxHtml5VideoSlide = (instance: FancyboxLike) => {
  if (typeof document === 'undefined') return
  const slide = instance?.getSlide?.()
  if (!slide || slide.type !== 'html5video') return
  const video = getSlideVideoElement(slide)
  if (!video) return
  const target = (slide.triggerEl || slide.thumbEl || video) as HTMLElement
  const extraTargets = [slide.thumbEl, slide.el?.querySelector?.('.f-thumbs__slide__img')].filter(Boolean) as HTMLElement[]
  const source = normalizeMediaPreviewUrl(slide.src || slide.triggerEl?.dataset?.src || getVideoElementSource(video))
  ensureFancyboxVideoThumbnail(video, target, extraTargets, { source, pauseOtherVideos: true })
  try { video.pause() } catch {}
}

export const ensureFancyboxVideoThumbnail = (video: HTMLVideoElement, target: HTMLElement = video, extraTargets: HTMLElement[] = [], options: VideoSurfaceRegisterOptions = {}) => {
  const src = normalizeMediaPreviewUrl(options.source || getVideoElementSource(video))
  if (!src) return
  const apply = (thumb: string, kind: 'first' | 'current' = 'first') => {
    if (!isImageSource(thumb)) return
    ;[target, ...extraTargets].forEach((item) => applyFrameToTarget(item, thumb))
    video.setAttribute('poster', thumb)
    updateVideoState(src, kind === 'current' ? { currentFrame: thumb } : { firstFrame: thumb }, { originVideo: video, syncTime: false })
  }
  const state = getVideoState(src)
  applyStoredFrameToVideo(video, target, state)
  const poster = video.getAttribute('poster') || target.dataset.poster || target.dataset.thumbSrc || ''
  if (poster && !state.firstFrame && !hasRememberedPlayback(state)) apply(poster, 'first')
  const refresh = () => {
    if (!hasLivePlayback(video) && !hasRememberedPlayback(getVideoState(src))) return
    const frame = captureVideoFrame(video)
    if (frame) apply(frame, 'current')
  }
  bindVideoPlaybackState(video, target, src, extraTargets, options)
  if (video.readyState >= 1 && video.paused) restoreStoredVideoTime(video, getVideoState(src))
  if (video.readyState >= 2) refresh()
  if (target.dataset.noiseVideoThumbBound === 'true') return
  target.dataset.noiseVideoThumbBound = 'true'
  const onReady = () => refresh()
  video.preload = 'auto'
  video.addEventListener('loadeddata', onReady)
  video.addEventListener('canplay', onReady)
  captureVideoFirstFrameFromSource(src).then((thumb) => {
    if (!thumb) return
    const latest = getVideoState(src)
    apply(thumb, 'first')
    if (hasRememberedPlayback(latest)) applyStoredFrameToVideo(video, target, latest)
  }).catch(() => {})
  try { video.load?.() } catch {}
}
