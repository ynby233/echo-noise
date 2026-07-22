export const AUDIO_PLAYBACK_RATES = [0.5, 1, 1.25, 1.5, 1.75, 2, 3] as const

export type AttachmentAudioPlaceholderOptions = {
  src: string
  name?: string
  size?: number | null
}

type PlayerCleanup = () => void

const playerCleanup = new WeakMap<HTMLElement, PlayerCleanup>()
const tableTriggerCleanup = new WeakMap<HTMLElement, PlayerCleanup>()
const audioSizeCache = new Map<string, Promise<number | null>>()

const escapeHtmlAttribute = (value: string) => String(value || '')
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;')
  .replace(/'/g, '&#39;')

const decodedPathSegment = (src: string) => {
  const clean = String(src || '').split(/[?#]/)[0]
  const segment = clean.split('/').filter(Boolean).pop() || ''
  try { return decodeURIComponent(segment) } catch { return segment }
}

export const audioFileName = (src: string, preferredName = '') => {
  const preferred = String(preferredName || '').trim()
  return preferred || decodedPathSegment(src) || '未命名音频'
}

export const audioFormatLabel = (src: string, preferredName = '') => {
  const source = audioFileName(src, preferredName).split(/[?#]/)[0]
  const match = source.match(/\.([a-z0-9]{1,12})$/i)
  return match ? match[1].toUpperCase() : 'AUDIO'
}

export const formatAudioFileSize = (bytes: number | null | undefined) => {
  if (!Number.isFinite(bytes) || Number(bytes) < 0) return '大小未知'
  const value = Number(bytes)
  if (value < 1024) return `${Math.round(value)} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let amount = value / 1024
  let unitIndex = 0
  while (amount >= 1024 && unitIndex < units.length - 1) {
    amount /= 1024
    unitIndex += 1
  }
  const rounded = amount >= 100 ? Math.round(amount) : Number(amount.toFixed(1))
  return `${rounded} ${units[unitIndex]}`
}

export const formatAudioTime = (seconds: number | null | undefined) => {
  const whole = Number.isFinite(seconds) && Number(seconds) > 0 ? Math.floor(Number(seconds)) : 0
  const hours = Math.floor(whole / 3600)
  const minutes = Math.floor((whole % 3600) / 60)
  const remaining = whole % 60
  if (hours > 0) return `${hours}:${String(minutes).padStart(2, '0')}:${String(remaining).padStart(2, '0')}`
  return `${minutes}:${String(remaining).padStart(2, '0')}`
}

export const buildAttachmentAudioPlaceholderHtml = ({ src, name = '', size = null }: AttachmentAudioPlaceholderOptions) => {
  const safeSrc = escapeHtmlAttribute(String(src || '').trim())
  const safeName = escapeHtmlAttribute(audioFileName(src, name))
  const sizeAttribute = Number.isFinite(size) && Number(size) >= 0 ? ` data-audio-size="${Number(size)}"` : ''
  return `<div class="site-attachment-audio" data-site-audio-player data-audio-src="${safeSrc}" data-audio-name="${safeName}"${sizeAttribute} data-site-attachment-kind="audio" data-site-attachment-url="${safeSrc}"></div>`
}

const responseSize = (response: Response) => {
  const contentRange = response.headers.get('content-range') || ''
  const rangeMatch = contentRange.match(/\/(\d+)\s*$/)
  if (rangeMatch) return Number(rangeMatch[1])
  const contentLength = Number(response.headers.get('content-length'))
  return Number.isFinite(contentLength) && contentLength >= 0 ? contentLength : null
}

const canProbeUrl = (src: string) => {
  if (typeof window === 'undefined' || typeof fetch === 'undefined') return false
  try {
    const url = new URL(src, window.location.href)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

export const probeAudioFileSize = (src: string) => {
  const raw = String(src || '').trim()
  if (!raw || !canProbeUrl(raw)) return Promise.resolve(null)
  const key = new URL(raw, window.location.href).toString()
  const cached = audioSizeCache.get(key)
  if (cached) return cached

  const promise = (async () => {
    try {
      const head = await fetch(key, { method: 'HEAD', cache: 'no-store', credentials: 'same-origin' })
      const headSize = responseSize(head)
      if (head.ok && headSize !== null) return headSize
      if (!head.ok && head.status !== 405 && head.status !== 501) return null

      const controller = typeof AbortController !== 'undefined' ? new AbortController() : null
      const partial = await fetch(key, {
        method: 'GET',
        headers: { Range: 'bytes=0-0' },
        cache: 'no-store',
        credentials: 'same-origin',
        signal: controller?.signal,
      })
      const partialSize = partial.ok ? responseSize(partial) : null
      controller?.abort()
      return partialSize
    } catch {
      return null
    }
  })()
  audioSizeCache.set(key, promise)
  return promise
}

const createElement = <K extends keyof HTMLElementTagNameMap>(tag: K, className = '') => {
  const element = document.createElement(tag)
  if (className) element.className = className
  return element
}

const clamp = (value: number, min: number, max: number) => Math.min(Math.max(value, min), Math.max(min, max))

const positionFloatingMenu = (
  trigger: HTMLElement,
  menu: HTMLElement,
  styleRef: { value: Record<string, string> },
  minWidth = 120,
  placement: 'above-right' = 'above-right',
) => {
  const zoom = Number.parseFloat(window.getComputedStyle(document.body).zoom || '1')
  const scale = Number.isFinite(zoom) && zoom > 0 ? zoom : 1
  const rawRect = trigger.getBoundingClientRect()
  const viewport = window.visualViewport
  const viewportLeft = (viewport?.offsetLeft || 0) / scale
  const viewportTop = (viewport?.offsetTop || 0) / scale
  const viewportWidth = (viewport?.width || window.innerWidth) / scale
  const viewportHeight = (viewport?.height || window.innerHeight) / scale
  const rect = {
    left: (rawRect.left + (viewport?.offsetLeft || 0)) / scale,
    right: (rawRect.right + (viewport?.offsetLeft || 0)) / scale,
    top: (rawRect.top + (viewport?.offsetTop || 0)) / scale,
    bottom: (rawRect.bottom + (viewport?.offsetTop || 0)) / scale,
    width: rawRect.width / scale,
  }
  const menuWidth = Math.max(menu.offsetWidth || minWidth, minWidth, rect.width)
  const menuHeight = menu.offsetHeight || 180
  const pad = 8
  const gap = 4
  const minLeft = viewportLeft + pad
  const maxLeft = Math.max(minLeft, viewportLeft + viewportWidth - menuWidth - pad)
  const minTop = viewportTop + pad
  const maxTop = Math.max(minTop, viewportTop + viewportHeight - menuHeight - pad)
  const idealLeft = placement === 'above-right' ? rect.right - menuWidth : rect.left
  const aboveTop = rect.top - menuHeight - gap
  const belowTop = rect.bottom + gap
  styleRef.value = {
    position: 'fixed',
    left: `${clamp(idealLeft, minLeft, maxLeft)}px`,
    top: `${clamp(aboveTop >= minTop ? aboveTop : belowTop, minTop, maxTop)}px`,
    right: 'auto',
    bottom: 'auto',
    transform: 'none',
    minWidth: `${Math.max(minWidth, rect.width)}px`,
  }
}

const scheduleFloatingMenuPosition = (positioner: () => void) => {
  positioner()
  window.requestAnimationFrame(() => {
    positioner()
    window.requestAnimationFrame(positioner)
  })
}

type ProjectIconKind = 'play' | 'pause' | 'volume' | 'muted' | 'chevron-down'

const PROJECT_ICON_PATHS: Record<ProjectIconKind, string> = {
  play: 'M8 5.14v14l11-7z',
  pause: 'M14 19h4V5h-4M6 19h4V5H6z',
  volume: 'M14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.84-5 6.7v2.07c4-.91 7-4.49 7-8.77s-3-7.86-7-8.77M16.5 12c0-1.77-1-3.29-2.5-4.03V16c1.5-.71 2.5-2.24 2.5-4M3 9v6h4l5 5V4L7 9z',
  muted: 'M12 4L9.91 6.09L12 8.18M4.27 3L3 4.27L7.73 9H3v6h4l5 5v-6.73l4.25 4.26c-.67.51-1.42.93-2.25 1.17v2.07c1.38-.32 2.63-.95 3.68-1.81L19.73 21L21 19.73l-9-9M19 12c0 .94-.2 1.82-.54 2.64l1.51 1.51A8.9 8.9 0 0 0 21 12c0-4.28-3-7.86-7-8.77v2.06c2.89.86 5 3.54 5 6.71m-2.5 0c0-1.77-1-3.29-2.5-4.03v2.21l2.45 2.45c.05-.2.05-.42.05-.63',
  'chevron-down': 'M7.41 8.58L12 13.17l4.59-4.59L18 10l-6 6l-6-6z',
}

const setProjectIcon = (icon: SVGSVGElement, kind: ProjectIconKind) => {
  icon.querySelector('path')?.setAttribute('d', PROJECT_ICON_PATHS[kind])
}

const createIcon = (kind: ProjectIconKind) => {
  const icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
  icon.setAttribute('viewBox', '0 0 24 24')
  icon.setAttribute('aria-hidden', 'true')
  icon.classList.add('site-attachment-audio__svg')
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path')
  path.setAttribute('fill', 'currentColor')
  path.setAttribute('d', PROJECT_ICON_PATHS[kind])
  icon.appendChild(path)
  return icon
}

const setRangeProgress = (input: HTMLInputElement, ratio: number) => {
  const percent = Math.max(0, Math.min(1, Number.isFinite(ratio) ? ratio : 0)) * 100
  input.style.setProperty('--audio-range-progress', `${percent}%`)
}

const mountAudioPlayer = (root: HTMLElement) => {
  if (root.dataset.noiseAudioMounted === 'true' && playerCleanup.has(root)) return
  const src = String(root.dataset.audioSrc || root.dataset.noiseAttachmentUrl || '').trim()
  if (!src) return
  const name = audioFileName(src, root.dataset.audioName || '')
  const format = audioFormatLabel(src, name)
  const knownSize = Number(root.dataset.audioSize)
  const initialSize = Number.isFinite(knownSize) && knownSize >= 0 ? knownSize : null

  root.dataset.noiseAudioMounted = 'true'
  root.replaceChildren()
  root.setAttribute('role', 'group')
  root.setAttribute('aria-label', `音频附件：${name}`)

  const audio = createElement('audio', 'site-attachment-audio__native')
  audio.src = src
  audio.preload = 'metadata'
  audio.dataset.noiseAttachmentKind = 'audio'
  audio.dataset.noiseAttachmentUrl = src

  const header = createElement('div', 'site-attachment-audio__header')
  const information = createElement('div', 'site-attachment-audio__information')
  const nameElement = createElement('div', 'site-attachment-audio__name')
  nameElement.textContent = name
  nameElement.classList.add('nw-tooltip-anchor')
  nameElement.dataset.tooltip = name
  const meta = createElement('div', 'site-attachment-audio__meta')
  const formatElement = createElement('span', 'site-attachment-audio__format')
  formatElement.textContent = format
  const separator = createElement('span', 'site-attachment-audio__meta-separator')
  separator.textContent = '·'
  separator.setAttribute('aria-hidden', 'true')
  const sizeElement = createElement('span', 'site-attachment-audio__size')
  sizeElement.textContent = initialSize === null ? '大小读取中' : formatAudioFileSize(initialSize)
  const statusElement = createElement('span', 'site-attachment-audio__status')
  meta.append(formatElement, separator, sizeElement, statusElement)
  information.append(nameElement)

  const playButton = createElement('button', 'site-attachment-audio__play')
  playButton.classList.add('nw-action-btn', 'nw-tooltip-anchor')
  playButton.type = 'button'
  playButton.setAttribute('aria-label', '播放')
  playButton.dataset.tooltip = '播放'
  const playIcon = createIcon('play')
  playButton.appendChild(playIcon)
  header.append(information)

  const seek = createElement('input', 'site-attachment-audio__range site-attachment-audio__seek')
  seek.type = 'range'
  seek.min = '0'
  seek.max = '0'
  seek.step = '0.01'
  seek.value = '0'
  seek.setAttribute('aria-label', '音频播放进度')
  seek.classList.add('nw-tooltip-anchor')
  seek.dataset.tooltip = '播放进度'
  setRangeProgress(seek, 0)

  const footer = createElement('div', 'site-attachment-audio__footer')
  const footerMeta = createElement('div', 'site-attachment-audio__footer-meta')
  const time = createElement('span', 'site-attachment-audio__time')
  time.textContent = '0:00 / 0:00'
  footerMeta.append(time, meta)

  const tools = createElement('div', 'site-attachment-audio__tools')
  const speedTrigger = createElement('button', 'site-attachment-audio__speed-trigger')
  speedTrigger.classList.add('nw-action-btn', 'nw-action-btn--label', 'nw-tooltip-anchor')
  speedTrigger.type = 'button'
  speedTrigger.setAttribute('aria-label', '播放速度')
  speedTrigger.setAttribute('aria-haspopup', 'listbox')
  speedTrigger.setAttribute('aria-expanded', 'false')
  speedTrigger.dataset.tooltip = '播放速度：1x'
  const speedValue = createElement('span', 'site-attachment-audio__speed-value')
  speedValue.textContent = '1x'
  const speedChevron = createIcon('chevron-down')
  speedChevron.classList.add('site-attachment-audio__speed-chevron')
  speedTrigger.append(speedValue, speedChevron)

  const speedMenu = createElement('div')
  speedMenu.className = 'site-attachment-audio__speed-menu floating-control-menu visibility-floating-menu nw-floating-menu'
  speedMenu.setAttribute('role', 'listbox')
  speedMenu.setAttribute('aria-label', '播放速度选项')
  const speedMenuStyle = { value: {} as Record<string, string> }
  const speedOptions = new Map<number, HTMLButtonElement>()
  AUDIO_PLAYBACK_RATES.forEach((rate) => {
    const option = createElement('button', 'floating-control-option nw-floating-option site-attachment-audio__speed-option')
    option.type = 'button'
    option.setAttribute('role', 'option')
    option.dataset.rate = String(rate)
    option.setAttribute('aria-selected', rate === 1 ? 'true' : 'false')
    option.textContent = `${rate}x`
    speedMenu.appendChild(option)
    speedOptions.set(rate, option)
  })

  const volumeGroup = createElement('div', 'site-attachment-audio__volume')
  const muteButton = createElement('button', 'site-attachment-audio__mute')
  muteButton.classList.add('nw-action-btn', 'nw-tooltip-anchor')
  muteButton.type = 'button'
  muteButton.setAttribute('aria-label', '静音')
  muteButton.dataset.tooltip = '静音'
  const volumeIcon = createIcon('volume')
  muteButton.appendChild(volumeIcon)
  const volume = createElement('input', 'site-attachment-audio__range site-attachment-audio__volume-range')
  volume.type = 'range'
  volume.min = '0'
  volume.max = '1'
  volume.step = '0.01'
  volume.value = '1'
  volume.setAttribute('aria-label', '音量')
  volume.classList.add('nw-tooltip-anchor')
  volume.dataset.tooltip = '调整音量'
  setRangeProgress(volume, 1)
  volumeGroup.append(muteButton, volume)
  tools.append(speedTrigger, volumeGroup)
  footer.append(footerMeta, playButton, tools)
  root.append(audio, header, seek, footer)

  let previousVolume = 1
  let durationRecoveryTimer: ReturnType<typeof setTimeout> | null = null
  let durationRecoveryPending = false
  let speedMenuOpen = false
  const updateDuration = () => {
    const duration = Number.isFinite(audio.duration) ? audio.duration : 0
    const currentTime = duration > 0 && Number.isFinite(audio.currentTime) ? Math.min(audio.currentTime, duration) : 0
    seek.max = String(duration)
    seek.value = String(currentTime)
    time.textContent = `${formatAudioTime(currentTime)} / ${formatAudioTime(duration)}`
    setRangeProgress(seek, duration > 0 ? currentTime / duration : 0)
  }
  const finishDurationRecovery = () => {
    if (!durationRecoveryPending || !Number.isFinite(audio.duration) || audio.duration <= 0) return
    durationRecoveryPending = false
    if (durationRecoveryTimer) clearTimeout(durationRecoveryTimer)
    durationRecoveryTimer = null
    audio.removeEventListener('durationchange', finishDurationRecovery)
    audio.removeEventListener('timeupdate', finishDurationRecovery)
    audio.currentTime = 0
    updateDuration()
  }
  const recoverUnboundedDuration = () => {
    if (durationRecoveryPending || Number.isFinite(audio.duration)) return
    durationRecoveryPending = true
    audio.addEventListener('durationchange', finishDurationRecovery)
    audio.addEventListener('timeupdate', finishDurationRecovery)
    try { audio.currentTime = Number.MAX_SAFE_INTEGER } catch {}
    durationRecoveryTimer = setTimeout(() => {
      if (!durationRecoveryPending) return
      durationRecoveryPending = false
      audio.removeEventListener('durationchange', finishDurationRecovery)
      audio.removeEventListener('timeupdate', finishDurationRecovery)
      try { audio.currentTime = 0 } catch {}
      updateDuration()
    }, 2000)
  }
  const onLoadedMetadata = () => {
    recoverUnboundedDuration()
    updateDuration()
  }
  const updatePlaying = () => {
    const playing = !audio.paused && !audio.ended
    root.classList.toggle('is-playing', playing)
    if (playing) {
      root.classList.remove('has-error')
      statusElement.textContent = ''
    }
    playButton.setAttribute('aria-label', playing ? '暂停' : '播放')
    playButton.dataset.tooltip = playing ? '暂停' : '播放'
    setProjectIcon(playIcon, playing ? 'pause' : 'play')
  }
  const updateVolume = () => {
    const effectiveVolume = audio.muted ? 0 : audio.volume
    volume.value = String(effectiveVolume)
    setRangeProgress(volume, effectiveVolume)
    root.classList.toggle('is-muted', effectiveVolume === 0)
    muteButton.setAttribute('aria-label', effectiveVolume === 0 ? '恢复音量' : '静音')
    muteButton.dataset.tooltip = effectiveVolume === 0 ? '恢复音量' : '静音'
    setProjectIcon(volumeIcon, effectiveVolume === 0 ? 'muted' : 'volume')
  }
  const updateSpeedState = () => {
    const rate = Number(audio.playbackRate) || 1
    speedValue.textContent = `${rate}x`
    speedTrigger.dataset.tooltip = `播放速度：${rate}x`
    speedOptions.forEach((option, optionRate) => {
      const selected = optionRate === rate
      option.classList.toggle('is-selected', selected)
      option.setAttribute('aria-selected', selected ? 'true' : 'false')
    })
  }
  const playerUsesDarkTheme = () => document.documentElement.classList.contains('dark')
    || !!root.closest('.theme-dark, .vditor--dark, .is-dark')
  const positionSpeedMenu = () => {
    if (!speedMenuOpen) return
    positionFloatingMenu(speedTrigger, speedMenu, speedMenuStyle, 50, 'above-right')
    Object.assign(speedMenu.style, speedMenuStyle.value)
  }
  const setSpeedMenuOpen = (open: boolean, focusSelected = false) => {
    if (speedMenuOpen === open) return
    speedMenuOpen = open
    speedTrigger.setAttribute('aria-expanded', open ? 'true' : 'false')
    speedTrigger.classList.toggle('is-open', open)
    if (!open) {
      speedMenu.remove()
      return
    }
    speedMenu.classList.toggle('is-dark', playerUsesDarkTheme())
    document.body.appendChild(speedMenu)
    scheduleFloatingMenuPosition(positionSpeedMenu)
    if (focusSelected) {
      window.requestAnimationFrame(() => (speedOptions.get(audio.playbackRate) || speedOptions.get(1))?.focus())
    }
  }
  const toggleSpeedMenu = () => setSpeedMenuOpen(!speedMenuOpen)
  const onSpeedTriggerKeydown = (event: KeyboardEvent) => {
    if (event.key !== 'ArrowDown') return
    event.preventDefault()
    setSpeedMenuOpen(true, true)
  }
  const onSpeedMenuClick = (event: MouseEvent) => {
    const option = event.target instanceof Element
      ? event.target.closest<HTMLButtonElement>('.site-attachment-audio__speed-option[data-rate]')
      : null
    if (!option || !speedMenu.contains(option)) return
    const rate = Number(option.dataset.rate)
    if (!AUDIO_PLAYBACK_RATES.includes(rate as typeof AUDIO_PLAYBACK_RATES[number])) return
    audio.playbackRate = rate
    updateSpeedState()
    setSpeedMenuOpen(false)
    speedTrigger.focus()
  }
  const onSpeedMenuKeydown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      setSpeedMenuOpen(false)
      speedTrigger.focus()
      return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const options = Array.from(speedOptions.values())
    const currentIndex = options.indexOf(document.activeElement as HTMLButtonElement)
    const nextIndex = event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? options.length - 1
        : event.key === 'ArrowUp'
          ? (currentIndex <= 0 ? options.length - 1 : currentIndex - 1)
          : (currentIndex + 1) % options.length
    options[nextIndex]?.focus()
  }
  const onDocumentMouseDown = (event: MouseEvent) => {
    const target = event.target as Node | null
    if (!target || speedTrigger.contains(target) || speedMenu.contains(target)) return
    setSpeedMenuOpen(false)
  }
  const onSpeedMenuViewportChange = () => {
    if (speedMenuOpen) positionSpeedMenu()
  }
  const togglePlayback = async () => {
    try {
      if (audio.paused || audio.ended) await audio.play()
      else audio.pause()
    } catch {
      root.classList.add('has-error')
      statusElement.textContent = '无法播放'
    }
  }
  const onSeek = () => {
    const nextTime = Number(seek.value)
    if (Number.isFinite(nextTime)) audio.currentTime = nextTime
    updateDuration()
  }
  const onVolume = () => {
    const nextVolume = Math.max(0, Math.min(1, Number(volume.value)))
    audio.muted = false
    audio.volume = nextVolume
    if (nextVolume > 0) previousVolume = nextVolume
    updateVolume()
  }
  const toggleMute = () => {
    if (audio.muted || audio.volume === 0) {
      audio.muted = false
      audio.volume = previousVolume || 1
    } else {
      previousVolume = audio.volume
      audio.muted = true
    }
    updateVolume()
  }
  const onError = () => {
    root.classList.add('has-error')
    statusElement.textContent = '无法播放'
  }

  playButton.addEventListener('click', togglePlayback)
  seek.addEventListener('input', onSeek)
  speedTrigger.addEventListener('click', toggleSpeedMenu)
  speedTrigger.addEventListener('keydown', onSpeedTriggerKeydown)
  speedMenu.addEventListener('click', onSpeedMenuClick)
  speedMenu.addEventListener('keydown', onSpeedMenuKeydown)
  volume.addEventListener('input', onVolume)
  muteButton.addEventListener('click', toggleMute)
  audio.addEventListener('loadedmetadata', onLoadedMetadata)
  audio.addEventListener('durationchange', updateDuration)
  audio.addEventListener('timeupdate', updateDuration)
  audio.addEventListener('play', updatePlaying)
  audio.addEventListener('pause', updatePlaying)
  audio.addEventListener('ended', updatePlaying)
  audio.addEventListener('volumechange', updateVolume)
  audio.addEventListener('ratechange', updateSpeedState)
  audio.addEventListener('error', onError)
  document.addEventListener('mousedown', onDocumentMouseDown, true)
  window.addEventListener('resize', onSpeedMenuViewportChange, { passive: true })
  window.addEventListener('scroll', onSpeedMenuViewportChange, { passive: true, capture: true })
  window.visualViewport?.addEventListener('resize', onSpeedMenuViewportChange)
  window.visualViewport?.addEventListener('scroll', onSpeedMenuViewportChange)
  updateSpeedState()

  if (initialSize === null) {
    void probeAudioFileSize(src).then((size) => {
      if (root.dataset.noiseAudioMounted !== 'true') return
      sizeElement.textContent = formatAudioFileSize(size)
    })
  }

  const cleanup = () => {
    audio.pause()
    if (durationRecoveryTimer) clearTimeout(durationRecoveryTimer)
    durationRecoveryTimer = null
    durationRecoveryPending = false
    audio.removeEventListener('durationchange', finishDurationRecovery)
    audio.removeEventListener('timeupdate', finishDurationRecovery)
    playButton.removeEventListener('click', togglePlayback)
    seek.removeEventListener('input', onSeek)
    speedTrigger.removeEventListener('click', toggleSpeedMenu)
    speedTrigger.removeEventListener('keydown', onSpeedTriggerKeydown)
    speedMenu.removeEventListener('click', onSpeedMenuClick)
    speedMenu.removeEventListener('keydown', onSpeedMenuKeydown)
    volume.removeEventListener('input', onVolume)
    muteButton.removeEventListener('click', toggleMute)
    audio.removeEventListener('loadedmetadata', onLoadedMetadata)
    audio.removeEventListener('durationchange', updateDuration)
    audio.removeEventListener('timeupdate', updateDuration)
    audio.removeEventListener('play', updatePlaying)
    audio.removeEventListener('pause', updatePlaying)
    audio.removeEventListener('ended', updatePlaying)
    audio.removeEventListener('volumechange', updateVolume)
    audio.removeEventListener('ratechange', updateSpeedState)
    audio.removeEventListener('error', onError)
    document.removeEventListener('mousedown', onDocumentMouseDown, true)
    window.removeEventListener('resize', onSpeedMenuViewportChange)
    window.removeEventListener('scroll', onSpeedMenuViewportChange, true)
    window.visualViewport?.removeEventListener('resize', onSpeedMenuViewportChange)
    window.visualViewport?.removeEventListener('scroll', onSpeedMenuViewportChange)
    setSpeedMenuOpen(false)
    root.dataset.noiseAudioMounted = 'false'
    playerCleanup.delete(root)
  }
  playerCleanup.set(root, cleanup)
}

const nodesMatching = <T extends Element>(root: ParentNode, selector: string) => {
  const nodes = Array.from(root.querySelectorAll<T>(selector))
  if (root instanceof Element && root.matches(selector)) nodes.unshift(root as T)
  return nodes
}

type ActiveAttachmentAudioPopover = {
  anchor: HTMLElement
  popover: HTMLElement
  player: HTMLElement
  positionFrame: number | null
  cleanup: PlayerCleanup[]
}

let activeAttachmentAudioPopover: ActiveAttachmentAudioPopover | null = null

const attachmentAudioPopoverUsesDarkTheme = (anchor: HTMLElement) => (
  document.documentElement.classList.contains('dark')
  || !!anchor.closest('.theme-dark, .vditor--dark, .is-dark, .editor-table-expand-overlay.is-dark, .rendered-table-expand-overlay.is-dark')
)

const positionAttachmentAudioPopover = (state: ActiveAttachmentAudioPopover) => {
  if (activeAttachmentAudioPopover !== state) return
  if (!state.anchor.isConnected) {
    closeAttachmentAudioPopover()
    return
  }

  const zoom = Number.parseFloat(window.getComputedStyle(document.body).zoom || '1')
  const scale = Number.isFinite(zoom) && zoom > 0 ? zoom : 1
  const viewport = window.visualViewport
  const viewportOffsetLeft = viewport?.offsetLeft || 0
  const viewportOffsetTop = viewport?.offsetTop || 0
  const viewportLeft = viewportOffsetLeft / scale
  const viewportTop = viewportOffsetTop / scale
  const viewportWidth = (viewport?.width || window.innerWidth) / scale
  const viewportHeight = (viewport?.height || window.innerHeight) / scale
  const anchorRect = state.anchor.getBoundingClientRect()
  const anchorCenter = (anchorRect.left + anchorRect.width / 2 + viewportOffsetLeft) / scale
  const anchorTop = (anchorRect.top + viewportOffsetTop) / scale
  const anchorBottom = (anchorRect.bottom + viewportOffsetTop) / scale
  const popoverWidth = state.popover.offsetWidth || Math.min(720, Math.max(280, viewportWidth - 24))
  const popoverHeight = state.popover.offsetHeight || 112
  const edge = 12
  const gap = 8
  const minLeft = viewportLeft + edge
  const maxLeft = Math.max(minLeft, viewportLeft + viewportWidth - popoverWidth - edge)
  const minTop = viewportTop + edge
  const maxTop = Math.max(minTop, viewportTop + viewportHeight - popoverHeight - edge)
  const belowTop = anchorBottom + gap
  const aboveTop = anchorTop - popoverHeight - gap
  const fitsBelow = belowTop + popoverHeight <= viewportTop + viewportHeight - edge
  const fitsAbove = aboveTop >= minTop
  const placement = fitsBelow ? 'below' : fitsAbove ? 'above' : 'viewport'
  const top = fitsBelow ? belowTop : fitsAbove ? aboveTop : clamp(belowTop, minTop, maxTop)

  state.popover.style.left = `${clamp(anchorCenter - popoverWidth / 2, minLeft, maxLeft)}px`
  state.popover.style.top = `${top}px`
  state.popover.dataset.placement = placement
}

const scheduleAttachmentAudioPopoverPosition = (state: ActiveAttachmentAudioPopover) => {
  if (activeAttachmentAudioPopover !== state || state.positionFrame !== null) return
  state.positionFrame = window.requestAnimationFrame(() => {
    state.positionFrame = null
    positionAttachmentAudioPopover(state)
  })
}

export const closeAttachmentAudioPopover = () => {
  const state = activeAttachmentAudioPopover
  if (!state) return
  activeAttachmentAudioPopover = null
  if (state.positionFrame !== null) window.cancelAnimationFrame(state.positionFrame)
  state.positionFrame = null
  state.anchor.setAttribute('aria-expanded', 'false')
  state.cleanup.splice(0).forEach((cleanup) => cleanup())
  playerCleanup.get(state.player)?.()
  state.popover.remove()
}

export const toggleAttachmentAudioPopover = (
  anchor: HTMLElement,
  { src, name = '', size = null }: AttachmentAudioPlaceholderOptions,
) => {
  if (typeof document === 'undefined' || !anchor?.isConnected) return
  const normalizedSrc = String(src || '').trim()
  if (!normalizedSrc) return
  if (activeAttachmentAudioPopover?.anchor === anchor
    && activeAttachmentAudioPopover.player.dataset.audioSrc === normalizedSrc) {
    closeAttachmentAudioPopover()
    return
  }

  closeAttachmentAudioPopover()
  const popover = createElement('div', 'site-table-audio-popover')
  popover.classList.toggle('is-dark', attachmentAudioPopoverUsesDarkTheme(anchor))
  popover.setAttribute('role', 'dialog')
  popover.setAttribute('aria-label', `音频预览：${audioFileName(normalizedSrc, name)}`)
  popover.innerHTML = buildAttachmentAudioPlaceholderHtml({ src: normalizedSrc, name, size })
  const player = popover.querySelector<HTMLElement>('[data-site-audio-player]')
  if (!player) return

  document.body.appendChild(popover)
  mountAudioPlayer(player)
  anchor.setAttribute('aria-haspopup', 'dialog')
  anchor.setAttribute('aria-expanded', 'true')

  const state: ActiveAttachmentAudioPopover = {
    anchor,
    popover,
    player,
    positionFrame: null,
    cleanup: [],
  }
  activeAttachmentAudioPopover = state

  const onDocumentMouseDown = (event: MouseEvent) => {
    const target = event.target as Node | null
    if (!target || state.anchor.contains(target) || state.popover.contains(target)) return
    if (target instanceof Element && target.closest(
      '.editor-table-expand-row-resize-handle, .editor-table-expand-column-resize-handle, .rendered-table-expand-row-resize-handle, .rendered-table-expand-column-resize-handle',
    )) return
    closeAttachmentAudioPopover()
  }
  const onDocumentKeydown = (event: KeyboardEvent) => {
    if (event.key === 'Escape' && activeAttachmentAudioPopover === state) {
      event.preventDefault()
      closeAttachmentAudioPopover()
      state.anchor.focus()
    }
  }
  const onViewportChange = () => scheduleAttachmentAudioPopoverPosition(state)
  document.addEventListener('mousedown', onDocumentMouseDown, true)
  document.addEventListener('keydown', onDocumentKeydown, true)
  window.addEventListener('resize', onViewportChange, { passive: true })
  window.addEventListener('scroll', onViewportChange, { passive: true, capture: true })
  window.visualViewport?.addEventListener('resize', onViewportChange)
  window.visualViewport?.addEventListener('scroll', onViewportChange)
  state.cleanup.push(
    () => document.removeEventListener('mousedown', onDocumentMouseDown, true),
    () => document.removeEventListener('keydown', onDocumentKeydown, true),
    () => window.removeEventListener('resize', onViewportChange),
    () => window.removeEventListener('scroll', onViewportChange, true),
    () => window.visualViewport?.removeEventListener('resize', onViewportChange),
    () => window.visualViewport?.removeEventListener('scroll', onViewportChange),
  )

  if (typeof ResizeObserver !== 'undefined') {
    const resizeObserver = new ResizeObserver(onViewportChange)
    resizeObserver.observe(anchor)
    const cell = anchor.closest<HTMLElement>('td,th')
    if (cell && cell !== anchor) resizeObserver.observe(cell)
    resizeObserver.observe(popover)
    state.cleanup.push(() => resizeObserver.disconnect())
  }
  if (typeof MutationObserver !== 'undefined') {
    const mutationObserver = new MutationObserver(() => {
      if (!anchor.isConnected) closeAttachmentAudioPopover()
    })
    mutationObserver.observe(document.body, { childList: true, subtree: true })
    state.cleanup.push(() => mutationObserver.disconnect())
  }

  positionAttachmentAudioPopover(state)
  scheduleFloatingMenuPosition(() => positionAttachmentAudioPopover(state))
}

const mountTableAudioTrigger = (trigger: HTMLButtonElement) => {
  if (trigger.dataset.noiseTableAudioMounted === 'true' && tableTriggerCleanup.has(trigger)) return
  const src = String(trigger.dataset.audioSrc || trigger.dataset.noiseAttachmentUrl || '').trim()
  if (!src) return
  const name = audioFileName(src, trigger.dataset.audioName || '')
  const knownSize = Number(trigger.dataset.audioSize)
  const size = Number.isFinite(knownSize) && knownSize >= 0 ? knownSize : null
  const sizeElement = trigger.querySelector<HTMLElement>('.site-table-audio-trigger__size')

  trigger.dataset.noiseTableAudioMounted = 'true'
  trigger.setAttribute('aria-haspopup', 'dialog')
  trigger.setAttribute('aria-expanded', 'false')
  trigger.setAttribute('aria-label', `预览音频附件：${name}`)
  const onMouseDown = (event: MouseEvent) => event.stopPropagation()
  const onClick = (event: MouseEvent) => {
    event.preventDefault()
    event.stopPropagation()
    toggleAttachmentAudioPopover(trigger, { src, name, size })
  }
  trigger.addEventListener('mousedown', onMouseDown)
  trigger.addEventListener('click', onClick)

  if (size === null && sizeElement) {
    void probeAudioFileSize(src).then((resolvedSize) => {
      if (!trigger.isConnected || trigger.dataset.noiseTableAudioMounted !== 'true') return
      sizeElement.textContent = formatAudioFileSize(resolvedSize)
    })
  }

  const cleanup = () => {
    trigger.removeEventListener('mousedown', onMouseDown)
    trigger.removeEventListener('click', onClick)
    trigger.dataset.noiseTableAudioMounted = 'false'
    tableTriggerCleanup.delete(trigger)
  }
  tableTriggerCleanup.set(trigger, cleanup)
}

const replaceTableAudioPlaceholder = (player: HTMLElement) => {
  const src = String(player.dataset.audioSrc || player.dataset.noiseAttachmentUrl || '').trim()
  if (!src) return
  const name = audioFileName(src, player.dataset.audioName || '')
  const knownSize = Number(player.dataset.audioSize)
  const size = Number.isFinite(knownSize) && knownSize >= 0 ? knownSize : null
  const trigger = createElement('button', 'site-table-audio-trigger site-attachment-file site-attachment-file--preview')
  trigger.type = 'button'
  trigger.dataset.audioSrc = src
  trigger.dataset.audioName = name
  trigger.dataset.noiseAttachmentKind = 'audio'
  trigger.dataset.noiseAttachmentUrl = src
  if (size !== null) trigger.dataset.audioSize = String(size)

  const icon = createElement('span', 'site-attachment-file__icon site-table-audio-trigger__icon')
  const iconSvg = createIcon('volume')
  iconSvg.classList.add('site-table-audio-trigger__svg')
  icon.appendChild(iconSvg)
  const body = createElement('span', 'site-attachment-file__body site-table-audio-trigger__body')
  const nameElement = createElement('span', 'site-attachment-file__name site-table-audio-trigger__name')
  nameElement.textContent = name
  const meta = createElement('span', 'site-attachment-file__meta site-table-audio-trigger__meta')
  const format = createElement('span', 'site-table-audio-trigger__format')
  format.textContent = audioFormatLabel(src, name)
  const separator = createElement('span', 'site-table-audio-trigger__separator')
  separator.textContent = ' · '
  separator.setAttribute('aria-hidden', 'true')
  const sizeElement = createElement('span', 'site-table-audio-trigger__size')
  sizeElement.textContent = size === null ? '大小读取中' : formatAudioFileSize(size)
  meta.append(format, separator, sizeElement)
  body.append(nameElement, meta)
  const action = createElement('span', 'site-attachment-file__action site-attachment-file__action--preview')
  action.setAttribute('aria-hidden', 'true')
  trigger.append(icon, body, action)
  player.replaceWith(trigger)
  mountTableAudioTrigger(trigger)
}

export const enhanceAttachmentAudioPlayers = (root: ParentNode) => {
  if (typeof document === 'undefined') return
  nodesMatching<HTMLAudioElement>(root, 'audio').forEach((audio) => {
    if (audio.closest('[data-site-audio-player]')) return
    if (audio.closest('.aplayer, meting-js, .netease-mini-player')) return
    const placeholder = createElement('div', 'site-attachment-audio')
    const src = audio.currentSrc || audio.getAttribute('src') || audio.querySelector<HTMLSourceElement>('source[src]')?.getAttribute('src') || ''
    if (!src) return
    placeholder.dataset.noiseAudioPlayer = ''
    placeholder.dataset.audioSrc = src
    placeholder.dataset.audioName = audio.dataset.audioName || audio.getAttribute('title') || audio.getAttribute('aria-label') || audioFileName(src)
    placeholder.dataset.noiseAttachmentKind = 'audio'
    placeholder.dataset.noiseAttachmentUrl = src
    audio.replaceWith(placeholder)
  })
  nodesMatching<HTMLElement>(root, '[data-site-audio-player]').forEach((player) => {
    if (player.closest('td,th')) replaceTableAudioPlaceholder(player)
    else mountAudioPlayer(player)
  })
  nodesMatching<HTMLButtonElement>(root, '.site-table-audio-trigger').forEach(mountTableAudioTrigger)
}

export const destroyAttachmentAudioPlayers = (root: ParentNode) => {
  nodesMatching<HTMLElement>(root, '[data-site-audio-player]').forEach((player) => playerCleanup.get(player)?.())
  nodesMatching<HTMLElement>(root, '.site-table-audio-trigger').forEach((trigger) => tableTriggerCleanup.get(trigger)?.())
  if (activeAttachmentAudioPopover
    && (root === activeAttachmentAudioPopover.anchor
      || (root instanceof Node && root.contains(activeAttachmentAudioPopover.anchor)))) {
    closeAttachmentAudioPopover()
  }
}
