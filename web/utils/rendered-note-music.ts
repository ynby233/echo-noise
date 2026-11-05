import { parseNoteMusicTrackID, type NoteMusicPlayer } from './note-music'

export function enhanceNoteMusicLinks(
  root: HTMLElement | null,
  messageID: number,
  play: NoteMusicPlayer,
  reportError: (message: string) => void,
  draftPreview = false,
): void {
  if (!root) return
  root.querySelectorAll<HTMLAnchorElement | HTMLButtonElement>('a[href], button.note-music-marker[data-track-id]').forEach((anchor) => {
    if (anchor.closest('pre, code')) return
    const trackID = parseNoteMusicTrackID(anchor.getAttribute('href') || `/music/notes/${anchor.dataset.trackId || ''}`)
    if (!trackID) return
    const button = anchor.tagName === 'BUTTON' ? anchor as HTMLButtonElement : document.createElement('button')
    button.type = 'button'
    const label = anchor.dataset.musicLabel || anchor.textContent || '本地音乐'
    button.className = 'note-music-marker'
    button.dataset.trackId = trackID
    button.dataset.musicLabel = label
    button.replaceChildren()
    const icon = document.createElement('span')
    icon.className = 'note-music-marker-icon'
    icon.textContent = '♫'
    icon.setAttribute('aria-hidden', 'true')
    const content = document.createElement('span')
    content.className = 'note-music-marker-content'
    const title = document.createElement('span')
    title.className = 'note-music-marker-title'
    title.textContent = label.replace(/^[♫♪♬]\s*/, '') || '本地音乐'
    const hint = document.createElement('span')
    hint.className = 'note-music-marker-hint'
    const idleHint = draftPreview ? '点击试听' : messageID ? '点击临时播放' : '发布后可播放'
    hint.textContent = idleHint
    content.append(title, hint)
    button.append(icon, content)
    button.title = messageID || draftPreview ? '使用音乐播放器临时播放，不改变原歌单' : '发布后可播放'
    button.setAttribute('aria-label', `${title.textContent}，${hint.textContent}`)
    button.disabled = !messageID && !draftPreview
    button.onclick = async (event) => {
      event.preventDefault()
      event.stopPropagation()
      if (button.disabled) return
      button.disabled = true
      button.setAttribute('aria-busy', 'true')
      hint.textContent = '正在加载…'
      reportError('')
      try {
        await play(draftPreview ? { trackID, messageID: 0, draft: true } : { trackID, messageID })
      } catch (error) {
        hint.textContent = '播放失败，点击重试'
        if (button.isConnected) reportError(error instanceof Error ? error.message : '音乐暂时无法播放，请稍后重试')
      } finally {
        button.disabled = false
        if (hint.textContent === '正在加载…') hint.textContent = idleHint
        button.removeAttribute('aria-busy')
      }
    }
    if (anchor !== button) anchor.replaceWith(button)
  })
}
