import { onActivated, onDeactivated, onMounted, onUnmounted, ref } from 'vue'
import { useRuntimeConfig } from '#imports'

export type PublicMusicSource = 'netease' | 'local'

export interface PublicMusicFrontendSettings {
  musicSource: PublicMusicSource
  musicEnabled: boolean
  musicPosition: string
  musicTheme: string
  musicLyric: boolean
  musicAutoplay: boolean
  musicDefaultMinimized: boolean
  musicEmbed: boolean
  musicHideOnMobile: boolean
  musicPlaylistId?: string
  musicSongId?: string
  musicCssCdnURL?: string
  musicJsCdnURL?: string
}

export interface PublicMusicTrack {
  trackID: string
  title: string
  artist: string
  album: string
  durationMS: number
  format: string
  mimeType: string
  coverURL: string
  lyricsAvailable: boolean
  lyricsURL: string
  streamURL: string
  fallbackStreamURL: string
  mediaVersion: string
}

export interface PublicMusic {
  source: PublicMusicSource
  revision: string
  frontendSettings: PublicMusicFrontendSettings
  tracks: PublicMusicTrack[]
}

export type PublicMusicState = PublicMusic

// Project the network response once; unrelated site settings never reach the page.
function parsePublicMusic(value: unknown): PublicMusic {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Invalid music response')
  const envelope = value as Record<string, unknown>
  if (envelope.code !== 1 || !envelope.data || typeof envelope.data !== 'object' || Array.isArray(envelope.data)) {
    throw new Error('Invalid music response')
  }
  const data = envelope.data as Record<string, unknown>
  if ((data.source !== 'local' && data.source !== 'netease') || typeof data.revision !== 'string' || !data.revision.trim()
    || !data.frontendSettings || typeof data.frontendSettings !== 'object' || Array.isArray(data.frontendSettings)
    || !Array.isArray(data.tracks)) {
    throw new Error('Invalid music response')
  }
  const settings = data.frontendSettings as Record<string, unknown>
  if (settings.musicSource !== undefined && settings.musicSource !== data.source) throw new Error('Invalid music source')
  for (const key of ['musicEnabled', 'musicLyric', 'musicAutoplay', 'musicDefaultMinimized', 'musicEmbed', 'musicHideOnMobile']) {
    if (settings[key] !== true && settings[key] !== false && settings[key] !== 'true' && settings[key] !== 'false') {
      throw new Error('Invalid music setting')
    }
  }
  if (typeof settings.musicPosition !== 'string' || typeof settings.musicTheme !== 'string') {
    throw new Error('Invalid music setting')
  }
  const frontendSettings: PublicMusicFrontendSettings = {
    musicSource: data.source,
    musicEnabled: settings.musicEnabled === true || settings.musicEnabled === 'true',
    musicPosition: settings.musicPosition,
    musicTheme: settings.musicTheme,
    musicLyric: settings.musicLyric === true || settings.musicLyric === 'true',
    musicAutoplay: settings.musicAutoplay === true || settings.musicAutoplay === 'true',
    musicDefaultMinimized: settings.musicDefaultMinimized === true || settings.musicDefaultMinimized === 'true',
    musicEmbed: settings.musicEmbed === true || settings.musicEmbed === 'true',
    musicHideOnMobile: settings.musicHideOnMobile === true || settings.musicHideOnMobile === 'true'
  }
  if (data.source === 'netease') {
    for (const key of ['musicPlaylistId', 'musicSongId', 'musicCssCdnURL', 'musicJsCdnURL'] as const) {
      if (settings[key] !== undefined && typeof settings[key] !== 'string') throw new Error('Invalid music setting')
      if (typeof settings[key] === 'string') frontendSettings[key] = settings[key]
    }
  }
  const trackIDs = new Set<string>()
  const tracks = data.tracks.map((value: unknown): PublicMusicTrack => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Invalid music track')
    const track = value as Record<string, unknown>
    if (typeof track.trackID !== 'string' || !track.trackID.trim() || trackIDs.has(track.trackID)
      || typeof track.title !== 'string' || typeof track.artist !== 'string' || typeof track.album !== 'string'
      || typeof track.durationMS !== 'number' || !Number.isFinite(track.durationMS) || track.durationMS < 0
      || typeof track.format !== 'string' || typeof track.mimeType !== 'string' || typeof track.coverURL !== 'string'
      || typeof track.lyricsAvailable !== 'boolean' || typeof track.lyricsURL !== 'string'
      || typeof track.streamURL !== 'string' || !track.streamURL.trim() || typeof track.fallbackStreamURL !== 'string'
      || typeof track.mediaVersion !== 'string') {
      throw new Error('Invalid music track')
    }
    trackIDs.add(track.trackID)
    return {
      trackID: track.trackID, title: track.title, artist: track.artist, album: track.album,
      durationMS: track.durationMS, format: track.format, mimeType: track.mimeType,
      coverURL: track.coverURL, lyricsAvailable: track.lyricsAvailable, lyricsURL: track.lyricsURL,
      streamURL: track.streamURL, fallbackStreamURL: track.fallbackStreamURL, mediaVersion: track.mediaVersion
    }
  })
  return {
    source: data.source, revision: data.revision, frontendSettings,
    tracks: data.source === 'local' && frontendSettings.musicEnabled ? tracks : []
  }
}

export function usePublicMusic() {
  // Capture Nuxt configuration during setup, before any event/timer callbacks.
  const baseApi = String(useRuntimeConfig().public.baseApi || '/api').replace(/\/+$/, '')
  const state = ref<PublicMusic | null>(null)
  const loading = ref(false)
  const error = ref('')
  let active = false
  let disposed = false
  let generation = 0
  let controller: AbortController | null = null
  let pollTimer: number | null = null

  async function refresh(): Promise<void> {
    if (!active || disposed || document.visibilityState === 'hidden') return
    const requestGeneration = ++generation
    controller?.abort()
    const requestController = new AbortController()
    controller = requestController
    loading.value = true
    try {
      const response = await $fetch<unknown>(`${baseApi}/music/public`, {
        method: 'GET', signal: requestController.signal, timeout: 8000, retry: 0, cache: 'no-store'
      })
      if (!active || disposed || requestGeneration !== generation || requestController.signal.aborted) return
      state.value = parsePublicMusic(response)
      error.value = ''
    } catch {
      if (!active || disposed || requestGeneration !== generation) return
      // A failed refresh must stop playback of a previously authorized playlist.
      state.value = null
      error.value = '音乐暂时不可用，请稍后重试'
    } finally {
      if (requestGeneration === generation) {
        controller = null
        loading.value = false
      }
    }
  }

  function pause() {
    if (pollTimer !== null) window.clearInterval(pollTimer)
    pollTimer = null
    ++generation
    controller?.abort()
    controller = null
    loading.value = false
  }

  function resume() {
    if (!active || disposed || document.visibilityState === 'hidden') return
    if (pollTimer === null) {
      pollTimer = window.setInterval(() => {
        if (document.visibilityState === 'hidden') pause()
        else if (!loading.value) void refresh()
      }, 30000)
    }
    void refresh()
  }

  function onVisibilityChange() {
    if (document.visibilityState === 'hidden') pause()
    else resume()
  }

  function onFocus() {
    resume()
  }

  function onConfigUpdated() {
    // Invalidate immediately, including when hidden, until the source is reread.
    state.value = null
    if (document.visibilityState === 'hidden') pause()
    else resume()
  }

  function start() {
    if (active || disposed || typeof window === 'undefined' || typeof document === 'undefined') return
    active = true
    document.addEventListener('visibilitychange', onVisibilityChange)
    window.addEventListener('focus', onFocus)
    window.addEventListener('frontend-config-updated', onConfigUpdated)
    resume()
  }

  function stop() {
    active = false
    pause()
    if (typeof window !== 'undefined' && typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', onVisibilityChange)
      window.removeEventListener('focus', onFocus)
      window.removeEventListener('frontend-config-updated', onConfigUpdated)
    }
    state.value = null
  }

  onMounted(start)
  onActivated(start)
  onDeactivated(stop)
  onUnmounted(() => {
    disposed = true
    stop()
  })

  return { state, loading, error, refresh }
}
