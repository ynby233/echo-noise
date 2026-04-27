<template>
  <div ref="previewElement" class="markdown-preview"></div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch, onBeforeUnmount, inject } from 'vue';
import { useRuntimeConfig } from '#imports';
import Vditor from 'vditor';

// 定义正则表达式
const BILIBILI_REG = /https:\/\/www\.bilibili\.com\/video\/(BV[\w]+)\/?/g;
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

const resolveImageUrl = (path: string) => {
  if (!path) return '';
  if (/^https?:\/\//.test(path)) return path;
  
  const base = String(BASE_API).replace(/\/$/, '');
  const p = path.startsWith('/') ? path : `/${path}`;
  
  if (p.startsWith('/api/') && base.endsWith('/api')) {
      return `${base.substring(0, base.length - 4)}${p}`;
  }
  return `${base}${p}`;
}

const previewElement = ref<HTMLDivElement | null>(null);
let zoom: any = null;
let themeClassObserver: MutationObserver | null = null
// 添加 window 类型声明
declare global {
  interface Window {
    handleTagClick: (tag: string) => void;
    mediumZoom: any;
    APlayer: any;
    MetingJSElement: any;
    meting_api?: string;
  }
}
let __gh_gid = 0
const ghInFlight: Record<string, Promise<any>> = {}
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
});

const contentTheme = inject('contentTheme') as any
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
}

const initializeZoom = () => {
  if (window.mediumZoom) {
    // 如果已存在zoom实例，先销毁
    if (zoom) {
      zoom.detach();
    }
    
    const images = previewElement.value?.getElementsByTagName('img');
    if (images && images.length > 0) {
      zoom = window.mediumZoom(images, {
        background: 'rgba(0, 0, 0, 0.9)',
        margin: 24,
        scrollOffset: 0,
      });
    }
  }
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
  const target = (event.target as HTMLElement | null)?.closest('.clickable-tag') as HTMLElement | null
  if (!target) return
  event.preventDefault()
  event.stopPropagation()
  const tag = String(target.dataset.tag || target.textContent || '').replace(/^#/, '').trim()
  if (!tag) return
  emit('tagClick', tag)
}

const applyImageGrid = () => {
  if (!previewElement.value) return;

  previewElement.value.querySelectorAll('.image-grid, .single-media').forEach((node) => {
    const parent = node.parentElement
    if (!parent) return
    while (node.firstChild) {
      parent.insertBefore(node.firstChild, node)
    }
    node.remove()
  })
  
  const isMediaNode = (node: any): boolean => {
    if (!node || node.nodeType !== Node.ELEMENT_NODE) return false;
    const el = node as Element;
    const tag = el.tagName.toLowerCase();
    return tag === 'img' || 
           tag === 'video' || 
           el.classList.contains('video-wrapper') || 
           (tag === 'a' && el.querySelector('img') !== null);
  };

  const isSingleMediaWrapper = (el: Element | null) => {
    if (!el) return false
    return el.classList.contains('single-media') && Array.from(el.children).some((child) => isMediaNode(child))
  }

  const isPureMediaParagraph = (p: Element) => {
    const children = Array.from(p.childNodes);
    if (children.length === 0) return false;
    let hasMedia = false;
    for (const child of children) {
      const node = child as Node;
      if (node.nodeType === Node.ELEMENT_NODE) {
        if (isMediaNode(node)) {
          hasMedia = true;
          continue;
        }
        if ((node as Element).tagName.toLowerCase() === 'br') continue;
        return false; 
      } else if (node.nodeType === Node.TEXT_NODE) {
        if ((node.textContent || '').trim() !== '') return false;
      }
    }
    return hasMedia;
  };

  const areAdjacent = (a: Element, b: Element) => {
     let next = a.nextSibling;
     while (next && next !== b) {
       if (next.nodeType === Node.ELEMENT_NODE) return false; 
       if (next.nodeType === Node.TEXT_NODE) {
         if ((next.textContent || '').trim() !== '') return false; 
       }
       next = next.nextSibling;
     }
     return next === b;
  };

  // 1. Identify Candidates
  const allCandidates = Array.from(previewElement.value.querySelectorAll('p, img, a, video, .video-wrapper, .single-media')) as HTMLElement[];
  const blocks: HTMLElement[] = [];
  
  for (const el of allCandidates) {
     const tag = el.tagName.toLowerCase();
     if (tag === 'p') {
       if (isPureMediaParagraph(el)) blocks.push(el);
     } else if (tag === 'img') {
       const parent = el.parentElement;
       if (!parent) continue;
       if (parent.tagName.toLowerCase() === 'p' && isPureMediaParagraph(parent)) continue;
       if (parent.tagName.toLowerCase() === 'a') continue;
       blocks.push(el);
     } else if (tag === 'a') {
       if (!el.querySelector('img')) continue;
       const parent = el.parentElement;
       if (!parent) continue;
       if (parent.tagName.toLowerCase() === 'p' && isPureMediaParagraph(parent)) continue;
       blocks.push(el);
     } else if (isSingleMediaWrapper(el)) {
       blocks.push(el)
     } else {
       const parent = el.parentElement;
       if (parent && parent.tagName.toLowerCase() === 'p' && isPureMediaParagraph(parent)) continue;
       if (tag === 'video' && parent && parent.classList.contains('video-wrapper')) continue;
       blocks.push(el);
     }
  }

  // 2. Group by Parent
  const blocksByParent = new Map<HTMLElement, HTMLElement[]>();
  for (const block of blocks) {
     const parent = block.parentElement;
     if (!parent) continue;
     if (!blocksByParent.has(parent)) blocksByParent.set(parent, []);
     blocksByParent.get(parent)!.push(block);
  }

  // 3. Process Runs
  for (const [parent, children] of blocksByParent) {
     const runs: HTMLElement[][] = [];
     let current: HTMLElement[] = [];
     for (const block of children) {
        if (current.length === 0) {
           current.push(block);
        } else {
           const last = current[current.length - 1];
           if (areAdjacent(last, block)) {
              current.push(block);
           } else {
              runs.push(current);
              current = [block];
           }
        }
     }
     if (current.length > 0) runs.push(current);

     for (const run of runs) {
        const mediaItems: { node: HTMLElement }[] = [];
        for (const block of run) {
           if (block.tagName.toLowerCase() === 'p') {
              block.childNodes.forEach((node) => {
                 if (isMediaNode(node)) mediaItems.push({ node: node as HTMLElement });
              });
           } else if (isSingleMediaWrapper(block)) {
             Array.from(block.children).forEach((node) => {
               if (isMediaNode(node)) mediaItems.push({ node: node as HTMLElement })
             })
           } else {
              mediaItems.push({ node: block });
           }
        }

        if (mediaItems.length < 2) {
          if (mediaItems.length === 1) {
            const firstBlock = run[0]
            const only = mediaItems[0]?.node
            if (firstBlock?.parentNode && only) {
              const wrapper = document.createElement('div')
              wrapper.className = 'single-media'
              firstBlock.parentNode.insertBefore(wrapper, firstBlock)
              wrapper.appendChild(only)

              if (firstBlock.tagName.toLowerCase() === 'p') firstBlock.remove()

              const applyPortrait = (w: number, h: number) => {
                if (w > 0 && h > 0 && h > w) {
                  wrapper.classList.add('ar-11')
                  const tagName = (only as Element).tagName.toLowerCase()
                  if (tagName === 'img') {
                    const img = only as HTMLImageElement
                    img.style.width = '100%'
                    img.style.height = '100%'
                    img.style.objectFit = 'contain'
                  } else if (tagName === 'video') {
                    const vid = only as HTMLVideoElement
                    vid.style.width = '100%'
                    vid.style.height = '100%'
                    vid.style.objectFit = 'contain'
                  } else if (tagName === 'a') {
                    const img = (only as HTMLAnchorElement).querySelector('img') as HTMLImageElement | null
                    if (img) {
                      img.style.width = '100%'
                      img.style.height = '100%'
                      img.style.objectFit = 'contain'
                    }
                  }
                }
              }

              const tagName = (only as Element).tagName.toLowerCase()
              if (tagName === 'img') {
                const img = only as HTMLImageElement
                if (img.complete && img.naturalWidth && img.naturalHeight) applyPortrait(img.naturalWidth, img.naturalHeight)
                else img.addEventListener('load', () => applyPortrait(img.naturalWidth, img.naturalHeight), { once: true })
              } else if (tagName === 'a') {
                const img = (only as HTMLAnchorElement).querySelector('img') as HTMLImageElement | null
                if (img) {
                  if (img.complete && img.naturalWidth && img.naturalHeight) applyPortrait(img.naturalWidth, img.naturalHeight)
                  else img.addEventListener('load', () => applyPortrait(img.naturalWidth, img.naturalHeight), { once: true })
                }
              } else if (tagName === 'video') {
                const vid = only as HTMLVideoElement
                const runCheck = () => applyPortrait(vid.videoWidth, vid.videoHeight)
                if (vid.readyState >= 1 && vid.videoWidth && vid.videoHeight) runCheck()
                else vid.addEventListener('loadedmetadata', runCheck, { once: true })
              }
            }
          }
          continue
        }

        const grid = document.createElement('div');
        const count = mediaItems.length;
        const cols = count === 2 || count === 4 ? 2 : Math.min(3, count);
        grid.className = `image-grid cols-${cols}`;
        const group = `grid-${Math.random().toString(36).slice(2)}`;

        const firstBlock = run[0];
        if (firstBlock.parentNode) firstBlock.parentNode.insertBefore(grid, firstBlock);

        for (const { node } of mediaItems) {
           const item = document.createElement('div');
           item.className = 'image-grid-item';
           
           const tagName = (node as Element).tagName.toLowerCase();
           if (tagName === 'video') {
               const vid = node as HTMLVideoElement;
               vid.style.width = '100%';
               vid.style.height = '100%';
               vid.style.objectFit = 'cover';
               item.appendChild(vid);
            } else if ((node as Element).classList.contains('video-wrapper')) {
               const wrapper = node as HTMLElement;
               wrapper.style.width = '100%';
               wrapper.style.height = '100%';
               item.appendChild(wrapper);
            } else if (tagName === 'a') {
               const a = node as HTMLAnchorElement;
               a.setAttribute('data-fancybox', group);
               const img = a.querySelector('img');
               if (img && !a.getAttribute('href')) a.setAttribute('href', img.src);
               item.appendChild(a);
            } else if (tagName === 'img') {
               const img = node as HTMLImageElement;
               const a = document.createElement('a');
               a.setAttribute('href', img.src);
               a.setAttribute('data-fancybox', group);
               a.appendChild(img);
               item.appendChild(a);
            }
            grid.appendChild(item);
         }
 
         for (const block of run) {
            if (block.tagName.toLowerCase() === 'p' || isSingleMediaWrapper(block)) block.remove();
         }
 
         const updateGridUniformity = () => {
           const items = Array.from(grid.querySelectorAll('.image-grid-item')) as HTMLElement[];
           let landscapeCount = 0;
           let portraitCount = 0;
           let squareCount = 0;

           items.forEach(item => {
             const img = item.querySelector('img');
             const vid = item.querySelector('video');
             const wrapper = item.querySelector('.video-wrapper');
             
             if (wrapper) {
               landscapeCount++;
             } else if (img) {
               if (img.complete && img.naturalWidth) {
                 const r = img.naturalWidth / img.naturalHeight;
                 if (Math.abs(r - 1) < 0.15) squareCount++; // 宽松判定方形
                 else if (r > 1) landscapeCount++;
                 else portraitCount++;
               }
             } else if (vid) {
               if (vid.readyState >= 1) {
                 const r = vid.videoWidth / vid.videoHeight;
                 if (Math.abs(r - 1) < 0.15) squareCount++;
                 else if (r > 1) landscapeCount++;
                 else portraitCount++;
               }
             }
           });

           let targetClass = 'ar-11'; 
           const typesPresent = [landscapeCount > 0, portraitCount > 0, squareCount > 0].filter(Boolean).length;
           
           if (typesPresent > 1) {
             targetClass = 'ar-11'; // 混合类型强制方形，确保对齐
           } else if (landscapeCount > 0) {
             targetClass = 'ar-169';
           } else if (portraitCount > 0) {
             targetClass = 'ar-34';
           } else if (squareCount > 0) {
             targetClass = 'ar-11';
           } else {
             targetClass = 'ar-169'; // 默认
           }
           
           items.forEach(item => {
             item.classList.remove('ar-169', 'ar-34', 'ar-11');
             item.classList.add(targetClass);
           });
         };

         grid.querySelectorAll('img').forEach((imgEl) => {
           const img = imgEl as HTMLImageElement;
           if (img.complete) updateGridUniformity();
           else img.addEventListener('load', updateGridUniformity);
         });
 
         grid.querySelectorAll('video').forEach((vidEl) => {
           const vid = vidEl as HTMLVideoElement;
           if (vid.readyState >= 1) updateGridUniformity();
           else vid.addEventListener('loadedmetadata', updateGridUniformity);
         });

         grid.querySelectorAll('.video-wrapper').forEach(() => {
           updateGridUniformity();
         });
         
         // 初始执行一次
         updateGridUniformity();
     }
  }
};


const applyImageLoadingPlaceholders = () => {
  if (!previewElement.value) return;
  const imgs = Array.from(previewElement.value.querySelectorAll('img')) as HTMLImageElement[];
  imgs.forEach((img) => {
    const container = (img.closest('.image-grid-item') || img.parentElement || previewElement.value) as HTMLElement;
    const needPlaceholder = !img.complete || !(img.naturalWidth && img.naturalHeight);
    if (!needPlaceholder) return;
    const ph = document.createElement('div');
    ph.className = 'image-loading-placeholder';
    ph.textContent = '图片正在加载中，请稍后';
    const ref = img.closest('.image-grid-item') ? (img.closest('.image-grid-item') as HTMLElement).firstChild : img;
    container.insertBefore(ph, ref as Node);
    img.style.opacity = '0';
    const onLoad = () => {
      img.style.opacity = '1';
      ph.remove();
    };
    const onError = () => {
      ph.textContent = '图片加载失败';
      ph.classList.add('image-loading-error');
      img.style.display = 'none';
    };
    img.addEventListener('load', onLoad, { once: true });
    img.addEventListener('error', onError, { once: true });
  });
};


// 修改正则，避免匹配 Markdown 图片链接
// 1. 匹配 markdown 普通链接（非图片）- 避免使用 lookbehind，兼容低版本运行环境
const GITHUB_MD_LINK_REG = /(^|[^!])\[([^\]]+)\]\((https:\/\/github\.com\/([\w-]+)\/([\w.-]+)(?:\/[^\s)]*)?)\)/g;
// 2. 匹配裸仓库链接（非图片）- 避免使用 lookbehind，兼容低版本运行环境
const GITHUB_BARE_LINK_REG = /(^|[\s>])(https:\/\/github\.com\/([\w-]+)\/([\w.-]+)(?:\/[^\s<\)]*)?)/g;

const buildYouTubeEmbedHtml = (videoId: string) => {
  const watchUrl = `https://www.youtube.com/watch?v=${videoId}`
  const thumbPrimary = `https://i.ytimg.com/vi/${videoId}/hqdefault.jpg`
  const thumbFallback = `https://img.youtube.com/vi/${videoId}/hqdefault.jpg`
  return `<div class='video-block youtube-video-block'><div class='video-wrapper youtube-video-wrapper'><iframe src='https://www.youtube.com/embed/${videoId}' title='YouTube video player' frameborder='0' allow='accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture' allowfullscreen></iframe></div><div class='video-fallback-card'><img class='video-fallback-thumb' src='${thumbPrimary}' data-fallback-src='${thumbFallback}' alt='YouTube preview' loading='lazy' referrerpolicy='no-referrer' onerror="const f=this.getAttribute('data-fallback-src'); if (f && this.src!==f){this.src=f}else{this.style.display='none'}" /><div class='video-fallback-content'><div class='video-fallback-title'>YouTube 受限网络兜底预览</div><a class='video-fallback-link' href='${watchUrl}' target='_blank' rel='noopener noreferrer'>打开原链接</a></div></div></div>`
}
const replaceNodeWithHtml = (node: HTMLElement, html: string) => {
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

const processMediaLinks = (content: string): string => {
  ensureMetingApiReady()
  // 先处理 markdown 链接与行内代码里的媒体链接，避免后续替换打断 markdown 结构
  const BILIBILI_MD_LINK_REG = /\[[^\]]*]\((https:\/\/www\.bilibili\.com\/video\/(BV[\w]+)\/?)\)/g;
  const YOUTUBE_MD_LINK_REG = /\[[^\]]*]\((https:\/\/(?:www\.)?youtube\.com\/watch\?v=([\w-]+)|https:\/\/youtu\.be\/([\w-]+))\)/g;
  const NETEASE_MD_LINK_REG = /\[[^\]]*]\((https:\/\/music\.163\.com(?:\/#)?\/song\?id=(\d+))\)/g;
  // 允许反引号内 URL 前后有空格，避免 ` https://... ` 这类写法漏匹配
  const NETEASE_INLINE_CODE_REG = /`[\t ]*https:\/\/music\.163\.com(?:\/#)?\/song\?id=(\d+)[\t ]*`/g;
  content = content
    .replace(BILIBILI_MD_LINK_REG, "<div class='video-wrapper'><iframe src='https://www.bilibili.com/blackboard/html5mobileplayer.html?bvid=$2&as_wide=1&high_quality=1&danmaku=0' scrolling='no' border='0' frameborder='no' framespacing='0' allowfullscreen='true' style='position:absolute;height:100%;width:100%'></iframe></div>")
    .replace(YOUTUBE_MD_LINK_REG, (_m, _full, id1, id2) => {
      const videoId = String(id1 || id2 || '').trim()
      if (!videoId) return _m
      return buildYouTubeEmbedHtml(videoId)
    })
    .replace(NETEASE_MD_LINK_REG, (_m, _full, songId) => buildMetingSongEmbed(songId) || _m)
    .replace(NETEASE_INLINE_CODE_REG, (_m, songId) => buildMetingSongEmbed(songId) || _m)

  // GitHub 卡片解析（可开关）
  if (props.enableGithubCard) {
    content = content.replace(GITHUB_MD_LINK_REG, (_match, prefix, _text, _url, owner, repo) => {
      const cardId = `github-card-${owner}-${repo}-${++__gh_gid}`;
      return `${prefix}<div class="github-card" id="${cardId}" data-owner="${owner}" data-repo="${repo}">
        <div class="github-card-loading">Loading GitHub Repo...</div>
      </div>`;
    });
    content = content.replace(GITHUB_BARE_LINK_REG, (_match, prefix, _url, owner, repo) => {
      const cardId = `github-card-${owner}-${repo}-${++__gh_gid}`;
      return `${prefix}<div class="github-card" id="${cardId}" data-owner="${owner}" data-repo="${repo}">
        <div class="github-card-loading">Loading GitHub Repo...</div>
      </div>`;
    });
  }
  // 将裸视频文件链接替换为内联视频标签（先于链接化处理）
  // 仅匹配前导为空白字符或行首的 URL，避免匹配 HTML 属性中的 URL（如 src="http..."）
  const VIDEO_FILE_REG = /(^|[\s>])((?:https?:\/\/|\/api\/video\/|\/video\/)[^\s<"']+\.(?:mp4|webm|mov|avi)(?:\?[^\s<"']*)?)/g;
  content = content.replace(VIDEO_FILE_REG, (_m, prefix, videoUrl) => {
    const src = resolveImageUrl(videoUrl);
    return `${prefix}<video src="${src}" controls preload="metadata" style="width:100%;height:auto"></video>`;
  });
  content = content
    .replace(BILIBILI_REG, "<div class='video-wrapper'><iframe src='https://www.bilibili.com/blackboard/html5mobileplayer.html?bvid=$1&as_wide=1&high_quality=1&danmaku=0' scrolling='no' border='0' frameborder='no' framespacing='0' allowfullscreen='true' style='position:absolute;height:100%;width:100%'></iframe></div>")
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
const fetchGitHubRepoInfo = async (owner: string, repo: string, cardId: string) => {
  const card = document.getElementById(cardId);
  if (!card) return;

  const svgStar = '<svg class="gh-icon" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M8 .25l2.317 4.7 5.183.754-3.75 3.654.885 5.167L8 12.347l-4.635 2.178.885-5.167-3.75-3.654 5.183-.754L8 .25z"></path></svg>'
  const svgFork = '<svg class="gh-icon" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><circle cx="4" cy="3" r="1.5" fill="currentColor"></circle><circle cx="12" cy="3" r="1.5" fill="currentColor"></circle><circle cx="8" cy="13" r="1.5" fill="currentColor"></circle><path d="M4 4.5v2a4 4 0 004 4h0a4 4 0 004-4v-2" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round"/></svg>'
  const svgLang = '<svg class="gh-icon" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M5 5 L2 8 L5 11" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round"/><path d="M11 5 L14 8 L11 11" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round"/><path d="M7 12 L9 4" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round"/></svg>'
  const svgMark = '<svg class="gh-badge" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8a8 8 0 005.47 7.59c.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.6 7.6 0 012 0c1.53-1.03 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.28.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8 8 0 0016 8c0-4.42-3.58-8-8-8z"></path></svg>'

  const skeleton = () => {
    card.innerHTML = `
      <div class="github-card-header">
        <div class="gh-avatar-slot">
          <div class="github-card-avatar placeholder-avatar"></div>
          ${svgMark}
        </div>
        <div>
          <a href="https://github.com/${owner}/${repo}" target="_blank" class="github-card-title">${owner}/${repo}</a>
        </div>
      </div>
    `
  }
  skeleton()

  const cacheKey = `gh_repo_cache_${owner}_${repo}`
  const ttlMs = 6 * 60 * 60 * 1000
  try {
    const cached = localStorage.getItem(cacheKey)
    if (cached) {
      const obj = JSON.parse(cached)
      if (obj && obj.ts && Date.now() - obj.ts < ttlMs && obj.data) {
        const d = obj.data
        card.innerHTML = `
          <div class="github-card-header">
            <div class="gh-avatar-slot">
              <img src="${d.owner.avatar_url}" class="github-card-avatar" referrerpolicy="no-referrer" loading="lazy" decoding="async" onerror="this.style.display='none'; this.nextElementSibling.style.display='flex';"/>
              <div class="avatar-fallback" style="display:none;">${owner.charAt(0).toUpperCase()}</div>
              ${svgMark}
            </div>
            <div>
              <a href="https://github.com/${owner}/${repo}" target="_blank" class="github-card-title">${d.full_name || owner + '/' + repo}</a>
            </div>
          </div>
        `
        card.classList.add('github-card-loaded')
        return
      }
    }
  } catch {}

  const tryFetch = async (url: string, timeoutMs = 6000): Promise<any> => {
    const ctrl = new AbortController()
    const t = setTimeout(() => ctrl.abort(), timeoutMs)
    try {
      const r = await fetch(url, { signal: ctrl.signal })
      if (!r.ok) throw new Error(String(r.status))
      const j = await r.json()
      return j
    } finally { clearTimeout(t) }
  }

  const k = `${owner}/${repo}`
  let data: any = null
  if (!ghInFlight[k]) {
    ghInFlight[k] = (async () => {
      try {
        const r1 = await tryFetch(`https://api.github.com/repos/${owner}/${repo}`)
        return r1
      } catch {
        try {
          const r2 = await tryFetch(`https://ghproxy.com/https://api.github.com/repos/${owner}/${repo}`)
          return r2
        } catch {}
      }
      return null
    })()
  }
  data = await ghInFlight[k]

  if (data) {
    try {
      localStorage.setItem(cacheKey, JSON.stringify({ ts: Date.now(), data }))
    } catch {}
    card.innerHTML = `
      <div class="github-card-header">
        <div class="gh-avatar-slot">
          <img src="${data.owner?.avatar_url || ''}" class="github-card-avatar" referrerpolicy="no-referrer" loading="lazy" decoding="async" onerror="this.style.display='none'; this.nextElementSibling.style.display='flex';"/>
          <div class="avatar-fallback" style="display:none;">${owner.charAt(0).toUpperCase()}</div>
          ${svgMark}
        </div>
        <div>
          <a href="${data.html_url || `https://github.com/${owner}/${repo}`}" target="_blank" class="github-card-title">${data.full_name || owner + '/' + repo}</a>
        </div>
      </div>
    `
    card.classList.add('github-card-loaded')
    return
  }

  card.innerHTML = `
    <div class="github-card-header">
      <div class="gh-avatar-slot">
        <div class="avatar-fallback" style="display:flex;">${owner.charAt(0).toUpperCase()}</div>
        ${svgMark}
      </div>
      <div>
        <a href="https://github.com/${owner}/${repo}" target="_blank" class="github-card-title">${owner}/${repo}</a>
      </div>
    </div>
  `
  card.classList.add('github-card-error')
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
    if (typeof Vditor === 'undefined') {
      console.error('Vditor is not loaded.');
      renderPlainFallback(markdown ?? '')
      return;
    }

    // 先处理媒体链接
    const processedContent = processMediaLinks(markdown ?? '');

    // 将裸露的 URL 转为可点击链接（新标签页打开）
    const linkifyBareUrls = (text: string): string => {
      return text.replace(/(^|\s)(https?:\/\/[^\s<]+)/g, (_match, pre, url) => {
        return `${pre}<a href="${url}" target="_blank" rel="noopener noreferrer">${url}</a>`;
      });
    };
    const withLinks = linkifyBareUrls(processedContent);
    
    // 修改标签匹配规则，排除HTML标签内的内容
    const finalContent = withLinks.replace(/<a /g, '<a target="_blank" ');

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
    Vditor.preview(previewElement.value!, finalContent, {
      mode: currentTheme as any,
      lang: 'zh_CN',
      theme: { current: currentTheme },
      hljs: { style: hljsStyle, lineNumber: true, enable: true },
      markdown: { sanitize: false },
      after: async () => {
        try {
          const images = previewElement.value?.querySelectorAll('img');
          images?.forEach(img => {
             const src = img.getAttribute('src');
             if (src && !/^https?:\/\//.test(src)) {
                 img.src = resolveImageUrl(src);
             }
          });
          const links = previewElement.value?.querySelectorAll('a');
          links?.forEach(link => {
            if (!link.hasAttribute('target')) {
              link.setAttribute('target', '_blank');
              link.setAttribute('rel', 'noopener noreferrer');
            }
          });
          applyThemeClass();
          const anchors = previewElement.value?.querySelectorAll('a[href]') || [] as any;
          anchors.forEach((a: HTMLAnchorElement) => {
            const href = a.getAttribute('href') || ''
            if (/\.(mp4|webm|mov|avi)(\?.*)?$/i.test(href)) {
              const v = document.createElement('video')
              v.setAttribute('src', resolveImageUrl(href))
              v.setAttribute('controls', 'true')
              v.setAttribute('preload', 'metadata')
              v.style.width = '100%'
              v.style.height = 'auto'
              a.replaceWith(v)
            }
          });
          await enhanceDouyinShortLinks()
          applyDouyinVideoLayout()
          applyClickableTags()
          
          // Explicitly handle existing video tags (e.g. from raw HTML or markdown)
          const existingVideos = previewElement.value?.querySelectorAll('video');
          existingVideos?.forEach((v: HTMLVideoElement) => {
              const src = v.getAttribute('src');
              if (src && !/^https?:\/\//.test(src)) {
                  v.setAttribute('src', resolveImageUrl(src));
              }
              if (!v.hasAttribute('controls')) {
                  v.setAttribute('controls', 'true');
              }
              // Ensure proper sizing to prevent collapse (fixes height="100%" issue)
              v.style.width = '100%';
              v.style.height = 'auto';
              v.style.maxWidth = '100%';
          });

          applyImageGrid();
          applyDouyinVideoLayout()
          setTimeout(() => {
            applyDouyinVideoLayout()
          }, 80)
          initializeZoom();
          applyImageLoadingPlaceholders();
          emit('rendered');
          const proc = (window as any).processNMPv2Shortcodes
          if (proc && previewElement.value) {
            proc(previewElement.value)
          }
          if (props.enableGithubCard) {
            const githubCards = previewElement.value?.querySelectorAll('.github-card');
            githubCards?.forEach(card => {
              const owner = card.getAttribute('data-owner');
              const repo = card.getAttribute('data-repo');
              const cardId = card.id;
              if (owner && repo && cardId) {
                fetchGitHubRepoInfo(owner, repo, cardId);
              }
            });
          }
        } catch (err) {
          console.error('Markdown post-processing failed:', err)
        }
      }
    });
  } catch (error) {
    console.error("Error rendering markdown:", error);
    renderPlainFallback(markdown ?? '')
  }
};
watch(
  () => props.content,
  async (newContent) => {
    await renderMarkdown(newContent);
  }
);

onMounted(() => {
  renderMarkdown(props.content);
  // 确保 MetingJS 正确初始化
  if (window.APlayer && window.MetingJSElement) {
    console.log('MetingJS is ready');
  } else {
    console.error('MetingJS or APlayer is not loaded properly');
  }
  applyThemeClass();
  try {
    themeClassObserver = new MutationObserver(() => applyThemeClass())
    themeClassObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  } catch {}
  previewElement.value?.addEventListener('click', onPreviewClick)
  try {
    window.addEventListener('resize', applyDouyinVideoLayout, { passive: true })
  } catch {}
});


onBeforeUnmount(() => {
  if (zoom) {
    zoom.detach();
    zoom = null;
  }
  previewElement.value?.removeEventListener('click', onPreviewClick)
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
        audio.style.border = '1px solid rgba(255,255,255,0.10)';
      } else {
        audio.style.backgroundColor = '#ffffff';
        audio.style.border = '1px solid #e5e7eb';
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
.markdown-preview {
  font-family: "LXGW WenKai Screen";
  line-height: 1.6;
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
.builtin-comments .markdown-preview.theme-dark { color: rgb(227, 220, 220) !important; }
.builtin-comments .markdown-preview.theme-light { color: #111111 !important; }
/* 通用主题文本颜色（非评论区域也适用） */
.markdown-preview.theme-dark { color: rgb(227, 220, 220) !important; }
.markdown-preview.theme-light { color: #111111 !important; }
.builtin-comments .markdown-preview.theme-dark h1,
.builtin-comments .markdown-preview.theme-dark h2,
.builtin-comments .markdown-preview.theme-dark h3,
.builtin-comments .markdown-preview.theme-dark h4,
.builtin-comments .markdown-preview.theme-dark h5,
.builtin-comments .markdown-preview.theme-dark h6 { color: rgb(251, 247, 247) !important; }
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

.markdown-preview p {
  margin: 6px 0 !important;
  line-height: 1.6;
}
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
.markdown-preview table thead tr {
  background-color: rgba(223, 226, 229, 0.49) !important;
}

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
  gap: 12px;
  margin-top: 10px;
  padding: 10px;
  border-radius: 12px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: rgba(148, 163, 184, 0.08);
}
.video-fallback-thumb {
  width: 160px;
  min-width: 160px;
  height: 90px;
  object-fit: cover;
  border-radius: 10px;
}
.video-fallback-content {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.video-fallback-title {
  font-size: 13px;
  font-weight: 600;
}
.video-fallback-link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: fit-content;
  padding: 5px 12px;
  border-radius: 999px;
  border: 1px solid rgba(59, 130, 246, 0.45);
  color: #2563eb;
  font-size: 12px;
}
.theme-dark .video-fallback-card {
  border-color: rgba(148, 163, 184, 0.4);
  background: rgba(51, 65, 85, 0.45);
}
.theme-dark .video-fallback-link {
  color: #93c5fd;
  border-color: rgba(147, 197, 253, 0.5);
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

/* GitHub 卡片背景与内容卡片一致（继承父背景，无边框） */
.markdown-preview :deep(.github-card),
.markdown-preview :deep(.github-card-header),
.markdown-preview :deep(.github-card-loading) {
  background-color: transparent !important;
  border: none !important;
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

.markdown-preview :deep(video),
.markdown-preview :deep(audio) {
  display: block;
  width: 100%;
  margin: 0.4em 0;
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

.markdown-preview :deep(a) { 
  color: #0366d6 !important; 
  text-decoration: none !important; 
  background-color: transparent !important;
  padding: 0 !important;
  border-radius: 0 !important;
  border: none !important;
  text-shadow: none !important;
}
.markdown-preview :deep(a:hover) { 
  text-decoration: underline !important; 
  color: #1d4ed8 !important;
}
.theme-light.markdown-preview :deep(a),
.theme-dark.markdown-preview :deep(a) { 
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
  border: 1px solid #e5e7eb !important;
  border-radius: 8px !important;
}

.theme-dark video {
  background-color: #202a36 !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
  border-radius: 8px !important;
}

.theme-dark audio {
  background-color: #202a36 !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
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

.github-card {
  border-radius: 8px;
  margin: 0.8em auto 0.4em;
  padding: 16px;
  width: 100%;
  max-width: 800px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  font-size: 15px;
  box-sizing: border-box;
  min-width: 0;
  overflow: hidden;
}
.theme-dark .github-card {
  border: none !important;
  background: transparent !important;
  color: inherit !important;
}
.theme-light .github-card {
  border: none !important;
  background: transparent !important;
  color: inherit !important;
}
.github-card-header { display: grid; grid-template-columns: 44px max-content; column-gap: 10px; align-items: center; justify-content: start; }
.gh-avatar-slot { position: relative; width: 40px; height: 40px; }
.github-card-avatar { width: 40px; height: 40px; border-radius: 10px; object-fit: cover; background: #222; }
.avatar-fallback { width: 40px; height: 40px; border-radius: 10px; background: #0366d6; color: #ffffff; display: none; align-items: center; justify-content: center; font-size: 15px; font-weight: 600; }
.gh-badge { position: absolute; right: -5px; bottom: -5px; width: 16px; height: 16px; border-radius: 50%; padding: 2px; }
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
.theme-light.markdown-preview :deep(a) {
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

.theme-dark.markdown-preview :deep(a),
.theme-light.markdown-preview :deep(a),
:global(html.dark) .markdown-preview :deep(a),
:global(html:not(.dark)) .markdown-preview :deep(a) {
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
.markdown-preview :deep(a) {
  background-color: transparent !important;
  padding: 0 !important;
  border-radius: 0 !important;
  border: none !important;
  text-shadow: none !important;
  color: #0366d6 !important;
}

.theme-dark.markdown-preview :deep(a:hover),
.theme-light.markdown-preview :deep(a:hover),
:global(html.dark) .markdown-preview :deep(a:hover),
:global(html:not(.dark)) .markdown-preview :deep(a:hover),
.markdown-preview :deep(a:hover) {
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
    padding: 10px;
    font-size: 14px;
  }
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
