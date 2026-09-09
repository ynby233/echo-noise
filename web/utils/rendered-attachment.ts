import { attachmentFailureDetail, attachmentFailureTitle, type AttachmentFailureKind } from './attachment-failure'
import { buildAttachmentAudioPlaceholderHtml } from './attachment-audio-player'
import { isBrowserPreviewableAttachmentUrl } from './attachment-preview'
import { getVideoPlaybackFrameForSource } from './fancybox-video-close'
import { isManagedAttachmentURL, resolveManagedAttachmentURL } from './media-url'

// Owns published attachment markup, failure probing and stale-safe DOM replacement.
export const escapeRenderedHtml = (value: string) => String(value || '')
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;')
  .replace(/'/g, '&#39;')

type AttachmentKind = AttachmentFailureKind

const attachmentKindFromLabel = (label: string): AttachmentKind => {
  if (label === '图片附件') return 'image'
  if (label === '视频附件') return 'video'
  if (label === '音频附件') return 'audio'
  return 'file'
}


const mediaPathFromUrl = (url: string) => {
  const raw = String(url || '').trim()
  if (!raw) return ''
  try {
    return decodeURIComponent(new URL(raw, typeof window !== 'undefined' ? window.location.href : 'http://local').pathname)
  } catch {
    try { return decodeURIComponent(raw.split(/[?#]/)[0]) } catch { return raw.split(/[?#]/)[0] }
  }
}

const mediaFileNameFromUrl = (url: string) => mediaPathFromUrl(url).split('/').filter(Boolean).pop() || ''
const RECORDING_NAME_RE = /录音|(^|[-_\s.])(recording|voice|memo|capture)([-_\s.]|$)/i
const AUDIO_MEDIA_EXT_RE = /\.(webm|ogg|mp3|m4a|wav|flac)(?:[?#].*)?$/i
const VIDEO_MEDIA_EXT_RE = /\.(mp4|webm|mov|avi)(?:[?#].*)?$/i

const isLikelyRecordingAttachment = (name: string, url: string) => {
  const source = `${String(name || '')} ${mediaFileNameFromUrl(url)}`
  return RECORDING_NAME_RE.test(source)
}

export const isAudioAttachmentUrl = (url: string, name = '') => {
  const path = mediaPathFromUrl(url).toLowerCase()
  if (path.includes('/api/audio/')) return true
  if (path.includes('/api/video/')) return isLikelyRecordingAttachment(name, url)
  return AUDIO_MEDIA_EXT_RE.test(path)
}

export const isVideoAttachmentUrl = (url: string, name = '') => {
  const path = mediaPathFromUrl(url).toLowerCase()
  if (isAudioAttachmentUrl(url, name)) return false
  if (path.includes('/api/video/') || path.includes('/video/')) return VIDEO_MEDIA_EXT_RE.test(path)
  return /\.(mp4|mov|avi)(?:[?#].*)?$/i.test(path)
}

const attachmentExtensionLabel = (name: string, url: string) => {
  const source = String(name || url || '').split(/[?#]/)[0]
  const decoded = (() => {
    try { return decodeURIComponent(source) } catch { return source }
  })()
  const match = decoded.match(/\.([a-z0-9]{1,12})$/i)
  return match ? match[1].toUpperCase() : 'FILE'
}

const buildAttachmentHtml = (kindLabel: string, name: string, rawUrl: string, baseApi: string) => {
  const url = resolveManagedAttachmentURL(String(baseApi || '/api'), String(rawUrl || '').trim())
  const safeUrl = escapeRenderedHtml(url)
  const safeName = escapeRenderedHtml(String(name || '').trim() || '未命名附件')
  const labeledKind = attachmentKindFromLabel(kindLabel)
  const kind = labeledKind === 'video' && isAudioAttachmentUrl(url, name) ? 'audio' : labeledKind
  if (!url) return ''
  if (kind === 'image') {
    return `<p class="site-attachment-paragraph" data-site-attachment-kind="${kind}" data-site-attachment-url="${safeUrl}"><img class="site-attachment-image" src="${safeUrl}" alt="${safeName}" loading="lazy" decoding="async" data-site-attachment-kind="${kind}" data-site-attachment-url="${safeUrl}" /></p>`
  }
  if (kind === 'video') {
    return `<div class="site-attachment-render site-attachment-render--video" data-site-attachment-kind="${kind}" data-site-attachment-url="${safeUrl}"><video src="${safeUrl}" controls preload="metadata" style="width:100%;height:auto" data-site-attachment-kind="${kind}" data-site-attachment-url="${safeUrl}"></video></div>`
  }
  if (kind === 'file') {
    const canPreview = isBrowserPreviewableAttachmentUrl(url)
    const previewAttrs = canPreview
      ? 'target="_blank" rel="noopener noreferrer"'
      : `download="${safeName}"`
    const actionLabel = canPreview ? '打开附件' : '下载附件'
    const meta = escapeRenderedHtml(attachmentExtensionLabel(name, url))
    const actionClass = canPreview ? 'site-attachment-file__action--preview' : 'site-attachment-file__action--download'
    return `<a class="site-attachment-file ${canPreview ? 'site-attachment-file--preview' : 'site-attachment-file--download'}" href="${safeUrl}" ${previewAttrs} aria-label="${actionLabel}：${safeName}" data-site-attachment-kind="${kind}" data-site-attachment-url="${safeUrl}"><span class="site-attachment-file__icon" aria-hidden="true"></span><span class="site-attachment-file__body"><span class="site-attachment-file__name">${safeName}</span><span class="site-attachment-file__meta">${meta}</span></span><span class="site-attachment-file__action ${actionClass}" aria-hidden="true"></span></a>`
  }
  return buildAttachmentAudioPlaceholderHtml({ src: url, name })
}

const buildDeletedAttachmentHtml = (kind: AttachmentKind, deleted: boolean) => {
  const title = escapeRenderedHtml(attachmentFailureTitle(kind))
  const detail = escapeRenderedHtml(attachmentFailureDetail(kind, deleted))
  return `<div class="site-attachment-file site-attachment-file--deleted site-attachment-file--deleted-${kind}" role="note" aria-label="${title}：${detail}"><span class="site-attachment-file__icon" aria-hidden="true"></span><span class="site-attachment-file__body"><span class="site-attachment-file__name">${title}</span><span class="site-attachment-file__meta">${detail}</span></span><span class="site-attachment-file__action site-attachment-file__action--deleted" aria-hidden="true"></span></div>`
}

const attachmentInfoFromRenderedAnchor = (anchor: HTMLAnchorElement) => {
  const label = (anchor.textContent || '').trim()
  const match = label.match(/^(图片附件|视频附件|音频附件)：(.+)$/)
  const href = anchor.getAttribute('href') || ''
  const fileMatch = label.match(/^文件附件：(.+)$/)
  if (fileMatch && href) return { kindLabel: '文件附件', name: fileMatch[1], url: href }
  if (!match || !href) return null
  return { kindLabel: match[1], name: match[2], url: href }
}

export const applyAttachmentRenders = (root: HTMLElement | null, baseApi: string) => {
  if (!root) return
  root.querySelectorAll<HTMLAnchorElement>('a[href]').forEach((anchor) => {
    const info = attachmentInfoFromRenderedAnchor(anchor)
    if (!info) return
    const { kindLabel, name, url } = info
    replaceNodeWithHtml(anchor, buildAttachmentHtml(kindLabel, name, url, baseApi))
  })
}

export const replaceNodeWithHtml = (node: HTMLElement, html: string) => {
  const holder = document.createElement('div')
  holder.innerHTML = html
  const next = holder.firstElementChild as HTMLElement | null
  if (!next) return
  const parent = node.parentElement
  if (parent && parent.tagName.toLowerCase() === 'p' && parent.childNodes.length === 1) {
    parent.replaceWith(next)
    return
  }
  node.replaceWith(next)
}

const deletedAttachmentProbeCache = new Map<string, Promise<boolean>>()

const canProbeAttachmentUrl = (url: string, baseApi: string) => {
  if (typeof window === 'undefined' || !isManagedAttachmentURL(url)) return false
  try {
    const parsed = new URL(url, window.location.href)
    const trustedAttachmentOrigins = new Set([window.location.origin])
    trustedAttachmentOrigins.add(new URL(String(baseApi || '/api'), window.location.origin).origin)
    return (parsed.protocol === 'http:' || parsed.protocol === 'https:') && trustedAttachmentOrigins.has(parsed.origin)
  } catch {
    return false
  }
}

const isDeletedAttachmentStatus = (status: number) => status === 404 || status === 410

const probeAttachmentDeleted = (url: string, baseApi: string) => {
  const raw = String(url || '').trim()
  if (!raw || !canProbeAttachmentUrl(raw, baseApi)) return Promise.resolve(false)
  const key = new URL(raw, window.location.href).toString()
  const existing = deletedAttachmentProbeCache.get(key)
  if (existing) return existing

  const promise = (async () => {
    try {
      const head = await fetch(key, { method: 'HEAD', cache: 'no-store', credentials: 'include' })
      if (isDeletedAttachmentStatus(head.status)) return true
      if (head.status !== 405 && head.status !== 501) return false
      const partial = await fetch(key, {
        method: 'GET',
        headers: { Range: 'bytes=0-0' },
        cache: 'no-store',
        credentials: 'include',
      })
      return isDeletedAttachmentStatus(partial.status)
    } catch {
      return false
    }
  })()
  promise.then((deleted) => {
    if (!deleted && deletedAttachmentProbeCache.get(key) === promise) {
      deletedAttachmentProbeCache.delete(key)
    }
  })
  deletedAttachmentProbeCache.set(key, promise)
  return promise
}

// 失败占位块会直接接管媒体包装层，而这些类只为"能正常显示的媒体"服务：
// inline-image-thumb 固定 96px 见方，ar-* 强制宽高比，两者都会把占位块的图标+双行文案裁掉。
// 占位块不是缩略图，所以接管时必须先卸掉这套几何，否则它拿不到自己的 min-height。
const MEDIA_GEOMETRY_CLASSES = ['inline-image-thumb', 'ar-11', 'ar-169', 'ar-34'] as const

const attachmentReplacementTarget = (node: HTMLElement, kind: AttachmentKind) => {
  if (kind === 'image') {
    return (node.closest('.site-attachment-paragraph') || node.closest('.image-grid-item') || node.closest('.single-media') || node.closest('.full-image-attachment') || node) as HTMLElement
  }
  if (kind === 'video') {
    return (node.closest('.site-attachment-render') || node.closest('.image-grid-item') || node.closest('.single-media') || node) as HTMLElement
  }
  if (kind === 'audio') {
    return (node.closest('.site-attachment-audio') || node.closest('.single-media') || node) as HTMLElement
  }
  return (node.closest('.site-attachment-file') || node) as HTMLElement
}

const videoPosterForFailure = (node: HTMLElement, url: string) => {
  if (!(node instanceof HTMLVideoElement)) return ''
  return [
    node.getAttribute('poster'),
    node.poster,
    node.parentElement?.dataset.poster,
    node.parentElement?.dataset.thumbSrc,
    getVideoPlaybackFrameForSource(url),
  ].map((value) => String(value || '').trim()).find(Boolean) || ''
}

const buildMediaAttachmentFailureContent = (kind: 'image' | 'video', deleted: boolean, poster = '') => {
  const title = escapeRenderedHtml(attachmentFailureTitle(kind))
  const detail = escapeRenderedHtml(attachmentFailureDetail(kind, deleted))
  const posterHtml = poster
    ? `<img class="site-attachment-failure__poster" src="${escapeRenderedHtml(poster)}" alt="" aria-hidden="true" />`
    : ''
  return `${posterHtml}<span class="site-attachment-failure__scrim" aria-hidden="true"></span><span class="site-attachment-failure__content"><span class="site-attachment-failure__icon" aria-hidden="true"></span><strong class="site-attachment-failure__title">${title}</strong><span class="site-attachment-failure__detail">${detail}</span></span>`
}

const renderMediaAttachmentFailure = (node: HTMLElement, kind: 'image' | 'video', deleted: boolean, url: string) => {
  let target = attachmentReplacementTarget(node, kind)
  if (target instanceof HTMLImageElement || target instanceof HTMLVideoElement || target instanceof HTMLAudioElement) {
    const replacement = document.createElement('div')
    replacement.className = kind === 'video' ? 'site-attachment-render site-attachment-render--video' : 'site-attachment-paragraph'
    target.replaceWith(replacement)
    target = replacement
  }

  const poster = kind === 'video' ? videoPosterForFailure(node, url) : ''
  const title = attachmentFailureTitle(kind)
  const detail = attachmentFailureDetail(kind, deleted)
  target.classList.remove(...MEDIA_GEOMETRY_CLASSES)
  target.classList.add('site-attachment-failure', `site-attachment-failure--${kind}`)
  target.classList.toggle('site-attachment-failure--with-poster', !!poster)
  target.setAttribute('role', 'note')
  target.setAttribute('aria-label', `${title}：${detail}`)
  target.innerHTML = buildMediaAttachmentFailureContent(kind, deleted, poster)

  const posterImage = target.querySelector<HTMLImageElement>('.site-attachment-failure__poster')
  if (posterImage) {
    const discardBrokenPoster = () => {
      posterImage.remove()
      target.classList.remove('site-attachment-failure--with-poster')
    }
    posterImage.addEventListener('error', discardBrokenPoster, { once: true })
    if (posterImage.complete && !posterImage.naturalWidth) discardBrokenPoster()
  }
}

const renderAttachmentFailure = (node: HTMLElement, kind: AttachmentKind, deleted: boolean, url: string) => {
  const target = attachmentReplacementTarget(node, kind)
  if (!target || target.classList.contains('site-attachment-file--deleted') || target.classList.contains('site-attachment-failure')) return
  if (kind === 'image' || kind === 'video') {
    renderMediaAttachmentFailure(node, kind, deleted, url)
    return
  }
  replaceNodeWithHtml(target, buildDeletedAttachmentHtml(kind, deleted))
}

export const applyDeletedAttachmentPlaceholders = (root: HTMLElement | null, baseApi: string) => {
  if (!root) return
  const nodes = Array.from(root.querySelectorAll<HTMLElement>(
    '[data-site-attachment-kind][data-site-attachment-url]'
  ))

  nodes.forEach((node) => {
    if (node.closest('.site-attachment-file--deleted')) return
    const kind = String(node.dataset.siteAttachmentKind || 'file') as AttachmentKind
    const url = String(node.dataset.siteAttachmentUrl || '')
    if (!['image', 'video', 'audio', 'file'].includes(kind) || !url) return

    if (node.dataset.siteDeletedAttachmentBound !== 'true') {
      node.dataset.siteDeletedAttachmentBound = 'true'
      if (node instanceof HTMLImageElement || node instanceof HTMLVideoElement || node instanceof HTMLAudioElement) {
        node.addEventListener('error', () => renderAttachmentFailure(node, kind, false, url), { once: true })
      }
    }

    if (node instanceof HTMLImageElement && node.complete && !node.naturalWidth) {
      renderAttachmentFailure(node, kind, false, url)
      return
    }

    void probeAttachmentDeleted(url, baseApi).then((deleted) => {
      if (!deleted || !node.isConnected || !root.contains(node)) return
      renderAttachmentFailure(node, kind, true, url)
    })
  })
}
