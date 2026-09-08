// Only Markdown containing a music embed needs these legacy globals. Preserve
// APlayer-before-Meting order and share concurrent loads; failures can be retried.
const assets = new Map<string, Promise<void>>()
const loadAsset = (url: string, stylesheet = false) => {
  if (assets.has(url)) return assets.get(url)!
  const pending = new Promise<void>((resolve, reject) => {
    const element = document.createElement(stylesheet ? 'link' : 'script')
    if (element instanceof HTMLLinkElement) { element.rel = 'stylesheet'; element.href = url }
    else { element.src = url; element.async = true }
    const timer = window.setTimeout(() => finish(new Error('音乐资源加载超时')), 15000)
    const finish = (error?: Error) => {
      window.clearTimeout(timer)
      element.onload = element.onerror = null
      if (error) { element.remove(); assets.delete(url); reject(error) }
      else resolve()
    }
    element.onload = () => finish()
    element.onerror = () => finish(new Error('音乐资源加载失败'))
    document.head.appendChild(element)
  })
  assets.set(url, pending)
  return pending
}

const loadMetingPlayer = async () => {
  await loadAsset('https://cdn.jsdelivr.net/npm/aplayer@1.10.1/dist/APlayer.min.css', true)
  if (!window.APlayer) await loadAsset('https://cdn.jsdelivr.net/npm/aplayer@1.10.1/dist/APlayer.min.js')
  if (!customElements.get('meting-js')) await loadAsset('https://cdn.jsdelivr.net/npm/meting@2.0.1/dist/Meting.min.js')
}

export const enhanceMetingPlayers = async (root: HTMLElement) => {
  if (!root.querySelector('meting-js')) return
  root.querySelector('.music-load-retry')?.remove()
  try { await loadMetingPlayer() }
  catch {
    if (!root.isConnected || root.querySelector('.music-load-retry')) return
    const retry = document.createElement('button')
    retry.type = 'button'
    retry.className = 'music-load-retry nw-action-btn'
    retry.textContent = '音乐加载失败，点击重试'
    retry.onclick = () => { void enhanceMetingPlayers(root) }
    root.appendChild(retry)
  }
}
