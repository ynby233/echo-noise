type RepositoryLink = { owner: string; repo: string; href: string }
type RepositoryInfo = { full_name?: string; owner?: { avatar_url?: string } }

export const parseGitHubRepositoryLink = (value: string): RepositoryLink | null => {
  try {
    const url = new URL(value)
    if (url.protocol !== 'https:' || url.hostname.toLowerCase() !== 'github.com' || url.port || url.username || url.password) return null
    const [, owner, repository] = url.pathname.split('/')
    const repo = (repository || '').replace(/\.git$/i, '')
    if (!/^[\w-]+$/.test(owner || '') || !/^[\w.-]+$/.test(repo) || repo === '.' || repo === '..') return null
    return { owner, repo, href: url.href }
  } catch { return null }
}

const pending = new Map<string, Promise<RepositoryInfo | null>>()
const loaded = new Map<string, { time: number; data: RepositoryInfo }>()
const hydrated = new WeakSet<Element>()
const ttl = 6 * 60 * 60 * 1000
const excluded = 'a, code, pre, script, style, textarea, button, .github-card, .site-attachment-tag'

const fetchRepository = (owner: string, repo: string): Promise<RepositoryInfo | null> => {
  const key = `${owner}/${repo}`.toLowerCase()
  const cached = loaded.get(key)
  if (cached && Date.now() - cached.time < ttl) return Promise.resolve(cached.data)
  const current = pending.get(key)
  if (current) return current
  const request = (async () => {
    const controller = new AbortController()
    const timeout = setTimeout(() => controller.abort(), 6000)
    try {
      const response = await fetch(`https://api.github.com/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`, { signal: controller.signal })
      if (!response.ok) return null
      const data = await response.json() as RepositoryInfo
      if (!data || typeof data !== 'object') return null
      loaded.set(key, { time: Date.now(), data })
      return data
    } catch { return null }
    finally { clearTimeout(timeout); pending.delete(key) }
  })()
  pending.set(key, request)
  return request
}

const makeCard = (document: Document, link: RepositoryLink): HTMLElement => {
  // Inline elements remain valid inside paragraphs and table cells; CSS supplies the card layout.
  const card = document.createElement('span')
  card.className = 'github-card'
  card.dataset.owner = link.owner
  card.dataset.repo = link.repo
  const header = document.createElement('span')
  header.className = 'github-card-header'
  const avatar = document.createElement('span')
  avatar.className = 'gh-avatar-slot'
  const fallback = document.createElement('span')
  fallback.className = 'avatar-fallback'
  fallback.textContent = link.owner.charAt(0).toUpperCase()
  fallback.style.display = 'flex'
  avatar.append(fallback)
  const mark = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
  mark.setAttribute('class', 'gh-badge')
  mark.setAttribute('viewBox', '0 0 16 16')
  mark.setAttribute('aria-hidden', 'true')
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path')
  path.setAttribute('fill', 'currentColor')
  path.setAttribute('d', 'M8 0C3.58 0 0 3.58 0 8a8 8 0 005.47 7.59c.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.6 7.6 0 012 0c1.53-1.03 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.28.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8 8 0 0016 8c0-4.42-3.58-8-8-8z')
  mark.append(path)
  avatar.append(mark)
  const title = document.createElement('a')
  title.className = 'github-card-title'
  title.href = link.href
  title.target = '_blank'
  title.rel = 'noopener noreferrer'
  title.textContent = `${link.owner}/${link.repo}`
  header.append(avatar, title)
  card.append(header)
  return card
}

/** Enhance rendered nodes, never Markdown source. Duplicate notes and expanded clones own their nodes. */
export const enhanceGitHubCards = (root: HTMLElement): Promise<void> => {
  for (const anchor of Array.from(root.querySelectorAll<HTMLAnchorElement>('a[href]'))) {
    if (anchor.closest('code, pre, .github-card, .site-attachment-tag') || anchor.querySelector('img, picture, svg')) continue
    const link = parseGitHubRepositoryLink(anchor.getAttribute('href') || '')
    if (link) anchor.replaceWith(makeCard(root.ownerDocument, link))
  }
  // Raw HTML table cells may contain text URLs which the Markdown engine does not linkify.
  const walker = root.ownerDocument.createTreeWalker(root, 4)
  const texts: Text[] = []
  while (walker.nextNode()) {
    const node = walker.currentNode as Text
    if (!node.parentElement?.closest(excluded)) texts.push(node)
  }
  for (const node of texts) {
    const expression = /https:\/\/github\.com\/[^\s<>"'，。！？；、（）【】]+/gi
    const fragment = root.ownerDocument.createDocumentFragment()
    let offset = 0
    for (const match of node.data.matchAll(expression)) {
      const value = match[0].replace(/[.,;:!?)\]]+$/, '')
      const link = parseGitHubRepositoryLink(value)
      if (!link) continue
      fragment.append(node.data.slice(offset, match.index), makeCard(root.ownerDocument, link))
      offset = match.index + value.length
    }
    if (offset) { fragment.append(node.data.slice(offset)); node.replaceWith(fragment) }
  }
  return Promise.all(Array.from(root.querySelectorAll<HTMLElement>('.github-card')).map(async card => {
    if (hydrated.has(card) || card.classList.contains('github-card-loaded')) return
    hydrated.add(card)
    const owner = card.dataset.owner || ''
    const repo = card.dataset.repo || ''
    const data = await fetchRepository(owner, repo)
    if (!data) { card.classList.add('github-card-error'); return }
    const title = card.querySelector<HTMLAnchorElement>('.github-card-title')
    if (title && typeof data.full_name === 'string') title.textContent = data.full_name
    const avatarUrl = data.owner?.avatar_url
    if (typeof avatarUrl === 'string' && /^https:\/\//i.test(avatarUrl)) {
      const slot = card.querySelector('.gh-avatar-slot')
      const fallback = card.querySelector<HTMLElement>('.avatar-fallback')
      const image = root.ownerDocument.createElement('img')
      image.className = 'github-card-avatar'
      image.alt = ''
      image.referrerPolicy = 'no-referrer'
      image.onload = () => { if (fallback) fallback.style.display = 'none' }
      image.onerror = () => { image.remove(); if (fallback) fallback.style.display = 'flex' }
      slot?.prepend(image)
      image.src = avatarUrl
    }
    card.classList.add('github-card-loaded')
    card.classList.remove('github-card-error')
  })).then(() => undefined)
}
