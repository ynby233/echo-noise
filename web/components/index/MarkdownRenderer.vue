<template>
  <div ref="previewElement" class="markdown-preview"></div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch, onBeforeUnmount, inject } from 'vue';
import Vditor from 'vditor';

// 定义正则表达式
const BILIBILI_REG = /https:\/\/www\.bilibili\.com\/video\/(BV[\w]+)\/?$/;
const YOUTUBE_REG = /https:\/\/(?:www\.)?youtube\.com\/watch\?v=([\w-]+)|https:\/\/youtu\.be\/([\w-]+)/;
const NETEASE_MUSIC_REG = /https:\/\/music\.163\.com(?:\/#)?\/song\?id=(\d+)/;
const QQMUSIC_REG = /https:\/\/y\.qq\.com\/n\/yqq\/song(\w+)\.html/;
const QQVIDEO_REG = /https:\/\/v\.qq\.com\/x\/cover\/\w+\/(\w+)\.html/;
const SPOTIFY_REG = /https:\/\/open\.spotify\.com\/(track|album|playlist)\/([a-zA-Z0-9]+)/;
const YOUKU_REG = /https:\/\/v\.youku\.com\/v_show\/id_([a-zA-Z0-9]+)\.html/;
const emit = defineEmits(['tagClick', 'rendered'])
const previewElement = ref<HTMLDivElement | null>(null);
let zoom: any = null;
// 添加 window 类型声明
declare global {
  interface Window {
    handleTagClick: (tag: string) => void;
    mediumZoom: any;
    APlayer: any;
    MetingJSElement: any;
  }
}
const props = defineProps({
  content: {
    type: String,
    required: true,
  },
  enableGithubCard: {
    type: Boolean,
    default: true,
  },
});

const contentTheme = inject('contentTheme') as any

const applyThemeClass = () => {
  if (!previewElement.value) return
  let isDark = false
  if (contentTheme && typeof (contentTheme as any).value !== 'undefined') {
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

const applyImageGrid = () => {
  if (!previewElement.value) return;
  const isPureImageParagraph = (p: Element) => {
    let ok = true;
    const children = Array.from(p.childNodes);
    if (children.length === 0) return false;
    for (const node of children) {
      if (node.nodeType === Node.ELEMENT_NODE) {
        const el = node as Element;
        const tag = el.tagName.toLowerCase();
        if (tag === 'img') continue;
        if (tag === 'a' && el.childElementCount === 1 && el.querySelector('img')) continue;
        if (tag === 'br') { ok = false; break; }
        ok = false; break;
      } else if (node.nodeType === Node.TEXT_NODE) {
        if ((node.textContent || '').trim() !== '') { ok = false; break; }
      }
    }
    return ok;
  };

  const paras = Array.from(previewElement.value.querySelectorAll('p'));
  const runs: Element[][] = [];
  let current: Element[] = [];
  for (const p of paras) {
    if (isPureImageParagraph(p)) {
      const last = current[current.length - 1];
      if (!last || last.nextElementSibling === p) {
        current.push(p);
      } else {
        if (current.length >= 2) runs.push(current);
        current = [p];
      }
    } else {
      if (current.length >= 2) runs.push(current);
      current = [];
    }
  }
  if (current.length >= 2) runs.push(current);

  for (const run of runs) {
    const grid = document.createElement('div');
    const count = run.length;
    const cols = count === 2 || count === 4 ? 2 : Math.min(3, count);
    grid.className = `image-grid cols-${cols}`;
    const group = `grid-${Math.random().toString(36).slice(2)}`;
    for (const p of run) {
      const img = p.querySelector('img') as HTMLImageElement | null;
      const a = p.querySelector('a') as HTMLAnchorElement | null;
      if (!img && !a) continue;
      const item = document.createElement('div');
      item.className = 'image-grid-item';
      let anchor: HTMLAnchorElement;
      if (a && a.querySelector('img')) {
        anchor = a;
        anchor.setAttribute('data-fancybox', group);
        if (!anchor.getAttribute('href') && a.querySelector('img')) {
          const innerImg = a.querySelector('img') as HTMLImageElement;
          anchor.setAttribute('href', innerImg.src);
        }
      } else if (img) {
        anchor = document.createElement('a');
        anchor.setAttribute('href', img.src);
        anchor.setAttribute('data-fancybox', group);
        anchor.appendChild(img);
      } else {
        continue;
      }
      item.appendChild(anchor);
      grid.appendChild(item);
    }
    grid.querySelectorAll('img').forEach((imgEl) => {
      const img = imgEl as HTMLImageElement;
      const item = img.closest('.image-grid-item') as HTMLElement;
      const setAR = () => {
        const w = img.naturalWidth;
        const h = img.naturalHeight;
        if (!item) return;
        item.classList.remove('ar-169','ar-34','ar-11');
        if (w > h) item.classList.add('ar-169');
        else if (h > w) item.classList.add('ar-34');
        else item.classList.add('ar-11');
      };
      if (img.complete && img.naturalWidth && img.naturalHeight) setAR();
      else img.addEventListener('load', setAR, { once: true });
    });
    const first = run[0];
    first.replaceWith(grid);
    for (let i = 1; i < run.length; i++) run[i].remove();
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
// 1. 匹配 markdown 普通链接（非图片）
const GITHUB_MD_LINK_REG = /(?<!!)\[([^\]]+)\]\((https:\/\/github\.com\/([\w-]+)\/([\w.-]+)(?:\/[^\s)]*)?)\)/g;
// 2. 匹配裸仓库链接（非图片）
const GITHUB_BARE_LINK_REG = /(?<!["'\(])\bhttps:\/\/github\.com\/([\w-]+)\/([\w.-]+)(?:\/[^\s<\)]*)?\b/g;

const processMediaLinks = (content: string): string => {
  // GitHub 卡片解析（可开关）
  if (props.enableGithubCard) {
    content = content.replace(GITHUB_MD_LINK_REG, (match, text, url, owner, repo) => {
      const cardId = `github-card-${owner}-${repo}`;
      return `<div class="github-card" id="${cardId}" data-owner="${owner}" data-repo="${repo}">
        <div class="github-card-loading">Loading GitHub Repo...</div>
      </div>`;
    });
    content = content.replace(GITHUB_BARE_LINK_REG, (match, owner, repo) => {
      const cardId = `github-card-${owner}-${repo}`;
      return `<div class="github-card" id="${cardId}" data-owner="${owner}" data-repo="${repo}">
        <div class="github-card-loading">Loading GitHub Repo...</div>
      </div>`;
    });
  }
  // 将裸视频文件链接替换为内联视频标签（先于链接化处理）
  const VIDEO_FILE_REG = /(?<!["'\(])\bhttps?:\/\/[^\s<]+\.(mp4|webm|mov|avi)(\?[^\s<\)]*)?\b/g;
  content = content.replace(VIDEO_FILE_REG, (m) => {
    const src = m;
    return `<video src="${src}" controls preload="metadata" style="width:100%;height:auto"></video>`;
  });
  return content
    .replace(BILIBILI_REG, "<div class='video-wrapper'><iframe src='https://www.bilibili.com/blackboard/html5mobileplayer.html?bvid=$1&as_wide=1&high_quality=1&danmaku=0' scrolling='no' border='0' frameborder='no' framespacing='0' allowfullscreen='true' style='position:absolute;height:100%;width:100%'></iframe></div>")
    .replace(YOUTUBE_REG, "<div class='video-wrapper'><iframe src='https://www.youtube.com/embed/$1$2' title='YouTube video player' frameborder='0' allow='accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture' allowfullscreen></iframe></div>")
    .replace(NETEASE_MUSIC_REG, "<div class='music-wrapper'><meting-js auto='https://music.163.com/#/song?id=$1'></meting-js></div>")
    .replace(QQMUSIC_REG, "<meting-js auto='https://y.qq.com/n/yqq/song$1.html'></meting-js>")
    .replace(QQVIDEO_REG, "<div class='video-wrapper'><iframe src='//v.qq.com/iframe/player.html?vid=$1' allowFullScreen='true' frameborder='no'></iframe></div>")
    .replace(SPOTIFY_REG, "<div class='spotify-wrapper'><iframe style='border-radius:12px' src='https://open.spotify.com/embed/$1/$2?utm_source=generator&theme=0' width='100%' frameBorder='0' allowfullscreen='' allow='autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture' loading='lazy'></iframe></div>")
    .replace(YOUKU_REG, "<div class='video-wrapper'><iframe src='https://player.youku.com/embed/$1' frameborder=0 'allowfullscreen'></iframe></div>");
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
              <img src="${d.owner.avatar_url}" class="github-card-avatar" onerror="this.style.display='none'; this.nextElementSibling.style.display='flex';"/>
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

  let data: any = null
  try {
    data = await tryFetch(`https://api.github.com/repos/${owner}/${repo}`)
  } catch {
    try {
      data = await tryFetch(`https://ghproxy.com/https://api.github.com/repos/${owner}/${repo}`)
    } catch {}
  }

  if (data) {
    try {
      localStorage.setItem(cacheKey, JSON.stringify({ ts: Date.now(), data }))
    } catch {}
    card.innerHTML = `
      <div class="github-card-header">
        <div class="gh-avatar-slot">
          <img src="${data.owner?.avatar_url || ''}" class="github-card-avatar" onerror="this.style.display='none'; this.nextElementSibling.style.display='flex';"/>
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
const renderMarkdown = async (markdown: string) => {
  if (!previewElement.value) return;

  try {
    if (typeof Vditor === 'undefined') {
      console.error('Vditor is not loaded.');
      return;
    }

    // 先处理媒体链接
    let normalizedContent = '';
    try {
      normalizedContent = normalizeInlineImageLinks(markdown ?? '');
    } catch {
      normalizedContent = markdown ?? '';
    }
    const processedContent = processMediaLinks(normalizedContent);

    // 将裸露的 URL 转为可点击链接（新标签页打开）
    const linkifyBareUrls = (text: string): string => {
      return text.replace(/(^|\s)(https?:\/\/[^\s<]+)/g, (_match, pre, url) => {
        return `${pre}<a href="${url}" target="_blank" rel="noopener noreferrer">${url}</a>`;
      });
    };
    const withLinks = linkifyBareUrls(processedContent);
    
    // 修改标签匹配规则，排除HTML标签内的内容
    let finalContent = '';
    try {
      finalContent = withLinks
        .replace(/<a /g, '<a target="_blank" ')
        .replace(
          /(?<!<[^>]*)#([^\s#<>]+)(?![^<]*>)/g,
          '<span class="clickable-tag" onclick="window.handleTagClick(\'$1\')" style="cursor: pointer;">#$1</span>'
        );
    } catch {
      finalContent = withLinks.replace(/<a /g, '<a target="_blank" ');
    }

    const currentTheme = (contentTheme && (contentTheme as any).value) ? ((contentTheme as any).value === 'dark' ? 'dark' : 'light') : (document.documentElement.classList.contains('dark') ? 'dark' : 'light')
    const hljsStyle = currentTheme === 'dark' ? 'github-dark' : 'github'
    Vditor.preview(previewElement.value!, finalContent, {
      lang: 'zh_CN',
      theme: { current: currentTheme },
      hljs: { style: hljsStyle, lineNumber: true, enable: true },
      markdown: { sanitize: false },
      after: () => {
        const links = previewElement.value?.querySelectorAll('a');
        links?.forEach(link => {
          if (!link.hasAttribute('target')) {
            link.setAttribute('target', '_blank');
            link.setAttribute('rel', 'noopener noreferrer');
          }
        });
        applyThemeClass();
        applyImageGrid();
        initializeZoom();
        const anchors = previewElement.value?.querySelectorAll('a[href]') || [] as any;
        anchors.forEach((a: HTMLAnchorElement) => {
          const href = a.getAttribute('href') || ''
          if (/\.(mp4|webm|mov|avi)(\?.*)?$/i.test(href)) {
            const v = document.createElement('video')
            v.setAttribute('src', href)
            v.setAttribute('controls', 'true')
            v.setAttribute('preload', 'metadata')
            v.style.width = '100%'
            v.style.height = 'auto'
            a.replaceWith(v)
          }
        });
        applyImageLoadingPlaceholders();
        emit('rendered');
        const proc = (window as any).processNMPv2Shortcodes
        if (proc && previewElement.value) {
          proc(previewElement.value)
        }
        const tags = previewElement.value?.querySelectorAll('.clickable-tag');
        tags?.forEach(tag => {
          tag.addEventListener('click', (e) => {
            e.preventDefault();
            const tagText = tag.textContent?.substring(1);
            if (tagText) {
              emit('tagClick', tagText);
            }
          });
        });
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
      }
    });
  } catch (error) {
    console.error("Error rendering markdown:", error);
    previewElement.value.innerHTML = '';
  }
};
watch(
  () => props.content,
  async (newContent) => {
    await renderMarkdown(newContent);
  },
  { immediate: true }
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
    const observer = new MutationObserver(() => applyThemeClass())
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  } catch {}
});


onBeforeUnmount(() => {
  if (zoom) {
    zoom.detach();
    zoom = null;
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
        video.style.backgroundColor = '#242b32';
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
        audio.style.backgroundColor = '#242b32';
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
  margin: 0.5em 0;
  line-height: 1.6;
}
.clickable-tag {
  color: #fb923c !important;
  cursor: pointer;
  transition: color 0.2s ease;
  padding: 0 2px;
}
.theme-dark .clickable-tag { color: #fb923c !important; }
.theme-light .clickable-tag { color: #b45309 !important; }

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
  margin: 0.4em auto;
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
  background-color: #242b32 !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
  border-radius: 8px !important;
}

.theme-dark audio {
  background-color: #242b32 !important;
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
.github-card {
  border-radius: 8px;
  margin: 1em 0;
  padding: 16px;
  width: 100%;
  box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  font-size: 15px;
  box-sizing: border-box;
  min-width: 0;
  overflow: hidden;
}
.theme-dark .github-card {
  border: 1px solid #30363d !important;
  background: #161b22 !important;
  color: #c9d1d9 !important;
}
.theme-light .github-card {
  border: 1px solid #e5e7eb !important;
  background: #ffffff !important;
  color: #111827 !important;
}
.github-card-header { display: grid; grid-template-columns: 44px 1fr; column-gap: 10px; align-items: center; }
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
  background: rgba(0,0,0,0.35) !important;
  color: #c9d1d9 !important;
  text-shadow: -1px -1px 0 rgba(0,0,0,0.6), 1px -1px 0 rgba(0,0,0,0.6), -1px 1px 0 rgba(0,0,0,0.6), 1px 1px 0 rgba(0,0,0,0.6) !important;
}
.theme-light .github-card-footer span { 
  background: rgba(255,255,255,0.65) !important;
  color: #111827 !important;
  border: 1px solid rgba(0,0,0,0.1) !important;
  text-shadow: -1px -1px 0 rgba(255,255,255,0.7), 1px -1px 0 rgba(255,255,255,0.7), -1px 1px 0 rgba(255,255,255,0.7), 1px 1px 0 rgba(255,255,255,0.7) !important;
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
:global(html:not(.dark)) .markdown-preview :deep(a),
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
