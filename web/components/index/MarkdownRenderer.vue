<template>
  <div ref="previewElement" :class="['markdown-preview', { 'markdown-preview--inherit-font': props.inheritFont }]" :data-task-list-editable="props.taskListEditable ? 'true' : 'false'"></div>
  <div v-if="previewLoadFailed" role="alert" class="markdown-load-error">
    正文排版加载失败，已显示文本。
    <button type="button" class="nw-action-btn" @click="renderMarkdown(props.content)">重试</button>
  </div>
  <Teleport to="body">
    <div
      v-if="showRenderedTableExpandDialog"
      :class="['rendered-table-expand-overlay', { 'is-dark': renderedTableExpandDark, 'is-closing': renderedTableExpandClosing }]"
      @click.self="closeRenderedTableExpand"
    >
      <section class="rendered-table-expand-dialog" role="dialog" aria-modal="true" aria-label="放大查看表格" @click.stop>
        <header class="rendered-table-expand-header">
          <div>
            <strong>放大查看表格</strong>
            <span>可滚动查看表格与附件内容</span>
          </div>
          <button type="button" class="rendered-table-expand-close nw-action-btn nw-tooltip-anchor" data-tooltip="关闭" aria-label="关闭放大表格" @click="closeRenderedTableExpand">
            <span class="table-expand-close-icon" aria-hidden="true"></span>
          </button>
        </header>
        <div ref="renderedTableExpandBody" class="rendered-table-expand-scroll" v-html="renderedTableExpandHtml"></div>
      </section>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref, watch, onBeforeUnmount, inject } from 'vue';
import { useRuntimeConfig } from '#imports';
import { useMessageStore } from '~/store/message';
import { ensureFancyboxVideoThumbnail, getVideoElementSource, normalizeMediaPreviewUrl } from '~/utils/fancybox-video-close'
import { bindMediaFancybox, unbindMediaFancybox, createMediaFancyboxOptions } from '~/utils/media-fancybox'
import { buildAttachmentAudioPlaceholderHtml, destroyAttachmentAudioPlayers, enhanceAttachmentAudioPlayers } from '~/utils/attachment-audio-player'
import { encodeMarkdownExtraBlankLines, markMarkdownPreservedBlankLineElements } from '~/utils/markdown-blank-lines'
import { isManagedAttachmentURL, resolveManagedAttachmentURL } from '~/utils/media-url'
import { loadVditorPreview } from '~/utils/vditor-preview'
import { enhanceMetingPlayers } from '~/utils/meting-player'
import { enhanceGitHubCards } from '~/utils/github-card'
import { createRenderedTaskListEnhancer } from '~/utils/rendered-task-list'
import { createRenderedTableDialog } from '~/utils/rendered-table-dialog'
import { applyImageLoadingPlaceholders, applyRenderedMediaLayout } from '~/utils/rendered-media-layout'
import { applyAttachmentRenders, applyDeletedAttachmentPlaceholders, escapeRenderedHtml, isAudioAttachmentUrl, isVideoAttachmentUrl, replaceNodeWithHtml } from '~/utils/rendered-attachment'

// 定义正则表达式
const BILIBILI_REG = /https:\/\/www\.bilibili\.com\/video\/(BV[\w]+)\/?(?:\?[^\s<)]*)?/g;
const YOUTUBE_REG = /https:\/\/(?:www\.)?youtube\.com\/watch\?v=([\w-]+)|https:\/\/youtu\.be\/([\w-]+)/g;
const NETEASE_MUSIC_REG = /https:\/\/music\.163\.com(?:\/#)?\/song\?id=(\d+)/g;
const QQMUSIC_REG = /https:\/\/y\.qq\.com\/n\/yqq\/song(\w+)\.html/g;
const QQVIDEO_REG = /https:\/\/v\.qq\.com\/x\/cover\/\w+\/(\w+)\.html/g;
const SPOTIFY_REG = /https:\/\/open\.spotify\.com\/(track|album|playlist)\/([a-zA-Z0-9]+)/g;
const YOUKU_REG = /https:\/\/v\.youku\.com\/v_show\/id_([a-zA-Z0-9]+)\.html/g;
const DOUYIN_REG = /https:\/\/www\.douyin\.com\/video\/(\d+)\/?/g;
const DOUYIN_SHORTCODE_REG = /\[VideoID=([a-zA-Z0-9]+)\]/g;
const DOUYIN_SHORT_REG = /^https?:\/\/(?:v\.douyin\.com|(?:www\.)?iesdouyin\.com)\/[^\s]+$/i;
// @ts-ignore
const emit = defineEmits(['tagClick', 'rendered'])
const config = useRuntimeConfig();
const BASE_API = config.public.baseApi || '/api';

const resolveImageUrl = (path: string) => resolveManagedAttachmentURL(String(BASE_API || '/api'), path)
const resolveAttachmentUrl = (path: string) => resolveManagedAttachmentURL(String(BASE_API || '/api'), path)

const previewElement = ref<HTMLDivElement | null>(null);
const previewLoadFailed = ref(false)
let renderSequence = 0
let zoom: any = null;
let themeClassObserver: MutationObserver | null = null
// 添加 window 类型声明
declare global {
  interface Window {
    handleTagClick: (tag: string) => void;
    mediumZoom: any;
    Fancybox?: any;
    APlayer: any;
    MetingJSElement: any;
    meting_api?: string;
  }
}
// @ts-ignore
const props = defineProps({
  content: {
    type: String,
    required: true,
  },
  themeMode: {
    type: [String, Object],
    default: undefined,
  },
  enableGithubCard: {
    type: Boolean,
    default: true,
  },
  messageId: {
    type: Number,
    default: 0,
  },
  taskListEditable: {
    type: Boolean,
    default: false,
  },
  inheritFont: {
    type: Boolean,
    default: false,
  },
});

const messageStore = useMessageStore();
const renderedTaskContent = ref(props.content)
const taskLists = createRenderedTaskListEnhancer({
  root: () => previewElement.value,
  content: () => renderedTaskContent.value,
  setContent: (content) => { renderedTaskContent.value = content },
  editable: () => props.taskListEditable,
  messageId: () => Number(props.messageId || 0),
  persist: (messageId, content) => messageStore.updateMessage(messageId, content),
  reportError: (error) => console.error('更新任务状态失败:', error),
})

const contentTheme = inject('contentTheme') as any
const FULL_IMAGE_ATTACHMENTS_MARKER_RE = /<!--\s*full-image-attachments\s*-->\s*/gi
const hasFullImageAttachmentsMarker = (content: string) => {
  FULL_IMAGE_ATTACHMENTS_MARKER_RE.lastIndex = 0
  return FULL_IMAGE_ATTACHMENTS_MARKER_RE.test(String(content || ''))
}
const stripFullImageAttachmentsMarker = (content: string) => String(content || '').replace(FULL_IMAGE_ATTACHMENTS_MARKER_RE, '').trimStart()
const HASHTAG_REG = /(^|[\s(（[{【])#([\p{L}\p{N}_-]+)/gu
const METING_API_FALLBACKS = [
  'https://meting.soopy.cn/api',
  'https://api.injahow.cn/meting/',
  'https://api.i-meto.com/meting/api',
]

const resolveMetingApiTemplate = () => {
  const fromWindow = String(window?.meting_api || '').trim()
  const base = fromWindow || METING_API_FALLBACKS[0]
  return `${base.replace(/\?$/, '')}?server=:server&type=:type&id=:id&auth=:auth&r=:r`
}

const ensureMetingApiReady = () => {
  if (typeof window === 'undefined') return
  if (!String(window.meting_api || '').trim()) {
    window.meting_api = METING_API_FALLBACKS[0]
  }
}

const buildMetingSongEmbed = (songId: string) => {
  const id = String(songId || '').trim()
  if (!id) return ''
  return `<div class='music-wrapper'><meting-js api='${resolveMetingApiTemplate()}' server='netease' type='song' id='${id}' auto='https://music.163.com/#/song?id=${id}'></meting-js></div>`
}

const applyThemeClass = () => {
  if (!previewElement.value) return
  const propTheme = (() => {
    const v: any = props.themeMode as any
    if (typeof v === 'string') return v.trim().toLowerCase()
    if (v && typeof v.value === 'string') return String(v.value).trim().toLowerCase()
    return ''
  })()
  let isDark = false
  if (propTheme === 'dark' || propTheme === 'light') {
    isDark = propTheme === 'dark'
  } else if (contentTheme && typeof (contentTheme as any).value !== 'undefined') {
    isDark = (contentTheme as any).value === 'dark'
  } else {
    isDark = document.documentElement.classList.contains('dark')
  }
  previewElement.value.classList.toggle('theme-dark', !!isDark)
  previewElement.value.classList.toggle('theme-light', !isDark)
  renderedTableExpandDark.value = !!isDark
}

const initializeMediaViewer = (customRoot?: HTMLElement | null) => {
  const root = customRoot || previewElement.value
  if (!root) return

  if (!customRoot && zoom) {
    try { zoom.detach?.() } catch {}
    zoom = null
  }

  const group = customRoot
    ? `markdown-table-media-${Date.now()}-${Math.random().toString(36).slice(2)}`
    : `markdown-media-${props.messageId || 'preview'}`
  root.querySelectorAll<HTMLAnchorElement>('a.site-attachment-tag[data-attachment-kind]').forEach((anchor) => {
    const kind = anchor.dataset.attachmentKind || ''
    const url = normalizeMediaPreviewUrl(anchor.dataset.attachmentUrl || anchor.getAttribute('href') || '')
    if (!url || kind === 'audio') return
    anchor.setAttribute('data-fancybox', anchor.getAttribute('data-fancybox') || group)
    anchor.setAttribute('href', url)
    anchor.dataset.src = url
    if (kind === 'video') anchor.dataset.type = 'html5video'
  })
  root.querySelectorAll('a[data-fancybox], img, video').forEach((node) => {
    const el = node as HTMLElement
    if (el.closest('.github-card, .video-wrapper, .douyin-video-wrapper, .bilibili-video-wrapper')) return
    const tag = el.tagName.toLowerCase()
    if (tag === 'a') {
      const anchor = el as HTMLAnchorElement
      anchor.setAttribute('data-fancybox', anchor.getAttribute('data-fancybox') || group)
      return
    }
    if (tag === 'img') {
      const img = el as HTMLImageElement
      const src = img.currentSrc || img.src || img.getAttribute('src') || ''
      if (!src) return
      const parent = img.parentElement as HTMLAnchorElement | null
      if (parent?.tagName?.toLowerCase() === 'a') {
        parent.setAttribute('data-fancybox', parent.getAttribute('data-fancybox') || group)
        if (!parent.getAttribute('href')) parent.setAttribute('href', src)
      } else if (img.parentNode) {
        const anchor = document.createElement('a')
        anchor.href = src
        anchor.setAttribute('data-fancybox', group)
        anchor.className = 'inline-image-link'
        img.parentNode.insertBefore(anchor, img)
        anchor.appendChild(img)
      }
      return
    }
    if (tag === 'video') {
      const video = el as HTMLVideoElement
      const src = normalizeMediaPreviewUrl(getVideoElementSource(video))
      if (!src) return
      const parent = video.parentElement as HTMLAnchorElement | null
      const trigger = parent?.tagName?.toLowerCase() === 'a' ? parent : video
      trigger.setAttribute('data-fancybox', trigger.getAttribute('data-fancybox') || group)
      if (trigger instanceof HTMLAnchorElement) trigger.href = src
      trigger.dataset.src = src
      trigger.dataset.type = 'html5video'
      ensureFancyboxVideoThumbnail(video, trigger)
      video.classList.add('fancybox-video-trigger')
    }
  })

  bindMediaFancybox(root, createMediaFancyboxOptions({ video: true }))
};

const shouldSkipHashtagNode = (node: Node | null) => {
  const parent = node?.parentElement
  if (!parent) return true
  return !!parent.closest('a, button, code, pre, script, style, textarea, input, .clickable-tag, .github-card, .video-wrapper, .aplayer')
}

const applyClickableTags = () => {
  if (!previewElement.value || typeof document === 'undefined') return
  const root = previewElement.value
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      if (!node.textContent || !node.textContent.includes('#')) return NodeFilter.FILTER_REJECT
      if (shouldSkipHashtagNode(node)) return NodeFilter.FILTER_REJECT
      return NodeFilter.FILTER_ACCEPT
    }
  })
  const textNodes: Text[] = []
  while (walker.nextNode()) {
    textNodes.push(walker.currentNode as Text)
  }

  textNodes.forEach((textNode) => {
    const raw = String(textNode.textContent || '')
    HASHTAG_REG.lastIndex = 0
    let match: RegExpExecArray | null = null
    let lastIndex = 0
    let changed = false
    const frag = document.createDocumentFragment()

    while ((match = HASHTAG_REG.exec(raw)) !== null) {
      changed = true
      const full = match[0] || ''
      const prefix = match[1] || ''
      const tag = String(match[2] || '').trim()
      const matchIndex = match.index
      const prefixStart = matchIndex + prefix.length

      if (matchIndex > lastIndex) {
        frag.appendChild(document.createTextNode(raw.slice(lastIndex, matchIndex)))
      }
      if (prefix) {
        frag.appendChild(document.createTextNode(prefix))
      }

      const button = document.createElement('button')
      button.type = 'button'
      button.className = 'clickable-tag'
      button.dataset.tag = tag
      button.textContent = `#${tag}`
      frag.appendChild(button)
      lastIndex = prefixStart + full.slice(prefix.length).length
    }

    if (!changed) return
    if (lastIndex < raw.length) {
      frag.appendChild(document.createTextNode(raw.slice(lastIndex)))
    }
    textNode.parentNode?.replaceChild(frag, textNode)
  })
}

const onPreviewClick = (event: Event) => {
  const element = event.target as HTMLElement | null
  const taskCheckbox = element?.closest('input[type="checkbox"]') as HTMLInputElement | null
  if (taskCheckbox && previewElement.value?.contains(taskCheckbox)) {
    event.stopPropagation()
    return
  }
  const target = element?.closest('.clickable-tag') as HTMLElement | null
  if (!target) return
  event.preventDefault()
  event.stopPropagation()
  const tag = String(target.dataset.tag || target.textContent || '').replace(/^#/, '').trim()
  if (!tag) return
  emit('tagClick', tag)
}

const renderedTables = createRenderedTableDialog({
  root: () => previewElement.value,
  enhance: (root) => {
    if (props.enableGithubCard) void enhanceGitHubCards(root)
    enhanceAttachmentAudioPlayers(root)
    applyDeletedAttachmentPlaceholders(root, String(BASE_API || '/api'))
    initializeMediaViewer(root)
  },
  cleanup: (root) => {
    unbindMediaFancybox(root)
    destroyAttachmentAudioPlayers(root)
  },
})
const {
  body: renderedTableExpandBody,
  visible: showRenderedTableExpandDialog,
  closing: renderedTableExpandClosing,
  html: renderedTableExpandHtml,
  dark: renderedTableExpandDark,
  close: closeRenderedTableExpand,
} = renderedTables
const buildYouTubeEmbedHtml = (videoId: string) => {
  const watchUrl = `https://www.youtube.com/watch?v=${videoId}`
  return `<div class='video-block youtube-video-block'><div class='video-wrapper youtube-video-wrapper'><iframe src='https://www.youtube.com/embed/${videoId}' title='YouTube video player' frameborder='0' allow='accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture' allowfullscreen></iframe></div><div class='video-fallback-card youtube-fallback-card'><div class='video-fallback-content'><div class='video-fallback-title'>当前网络若无法加载 YouTube，可直接访问：</div><a class='video-fallback-link' href='${watchUrl}' target='_blank' rel='noopener noreferrer'>${watchUrl}</a></div></div></div>`
}
const buildBilibiliEmbedHtml = (bvid: string, page?: string) => {
  const bv = String(bvid || '').trim()
  if (!bv) return ''
  const p = String(page || '1').trim() || '1'
  const src = `https://player.bilibili.com/player.html?isOutside=true&bvid=${encodeURIComponent(bv)}&p=${encodeURIComponent(p)}&autoplay=0&high_quality=1&danmaku=0&muted=0`
  return `<div class='video-wrapper'><iframe src='${src}' scrolling='no' frameborder='0' allowfullscreen allow='autoplay; fullscreen; picture-in-picture; encrypted-media' referrerpolicy='no-referrer-when-downgrade' loading='lazy'></iframe></div>`
}

const processMediaLinks = (content: string): string => {
  ensureMetingApiReady()
  // 先处理 markdown 链接与行内代码里的媒体链接，避免后续替换打断 markdown 结构
  const BILIBILI_MD_LINK_REG = /\[[^\]]*]\((https:\/\/www\.bilibili\.com\/video\/(BV[\w]+)\/?(?:\?[^\s)]*)?)\)/g;
  const YOUTUBE_MD_LINK_REG = /\[[^\]]*]\((https:\/\/(?:www\.)?youtube\.com\/watch\?v=([\w-]+)|https:\/\/youtu\.be\/([\w-]+))\)/g;
  const NETEASE_MD_LINK_REG = /\[[^\]]*]\((https:\/\/music\.163\.com(?:\/#)?\/song\?id=(\d+))\)/g;
  // 允许反引号内 URL 前后有空格，避免 ` https://... ` 这类写法漏匹配
  const NETEASE_INLINE_CODE_REG = /`[\t ]*https:\/\/music\.163\.com(?:\/#)?\/song\?id=(\d+)[\t ]*`/g;
  content = content
    .replace(BILIBILI_MD_LINK_REG, (_m, fullUrl, bvid) => {
      let page = '1'
      try {
        const u = new URL(String(fullUrl))
        page = u.searchParams.get('p') || u.searchParams.get('page') || '1'
      } catch {}
      return buildBilibiliEmbedHtml(bvid, page) || _m
    })
    .replace(YOUTUBE_MD_LINK_REG, (_m, _full, id1, id2) => {
      const videoId = String(id1 || id2 || '').trim()
      if (!videoId) return _m
      return buildYouTubeEmbedHtml(videoId)
    })
    .replace(NETEASE_MD_LINK_REG, (_m, _full, songId) => buildMetingSongEmbed(songId) || _m)
    .replace(NETEASE_INLINE_CODE_REG, (_m, songId) => buildMetingSongEmbed(songId) || _m)

  // 平台附件标记保留为 Markdown 链接，等 Markdown 表格先正常渲染后再替换成媒体节点。

  // 将裸媒体文件链接替换为内联播放器（先于链接化处理）。
  // 仅匹配前导为空白字符或行首的 URL，避免匹配 HTML 属性中的 URL（如 src="http..."）。
  const AUDIO_FILE_REG = /(^|[\s>])((?:https?:\/\/|\/api\/audio\/)[^\s<"']+\.(?:webm|ogg|mp3|m4a|wav|flac)(?:\?[^\s<"']*)?)/g;
  content = content.replace(AUDIO_FILE_REG, (_m, prefix, audioUrl) => {
    const src = resolveImageUrl(audioUrl);
    return `${prefix}${buildAttachmentAudioPlaceholderHtml({ src })}`;
  });
  const VIDEO_FILE_REG = /(^|[\s>])((?:https?:\/\/|\/api\/video\/|\/video\/)[^\s<"']+\.(?:mp4|webm|mov|avi)(?:\?[^\s<"']*)?)/g;
  content = content.replace(VIDEO_FILE_REG, (_m, prefix, videoUrl) => {
    const src = resolveImageUrl(videoUrl);
    const safeSrc = escapeRenderedHtml(src)
    if (isAudioAttachmentUrl(src)) {
      return `${prefix}${buildAttachmentAudioPlaceholderHtml({ src })}`;
    }
    return `${prefix}<video src="${safeSrc}" controls preload="metadata" style="width:100%;height:auto" data-site-attachment-kind="video" data-site-attachment-url="${safeSrc}"></video>`;
  });
  content = content
    .replace(BILIBILI_REG, (m, bvid) => {
      let page = '1'
      try {
        const u = new URL(String(m))
        page = u.searchParams.get('p') || u.searchParams.get('page') || '1'
      } catch {}
      return buildBilibiliEmbedHtml(bvid, page) || m
    })
    .replace(NETEASE_MUSIC_REG, (_m, songId) => buildMetingSongEmbed(songId) || _m)
    .replace(QQMUSIC_REG, "<meting-js auto='https://y.qq.com/n/yqq/song$1.html'></meting-js>")
    .replace(QQVIDEO_REG, "<div class='video-wrapper'><iframe src='//v.qq.com/iframe/player.html?vid=$1' allowFullScreen='true' frameborder='no'></iframe></div>")
    .replace(SPOTIFY_REG, "<div class='spotify-wrapper'><iframe style='border-radius:12px' src='https://open.spotify.com/embed/$1/$2?utm_source=generator&theme=0' width='100%' frameBorder='0' allowfullscreen='' allow='autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture' loading='lazy'></iframe></div>")
    .replace(YOUKU_REG, "<div class='video-wrapper'><iframe src='https://player.youku.com/embed/$1' frameborder=0 'allowfullscreen'></iframe></div>")
    .replace(DOUYIN_REG, "<div class='video-wrapper douyin-video-wrapper' data-douyin-vid='$1'><iframe src='https://open.douyin.com/player/video?vid=$1&autoplay=0' frameborder='0' scrolling='no' allow='autoplay; encrypted-media' allowfullscreen='true' referrerpolicy='unsafe-url'></iframe></div>")
    .replace(DOUYIN_SHORTCODE_REG, "<div class='video-wrapper douyin-video-wrapper' data-douyin-vid='$1'><iframe src='https://open.douyin.com/player/video?vid=$1&autoplay=0' frameborder='0' scrolling='no' allow='autoplay; encrypted-media' allowfullscreen='true' referrerpolicy='unsafe-url'></iframe></div>");
  content = content.replace(YOUTUBE_REG, (_m, id1, id2) => {
    const videoId = String(id1 || id2 || '').trim()
    if (!videoId) return _m
    return buildYouTubeEmbedHtml(videoId)
  })
  return content
};
const buildDouyinEmbedHtml = (videoId: string) => {
  const vid = String(videoId || '').trim()
  if (!vid) return ''
  return `<div class='video-wrapper douyin-video-wrapper' data-douyin-vid='${vid}'><iframe src='https://open.douyin.com/player/video?vid=${vid}&autoplay=0' frameborder='0' scrolling='no' allow='autoplay; encrypted-media' allowfullscreen='true' referrerpolicy='unsafe-url'></iframe></div>`
}
const buildDouyinFallbackHtml = (link: string) => `<div class='video-fallback-card douyin-fallback-card'><div class='video-fallback-content'><div class='video-fallback-title'>抖音短链解析失败</div><a class='video-fallback-link' href='${link}' target='_blank' rel='noopener noreferrer'>打开原链接</a></div></div>`
const resolveDouyinShortToVideoInfo = async (link: string): Promise<{ videoId: string }> => {
  try {
    const endpoint = `${String(BASE_API).replace(/\/$/, '')}/douyin/resolve?url=${encodeURIComponent(link)}`
    const res = await fetch(endpoint, { method: 'GET', credentials: 'omit' })
    const data = await res.json().catch(() => ({} as any))
    if (data?.code === 1) {
      return {
        videoId: String(data?.data?.video_id || '').trim(),
      }
    }
    return { videoId: '' }
  } catch {
    return { videoId: '' }
  }
}
const enhanceDouyinShortLinks = async () => {
  if (!previewElement.value) return
  const anchors = Array.from(previewElement.value.querySelectorAll('a[href]')) as HTMLAnchorElement[]
  const targets = anchors.filter((a) => {
    const href = String(a.getAttribute('href') || '').trim()
    if (!DOUYIN_SHORT_REG.test(href)) return false
    if ((a as any).__dyResolved) return false
    return true
  })
  if (!targets.length) return
  const cache = new Map<string, { videoId: string }>()
  for (const a of targets) {
    const href = String(a.getAttribute('href') || '').trim()
    if (!href) continue
    let info = cache.get(href) || { videoId: '' }
    if (!info.videoId) {
      info = await resolveDouyinShortToVideoInfo(href)
      cache.set(href, info)
    }
    ;(a as any).__dyResolved = true
    if (!info.videoId) {
      replaceNodeWithHtml(a, buildDouyinFallbackHtml(href))
      continue
    }
    replaceNodeWithHtml(a, buildDouyinEmbedHtml(info.videoId))
  }
}
const applyDouyinVideoLayout = () => {
  if (!previewElement.value || typeof window === 'undefined') return
  const douyinIframes = Array.from(
    previewElement.value.querySelectorAll("iframe[src*='open.douyin.com/player/video']")
  ) as HTMLIFrameElement[]
  douyinIframes.forEach((iframe) => {
    const wrap = iframe.closest('.video-wrapper') as HTMLElement | null
    if (wrap) wrap.classList.add('douyin-video-wrapper')
  })
  const wrappers = Array.from(previewElement.value.querySelectorAll('.douyin-video-wrapper')) as HTMLElement[]
  if (!wrappers.length) return
  const ua = String(window.navigator?.userAgent || '').toLowerCase()
  const isRealMobileDevice = /android|iphone|ipod|ipad|mobile|windows phone/.test(ua)
    || (window.matchMedia('(pointer: coarse)').matches && window.matchMedia('(hover: none)').matches)
  const isMobileViewport = window.matchMedia('(max-width: 1024px)').matches
  const useMobilePortrait = isRealMobileDevice && isMobileViewport
  const rootInThreeColumn = !!previewElement.value.closest('.layout-container.grid-3, .feed-grid-three')
  wrappers.forEach((el) => {
    const inThreeColumnByClass = !!(
      rootInThreeColumn
      || el.closest('.layout-container.grid-3')
      || el.closest('.feed-grid-three')
      || previewElement.value?.closest('.layout-container.grid-3')
      || previewElement.value?.closest('.feed-grid-three')
    )
    const card = el.closest('.content-container, .feed-item-card, .message-item, .feed-summary-markdown') as HTMLElement | null
    const cardWidth = Math.max(
      0,
      Number(card?.clientWidth || 0),
      Number((el.parentElement as HTMLElement | null)?.clientWidth || 0)
    )
    // 类名判定 + 卡片宽度双重判定，避免三栏样式漏判
    const inferredThreeColumn = inThreeColumnByClass || (cardWidth > 0 && cardWidth <= 560)

    el.classList.toggle('douyin-three-col', inferredThreeColumn)
    // 三栏优先级最高：强制横屏，避免与 mobile-portrait 同时生效造成竖屏拉高
    el.classList.toggle('douyin-mobile-portrait', !inferredThreeColumn && useMobilePortrait)
    // 三栏下使用“半尺寸画布”策略，缓解官方播放器在窄卡片里的竖屏回退
    el.classList.toggle('douyin-half-canvas', inferredThreeColumn && !useMobilePortrait)

    el.style.margin = '0.4em auto'
    el.style.height = 'auto'
    el.style.paddingBottom = '0'
    const currentVid = String(el.getAttribute('data-douyin-vid') || '').trim()
    if (currentVid && !el.querySelector("iframe[src*='open.douyin.com/player/video']")) {
      el.innerHTML = `<iframe src='https://open.douyin.com/player/video?vid=${currentVid}&autoplay=0' frameborder='0' scrolling='no' allow='autoplay; encrypted-media' allowfullscreen='true' referrerpolicy='unsafe-url'></iframe>`
    }
    if (inferredThreeColumn) {
      el.style.width = '100%'
      el.style.maxWidth = '100%'
      el.style.aspectRatio = '16 / 9'
      return
    }
    if (useMobilePortrait) {
      el.style.width = '100%'
      el.style.maxWidth = '100%'
      el.style.aspectRatio = '9 / 16'
      return
    }
    el.style.width = '100%'
    el.style.maxWidth = '100%'
    el.style.aspectRatio = '16 / 9'
  })
}
const renderMarkdown = async (markdown: string) => {
  if (!previewElement.value) return;
  const sequence = ++renderSequence
  const renderRoot = previewElement.value
  previewLoadFailed.value = false
  destroyAttachmentAudioPlayers(previewElement.value)

  const renderPlainFallback = (raw: string) => {
    if (!previewElement.value) return
    const text = String(raw || '')
      .replace(/<br\s*\/?>/gi, '\n')
      .replace(/<\/p>/gi, '\n\n')
      .replace(/<[^>]+>/g, '')
      .trim()
    previewElement.value.textContent = text
    applyThemeClass()
  }

  try {
    ensureMetingApiReady()
    if (!renderRoot.hasChildNodes()) renderPlainFallback(markdown ?? '')
    const Vditor = await loadVditorPreview()
    if (sequence !== renderSequence || previewElement.value !== renderRoot) return

    // 先处理媒体链接
    const keepImagesFullSize = hasFullImageAttachmentsMarker(markdown ?? '')
    const renderContent = encodeMarkdownExtraBlankLines(stripFullImageAttachmentsMarker(markdown ?? ''))
    const processedContent = processMediaLinks(renderContent);

    // Let Markdown parse links without rewriting code blocks or HTML attributes.
    const finalContent = processedContent;

    const currentTheme = (() => {
      const v: any = props.themeMode as any
      if (typeof v === 'string') {
        const out = v.trim().toLowerCase()
        if (out === 'dark' || out === 'light') return out
      }
      if (v && typeof v.value === 'string') {
        const out = String(v.value).trim().toLowerCase()
        if (out === 'dark' || out === 'light') return out
      }
      if (contentTheme && (contentTheme as any).value) {
        return (contentTheme as any).value === 'dark' ? 'dark' : 'light'
      }
      return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
    })()
    const hljsStyle = currentTheme === 'dark' ? 'github-dark' : 'github'
    await Vditor.preview(renderRoot, finalContent, {
      mode: currentTheme as any,
      lang: 'zh_CN',
      theme: { current: currentTheme },
      hljs: { style: hljsStyle, lineNumber: true, enable: true },
      markdown: { sanitize: false },
      after: async () => {
        if (sequence !== renderSequence || previewElement.value !== renderRoot) return
        try {
          const images = previewElement.value?.querySelectorAll('img');
          images?.forEach(img => {
             const src = img.getAttribute('src');
             if (src) {
                 img.src = isManagedAttachmentURL(src) ? resolveAttachmentUrl(src) : resolveImageUrl(src);
             }
          });
          const links = previewElement.value?.querySelectorAll('a');
          links?.forEach(link => {
            if (!link.hasAttribute('target')) {
              link.setAttribute('target', '_blank');
              link.setAttribute('rel', 'noopener noreferrer');
            }
          });
          if (props.enableGithubCard && previewElement.value) {
            void enhanceGitHubCards(previewElement.value).then(() => emit('rendered'))
          }
          markMarkdownPreservedBlankLineElements(previewElement.value)
          applyAttachmentRenders(previewElement.value, String(BASE_API || '/api'))
          applyThemeClass();
          const anchors = previewElement.value?.querySelectorAll<HTMLAnchorElement>('a[href]') || [];
          anchors.forEach((a: HTMLAnchorElement) => {
            if (a.classList.contains('site-attachment-tag')) return
            const href = a.getAttribute('href') || ''
            if (isAudioAttachmentUrl(href, a.textContent || '')) {
              const src = resolveAttachmentUrl(href)
              replaceNodeWithHtml(a, buildAttachmentAudioPlaceholderHtml({ src, name: a.textContent || '' }))
              return
            }
            if (isVideoAttachmentUrl(href, a.textContent || '')) {
              const v = document.createElement('video')
              const src = resolveAttachmentUrl(href)
              v.setAttribute('src', src)
              v.setAttribute('controls', 'true')
              v.setAttribute('preload', 'metadata')
              v.dataset.siteAttachmentKind = 'video'
              v.dataset.siteAttachmentUrl = src
              v.style.width = '100%'
              v.style.height = 'auto'
              a.replaceWith(v)
            }
          });
          await enhanceDouyinShortLinks()
          applyDouyinVideoLayout()
          applyClickableTags()
          taskLists.update()
          renderedTables.update()
          await nextTick()
          taskLists.update()
          
          // Explicitly handle existing video tags (e.g. from raw HTML or markdown)
          const existingVideos = Array.from(previewElement.value?.querySelectorAll('video') || []) as HTMLVideoElement[];
          existingVideos.forEach((v: HTMLVideoElement) => {
              const src = v.getAttribute('src');
              if (src) {
                  v.setAttribute('src', isManagedAttachmentURL(src) ? resolveAttachmentUrl(src) : resolveImageUrl(src));
              }
              const resolvedSrc = v.getAttribute('src') || ''
              if (resolvedSrc && isAudioAttachmentUrl(resolvedSrc, v.getAttribute('aria-label') || v.getAttribute('title') || '')) {
                  replaceNodeWithHtml(v, buildAttachmentAudioPlaceholderHtml({
                    src: resolvedSrc,
                    name: v.getAttribute('aria-label') || v.getAttribute('title') || '',
                  }))
                  return
              }
              if (!v.hasAttribute('controls')) {
                  v.setAttribute('controls', 'true');
              }
              if (resolvedSrc && !v.dataset.siteAttachmentKind && isVideoAttachmentUrl(resolvedSrc)) {
                  v.dataset.siteAttachmentKind = 'video'
                  v.dataset.siteAttachmentUrl = resolvedSrc
              }
              // Ensure proper sizing to prevent collapse (fixes height="100%" issue)
              v.style.width = '100%';
              v.style.height = 'auto';
              v.style.maxWidth = '100%';
          });
          const existingAudios = Array.from(previewElement.value?.querySelectorAll('audio') || []) as HTMLAudioElement[];
          existingAudios.forEach((audio: HTMLAudioElement) => {
              const src = audio.getAttribute('src')
              if (src) audio.setAttribute('src', isManagedAttachmentURL(src) ? resolveAttachmentUrl(src) : resolveImageUrl(src))
              const resolvedSrc = audio.getAttribute('src') || ''
              if (resolvedSrc && !audio.dataset.siteAttachmentKind && isAudioAttachmentUrl(resolvedSrc)) {
                  audio.dataset.siteAttachmentKind = 'audio'
                  audio.dataset.siteAttachmentUrl = resolvedSrc
              }
          });

          if (previewElement.value) enhanceAttachmentAudioPlayers(previewElement.value)
          applyDeletedAttachmentPlaceholders(previewElement.value, String(BASE_API || '/api'));
          applyRenderedMediaLayout(previewElement.value, keepImagesFullSize);
          applyDouyinVideoLayout()
          setTimeout(() => {
            applyDouyinVideoLayout()
          }, 80)
          initializeMediaViewer();
          if (previewElement.value) void enhanceMetingPlayers(previewElement.value)
          applyImageLoadingPlaceholders(previewElement.value);
          emit('rendered');
          const proc = (window as any).processNMPv2Shortcodes
          if (proc && previewElement.value) {
            proc(previewElement.value)
          }

        } catch (err) {
          console.error('Markdown post-processing failed:', err)
        }
      }
    });
  } catch (error) {
    if (sequence !== renderSequence || previewElement.value !== renderRoot) return
    console.error("Error rendering markdown:", error);
    previewLoadFailed.value = true
    renderPlainFallback(markdown ?? '')
  }
};
watch(
  () => props.content,
  async (newContent) => {
    renderedTaskContent.value = newContent
    await renderMarkdown(newContent);
  }
);

watch(
  () => props.taskListEditable,
  () => taskLists.update()
);

onMounted(() => {
  renderMarkdown(props.content);
  applyThemeClass();
  try {
    themeClassObserver = new MutationObserver(() => applyThemeClass())
    themeClassObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  } catch {}
  previewElement.value?.addEventListener('click', onPreviewClick)
  taskLists.mount()
  renderedTables.mount()
  try {
    window.addEventListener('resize', applyDouyinVideoLayout, { passive: true })
  } catch {}
});


onBeforeUnmount(() => {
  renderSequence++
  if (previewElement.value) unbindMediaFancybox(previewElement.value)
  if (previewElement.value) destroyAttachmentAudioPlayers(previewElement.value)
  if (zoom) {
    zoom.detach();
    zoom = null;
  }
  previewElement.value?.removeEventListener('click', onPreviewClick)
  taskLists.dispose()
  renderedTables.dispose()
  try {
    window.removeEventListener('resize', applyDouyinVideoLayout)
  } catch {}
  if (themeClassObserver) {
    themeClassObserver.disconnect()
    themeClassObserver = null
  }
});

watch(() => contentTheme && contentTheme.value, () => {
  // 只应用主题类，不重新渲染内容，避免重新加载
  applyThemeClass();
  
  // 只更新嵌入组件的主题，不重新渲染整个 markdown
  // 更新 GitHub 卡片主题
  const githubCards = previewElement.value?.querySelectorAll('.github-card');
  if (githubCards) {
    githubCards.forEach(card => {
      const isDark = contentTheme && contentTheme.value === 'dark';
      if (isDark) {
        card.classList.add('theme-dark');
        card.classList.remove('theme-light');
      } else {
        card.classList.add('theme-light');
        card.classList.remove('theme-dark');
      }
    });
  }
  
  // 更新播放器主题
  const aplayers = previewElement.value?.querySelectorAll('.aplayer');
  if (aplayers) {
    aplayers.forEach(player => {
      const isDark = contentTheme && contentTheme.value === 'dark';
      if (isDark) {
        player.classList.add('theme-dark');
        player.classList.remove('theme-light');
      } else {
        player.classList.add('theme-light');
        player.classList.remove('theme-dark');
      }
    });
  }
  
  // 更新视频和音频元素主题
  const videos = previewElement.value?.querySelectorAll('video');
  const audios = previewElement.value?.querySelectorAll('audio');
  if (videos) {
    videos.forEach(video => {
      const isDark = contentTheme && contentTheme.value === 'dark';
      if (isDark) {
        video.style.backgroundColor = '#202a36';
        video.style.border = '1px solid rgba(255,255,255,0.10)';
      } else {
        video.style.backgroundColor = '#ffffff';
        video.style.border = '1px solid #e5e7eb';
      }
    });
  }
  if (audios) {
    audios.forEach(audio => {
      const isDark = contentTheme && contentTheme.value === 'dark';
      if (isDark) {
        audio.style.backgroundColor = '#202a36';
        audio.style.border = 'none';
      } else {
        audio.style.backgroundColor = '#ffffff';
        audio.style.border = 'none';
      }
    });
  }
  
  // 更新 iframe 元素主题
  const iframes = previewElement.value?.querySelectorAll('iframe');
  if (iframes) {
    iframes.forEach(iframe => {
      const isDark = contentTheme && contentTheme.value === 'dark';
      if (isDark) {
        iframe.style.border = '1px solid rgba(255,255,255,0.10)';
      } else {
        iframe.style.border = '1px solid #e5e7eb';
      }
    });
  }
});

watch(() => props.enableGithubCard, () => {
  renderMarkdown(props.content)
})
</script>

<style>
/*
 * Vditor.preview() 会给容器打上 .vditor-reset，而 vditor/dist/index.css 里 .vditor-reset 带
 * overflow:auto。滚动容器一律在 padding box 处裁剪，附件卡片与失败占位块的外阴影因此被贴边切掉；
 * 收起态与编辑弹窗预览框这类“外层已放开裁剪”的场景也救不回来，因为裁剪就发生在这一层。
 * 正文预览自身没有滚动需求（宽表格由 .site-scrollable-table 自己承担），所以在源头复位。
 */
.markdown-preview.vditor-reset {
  overflow: visible;
}

.markdown-preview,
.markdown-preview.vditor-reset,
.markdown-preview .vditor-reset {
  font-family: "LXGW WenKai Screen" !important;
  line-height: 1.6;
}

.markdown-preview :where(h1, h2, h3, h4, h5, h6, p, li, blockquote, figcaption, caption, td, th, a, span, strong, em, del, mark, small, label, button, input, textarea, select, option) {
  font-family: inherit !important;
}

.markdown-preview--inherit-font,
.markdown-preview--inherit-font.vditor-reset,
.markdown-preview--inherit-font .vditor-reset,
.markdown-preview--inherit-font :where(h1, h2, h3, h4, h5, h6, p, li, blockquote, figcaption, caption, td, th, a, span, strong, em, del, mark, small, label, button, input, textarea, select, option) {
  font-family: inherit !important;
}

.markdown-preview.vditor-reset :where(h1, h2, h3, h4, h5, h6),
.markdown-preview .vditor-reset :where(h1, h2, h3, h4, h5, h6) {
  margin-top: 24px !important;
  margin-bottom: 16px !important;
  font-weight: 600 !important;
  line-height: 1.25 !important;
}

.markdown-preview.vditor-reset h1,
.markdown-preview .vditor-reset h1 { font-size: 1.75em !important; }
.markdown-preview.vditor-reset h2,
.markdown-preview .vditor-reset h2 { font-size: 1.55em !important; }
.markdown-preview.vditor-reset h3,
.markdown-preview .vditor-reset h3 { font-size: 1.38em !important; }
.markdown-preview.vditor-reset h4,
.markdown-preview .vditor-reset h4 { font-size: 1.25em !important; }
.markdown-preview.vditor-reset h5,
.markdown-preview .vditor-reset h5 { font-size: 1.13em !important; }
.markdown-preview.vditor-reset h6,
.markdown-preview .vditor-reset h6 { font-size: 1em !important; }

.markdown-preview[data-task-list-editable="false"] input[type="checkbox"] {
  pointer-events: none !important;
  cursor: default !important;
}

.markdown-preview[data-task-list-editable="true"] input[type="checkbox"] {
  cursor: pointer;
}

/* 信息流正文兜底：即使第三方样式注入异常，也保证文本可见 */
.markdown-preview,
.markdown-preview .vditor-reset,
.markdown-preview .vditor-reset p,
.markdown-preview .vditor-reset li,
.markdown-preview .vditor-reset span {
  opacity: 1 !important;
}

/* 主题化整体与标题颜色（容器自身带主题类） */
.builtin-comments .markdown-preview.theme-dark { color: #ffffff !important; }
.builtin-comments .markdown-preview.theme-light { color: #111111 !important; }
/* 通用主题文本颜色（非评论区域也适用） */
.markdown-preview.theme-dark { color: rgb(227, 220, 220) !important; }
.markdown-preview.theme-light { color: #111111 !important; }
.builtin-comments .markdown-preview.theme-dark h1,
.builtin-comments .markdown-preview.theme-dark h2,
.builtin-comments .markdown-preview.theme-dark h3,
.builtin-comments .markdown-preview.theme-dark h4,
.builtin-comments .markdown-preview.theme-dark h5,
.builtin-comments .markdown-preview.theme-dark h6 { color: #ffffff !important; }
.builtin-comments .markdown-preview.theme-light h1,
.builtin-comments .markdown-preview.theme-light h2,
.builtin-comments .markdown-preview.theme-light h3,
.builtin-comments .markdown-preview.theme-light h4,
.builtin-comments .markdown-preview.theme-light h5,
.builtin-comments .markdown-preview.theme-light h6 { color: #111111 !important; }
/* 通用标题颜色（非评论区域） */
.markdown-preview.theme-dark h1,
.markdown-preview.theme-dark h2,
.markdown-preview.theme-dark h3,
.markdown-preview.theme-dark h4,
.markdown-preview.theme-dark h5,
.markdown-preview.theme-dark h6 { color: #ffffff !important; }
.markdown-preview.theme-light h1,
.markdown-preview.theme-light h2,
.markdown-preview.theme-light h3,
.markdown-preview.theme-light h4,
.markdown-preview.theme-light h5,
.markdown-preview.theme-light h6 { color: #111111 !important; }

/* 链接样式（蓝色，可悬停下划线） */
.builtin-comments .markdown-preview.theme-light a { color: #1d4ed8 !important; text-decoration: none; }
.builtin-comments .markdown-preview.theme-light a:hover { text-decoration: underline; }
.builtin-comments .markdown-preview.theme-dark a { color: #60a5fa !important; text-decoration: none; }
.builtin-comments .markdown-preview.theme-dark a:hover { text-decoration: underline; }

.clickable-tag {
  color: #fb923c !important;
  cursor: pointer;
  transition: color 0.2s ease;
  padding: 0 2px;
  background: transparent !important;
  border: 0 !important;
  appearance: none;
  font: inherit;
  line-height: inherit;
  display: inline;
  margin: 0;
  box-shadow: none !important;
  text-shadow: none !important;
}
.theme-dark .clickable-tag { color: #fb923c !important; }
.theme-light .clickable-tag { color: #fb923c !important; }

.clickable-tag:hover {
  color: #f97316 !important;
  text-decoration: underline;
}

.markdown-preview .site-table-scroll {
  max-width: 100%;
  margin: 8px 0;
  padding-top: 10px;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
  scrollbar-color: rgba(100, 116, 139, 0.62) rgba(148, 163, 184, 0.18);
}

.markdown-preview .site-table-scroll::-webkit-scrollbar {
  height: 9px;
}

.markdown-preview .site-table-scroll::-webkit-scrollbar-track {
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.18);
}

.markdown-preview .site-table-scroll::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgba(100, 116, 139, 0.62);
}

.markdown-preview .site-table-scroll::-webkit-scrollbar-thumb:hover {
  background: rgba(71, 85, 105, 0.82);
}

.markdown-preview .site-table-scroll {
  position: relative;
}

.markdown-preview .site-table-scroll > .site-rendered-table-expand-button {
  box-sizing: border-box;
  display: inline-flex !important;
  align-items: center !important;
  justify-content: center !important;
  position: absolute !important;
  top: 0;
  left: 0;
  width: 10px !important;
  min-width: 10px !important;
  height: 10px !important;
  min-height: 10px !important;
  padding: 0 !important;
  border-radius: 2px !important;
  border-color: rgba(148, 163, 184, 0.46) !important;
  background: rgba(255, 255, 255, 0.96) !important;
  color: rgba(51, 65, 85, 0.96) !important;
  font-size: 8px !important;
  line-height: 1;
  opacity: 0;
  transform: none !important;
  transform-origin: 0 100% !important;
  transition: opacity 150ms ease, background-color 150ms ease, border-color 150ms ease, color 150ms ease;
  z-index: 2;
}

.markdown-preview .site-table-scroll:hover .site-rendered-table-expand-button,
.site-rendered-table-expand-button:focus-visible {
  opacity: 1;
  transform: none !important;
}

.markdown-preview.theme-dark .site-rendered-table-expand-button {
  border-color: rgba(148, 163, 184, 0.38) !important;
  background: rgba(30, 41, 59, 0.96) !important;
  color: rgba(226, 232, 240, 0.96) !important;
}

.markdown-preview .site-scrollable-table {
  width: max-content;
  min-width: 100%;
  max-width: none;
  border-collapse: collapse;
}

.markdown-preview .site-scrollable-table th,
.markdown-preview .site-scrollable-table td {
  min-width: 88px;
  max-width: 280px;
  white-space: pre-wrap;
  overflow-wrap: break-word;
  word-break: break-word;
  vertical-align: top;
}

.rendered-table-expand-overlay {
  position: fixed;
  inset: 0;
  z-index: 10030;
  display: grid;
  place-items: center;
  padding: 12px;
  background: rgba(15, 23, 42, 0.38);
  backdrop-filter: blur(8px) saturate(115%);
  animation: renderedTableOverlayIn 180ms ease both;
}

.rendered-table-expand-overlay.is-closing {
  animation: renderedTableOverlayOut 180ms ease both;
}

.rendered-table-expand-dialog {
  width: min(1680px, calc(100vw - 24px));
  height: min(88vh, 900px);
  height: min(88dvh, 900px);
  max-height: calc(100vh - 32px);
  max-height: calc(100dvh - 32px);
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  overflow: hidden;
  border: 1px solid rgba(15, 23, 42, 0.14);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.96);
  color: #111827;
  box-shadow: 0 26px 70px rgba(15, 23, 42, 0.30);
  animation: renderedTableDialogIn 180ms cubic-bezier(.2, .85, .2, 1) both;
}

.rendered-table-expand-overlay.is-closing .rendered-table-expand-dialog {
  animation: renderedTableDialogOut 180ms ease both;
}

.rendered-table-expand-overlay.is-dark .rendered-table-expand-dialog {
  border-color: rgba(255, 255, 255, 0.16);
  background: rgba(15, 23, 42, 0.96);
  color: #f8fafc;
}

.rendered-table-expand-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 14px 16px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.20);
}

.rendered-table-expand-header > div { display: grid; gap: 2px; }
.rendered-table-expand-header strong { font-size: 15px; font-weight: 700; }
.rendered-table-expand-header span { font-size: 12px; color: rgba(71, 85, 105, 0.86); }
.rendered-table-expand-overlay.is-dark .rendered-table-expand-header { border-bottom-color: rgba(255,255,255,.12); }
.rendered-table-expand-overlay.is-dark .rendered-table-expand-header span { color: rgba(203, 213, 225, 0.78); }

.rendered-table-expand-close {
  display: grid !important;
  place-items: center !important;
  position: relative;
  width: 30px !important;
  min-width: 30px !important;
  height: 30px !important;
  min-height: 30px !important;
  padding: 0 !important;
  font-size: 0;
  line-height: 1;
}

.table-expand-close-icon {
  position: relative;
  display: block;
  width: 14px;
  height: 14px;
}

.table-expand-close-icon::before,
.table-expand-close-icon::after {
  content: '';
  position: absolute;
  left: 50%;
  top: 50%;
  width: 14px;
  height: 2px;
  border-radius: 999px;
  background: currentColor;
  transform-origin: center;
}

.table-expand-close-icon::before {
  transform: translate(-50%, -50%) rotate(45deg);
}

.table-expand-close-icon::after {
  transform: translate(-50%, -50%) rotate(-45deg);
}

.rendered-table-expand-scroll {
  min-width: 0;
  min-height: 0;
  overflow: auto;
  padding: 12px;
  scrollbar-gutter: stable;
  scrollbar-width: thin;
  scrollbar-color: rgba(100, 116, 139, 0.62) rgba(148, 163, 184, 0.18);
}

.rendered-table-expand-scroll:not(.has-real-horizontal-overflow) {
  overflow-x: hidden;
}

.rendered-table-expand-scroll:not(.has-real-vertical-overflow) {
  overflow-y: hidden;
}

.rendered-table-expand-scroll::-webkit-scrollbar { width: 9px; height: 9px; }
.rendered-table-expand-scroll::-webkit-scrollbar-track { border-radius: 999px; background: rgba(148, 163, 184, 0.18); }
.rendered-table-expand-scroll::-webkit-scrollbar-thumb { border-radius: 999px; background: rgba(100, 116, 139, 0.62); }
.rendered-table-expand-scroll::-webkit-scrollbar-thumb:hover { background: rgba(71, 85, 105, 0.82); }

.rendered-table-expanded-table {
  width: max-content;
  min-width: 0;
  max-width: none;
  border-collapse: collapse;
  table-layout: fixed;
}

.rendered-table-expanded-table th,
.rendered-table-expanded-table td {
  position: relative;
  box-sizing: border-box;
  min-width: 48px;
  padding: 7px 8px;
  border: 1px solid rgba(148, 163, 184, 0.42);
  background: rgba(255, 255, 255, 0.94);
  color: inherit;
  font-weight: 400;
  white-space: pre-wrap;
  overflow-wrap: break-word;
  word-break: break-word;
  vertical-align: top;
}

.rendered-table-expand-row-resize-handle,
.rendered-table-expand-column-resize-handle {
  position: absolute;
  z-index: 4;
  display: block;
  background: transparent;
  touch-action: none;
}

.rendered-table-expand-row-resize-handle {
  left: 0;
  right: 0;
  bottom: -0.5px;
  height: 1px;
  cursor: var(--table-row-resize-cursor);
}

.rendered-table-expand-column-resize-handle {
  top: 0;
  right: -0.5px;
  bottom: 0;
  width: 1px;
  cursor: var(--table-column-resize-cursor);
}

.rendered-table-expand-row-resize-handle::after,
.rendered-table-expand-column-resize-handle::after {
  content: '';
  position: absolute;
  border-radius: 999px;
  background: rgba(249, 115, 22, 0.72);
  opacity: 0;
  transition: opacity .12s ease;
}

.rendered-table-expand-row-resize-handle::after {
  left: 0;
  right: 0;
  top: 50%;
  height: 2px;
  transform: translateY(-50%);
}

.rendered-table-expand-column-resize-handle::after {
  top: 0;
  bottom: 0;
  left: 50%;
  width: 2px;
  transform: translateX(-50%);
}

.rendered-table-expand-row-resize-handle:hover::after,
.rendered-table-expand-row-resize-handle:focus-visible::after,
.rendered-table-expand-column-resize-handle:hover::after,
.rendered-table-expand-column-resize-handle:focus-visible::after,
.rendered-table-expand-row-resize-handle.is-resizing::after,
.rendered-table-expand-column-resize-handle.is-resizing::after {
  opacity: 1;
}

body.is-resizing-rendered-table-row,
body.is-resizing-rendered-table-row * {
  cursor: var(--table-row-resize-cursor) !important;
  user-select: none !important;
}

body.is-resizing-rendered-table-column,
body.is-resizing-rendered-table-column * {
  cursor: var(--table-column-resize-cursor) !important;
  user-select: none !important;
}

.rendered-table-expand-overlay.is-dark .rendered-table-expanded-table th,
.rendered-table-expand-overlay.is-dark .rendered-table-expanded-table td {
  border-color: rgba(226, 232, 240, 0.20);
  background: rgba(30, 41, 59, 0.74);
}

.markdown-preview :deep(.site-attachment-tag),
.rendered-table-expand-scroll .site-attachment-tag {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  min-height: 24px;
  padding: 0 8px;
  border: 1px solid rgba(249, 115, 22, 0.36);
  border-radius: 8px;
  background: rgba(249, 115, 22, 0.10);
  color: #ea580c !important;
  font-size: 11px;
  font-weight: 650;
  line-height: 1;
  text-decoration: none !important;
  cursor: zoom-in;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.markdown-preview.theme-dark :deep(.site-attachment-tag),
.rendered-table-expand-overlay.is-dark .site-attachment-tag {
  border-color: rgba(251, 146, 60, 0.42);
  background: rgba(249, 115, 22, 0.18);
  color: #fed7aa !important;
}

.markdown-preview :deep(.site-attachment-tag:hover),
.markdown-preview :deep(.site-attachment-tag:focus-visible),
.rendered-table-expand-scroll .site-attachment-tag:hover,
.rendered-table-expand-scroll .site-attachment-tag:focus-visible {
  outline: none;
  border-color: rgba(249, 115, 22, 0.68);
  background: rgba(249, 115, 22, 0.18);
  color: #c2410c !important;
}

@keyframes renderedTableOverlayIn { from { opacity: 0; } to { opacity: 1; } }
@keyframes renderedTableOverlayOut { from { opacity: 1; } to { opacity: 0; } }
@keyframes renderedTableDialogIn { from { opacity: 0; } to { opacity: 1; } }
@keyframes renderedTableDialogOut { from { opacity: 1; } to { opacity: 0; } }

.markdown-preview table tbody tr {
  background-color: rgba(232, 232, 237, 0.39) !important;
}

.video-wrapper {
  position: relative;
  width: 100%;
  padding-bottom: 56.25%; /* 16:9 宽高比 */
  margin: 0.4em 0;
}

.video-wrapper iframe {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
}
.video-block {
  margin: 0.4em 0;
}
.video-fallback-card {
  display: flex;
  align-items: center;
  margin-top: 10px;
  padding: 10px;
  border-radius: 12px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: rgba(148, 163, 184, 0.08);
}
.video-fallback-content {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}
.video-fallback-title {
  font-size: 13px;
  font-weight: 600;
}
.video-fallback-link {
  display: inline-block;
  width: fit-content;
  color: #2563eb;
  font-size: 12px;
  word-break: break-all;
}
.theme-dark .video-fallback-card {
  border-color: rgba(148, 163, 184, 0.4);
  background: rgba(51, 65, 85, 0.45);
}
.theme-dark .video-fallback-link {
  color: #93c5fd;
}
.douyin-fallback-card {
  margin-top: 4px;
}
.douyin-video-wrapper {
  position: relative;
  width: 100%;
  max-width: 100%;
  padding-bottom: 0 !important; /* 覆盖 .video-wrapper 的 56.25% 撑高，避免三栏尺寸被撑乱 */
  margin: 0.4em auto;
  aspect-ratio: 16 / 9; /* PC 默认横屏 */
  height: auto;
  min-height: 0;
  max-height: 100%;
  border-radius: 12px;
  overflow: hidden;
  background: #000;
}
.douyin-video-wrapper iframe {
  position: absolute;
  inset: 0;
  width: 100% !important;
  height: 100% !important;
  display: block;
  background: #000;
}
.douyin-video-wrapper.douyin-half-canvas iframe {
  width: 200% !important;
  height: 200% !important;
  transform: scale(0.5);
  transform-origin: left top;
}
.douyin-video-wrapper .douyin-video-el {
  position: absolute;
  inset: 0;
  width: 100% !important;
  height: 100% !important;
  display: block;
  object-fit: contain;
  background: #000;
}
.douyin-video-wrapper.douyin-landscape {
  width: 100%;
  max-width: 100%;
  aspect-ratio: 16 / 9;
  height: auto;
}
:global(.layout-container.grid-3) .douyin-video-wrapper,
:global(.layout-container.grid-3) .markdown-preview .douyin-video-wrapper,
:global(.feed-grid-three) .douyin-video-wrapper,
.douyin-video-wrapper.douyin-three-col {
  width: 100%;
  max-width: 100%;
  aspect-ratio: 16 / 9;
  height: auto;
  margin: 0.4em 0;
}
.douyin-video-wrapper.douyin-three-col.douyin-mobile-portrait,
:global(.layout-container.grid-3) .douyin-video-wrapper.douyin-mobile-portrait,
:global(.feed-grid-three) .douyin-video-wrapper.douyin-mobile-portrait {
  width: 100%;
  max-width: 100%;
  aspect-ratio: 16 / 9;
}
.image-grid-item .douyin-video-wrapper {
  width: 100%;
  max-width: 100%;
}
.douyin-video-wrapper.douyin-mobile-portrait {
  width: 100% !important;
  max-width: 100% !important;
  aspect-ratio: 9 / 16 !important;
  border-radius: 10px;
}

.music-wrapper {
  width: 100%;
  margin: 0.4em 0;
}

.spotify-wrapper {
  width: 100%;
  margin: 0.4em 0;
}

.spotify-wrapper iframe {
  width: 100%;
  height: 352px;
}

.markdown-preview :deep(img) {
  max-width: 100%;
  height: auto;
  display: block;
  margin: 6px auto !important;
}
.markdown-preview :deep(p > img),
.markdown-preview :deep(p > a > img) {
  margin-top: 6px !important;
  margin-bottom: 6px !important;
}
.markdown-preview :deep(.image-grid img) {
  margin: 0 !important;
}

.image-loading-placeholder {
  display: block;
  width: 100%;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  text-align: center;
  font-size: 0.875rem;
  color: #6b7280;
  background-color: rgba(0,0,0,0.05);
}
.theme-dark .image-loading-placeholder {
  color: #cbd5e1;
  background-color: rgba(255,255,255,0.08);
}
.image-loading-error {
  color: #ef4444;
}

.markdown-preview :deep(.fancybox-video-trigger) {
  cursor: zoom-in;
}

.markdown-preview :deep(video),
.markdown-preview :deep(audio) {
  display: block;
  width: 100%;
  margin: 0.4em 0;
}

.markdown-preview :deep(.site-attachment-render) {
  display: block;
  margin: 0.45em 0;
}

.markdown-preview :deep(.site-attachment-render--image) {
  width: fit-content;
  max-width: 100%;
}

.markdown-preview :deep(.site-attachment-render--image img) {
  display: block;
  max-width: 100%;
  height: auto;
  border-radius: 8px;
}

/* 视觉令牌与内部结构统一由 assets/css/attachment-failure.css 提供，这里只补正文里的尺寸。 */
.markdown-preview .site-attachment-failure,
.rendered-table-expand-scroll .site-attachment-failure {
  width: calc(100% - 16px);
  min-height: 176px;
  max-width: calc(100% - 16px);
  margin: 8px;
}

.markdown-preview .image-grid-item.site-attachment-failure,
.rendered-table-expand-scroll .image-grid-item.site-attachment-failure {
  width: 100%;
  min-height: 100%;
  max-width: 100%;
  margin: 0;
}

/*
 * 占位块长在 .single-media 包装层上时，height 必须显式复位：
 * 该包装层可能带 inline-image-thumb 的固定 96px，导致占位块内容被 overflow:hidden 裁断。
 * 这里也不能用 min-height:100%——父级是 auto 高度，百分比会退化成 0，
 * 反而盖掉基础规则的 176px 与视频的 clamp() 下限。
 */
.markdown-preview .single-media.site-attachment-failure,
.rendered-table-expand-scroll .single-media.site-attachment-failure {
  width: calc(100% - 16px);
  height: auto;
  max-width: calc(100% - 16px);
  margin: 8px;
}

.markdown-preview .site-attachment-failure--video,
.rendered-table-expand-scroll .site-attachment-failure--video {
  min-height: clamp(190px, 42vw, 420px);
}

.markdown-preview .site-attachment-failure__poster,
.rendered-table-expand-scroll .site-attachment-failure__poster {
  position: absolute;
  z-index: 0;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
  transform: none !important;
  filter: saturate(0.82) brightness(0.76);
  box-shadow: none !important;
}

.markdown-preview .site-attachment-failure__scrim,
.rendered-table-expand-scroll .site-attachment-failure__scrim {
  position: absolute;
  z-index: 1;
  inset: 0;
  display: none;
  background: linear-gradient(180deg, rgba(2, 6, 23, 0.10) 16%, rgba(2, 6, 23, 0.82) 100%);
}

.markdown-preview .site-attachment-failure--video.site-attachment-failure--with-poster,
.rendered-table-expand-scroll .site-attachment-failure--video.site-attachment-failure--with-poster {
  place-items: end start;
  border-color: rgba(255, 255, 255, 0.16);
  background: #17191d;
}

.markdown-preview .site-attachment-failure--with-poster .site-attachment-failure__scrim,
.rendered-table-expand-scroll .site-attachment-failure--with-poster .site-attachment-failure__scrim {
  display: block;
}

.markdown-preview .site-attachment-failure--with-poster .site-attachment-failure__content,
.rendered-table-expand-scroll .site-attachment-failure--with-poster .site-attachment-failure__content {
  max-width: 100%;
  padding: 22px;
  align-items: flex-start;
  text-align: left;
}

.markdown-preview .site-attachment-failure--with-poster .site-attachment-failure__icon,
.rendered-table-expand-scroll .site-attachment-failure--with-poster .site-attachment-failure__icon {
  width: 38px;
  height: 38px;
  margin-bottom: 10px;
  border-color: rgba(255, 255, 255, 0.20);
  background: rgba(15, 23, 42, 0.62);
  color: #ffffff;
  backdrop-filter: blur(8px);
}

.markdown-preview .site-attachment-failure--with-poster .site-attachment-failure__title,
.rendered-table-expand-scroll .site-attachment-failure--with-poster .site-attachment-failure__title {
  color: #ffffff;
}

.markdown-preview .site-attachment-failure--with-poster .site-attachment-failure__detail,
.rendered-table-expand-scroll .site-attachment-failure--with-poster .site-attachment-failure__detail {
  color: rgba(255, 255, 255, 0.78);
}

@media (max-width: 520px) {
  .markdown-preview .image-grid > .image-grid-item.site-attachment-failure,
  .rendered-table-expand-scroll .image-grid > .image-grid-item.site-attachment-failure {
    grid-column: 1 / -1;
    min-height: 164px;
    aspect-ratio: 16 / 9 !important;
  }

  .markdown-preview .site-attachment-failure .site-attachment-failure__content,
  .rendered-table-expand-scroll .site-attachment-failure .site-attachment-failure__content {
    max-width: 94%;
    padding: 16px;
  }

  .markdown-preview .site-attachment-failure--with-poster .site-attachment-failure__content,
  .rendered-table-expand-scroll .site-attachment-failure--with-poster .site-attachment-failure__content {
    padding: 16px;
  }
}

.markdown-preview .site-attachment-file,
.rendered-table-expand-scroll .site-attachment-file,
.markdown-preview .github-card,
.rendered-table-expand-scroll .github-card {
  --file-card-bg: #ffffff;
  --file-card-bg-hover: #f8fafc;
  --file-card-border: rgba(15, 23, 42, 0.10);
  --file-card-shadow: 0 14px 30px rgba(15, 23, 42, 0.12);
  --file-card-icon-bg: rgba(15, 23, 42, 0.06);
  --file-card-icon-border: rgba(15, 23, 42, 0.08);
  --file-card-icon: #374151;
  --file-card-text: #111111;
  --file-card-name: #000000;
  --file-card-meta: #6b7280;
  --file-card-action: #374151;
}

.markdown-preview .site-attachment-file,
.rendered-table-expand-scroll .site-attachment-file {
  display: grid !important;
  grid-template-columns: 44px minmax(0, 1fr) 28px !important;
  align-items: center !important;
  gap: 12px !important;
  width: calc(100% - 16px) !important;
  min-height: 72px !important;
  max-width: calc(100% - 16px) !important;
  margin: 6px 8px !important;
  padding: 12px !important;
  border: 1px solid var(--file-card-border) !important;
  border-radius: 12px !important;
  background: var(--file-card-bg) !important;
  color: var(--file-card-text) !important;
  font-family: inherit !important;
  font-size: 16px !important;
  font-weight: 400 !important;
  line-height: 1.6 !important;
  text-decoration: none !important;
  box-shadow: var(--file-card-shadow) !important;
  box-sizing: border-box !important;
  overflow: visible !important;
  cursor: pointer;
  transition: background-color .16s ease;
}

.markdown-preview .site-attachment-file:hover,
.markdown-preview .site-attachment-file:active,
.markdown-preview .site-attachment-file:focus-visible,
.rendered-table-expand-scroll .site-attachment-file:hover,
.rendered-table-expand-scroll .site-attachment-file:active,
.rendered-table-expand-scroll .site-attachment-file:focus-visible {
  background: var(--file-card-bg-hover) !important;
  color: inherit !important;
  text-decoration: none !important;
  outline: none;
}

.markdown-preview .site-attachment-file:focus-visible,
.rendered-table-expand-scroll .site-attachment-file:focus-visible {
  box-shadow: var(--file-card-shadow), 0 0 0 2px rgba(249, 115, 22, 0.18) !important;
}

.markdown-preview .site-attachment-file--deleted,
.rendered-table-expand-scroll .site-attachment-file--deleted {
  --file-card-bg: #fffaf7;
  --file-card-bg-hover: #fffaf7;
  --file-card-border: rgba(194, 65, 12, 0.18);
  --file-card-icon-bg: rgba(234, 88, 12, 0.10);
  --file-card-icon-border: rgba(194, 65, 12, 0.16);
  --file-card-icon: #c2410c;
  --file-card-name: #7c2d12;
  --file-card-meta: #9a3412;
  --file-card-action: #c2410c;
  cursor: default;
}

.markdown-preview .site-attachment-file--deleted .site-attachment-file__action,
.rendered-table-expand-scroll .site-attachment-file--deleted .site-attachment-file__action {
  width: 24px !important;
  height: 24px !important;
  border: 1px solid currentColor;
  border-radius: 999px;
  opacity: .72;
}

.markdown-preview .site-attachment-file--deleted .site-attachment-file__action::before,
.rendered-table-expand-scroll .site-attachment-file--deleted .site-attachment-file__action::before {
  content: '!';
  font-size: 14px;
  font-weight: 700;
  line-height: 1;
}

.markdown-preview .site-attachment-file + p,
.rendered-table-expand-scroll .site-attachment-file + p {
  margin-top: 0 !important;
}

.markdown-preview .site-attachment-file + .site-attachment-file,
.rendered-table-expand-scroll .site-attachment-file + .site-attachment-file {
  margin-top: 10px !important;
}

.markdown-preview p:has(+ .site-attachment-file),
.rendered-table-expand-scroll p:has(+ .site-attachment-file) {
  margin-bottom: 6px !important;
}

.markdown-preview .site-attachment-file__icon,
.rendered-table-expand-scroll .site-attachment-file__icon {
  position: relative;
  display: inline-flex !important;
  align-items: center !important;
  justify-content: center !important;
  width: 44px !important;
  height: 44px !important;
  border: 1px solid var(--file-card-icon-border) !important;
  border-radius: 12px !important;
  background: var(--file-card-icon-bg) !important;
  color: var(--file-card-icon) !important;
  flex: 0 0 auto !important;
}

.markdown-preview .site-attachment-file__icon::before,
.rendered-table-expand-scroll .site-attachment-file__icon::before {
  content: '';
  display: block;
  width: 17px;
  height: 21px;
  border: 1.6px solid currentColor;
  border-radius: 3px;
  box-sizing: border-box;
}

.markdown-preview .site-attachment-file__icon::after,
.rendered-table-expand-scroll .site-attachment-file__icon::after {
  content: '';
  position: absolute;
  top: 12px;
  right: 13px;
  width: 7px;
  height: 7px;
  border-left: 1.6px solid currentColor;
  border-bottom: 1.6px solid currentColor;
  background: var(--file-card-icon-bg);
  transform: rotate(-180deg);
}

.markdown-preview .site-attachment-file__body,
.rendered-table-expand-scroll .site-attachment-file__body {
  display: flex !important;
  min-width: 0 !important;
  flex-direction: column !important;
  justify-content: center !important;
  gap: 4px !important;
}

.markdown-preview .site-attachment-file__name,
.rendered-table-expand-scroll .site-attachment-file__name {
  display: block !important;
  min-width: 0 !important;
  color: var(--file-card-name) !important;
  font-size: 14px !important;
  font-weight: 400 !important;
  line-height: 1.35 !important;
  overflow-wrap: anywhere !important;
  word-break: break-word !important;
}

.markdown-preview .site-attachment-file__meta,
.rendered-table-expand-scroll .site-attachment-file__meta {
  display: block !important;
  color: var(--file-card-meta) !important;
  font-size: 12px !important;
  line-height: 1.35 !important;
  text-transform: uppercase !important;
}

/*
 * 已删除附件的卡片（音频/其他文件）与图片、视频的失败占位块讲的是同一件事，文案排版必须一致。
 * 卡片默认的 __name/__meta 是为"文件名 + 扩展名"设计的（14px/400、扩展名大写），
 * 用在失败文案上就会比 .site-attachment-failure__title/__detail 更小更轻。
 * 这里对齐 assets/css/attachment-failure.css 里的同一套排版令牌：标题 15px/600、
 * 说明 12px 且间距 5px，并去掉只对扩展名有意义的 uppercase。
 */
.markdown-preview .site-attachment-file--deleted .site-attachment-file__body,
.rendered-table-expand-scroll .site-attachment-file--deleted .site-attachment-file__body {
  gap: 0 !important;
}

.markdown-preview .site-attachment-file--deleted .site-attachment-file__name,
.rendered-table-expand-scroll .site-attachment-file--deleted .site-attachment-file__name {
  font-size: 15px !important;
  font-weight: 600 !important;
  line-height: 1.4 !important;
}

.markdown-preview .site-attachment-file--deleted .site-attachment-file__meta,
.rendered-table-expand-scroll .site-attachment-file--deleted .site-attachment-file__meta {
  margin-top: 5px !important;
  font-size: 12px !important;
  font-weight: 400 !important;
  line-height: 1.5 !important;
  text-transform: none !important;
}

.markdown-preview .site-attachment-file__action,
.rendered-table-expand-scroll .site-attachment-file__action {
  position: relative;
  display: inline-flex !important;
  align-items: center !important;
  justify-content: center !important;
  width: 28px !important;
  height: 28px !important;
  color: var(--file-card-action) !important;
  flex: 0 0 auto !important;
  opacity: .78;
}

.markdown-preview .site-attachment-file__action--download::before,
.rendered-table-expand-scroll .site-attachment-file__action--download::before {
  content: '';
  width: 10px;
  height: 10px;
  border-right: 1.8px solid currentColor;
  border-bottom: 1.8px solid currentColor;
  transform: translateY(-3px) rotate(45deg);
  box-sizing: border-box;
}

.markdown-preview .site-attachment-file__action--download::after,
.rendered-table-expand-scroll .site-attachment-file__action--download::after {
  content: '';
  position: absolute;
  bottom: 5px;
  left: 50%;
  width: 16px;
  height: 1.8px;
  border-radius: 999px;
  background: currentColor;
  transform: translateX(-50%);
}

.markdown-preview .site-attachment-file__action--preview::before,
.rendered-table-expand-scroll .site-attachment-file__action--preview::before {
  content: '';
  width: 19px;
  height: 12px;
  border: 1.8px solid currentColor;
  border-radius: 999px / 70%;
  box-sizing: border-box;
}

.markdown-preview .site-attachment-file__action--preview::after,
.rendered-table-expand-scroll .site-attachment-file__action--preview::after {
  content: '';
  position: absolute;
  width: 5px;
  height: 5px;
  border-radius: 999px;
  background: currentColor;
}

.markdown-preview.theme-dark .site-attachment-file,
.rendered-table-expand-overlay.is-dark .rendered-table-expand-scroll .site-attachment-file,
.markdown-preview.theme-dark .github-card,
.rendered-table-expand-overlay.is-dark .github-card {
  --file-card-bg: rgba(15, 23, 42, 0.52);
  --file-card-bg-hover: rgba(30, 41, 59, 0.68);
  --file-card-border: rgba(255, 255, 255, 0.12);
  --file-card-shadow: 0 16px 32px rgba(2, 6, 23, 0.52);
  --file-card-icon-bg: rgba(255, 255, 255, 0.06);
  --file-card-icon-border: rgba(255, 255, 255, 0.12);
  --file-card-icon: #cbd5e1;
  --file-card-text: #ffffff;
  --file-card-name: #ffffff;
  --file-card-meta: #9ca3af;
  --file-card-action: #cbd5e1;
}

.markdown-preview.theme-dark .site-attachment-file--deleted,
.rendered-table-expand-overlay.is-dark .rendered-table-expand-scroll .site-attachment-file--deleted {
  --file-card-bg: #241d1a;
  --file-card-bg-hover: #241d1a;
  --file-card-border: rgba(251, 146, 60, 0.22);
  --file-card-icon-bg: rgba(251, 146, 60, 0.13);
  --file-card-icon-border: rgba(251, 146, 60, 0.20);
  --file-card-icon: #fb923c;
  --file-card-name: #fed7aa;
  --file-card-meta: #fdba74;
  --file-card-action: #fb923c;
}

@media (max-width: 520px) {
  .markdown-preview .site-attachment-file,
  .rendered-table-expand-scroll .site-attachment-file {
    grid-template-columns: 40px minmax(0, 1fr) 24px !important;
    gap: 10px !important;
    min-height: 64px !important;
    padding: 10px !important;
    width: calc(100% - 16px) !important;
    max-width: calc(100% - 16px) !important;
    margin: 6px 8px !important;
  }

  .markdown-preview .site-attachment-file__icon,
  .rendered-table-expand-scroll .site-attachment-file__icon {
    width: 40px !important;
    height: 40px !important;
  }
}

.markdown-preview :deep(.site-attachment-render--video video) {
  margin: 0;
}

.markdown-preview :deep(pre) {
  overflow-x: auto;
  border-radius: 6px;
  padding: 16px;
  margin: 1em 0;
  max-width: 100%;
  white-space: pre-wrap;
  word-wrap: break-word;
  box-sizing: border-box;
}
.theme-dark.markdown-preview :deep(pre) {
  background-color: #0d1117;
  border: 1px solid #30363d;
}
.theme-light.markdown-preview :deep(pre) {
  background-color: #f5f5f5;
  border: 1px solid #e5e7eb;
}


.markdown-preview :deep(.hljs) {
  background-color: transparent;
  padding: 0;
}
.theme-dark.markdown-preview :deep(.hljs) { color: #c9d1d9; }
.theme-light.markdown-preview :deep(.hljs) { color: #1f2937; }

.markdown-preview :deep(.hljs-keyword) {
  color: #ff7b72;
}

.markdown-preview :deep(.hljs-string) {
  color: #a5d6ff;
}

.markdown-preview :deep(.hljs-comment) {
  color: #8b949e;
  font-style: italic;
}

.markdown-preview :deep(.hljs-function) {
  color: #d2a8ff;
}

.markdown-preview :deep(.hljs-number) {
  color: #79c0ff;
}

.markdown-preview :deep(.hljs-operator) {
  color: #ff7b72;
}

.markdown-preview :deep(.hljs-class) {
  color: #ffa657;
}

.markdown-preview :deep(.hljs-variable) {
  color: #ffa657;
}

.markdown-preview :deep(.hljs-line-numbers) {
  border-right: 1px solid #30363d;
  padding-right: 1em;
  margin-right: 1em;
  color: #6e7681;
  -webkit-user-select: none;
  user-select: none;
}

.markdown-preview :deep(blockquote) {
  border-left: 4px solid #14141484;
  margin: 1em 0;
  padding: 0.5em 1em;
  background-color: rgba(0, 0, 0, 0.05);
}

.markdown-preview :deep(a:not(.site-attachment-file)) {
  color: #0366d6 !important; 
  text-decoration: none !important; 
  background-color: transparent !important;
  padding: 0 !important;
  border-radius: 0 !important;
  border: none !important;
  text-shadow: none !important;
}
.markdown-preview :deep(a:not(.site-attachment-file):hover) {
  text-decoration: underline !important; 
  color: #1d4ed8 !important;
}
.theme-light.markdown-preview :deep(a:not(.site-attachment-file)),
.theme-dark.markdown-preview :deep(a:not(.site-attachment-file)) {
  color: #0366d6 !important;
  background-color: transparent !important;
  padding: 0 !important;
  border-radius: 0 !important;
  border: none !important;
  text-shadow: none !important;
}
.builtin-comments .markdown-preview.theme-light a { 
  color: #0366d6 !important; 
  text-decoration: none !important;
  background-color: transparent !important;
  padding: 0 !important;
  border-radius: 0 !important;
  border: none !important;
  text-shadow: none !important;
}
.builtin-comments .markdown-preview.theme-dark a { 
  color: #0366d6 !important; 
  text-decoration: none !important;
  background-color: transparent !important;
  padding: 0 !important;
  border-radius: 0 !important;
  border: none !important;
  text-shadow: none !important;
}
.builtin-comments .markdown-preview.theme-light a:hover,
.builtin-comments .markdown-preview.theme-dark a:hover { 
  text-decoration: underline !important;
  color: #1d4ed8 !important;
}

.markdown-preview :deep(table) {
  border-collapse: collapse;
  width: 100%;
  margin: 1em 0;
}

.markdown-preview :deep(th),
.markdown-preview :deep(td) {
  border: 1px solid #ddd;
  padding: 8px;
  text-align: left;
}

.markdown-preview :deep(ul),
.markdown-preview :deep(ol) {
  padding-left: 2em;
}

.markdown-preview :deep(hr) {
  border: none;
  border-top: 1px solid #ddd;
  margin: 1em 0;
}
.music-wrapper {
  width: 100%;
  margin: 0.4em 0;
  max-width: 800px;
  margin-left: auto;
  margin-right: auto;
}

.aplayer { box-shadow: 0 0 10px rgba(0,0,0,0.1); border-radius: 4px; margin: 0.5em 0 !important; }
.theme-dark .aplayer { 
  background: rgba(22,27,34,0.85) !important; 
  color: #c9d1d9 !important; 
  border: 1px solid rgba(255,255,255,0.1) !important;
}
.theme-light .aplayer { 
  background: rgba(255,255,255,0.95) !important; 
  color: #111827 !important; 
  border: 1px solid #e5e7eb !important; 
  box-shadow: 0 4px 12px rgba(0,0,0,0.08) !important;
}
.theme-light .aplayer .aplayer-title,
.theme-light .aplayer .aplayer-author,
.theme-light .aplayer .aplayer-lrc p { color: #1f2937 !important; }
.theme-light .aplayer .aplayer-bar-wrap .aplayer-bar { background-color: #e5e7eb !important; }
.theme-light .aplayer .aplayer-played { background-color: #3b82f6 !important; }
.theme-light .aplayer .aplayer-loaded { background-color: #9ca3af !important; }
.theme-light .aplayer .aplayer-info { color: #111827 !important; }
.theme-light .aplayer .aplayer-icon,
.theme-light .aplayer .aplayer-list-index { color: #374151 !important; }
.theme-dark .aplayer .aplayer-title,
.theme-dark .aplayer .aplayer-author,
.theme-dark .aplayer .aplayer-lrc p { color: #ffffff !important; }
.theme-dark .aplayer .aplayer-bar-wrap .aplayer-bar { background-color: #30363d !important; }
.theme-dark .aplayer .aplayer-played { background-color: #60a5fa !important; }
.theme-dark .aplayer .aplayer-loaded { background-color: #64748b !important; }
.theme-dark .aplayer .aplayer-info { color: #e5e7eb !important; }
.theme-dark .aplayer .aplayer-icon,
.theme-dark .aplayer .aplayer-list-index { color: #e5e7eb !important; }

/* 视频和音频播放器的主题适配 */
.theme-light video {
  background-color: #ffffff !important;
  border: 1px solid #e5e7eb !important;
  border-radius: 8px !important;
}

.theme-light audio {
  background-color: #ffffff !important;
  border: none !important;
  border-radius: 8px !important;
}

.theme-dark video {
  background-color: #202a36 !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
  border-radius: 8px !important;
}

.theme-dark audio {
  background-color: #202a36 !important;
  border: none !important;
  border-radius: 8px !important;
}

/* iframe 嵌入内容的主题适配 */
.theme-light iframe {
  border: 1px solid #e5e7eb !important;
  border-radius: 8px !important;
}

.theme-dark iframe {
  border: 1px solid rgba(255,255,255,0.10) !important;
  border-radius: 8px !important;
}
/* 添加 medium-zoom 相关样式 */
.medium-zoom-overlay {
  z-index: 999;
}

.medium-zoom-image {
  cursor: pointer;
  transition: transform 0.3s cubic-bezier(0.2, 0, 0.2, 1) !important;
}

.medium-zoom-image--opened {
  z-index: 1000;
}
/* 图像宫格布局样式 */
.image-grid {
  display: grid;
  gap: 4px;
  width: 100%;
  margin: 0.5em 0;
}
.image-grid.cols-2 {
  grid-template-columns: repeat(2, 1fr);
}
.image-grid.cols-3 {
  grid-template-columns: repeat(3, 1fr);
}
.image-grid-item {
  position: relative;
  width: 100%;
  overflow: hidden;
  border-radius: 4px;
}
.image-grid-item img,
.image-grid-item video,
.image-grid-item .video-wrapper,
.image-grid-item iframe,
.image-grid-item a {
  width: 100%;
  height: 100% !important;
  object-fit: cover;
  display: block;
}

/* 宫格内视频容器覆盖默认样式 */
.image-grid-item .video-wrapper {
  padding-bottom: 0 !important;
  height: 100% !important;
}

/* 宽高比自适应类 */
.ar-169 { aspect-ratio: 16/9; }
.ar-34 { aspect-ratio: 3/4; }
.ar-11 { aspect-ratio: 1/1; }

.single-media {
  width: 100%;
  margin: 0.5em 0;
}
.single-media.ar-11 {
  overflow: hidden;
  border-radius: 12px;
}
.single-media.ar-11 img,
.single-media.ar-11 video,
.single-media.ar-11 a,
.single-media.ar-11 .video-wrapper,
.single-media.ar-11 iframe {
  width: 100%;
  height: 100% !important;
  display: block;
}

.markdown-preview .inline-image-thumb,
.rendered-table-expanded-table .inline-image-thumb {
  width: var(--inline-image-thumb-size);
  height: var(--inline-image-thumb-size);
  max-width: 100%;
  margin: 6px 0;
  overflow: hidden;
  border-radius: 10px;
  display: block;
}

.markdown-preview .inline-image-thumb > a,
.markdown-preview .inline-image-thumb > img,
.rendered-table-expanded-table .inline-image-thumb > a,
.rendered-table-expanded-table .inline-image-thumb > img {
  display: block;
  width: 100% !important;
  height: 100% !important;
}

.markdown-preview .inline-image-thumb img,
.rendered-table-expanded-table .inline-image-thumb img {
  width: 100% !important;
  height: 100% !important;
  min-height: 0 !important;
  margin: 0 !important;
  object-fit: cover !important;
  object-position: center;
  border-radius: inherit;
}

.markdown-preview :deep(.full-image-attachment) {
  width: 100%;
  max-width: 100%;
  margin: 8px 0;
  overflow: visible;
}

.markdown-preview :deep(.full-image-attachment > a) {
  display: block;
  width: 100%;
  max-width: 100%;
}

.markdown-preview :deep(.full-image-attachment img) {
  display: block;
  width: auto !important;
  max-width: 100% !important;
  height: auto !important;
  min-height: 0 !important;
  margin: 0 !important;
  object-fit: contain !important;
  object-position: center;
  border-radius: 12px;
  contain-intrinsic-size: auto !important;
}

.github-card {
  display: block;
  border-radius: 12px;
  margin: 6px 8px;
  padding: 12px;
  width: calc(100% - 16px);
  max-width: calc(100% - 16px);
  min-height: 72px;
  border: 1px solid var(--file-card-border);
  background: var(--file-card-bg);
  color: var(--file-card-text);
  box-shadow: var(--file-card-shadow);
  font-size: 16px;
  box-sizing: border-box;
  min-width: 0;
  transition: background-color .16s ease;
}
.github-card:hover,
.github-card:focus-within { background: var(--file-card-bg-hover); }
.markdown-preview .github-card a.github-card-title,
.rendered-table-expand-scroll .github-card a.github-card-title {
  color: var(--file-card-name) !important;
  font-size: 14px !important;
  font-weight: 400 !important;
  line-height: 1.35 !important;
  text-decoration: none !important;
}
.github-card-header { display: grid; grid-template-columns: 44px minmax(0, 1fr); min-height: 46px; column-gap: 12px; align-items: center; justify-content: start; }
.gh-avatar-slot { position: relative; width: 40px; height: 40px; }
.github-card-avatar { width: 40px; height: 40px; border-radius: 10px; object-fit: cover; background: #222; }
.github-card .github-card-avatar { position: absolute; inset: 0; margin: 0 !important; }
.avatar-fallback { width: 40px; height: 40px; border-radius: 10px; background: #0366d6; color: #ffffff; display: none; align-items: center; justify-content: center; font-size: 15px; font-weight: 600; }
.gh-badge { position: absolute; right: -5px; bottom: -5px; width: 16px; height: 16px; border-radius: 50%; padding: 2px; }
.github-card .gh-badge { width: 20px; height: 20px; box-sizing: border-box; }
.theme-light .gh-badge { background: #ffffff; border: 1px solid rgba(0,0,0,0.12); fill: #161b22; }
.theme-dark .gh-badge { background: #161b22; border: 1px solid #30363d; fill: #c9d1d9; }
.github-card-header > div {
  flex: 1 1 0%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
}
.github-card-title {
  min-width: 0;
  font-weight: bold;
  text-decoration: none;
  font-size: 17px;
  word-break: break-all;
  white-space: pre-line;
  overflow-wrap: anywhere;
}
.theme-dark .github-card-title { color: #58a6ff !important; }
.theme-light .github-card-title { color: #0366d6 !important; }
.github-card-desc {
  margin-top: 4px;
  font-size: 14px;
  word-break: break-all;
  white-space: pre-line;
  overflow-wrap: anywhere;
}
.theme-dark .github-card-desc { color: #8b949e !important; }
.theme-light .github-card-desc { color: #6b7280 !important; }
.github-card-footer {
  margin-top: 12px;
  display: flex;
  gap: 16px;
  font-size: 13px;
  flex-wrap: wrap;
}
.theme-dark .github-card-footer { color: #8b949e !important; }
.theme-light .github-card-footer { color: #6b7280 !important; }

.github-card-footer .gh-pill { display: inline-flex; align-items: center; gap: 6px; padding: 4px 8px; border-radius: 6px; }
.gh-icon { width: 14px; height: 14px; display: inline-block; }
.theme-dark .github-card-footer span { 
  background: transparent !important;
  color: inherit !important;
  border: none !important;
  text-shadow: none !important;
}
.theme-light .github-card-footer span { 
  background: transparent !important;
  color: inherit !important;
  border: none !important;
  text-shadow: none !important;
}

.theme-dark.markdown-preview :deep(p) {
  text-shadow: -1px -1px 0 rgba(0,0,0,0.6), 1px -1px 0 rgba(0,0,0,0.6), -1px 1px 0 rgba(0,0,0,0.6), 1px 1px 0 rgba(0,0,0,0.6);
}
.theme-light.markdown-preview :deep(p) {
  text-shadow: none;
}
/* 白天模式下内容区链接颜色加深为深橙色 */
.theme-light.markdown-preview :deep(a:not(.site-attachment-file)) {
  color: #0366d6;
}
/* 图片悬停与盒子效果（与内容样式一致） */
.markdown-preview :deep(img) {
  border-radius: 12px;
  display: block;
  width: 100%;
  height: auto;
  box-shadow: 0 1px 2px rgba(0,0,0,0.10);
  transition: transform .18s ease, box-shadow .18s ease, filter .18s ease;
}
.markdown-preview :deep(img:hover) {
  transform: translate3d(0,0,0) scale(1.02);
  box-shadow: 0 6px 18px rgba(0,0,0,0.28);
  filter: saturate(1.06) contrast(1.02);
}
.image-grid-item img {
  border-radius: 12px;
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
  box-shadow: 0 1px 2px rgba(0,0,0,0.10);
  transition: transform .18s ease, box-shadow .18s ease, filter .18s ease;
}
.image-grid-item img:hover {
  transform: translate3d(0,0,0) scale(1.02);
  box-shadow: 0 6px 18px rgba(0,0,0,0.28);
  filter: saturate(1.06) contrast(1.02);
}
@media (prefers-color-scheme: dark) {
  .markdown-preview :deep(img) { box-shadow: 0 1px 2px rgba(255,255,255,0.06); }
  .markdown-preview :deep(img:hover) { box-shadow: 0 8px 22px rgba(255,255,255,0.12); }
  .image-grid-item img { box-shadow: 0 1px 2px rgba(255,255,255,0.06); }
  .image-grid-item img:hover { box-shadow: 0 8px 22px rgba(255,255,255,0.12); }
}

.theme-dark.markdown-preview :deep(a:not(.site-attachment-file)),
.theme-light.markdown-preview :deep(a:not(.site-attachment-file)),
:global(html.dark) .markdown-preview :deep(a:not(.site-attachment-file)),
:global(html:not(.dark)) .markdown-preview :deep(a:not(.site-attachment-file)) {
  color: #0366d6 !important;
}

/* Image Grid Layout Styles */
.image-grid {
  display: grid;
  gap: 4px;
  width: 100%;
  margin: 0.5em 0;
}

.image-grid.cols-2 {
  grid-template-columns: repeat(2, 1fr);
}

.image-grid.cols-3 {
  grid-template-columns: repeat(3, 1fr);
}

.image-grid-item {
  position: relative;
  width: 100%;
  height: 0;
  padding-bottom: 100%; /* Default square */
  overflow: hidden;
  border-radius: 8px;
}

.image-grid-item.ar-169 {
  padding-bottom: 56.25%; /* 16:9 */
}

.image-grid-item.ar-34 {
  padding-bottom: 133.33%; /* 3:4 */
}

.image-grid-item.ar-11 {
  padding-bottom: 100%; /* 1:1 */
}

.image-grid-item > * {
  position: absolute;
  top: 0;
  left: 0;
  width: 100% !important;
  height: 100% !important;
  object-fit: cover;
}

/* Fix for video-wrapper inside grid */
.image-grid-item .video-wrapper {
  padding-bottom: 0 !important;
  height: 100% !important;
  margin: 0 !important;
}

.image-grid-item .video-wrapper iframe {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
}
.markdown-preview :deep(a:not(.site-attachment-file)) {
  background-color: transparent !important;
  padding: 0 !important;
  border-radius: 0 !important;
  border: none !important;
  text-shadow: none !important;
  color: #0366d6 !important;
}

.theme-dark.markdown-preview :deep(a:not(.site-attachment-file):hover),
.theme-light.markdown-preview :deep(a:not(.site-attachment-file):hover),
:global(html.dark) .markdown-preview :deep(a:not(.site-attachment-file):hover),
:global(html:not(.dark)) .markdown-preview :deep(a:not(.site-attachment-file):hover),
.markdown-preview :deep(a:not(.site-attachment-file):hover) {
  color: #1d4ed8 !important;
  text-decoration: underline !important;
}
.github-card-loading {
  font-style: italic;
}
.theme-dark .github-card-loading { color: #8b949e; }
.theme-light .github-card-loading { color: #6b7280; }

/* 占位符和错误状态样式 */
.placeholder-avatar {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: linear-gradient(135deg, #f0f0f0, #e0e0e0);
  animation: pulse 1.5s infinite;
}
.theme-dark .placeholder-avatar {
  background: linear-gradient(135deg, #30363d, #262c36);
}

.placeholder-text {
  animation: pulse 1.5s infinite;
  border-radius: 4px;
  height: 16px;
  background: linear-gradient(90deg, #f0f0f0 25%, #e0e0e0 50%, #f0f0f0 75%);
  background-size: 200% 100%;
  animation: loading-shimmer 1.5s infinite;
}
.theme-dark .placeholder-text {
  background: linear-gradient(90deg, #30363d 25%, #262c36 50%, #30363d 75%);
  background-size: 200% 100%;
}

.error-avatar {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background-color: #ff6b6b;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  color: white;
}

.github-card-error {
  opacity: 0.8;
  border: 2px dashed #ff6b6b;
}

.github-card-loaded {
  opacity: 1;
}

/* 加载动画 */
@keyframes pulse {
  0% { opacity: 0.8; }
  50% { opacity: 1; }
  100% { opacity: 0.8; }
}

@keyframes loading-shimmer {
  0% { background-position: -200% 0; }
  100% { background-position: 200% 0; }
}
@media (max-width: 520px) {
  .github-card {
    min-height: 64px;
    padding: 10px;
    font-size: 14px;
  }
  .github-card-header { grid-template-columns: 40px minmax(0, 1fr); min-height: 42px; column-gap: 10px; }
  .github-card-avatar {
    width: 36px;
    height: 36px;
  }
  .github-card-title {
    font-size: 15px;
  }
}
.image-grid {
  display: grid;
  gap: 6px;
  margin: 0;
  width: 100%;
  grid-auto-flow: dense;
  align-items: stretch;
  justify-items: stretch;
}
.image-grid.cols-2 { grid-template-columns: repeat(2, 1fr); }
.image-grid.cols-3 { grid-template-columns: repeat(3, 1fr); }
.image-grid-item {
  position: relative;
  aspect-ratio: 1 / 1;
  overflow: hidden;
  border-radius: 10px;
}
.image-grid-item > a { display: block; width: 100%; height: 100%; }
.image-grid-item > a > img { width: 100%; height: 100%; object-fit: cover; object-position: center; display: block; }
.image-grid-item.ar-169 { aspect-ratio: 16 / 9; }
.image-grid-item.ar-34 { aspect-ratio: 3 / 4; }
.image-grid-item.ar-11 { aspect-ratio: 1 / 1; }
.image-grid-item img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center center;
  display: block;
  margin: 0;
}
</style>
