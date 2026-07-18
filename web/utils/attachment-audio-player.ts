export const AUDIO_PLAYBACK_RATES = [0.5, 1, 1.25, 1.5, 1.75, 2, 3] as const

type AttachmentAudioPlaceholderOptions = {
  src: string
  name?: string
  size?: number | null
}

type PlayerCleanup = () => void

const playerCleanup = new WeakMap<HTMLElement, PlayerCleanup>()
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
  return `<div class="noise-attachment-audio" data-noise-audio-player data-audio-src="${safeSrc}" data-audio-name="${safeName}"${sizeAttribute} data-noise-attachment-kind="audio" data-noise-attachment-url="${safeSrc}"></div>`
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

const createIcon = (kind: 'play' | 'volume') => {
  const icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
  icon.setAttribute('viewBox', '0 0 24 24')
  icon.setAttribute('aria-hidden', 'true')
  icon.classList.add('noise-attachment-audio__svg')
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path')
  path.setAttribute('fill', 'currentColor')
  path.setAttribute('d', kind === 'play'
    ? 'M8 5.6v12.8c0 .78.86 1.26 1.53.85l10.2-6.4a1 1 0 0 0 0-1.7l-10.2-6.4A1 1 0 0 0 8 5.6Z'
    : 'M4 9v6h4l5 4V5L8 9H4Zm11.5.12v5.76a4 4 0 0 0 0-5.76Zm0-3.46v2.06a6.5 6.5 0 0 1 0 8.56v2.06a8.5 8.5 0 0 0 0-12.68Z')
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

  const audio = createElement('audio', 'noise-attachment-audio__native')
  audio.src = src
  audio.preload = 'metadata'
  audio.dataset.noiseAttachmentKind = 'audio'
  audio.dataset.noiseAttachmentUrl = src

  const header = createElement('div', 'noise-attachment-audio__header')
  const information = createElement('div', 'noise-attachment-audio__information')
  const nameElement = createElement('div', 'noise-attachment-audio__name')
  nameElement.textContent = name
  nameElement.title = name
  const meta = createElement('div', 'noise-attachment-audio__meta')
  const formatElement = createElement('span', 'noise-attachment-audio__format')
  formatElement.textContent = format
  const separator = createElement('span', 'noise-attachment-audio__meta-separator')
  separator.textContent = '·'
  separator.setAttribute('aria-hidden', 'true')
  const sizeElement = createElement('span', 'noise-attachment-audio__size')
  sizeElement.textContent = initialSize === null ? '大小读取中' : formatAudioFileSize(initialSize)
  const statusElement = createElement('span', 'noise-attachment-audio__status')
  meta.append(formatElement, separator, sizeElement, statusElement)
  information.append(nameElement, meta)

  const playButton = createElement('button', 'noise-attachment-audio__play')
  playButton.type = 'button'
  playButton.setAttribute('aria-label', '播放')
  playButton.title = '播放'
  playButton.appendChild(createIcon('play'))
  header.append(information, playButton)

  const seek = createElement('input', 'noise-attachment-audio__range noise-attachment-audio__seek')
  seek.type = 'range'
  seek.min = '0'
  seek.max = '0'
  seek.step = '0.01'
  seek.value = '0'
  seek.setAttribute('aria-label', '音频播放进度')
  setRangeProgress(seek, 0)

  const footer = createElement('div', 'noise-attachment-audio__footer')
  const time = createElement('span', 'noise-attachment-audio__time')
  time.textContent = '0:00 / 0:00'

  const tools = createElement('div', 'noise-attachment-audio__tools')
  const speedLabel = createElement('label', 'noise-attachment-audio__speed')
  const speedText = createElement('span', 'noise-attachment-audio__sr-only')
  speedText.textContent = '播放速度'
  const speed = createElement('select', 'noise-attachment-audio__speed-select')
  speed.setAttribute('aria-label', '播放速度')
  AUDIO_PLAYBACK_RATES.forEach((rate) => {
    const option = createElement('option')
    option.value = String(rate)
    option.textContent = `${rate}x`
    option.selected = rate === 1
    speed.appendChild(option)
  })
  speedLabel.append(speedText, speed)

  const volumeGroup = createElement('div', 'noise-attachment-audio__volume')
  const muteButton = createElement('button', 'noise-attachment-audio__mute')
  muteButton.type = 'button'
  muteButton.setAttribute('aria-label', '静音')
  muteButton.title = '静音'
  muteButton.appendChild(createIcon('volume'))
  const volume = createElement('input', 'noise-attachment-audio__range noise-attachment-audio__volume-range')
  volume.type = 'range'
  volume.min = '0'
  volume.max = '1'
  volume.step = '0.01'
  volume.value = '1'
  volume.setAttribute('aria-label', '音量')
  setRangeProgress(volume, 1)
  volumeGroup.append(muteButton, volume)
  tools.append(speedLabel, volumeGroup)
  footer.append(time, tools)
  root.append(audio, header, seek, footer)

  let previousVolume = 1
  let durationRecoveryTimer: ReturnType<typeof setTimeout> | null = null
  let durationRecoveryPending = false
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
    playButton.title = playing ? '暂停' : '播放'
  }
  const updateVolume = () => {
    const effectiveVolume = audio.muted ? 0 : audio.volume
    volume.value = String(effectiveVolume)
    setRangeProgress(volume, effectiveVolume)
    root.classList.toggle('is-muted', effectiveVolume === 0)
    muteButton.setAttribute('aria-label', effectiveVolume === 0 ? '恢复音量' : '静音')
    muteButton.title = effectiveVolume === 0 ? '恢复音量' : '静音'
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
  const onSpeed = () => {
    const nextRate = Number(speed.value)
    if (AUDIO_PLAYBACK_RATES.includes(nextRate as typeof AUDIO_PLAYBACK_RATES[number])) audio.playbackRate = nextRate
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
  speed.addEventListener('change', onSpeed)
  volume.addEventListener('input', onVolume)
  muteButton.addEventListener('click', toggleMute)
  audio.addEventListener('loadedmetadata', onLoadedMetadata)
  audio.addEventListener('durationchange', updateDuration)
  audio.addEventListener('timeupdate', updateDuration)
  audio.addEventListener('play', updatePlaying)
  audio.addEventListener('pause', updatePlaying)
  audio.addEventListener('ended', updatePlaying)
  audio.addEventListener('volumechange', updateVolume)
  audio.addEventListener('error', onError)

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
    speed.removeEventListener('change', onSpeed)
    volume.removeEventListener('input', onVolume)
    muteButton.removeEventListener('click', toggleMute)
    audio.removeEventListener('loadedmetadata', onLoadedMetadata)
    audio.removeEventListener('durationchange', updateDuration)
    audio.removeEventListener('timeupdate', updateDuration)
    audio.removeEventListener('play', updatePlaying)
    audio.removeEventListener('pause', updatePlaying)
    audio.removeEventListener('ended', updatePlaying)
    audio.removeEventListener('volumechange', updateVolume)
    audio.removeEventListener('error', onError)
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

export const enhanceAttachmentAudioPlayers = (root: ParentNode) => {
  if (typeof document === 'undefined') return
  nodesMatching<HTMLAudioElement>(root, 'audio').forEach((audio) => {
    if (audio.closest('[data-noise-audio-player]')) return
    if (audio.closest('.aplayer, meting-js, .netease-mini-player')) return
    const placeholder = createElement('div', 'noise-attachment-audio')
    const src = audio.currentSrc || audio.getAttribute('src') || audio.querySelector<HTMLSourceElement>('source[src]')?.getAttribute('src') || ''
    if (!src) return
    placeholder.dataset.noiseAudioPlayer = ''
    placeholder.dataset.audioSrc = src
    placeholder.dataset.audioName = audio.dataset.audioName || audio.getAttribute('title') || audio.getAttribute('aria-label') || audioFileName(src)
    placeholder.dataset.noiseAttachmentKind = 'audio'
    placeholder.dataset.noiseAttachmentUrl = src
    audio.replaceWith(placeholder)
  })
  nodesMatching<HTMLElement>(root, '[data-noise-audio-player]').forEach(mountAudioPlayer)
}

export const destroyAttachmentAudioPlayers = (root: ParentNode) => {
  nodesMatching<HTMLElement>(root, '[data-noise-audio-player]').forEach((player) => playerCleanup.get(player)?.())
}
