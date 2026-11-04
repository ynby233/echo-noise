export type MusicPickerTrack = {
  trackID: string
  title: string
  artist: string
  album: string
  available: boolean
}

export type MusicPickerPage = {
  items: MusicPickerTrack[]
  total: number
  page: number
  pageSize: number
}

export const parseMusicPickerPage = (input: unknown): MusicPickerPage => {
  if (!input || typeof input !== 'object') throw new Error('音乐库响应无效')
  const data = input as Record<string, unknown>
  if (!Array.isArray(data.items) || !Number.isSafeInteger(data.total) || Number(data.total) < 0
    || !Number.isSafeInteger(data.page) || Number(data.page) < 1
    || !Number.isSafeInteger(data.pageSize) || Number(data.pageSize) < 1 || Number(data.pageSize) > 100) {
    throw new Error('音乐库响应无效')
  }
  const items = data.items.map((value: unknown): MusicPickerTrack => {
    if (!value || typeof value !== 'object') throw new Error('音乐库响应无效')
    const track = value as Record<string, unknown>
    if (typeof track.trackID !== 'string' || !track.trackID || typeof track.title !== 'string'
      || typeof track.artist !== 'string' || typeof track.album !== 'string' || typeof track.available !== 'boolean') {
      throw new Error('音乐库响应无效')
    }
    return { trackID: track.trackID, title: track.title, artist: track.artist, album: track.album, available: track.available }
  })
  return { items, total: Number(data.total), page: Number(data.page), pageSize: Number(data.pageSize) }
}
