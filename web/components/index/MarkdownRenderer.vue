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
  const isDark = contentTheme && contentTheme.value === 'dark'
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
  try {
    const res = await fetch(`https://api.github.com/repos/${owner}/${repo}`);
    if (!res.ok) return;
    const data = await res.json();
    const card = document.getElementById(cardId);
    if (card) {
      card.innerHTML = `
        <div class="github-card-header">
          <img src="${data.owner.avatar_url}" class="github-card-avatar" />
          <div>
            <a href="${data.html_url}" target="_blank" class="github-card-title">${data.full_name}</a>
            <div class="github-card-desc">${data.description || ''}</div>
          </div>
        </div>
        <div class="github-card-footer">
          <span>⭐ ${data.stargazers_count}</span>
          <span>🍴 ${data.forks_count}</span>
          <span>🛠️ ${data.language || ''}</span>
        </div>
      `;
    }
  } catch (e) {
    // 忽略错误
  }
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

    const currentTheme = contentTheme && contentTheme.value === 'dark' ? 'dark' : 'light'
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
});


onBeforeUnmount(() => {
  if (zoom) {
    zoom.detach();
    zoom = null;
  }
});

watch(() => contentTheme && contentTheme.value, () => {
  applyThemeClass();
  renderMarkdown(props.content);
});
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
  color: #0366d6;
  text-decoration: none;
}

.markdown-preview :deep(a:hover) {
  text-decoration: underline;
}
.theme-light.markdown-preview :deep(a:hover) {
  color: #0366d6;
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

.aplayer {
  box-shadow: 0 0 10px rgba(0,0,0,0.1);
  border-radius: 4px;
  margin: 0.5em 0 !important;
}
.theme-dark .aplayer {
  background: rgba(22,27,34,0.85);
  color: #c9d1d9;
}
.theme-light .aplayer {
  background: rgba(255,255,255,0.85);
  color: #111827;
  border: 1px solid #e5e7eb;
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
  border: 1px solid #30363d;
  background: #161b22;
  color: #c9d1d9;
}
.theme-light .github-card {
  border: 1px solid #e5e7eb;
  background: #ffffff;
  color: #111827;
}
.github-card-header {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  min-width: 0;
}
.github-card-avatar {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  flex-shrink: 0;
  margin-right: 0;
  object-fit: cover;
  background: #222;
}
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
.theme-dark .github-card-title { color: #58a6ff; }
.theme-light .github-card-title { color: #0366d6; }
.github-card-desc {
  margin-top: 4px;
  font-size: 14px;
  word-break: break-all;
  white-space: pre-line;
  overflow-wrap: anywhere;
}
.theme-dark .github-card-desc { color: #8b949e; }
.theme-light .github-card-desc { color: #6b7280; }
.github-card-footer {
  margin-top: 12px;
  display: flex;
  gap: 16px;
  font-size: 13px;
  flex-wrap: wrap;
}
.theme-dark .github-card-footer { color: #8b949e; }
.theme-light .github-card-footer { color: #6b7280; }

.github-card-footer span {
  padding: 2px 6px;
  border-radius: 4px;
}
.theme-dark .github-card-footer span { 
  background: rgba(0,0,0,0.35);
  color: #c9d1d9;
  text-shadow: -1px -1px 0 rgba(0,0,0,0.6), 1px -1px 0 rgba(0,0,0,0.6), -1px 1px 0 rgba(0,0,0,0.6), 1px 1px 0 rgba(0,0,0,0.6);
}
.theme-light .github-card-footer span { 
  background: rgba(255,255,255,0.65);
  color: #111827;
  border: 1px solid rgba(0,0,0,0.1);
  text-shadow: -1px -1px 0 rgba(255,255,255,0.7), 1px -1px 0 rgba(255,255,255,0.7), -1px 1px 0 rgba(255,255,255,0.7), 1px 1px 0 rgba(255,255,255,0.7);
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

.theme-dark.markdown-preview :deep(a) {
  background-color: rgba(0,0,0,0.35);
  padding: 0 3px;
  border-radius: 4px;
  text-shadow: -1px -1px 0 rgba(0,0,0,0.6), 1px -1px 0 rgba(0,0,0,0.6), -1px 1px 0 rgba(0,0,0,0.6), 1px 1px 0 rgba(0,0,0,0.6);
}

.theme-light.markdown-preview :deep(a) {
  background-color: transparent;
  padding: 0;
  border-radius: 0;
  border: none;
  text-shadow: none;
}
.github-card-loading {
  font-style: italic;
}
.theme-dark .github-card-loading { color: #8b949e; }
.theme-light .github-card-loading { color: #6b7280; }
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
