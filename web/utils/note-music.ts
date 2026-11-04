export interface NoteMusicSelection {
  trackID: string
  title: string
  artist?: string
}

export interface NoteMusicRequest {
  trackID: string
  messageID: number
}

export const NOTE_MUSIC_PLAYER_KEY = 'note-music-player'
export type NoteMusicPlayer = (request: NoteMusicRequest) => Promise<void>

export function parseNoteMusicTrackID(href: string): string {
  const match = /^\/music\/notes\/([A-Za-z0-9_-]+)$/.exec(String(href || ''))
  return match?.[1] || ''
}

export function createNoteMusicMarkdown(track: NoteMusicSelection): string {
  const trackID = String(track.trackID || '')
  if (!/^[A-Za-z0-9_-]+$/.test(trackID)) throw new Error('无效的音乐标记')
  const clean = (value: string) => String(value || '').replace(/[\\\[\]<>`*\r\n]/g, ' ').replace(/\s+/g, ' ').trim()
  const label = [clean(track.title) || '本地音乐', clean(track.artist || '')].filter(Boolean).join(' — ')
  return `[♫ ${label}](/music/notes/${encodeURIComponent(trackID)})`
}

export function noteMusicPlaybackLocation(request: NoteMusicRequest): string {
  return `/?noteMusic=${encodeURIComponent(request.trackID)}&noteMusicMessage=${request.messageID}`
}
