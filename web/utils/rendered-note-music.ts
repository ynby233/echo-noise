import { parseNoteMusicTrackID, type NoteMusicPlayer } from './note-music'

export function enhanceNoteMusicLinks(
  root: HTMLElement | null,
  messageID: number,
  play: NoteMusicPlayer,
  reportError: (message: string) => void,
): void {
  if (!root) return
  root.querySelectorAll<HTMLAnchorElement | HTMLButtonElement>('a[href], button.note-music-marker[data-track-id]').forEach((anchor) => {
    if (anchor.closest('pre, code')) return
    const trackID = parseNoteMusicTrackID(anchor.getAttribute('href') || `/music/notes/${anchor.dataset.trackId || ''}`)
    if (!trackID) return
    const button = anchor.tagName === 'BUTTON' ? anchor as HTMLButtonElement : document.createElement('button')
    button.type = 'button'
    button.className = 'note-music-marker nw-action-btn'
    button.dataset.trackId = trackID
    button.textContent = anchor.textContent || '♫ 本地音乐'
    button.title = '临时播放此歌曲'
    button.disabled = !messageID
    button.onclick = async (event) => {
      event.preventDefault()
      event.stopPropagation()
      if (button.disabled) return
      button.disabled = true
      button.setAttribute('aria-busy', 'true')
      reportError('')
      try {
        await play({ trackID, messageID })
      } catch (error) {
        if (button.isConnected) reportError(error instanceof Error ? error.message : '音乐暂时无法播放，请稍后重试')
      } finally {
        button.disabled = false
        button.removeAttribute('aria-busy')
      }
    }
    if (anchor !== button) anchor.replaceWith(button)
  })
}
