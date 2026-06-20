<template>
  <div ref="editorContainer" class="vditor-container"></div>
  <Teleport to="body">
    <div
      v-if="showHeadingMenu"
      ref="headingMenuRef"
      :class="['floating-control-menu visibility-floating-menu nw-floating-menu vditor-heading-menu', { 'is-dark': props.theme === 'dark' }]"
      :style="headingMenuStyle"
      role="listbox"
      @mousedown.prevent.stop
      @click.stop
    >
      <button
        v-for="option in headingOptions"
        :key="option.tag"
        type="button"
        :class="['floating-control-option nw-floating-option', { 'is-selected': selectedHeadingTag === option.tag }]"
        :aria-selected="selectedHeadingTag === option.tag"
        role="option"
        @mousedown.prevent.stop
        @click="selectHeading(option)"
      >
        {{ option.label }}
      </button>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, nextTick } from "vue";
import { positionFloatingMenu, scheduleFloatingMenuPosition } from '~/utils/floating-menu'
import Vditor from "vditor";
import { Fancybox } from "@fancyapps/ui";
import "@fancyapps/ui/dist/fancybox/fancybox.css";
import "vditor/dist/index.css";

const props = defineProps({
  modelValue: {
    type: String,
    default: "",
  },
  theme: {
    type: String,
    default: 'classic'
  }
});

const emit = defineEmits(["update:modelValue", "ready"]);

const editorContainer = ref<HTMLElement>();
let vditorInstance: Vditor | null = null;
let toolbarEl: HTMLElement | null = null;
let placeholderEl: HTMLElement | null = null;
let mutationObserver: MutationObserver | null = null;
let fixedCleanup: (() => void) | null = null;
let panelCleanup: (() => void) | null = null;
let imagePreviewCleanup: (() => void) | null = null;
let attachmentPreviewCleanup: (() => void) | null = null;
const isReady = ref(false);
const showHeadingMenu = ref(false);
const headingMenuRef = ref<HTMLElement | null>(null);
const headingMenuStyle = ref<Record<string, string>>({});
const selectedHeadingTag = ref('');
const headingTrigger = ref<HTMLElement | null>(null);
const nativeHeadingPanel = ref<HTMLElement | null>(null);
const headingOptions = [
  { tag: 'h1', value: '# ', label: '一级标题 <Alt+Ctrl+1>' },
  { tag: 'h2', value: '## ', label: '二级标题 <Alt+Ctrl+2>' },
  { tag: 'h3', value: '### ', label: '三级标题 <Alt+Ctrl+3>' },
  { tag: 'h4', value: '#### ', label: '四级标题 <Alt+Ctrl+4>' },
  { tag: 'h5', value: '##### ', label: '五级标题 <Alt+Ctrl+5>' },
  { tag: 'h6', value: '###### ', label: '六级标题 <Alt+Ctrl+6>' }
];

type EditorAttachmentInfo = { type: 'image' | 'video' | 'audio'; title: string; name: string; url: string }
const ATTACHMENT_MARKER_RE = /\[(图片附件|视频附件|音频附件)：([^\]]+)\]\(([^)\s]+)\)/
const ATTACHMENT_ANCHOR_LABEL_RE = /^(图片附件|视频附件|音频附件)：(.+)$/

const normalizeAttachmentInfo = (kindLabel: string, name: string, url: string): EditorAttachmentInfo | null => {
  const href = String(url || '').trim()
  if (!href) return null
  const type = kindLabel === '图片附件' ? 'image' : (kindLabel === '视频附件' ? 'video' : 'audio')
  const cleanName = String(name || '').trim() || '未命名附件'
  return { type, title: `${kindLabel}：${cleanName}`, name: cleanName, url: href }
}

const attachmentInfoFromText = (text: string) => {
  const match = String(text || '').match(ATTACHMENT_MARKER_RE)
  if (!match) return null
  return normalizeAttachmentInfo(match[1], match[2], match[3])
}

const escapeAttachmentHtmlAttr = (value: string) => String(value || '')
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/\"/g, '&quot;')
  .replace(/'/g, '&#39;')

const getAttachmentFancyboxOptions = (startIndex = 0) => ({
  animated: true,
  backdropClick: 'close' as const,
  closeButton: true,
  contentClick: false as const,
  dragToClose: false,
  startIndex,
  Carousel: { infinite: false }
})

const buildVideoFancyboxHtml = (info: EditorAttachmentInfo) => {
  const safeUrl = escapeAttachmentHtmlAttr(info.url)
  const safeName = escapeAttachmentHtmlAttr(info.name)
  return `<div class="editor-attachment-fancybox-video"><video src="${safeUrl}" controls preload="metadata" playsinline aria-label="${safeName}"></video></div>`
}

const showAttachmentGallery = (items: EditorAttachmentInfo[], current: EditorAttachmentInfo) => {
  const sameType = items.filter((item) => item.type === current.type)
  const galleryItems = sameType.length ? sameType : [current]
  const startIndex = Math.max(0, galleryItems.findIndex((item) => item.url === current.url && item.name === current.name))
  const slides = galleryItems.map((item) => item.type === 'video'
    ? { src: buildVideoFancyboxHtml(item), type: 'html', caption: item.name }
    : { src: item.url, type: 'image', caption: item.name }
  )
  try {
    Fancybox.show(slides as any, getAttachmentFancyboxOptions(startIndex))
  } catch {
    try {
      ;(window as any).Fancybox?.show?.(slides, getAttachmentFancyboxOptions(startIndex))
    } catch {}
  }
}

const showImageInProjectViewer = (info: EditorAttachmentInfo) => showAttachmentGallery([info], info)

const setupInlineImagePreview = () => {
  const root = editorContainer.value;
  if (!root) return;

  const onImageClick = (event: MouseEvent) => {
    const img = (event.target as HTMLElement | null)?.closest('.vditor-reset img') as HTMLImageElement | null;
    if (!img || !root.contains(img) || img.closest('.vditor-toolbar, .vditor-panel, .vditor-hint, .editor-attachment-preview')) return;
    const src = img.currentSrc || img.src || img.getAttribute('src') || '';
    if (!src) return;
    event.preventDefault();
    event.stopPropagation();
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    showImageInProjectViewer({ type: 'image', title: img.alt || '图片预览', name: img.alt || '图片预览', url: src });
  };

  root.addEventListener('click', onImageClick, true);
  imagePreviewCleanup = () => root.removeEventListener('click', onImageClick, true);
};

const attachmentInfoFromAnchor = (anchor: HTMLAnchorElement | null) => {
  if (!anchor) return null
  const label = (anchor.textContent || '').trim()
  const match = label.match(ATTACHMENT_ANCHOR_LABEL_RE)
  const href = anchor.getAttribute('data-attachment-url') || anchor.getAttribute('href') || anchor.href || ''
  if (match && href) return normalizeAttachmentInfo(match[1], match[2], href)
  return null
}

const attachmentInfoFromIrNode = (node: HTMLElement | null) => {
  if (!node) return null
  const label = (node.querySelector<HTMLElement>('.vditor-ir__link')?.textContent || '').trim()
  const url = (node.querySelector<HTMLElement>('.vditor-ir__marker--link')?.textContent || '').trim()
  const match = label.match(ATTACHMENT_ANCHOR_LABEL_RE)
  if (match && url) return normalizeAttachmentInfo(match[1], match[2], url)
  return null
}

const attachmentInfoFromIrLabel = (label: HTMLElement | null) => {
  if (!label?.classList.contains('vditor-ir__link')) return null
  const node = label.closest<HTMLElement>('[data-type="a"]')
  return attachmentInfoFromIrNode(node)
}

const setupAttachmentPreview = () => {
  const root = editorContainer.value
  if (!root || attachmentPreviewCleanup) return

  const refreshAttachmentLinks = () => {
    root.querySelectorAll('a').forEach((node) => {
      const anchor = node as HTMLAnchorElement
      const info = attachmentInfoFromAnchor(anchor)
      anchor.classList.toggle('editor-attachment-link', !!info)
      if (!info) return
      anchor.setAttribute('role', 'button')
      anchor.setAttribute('aria-label', `预览${info.title}`)
      anchor.setAttribute('data-attachment-kind', info.type)
      anchor.setAttribute('data-attachment-url', info.url)
      anchor.removeAttribute('title')
      anchor.setAttribute('draggable', 'false')
    })

    root.querySelectorAll('[data-type="a"]').forEach((node) => {
      const marker = node as HTMLElement
      const label = marker.querySelector<HTMLElement>('.vditor-ir__link')
      const info = attachmentInfoFromIrNode(marker)
      marker.classList.toggle('editor-attachment-node', !!info)
      label?.classList.toggle('editor-attachment-link', !!info)
      if (!label) return
      if (!info) {
        label.removeAttribute('role')
        label.removeAttribute('aria-label')
        label.removeAttribute('data-attachment-kind')
        label.removeAttribute('data-attachment-url')
        return
      }
      label.setAttribute('role', 'button')
      label.setAttribute('aria-label', `预览${info.title}`)
      label.setAttribute('data-attachment-kind', info.type)
      label.setAttribute('data-attachment-url', info.url)
    })
  }

  const closeSiblingPreview = (block: HTMLElement) => {
    const next = block.nextElementSibling as HTMLElement | null
    if (next?.classList.contains('editor-attachment-preview')) {
      next.remove()
      return true
    }
    return false
  }

  const toggleAttachmentPreview = (target: HTMLElement, fallbackInfo?: EditorAttachmentInfo | null) => {
    const anchor = target.closest('a.editor-attachment-link') as HTMLAnchorElement | null
    const markerNode = target.closest<HTMLElement>('[data-type="a"].editor-attachment-node')
    const block = (target.closest('p, li') || markerNode?.closest('p, li') || target.closest('pre.vditor-reset, .vditor-ir__node, pre') || anchor?.closest('pre.vditor-reset, .vditor-ir__node, p, li, pre') || target.parentElement) as HTMLElement | null
    const info = attachmentInfoFromAnchor(anchor) || attachmentInfoFromIrLabel(target) || fallbackInfo
    if (!info || !block || !root.contains(block)) return

    if (info.type === 'image' || info.type === 'video') {
      root.querySelectorAll('.editor-attachment-preview').forEach((node) => node.remove())
      showAttachmentGallery(getAttachmentInfosByType(info.type), info)
      return
    }

    if (closeSiblingPreview(block)) return

    root.querySelectorAll('.editor-attachment-preview').forEach((node) => node.remove())
    const preview = document.createElement('div')
    preview.className = `editor-attachment-preview editor-attachment-preview--${info.type}`
    preview.setAttribute('contenteditable', 'false')

    const header = document.createElement('div')
    header.className = 'editor-attachment-preview__header'
    header.textContent = info.title
    preview.appendChild(header)

    const audio = document.createElement('audio')
    audio.src = info.url
    audio.controls = true
    audio.preload = 'metadata'
    preview.appendChild(audio)
    block.insertAdjacentElement('afterend', preview)
  }

  const attachmentTargetFromEvent = (event: Event): { target: HTMLElement; info: EditorAttachmentInfo } | null => {
    const target = event.target as HTMLElement | null
    if (!target || !root.contains(target)) return null

    const irLabel = target.closest<HTMLElement>('.vditor-ir__link.editor-attachment-link')
    if (irLabel && root.contains(irLabel)) {
      const irInfo = attachmentInfoFromIrLabel(irLabel)
      if (irInfo) return { target: irLabel, info: irInfo }
    }

    const anchor = target.closest('a.editor-attachment-link') as HTMLAnchorElement | null
    if (!anchor || !root.contains(anchor)) return null
    const anchorInfo = attachmentInfoFromAnchor(anchor)
    if (anchorInfo) return { target: anchor, info: anchorInfo }
    return null
  }

  const getAttachmentInfosByType = (type: EditorAttachmentInfo['type']) => {
    const seen = new Set<string>()
    const items: EditorAttachmentInfo[] = []
    const pushInfo = (info: EditorAttachmentInfo | null) => {
      if (!info || info.type !== type) return
      const key = `${info.type}\n${info.url}\n${info.name}`
      if (seen.has(key)) return
      seen.add(key)
      items.push(info)
    }
    root.querySelectorAll('a.editor-attachment-link').forEach((node) => pushInfo(attachmentInfoFromAnchor(node as HTMLAnchorElement)))
    root.querySelectorAll('[data-type="a"].editor-attachment-node').forEach((node) => pushInfo(attachmentInfoFromIrNode(node as HTMLElement)))
    return items
  }

  const preventAttachmentNavigation = (event: Event) => {
    const hit = attachmentTargetFromEvent(event)
    if (!hit) return
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
  }

  const onAttachmentClick = (event: MouseEvent) => {
    const hit = attachmentTargetFromEvent(event)
    if (!hit) return
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    toggleAttachmentPreview(hit.target, hit.info)
  }

  const onAttachmentKeydown = (event: KeyboardEvent) => {
    if (event.key !== 'Enter' && event.key !== ' ') return
    const hit = attachmentTargetFromEvent(event)
    if (!hit) return
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    toggleAttachmentPreview(hit.target, hit.info)
  }

  refreshAttachmentLinks()
  const previewObserver = new MutationObserver(() => refreshAttachmentLinks())
  previewObserver.observe(root, { childList: true, subtree: true, characterData: true })
  root.addEventListener('pointerdown', preventAttachmentNavigation, true)
  root.addEventListener('mousedown', preventAttachmentNavigation, true)
  root.addEventListener('click', onAttachmentClick, true)
  root.addEventListener('keydown', onAttachmentKeydown, true)
  attachmentPreviewCleanup = () => {
    previewObserver.disconnect()
    root.removeEventListener('pointerdown', preventAttachmentNavigation, true)
    root.removeEventListener('mousedown', preventAttachmentNavigation, true)
    root.removeEventListener('click', onAttachmentClick, true)
    root.removeEventListener('keydown', onAttachmentKeydown, true)
    root.querySelectorAll('.editor-attachment-preview').forEach((node) => node.remove())
    attachmentPreviewCleanup = null
  }
}

const editorOptions: IOptions = {
  mode: "ir",
  height: "auto",
  minHeight: 150,
  resize: {
    enable: true,
    position: 'bottom'
  },
  icon: "ant",
  lang: "zh_CN" as keyof II18n,
  theme: "classic",
  toolbar: [
    "headings",
    "bold",
    "italic",
    "strike",
    "link",
    "|",
    "list",
    "ordered-list",
    "check",
    "|",
    "quote",
    "line",
    "code",
    "inline-code",
    "table",
    "undo",
    "redo",
    "|",
    "preview",
    "fullscreen"
  ],
  toolbarConfig: {
    pin: true,
  },
  counter: {
    enable: false,
  },
  cache: {
    enable: true,
    id: "vue-vditor",
  },
  input: (content: string) => {
    emit("update:modelValue", content);
  },
  preview: {
    hljs: {
      style: "native",
    },
    markdown: {  
      listStyle: true,
      mark: true,
    },
    actions: [],
  },
  placeholder: "灵感记录~"
};

const getCurrentHeadingTag = () => {
  const selection = typeof window !== 'undefined' ? window.getSelection() : null
  const node = selection?.anchorNode || null
  const element = node instanceof Element ? node : node?.parentElement || null
  const heading = element?.closest?.('h1,h2,h3,h4,h5,h6') as HTMLElement | null
  if (heading?.tagName) return heading.tagName.toLowerCase()
  const block = element?.closest?.('.vditor-ir__node, [data-type="heading"]') as HTMLElement | null
  const marker = block?.querySelector?.('.vditor-ir__marker--heading, [data-type="heading-marker"]') as HTMLElement | null
  const markerText = (marker?.textContent || '').trim()
  const level = markerText.match(/^#{1,6}/)?.[0]?.length || 0
  return level ? `h${level}` : ''
}

const positionHeadingMenu = () => {
  positionFloatingMenu(headingTrigger.value, headingMenuRef.value, headingMenuStyle, 152, 'above-align-left')
}

const closeHeadingMenu = () => {
  showHeadingMenu.value = false
  nativeHeadingPanel.value?.classList.add('vditor-panel--none')
  if (nativeHeadingPanel.value) nativeHeadingPanel.value.style.display = 'none'
}

const applyHeadingFallback = (option: typeof headingOptions[number]) => {
  if (!vditorInstance) return
  const value = vditorInstance.getValue?.() || ''
  if (!value.trim()) {
    vditorInstance.setValue(option.value)
    emit('update:modelValue', vditorInstance.getValue?.() || option.value)
    return
  }
  const editorRoot = editorContainer.value
  const active = typeof document !== 'undefined' ? document.activeElement as HTMLElement | null : null
  const selection = typeof window !== 'undefined' ? window.getSelection() : null
  const selectedText = selection?.toString() || ''
  let lineIndex = 0

  const focusedBlock = active?.closest?.('.vditor-ir__node, .vditor-reset [data-block], .vditor-reset p, .vditor-reset h1, .vditor-reset h2, .vditor-reset h3, .vditor-reset h4, .vditor-reset h5, .vditor-reset h6')
  const focusedText = (focusedBlock?.textContent || '').trim()
  const lines = value.split('\n')
  if (focusedText) {
    const normalizedFocused = focusedText.replace(/^#{1,6}\s+/, '').trim()
    const found = lines.findIndex((line) => line.replace(/^#{1,6}\s+/, '').trim() === normalizedFocused)
    if (found >= 0) lineIndex = found
  } else if (selectedText) {
    const found = lines.findIndex((line) => line.includes(selectedText.trim()))
    if (found >= 0) lineIndex = found
  } else if (editorRoot) {
    const editableText = editorRoot.querySelector<HTMLElement>('.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset, .vditor-reset')?.textContent || ''
    const firstLine = editableText.split('\n').find((line) => line.trim())?.trim()
    const found = firstLine ? lines.findIndex((line) => line.replace(/^#{1,6}\s+/, '').trim() === firstLine.replace(/^#{1,6}\s+/, '').trim()) : -1
    if (found >= 0) lineIndex = found
  }

  if (!lines.length) lines.push('')
  const text = lines[lineIndex] || ''
  const content = text.replace(/^#{1,6}\s*/, '').trimStart()
  lines[lineIndex] = `${option.value}${content || '标题'}`
  const nextValue = lines.join('\n')
  vditorInstance.setValue(nextValue)
  emit('update:modelValue', vditorInstance.getValue?.() || nextValue)
}

const selectHeading = (option: typeof headingOptions[number]) => {
  applyHeadingFallback(option)
  selectedHeadingTag.value = option.tag
  closeHeadingMenu()
}

const setupVditorPanelPositioning = () => {
  if (panelCleanup) return

  const isHeadingsItem = (item: HTMLElement | null) => {
    if (!item) return false
    const action = item.matches('[data-type="headings"]')
      ? item
      : item.querySelector<HTMLElement>('[data-type="headings"], [aria-label*="标题"], [aria-label*="Heading"], [title*="标题"], [title*="Heading"]')
    const type = action?.getAttribute('data-type') || ''
    const label = action?.getAttribute('aria-label') || action?.getAttribute('title') || ''
    return type === 'headings' || /标题|Heading|Headings/i.test(label)
  }

  const openHeadingMenu = async (item: HTMLElement) => {
    headingTrigger.value = item
    nativeHeadingPanel.value = item.querySelector<HTMLElement>('.vditor-hint, .vditor-panel')
    if (nativeHeadingPanel.value) {
      nativeHeadingPanel.value.classList.add('vditor-panel--none')
      nativeHeadingPanel.value.style.display = 'none'
    }
    selectedHeadingTag.value = getCurrentHeadingTag()
    showHeadingMenu.value = true
    await nextTick()
    scheduleFloatingMenuPosition(positionHeadingMenu)
  }

  const handleToolbarClick = (event: Event) => {
    const target = event.target instanceof Element ? event.target : null
    const item = target?.closest('.vditor-toolbar__item') as HTMLElement | null
    if (!item || !editorContainer.value?.contains(item)) return
    if (!item || !isHeadingsItem(item)) return
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    if (showHeadingMenu.value && headingTrigger.value === item) {
      closeHeadingMenu()
      return
    }
    openHeadingMenu(item)
  }

  const handleDocumentPointer = (event: Event) => {
    if (!showHeadingMenu.value) return
    const target = event.target instanceof Element ? event.target : null
    if (target?.closest('.vditor-heading-menu')) return
    if (target?.closest('.vditor-toolbar__item') === headingTrigger.value) return
    closeHeadingMenu()
  }

  const handleFloatingReposition = () => {
    if (showHeadingMenu.value) scheduleFloatingMenuPosition(positionHeadingMenu)
  }
  const scrollContainers = Array.from(document.querySelectorAll('.center-col, .content-wrapper')) as HTMLElement[]
  toolbarEl?.addEventListener('click', handleToolbarClick, true)
  document.addEventListener('click', handleToolbarClick, true)
  document.addEventListener('mousedown', handleDocumentPointer, true)
  window.addEventListener('resize', handleFloatingReposition)
  window.addEventListener('scroll', handleFloatingReposition, { passive: true, capture: true })
  scrollContainers.forEach((el) => el.addEventListener('scroll', handleFloatingReposition, { passive: true }))
  window.visualViewport?.addEventListener('resize', handleFloatingReposition, { passive: true })
  window.visualViewport?.addEventListener('scroll', handleFloatingReposition, { passive: true })
  panelCleanup = () => {
    toolbarEl?.removeEventListener('click', handleToolbarClick, true)
    document.removeEventListener('click', handleToolbarClick, true)
    document.removeEventListener('mousedown', handleDocumentPointer, true)
    window.removeEventListener('resize', handleFloatingReposition)
    window.removeEventListener('scroll', handleFloatingReposition, true)
    scrollContainers.forEach((el) => el.removeEventListener('scroll', handleFloatingReposition))
    window.visualViewport?.removeEventListener('resize', handleFloatingReposition)
    window.visualViewport?.removeEventListener('scroll', handleFloatingReposition)
    closeHeadingMenu()
    panelCleanup = null
  }
}

onMounted(async () => {
  if (!editorContainer.value) return;

  const opts: IOptions = {
    ...editorOptions,
    theme: props.theme === 'dark' ? 'dark' : 'classic',
    preview: {
      ...editorOptions.preview,
      hljs: { style: props.theme === 'dark' ? 'native' : 'github' }
    },
    after: () => {
      vditorInstance?.setValue(props.modelValue);
      vditorInstance?.setTheme(props.theme === 'dark' ? 'dark' : 'classic');
      isReady.value = true;
      emit("ready");
      nextTick(() => {
        setupFixedToolbar();
        window.setTimeout(setupFixedToolbar, 80);
      });
    },
  }
  vditorInstance = new Vditor(editorContainer.value, opts);
  setupVditorPanelPositioning();
  // 等待渲染完成后设置工具栏固定到视窗顶部
  const setupFixedToolbar = () => {
    const root = editorContainer.value?.querySelector('.vditor') as HTMLElement | null;
    toolbarEl = root?.querySelector('.vditor-toolbar') as HTMLElement | null;
    if (!root || !toolbarEl) return;
    setupVditorPanelPositioning();
    if (placeholderEl) return;

    // 占位元素，避免工具栏脱离文档流后遮挡内容
    placeholderEl = document.createElement('div');
    placeholderEl.style.width = '100%';
    placeholderEl.style.height = `${toolbarEl.offsetHeight}px`;
    placeholderEl.style.pointerEvents = 'none';
    root.insertBefore(placeholderEl, toolbarEl.nextSibling);

    const updateToolbarPosition = () => {
      if (!root || !toolbarEl) return;
      const isFullscreen = root.classList.contains('vditor--fullscreen');
      const h = toolbarEl.offsetHeight;

      if (isFullscreen) {
        toolbarEl.style.position = 'fixed';
        toolbarEl.style.top = '0px';
        toolbarEl.style.left = '0px';
        toolbarEl.style.width = `${window.innerWidth}px`;
        toolbarEl.style.zIndex = '1002';
        if (placeholderEl) placeholderEl.style.height = `${h}px`;
        return;
      }

      // 保证容器可作为绝对定位参考
      root.style.position = root.style.position || 'relative';

      const rect = root.getBoundingClientRect();
      const shouldStick = rect.top < 0 && rect.bottom > h;
      const reachedTop = rect.top >= 0;
      const reachedBottom = rect.bottom <= h;

      if (shouldStick) {
        // 在容器范围内贴顶滚动
        toolbarEl.style.position = 'fixed';
        toolbarEl.style.top = '0px';
        toolbarEl.style.left = `${rect.left}px`;
        toolbarEl.style.width = `${rect.width}px`;
      } else if (reachedTop) {
        // 还未到达视窗顶端，保持在容器顶部
        toolbarEl.style.position = 'absolute';
        toolbarEl.style.top = '0px';
        toolbarEl.style.left = '0px';
        toolbarEl.style.width = '100%';
      } else if (reachedBottom) {
        // 接近容器底部，固定在容器底端，避免越界
        const containerHeight = root.offsetHeight;
        toolbarEl.style.position = 'absolute';
        toolbarEl.style.top = `${containerHeight - h}px`;
        toolbarEl.style.left = '0px';
        toolbarEl.style.width = '100%';
      }

      toolbarEl.style.zIndex = '1002';
      if (placeholderEl) placeholderEl.style.height = `${h}px`;
    };

    const scrollContainers = Array.from(document.querySelectorAll('.center-col, .content-wrapper')) as HTMLElement[];
    scrollContainers.forEach((el) => el.addEventListener('scroll', updateToolbarPosition, { passive: true }));
    window.addEventListener('resize', updateToolbarPosition);
    window.addEventListener('scroll', updateToolbarPosition, { passive: true });
    updateToolbarPosition();

    mutationObserver = new MutationObserver(() => updateToolbarPosition());
    mutationObserver.observe(root, { attributes: true, attributeFilter: ['class'] });

    fixedCleanup = () => {
      scrollContainers.forEach((el) => el.removeEventListener('scroll', updateToolbarPosition));
      window.removeEventListener('resize', updateToolbarPosition);
      window.removeEventListener('scroll', updateToolbarPosition);
      mutationObserver?.disconnect();
      mutationObserver = null;
      if (toolbarEl) {
        toolbarEl.style.position = '';
        toolbarEl.style.top = '';
        toolbarEl.style.left = '';
        toolbarEl.style.width = '';
        toolbarEl.style.zIndex = '';
      }
      if (placeholderEl) {
        placeholderEl.remove();
        placeholderEl = null;
      }
    };
  };

  // Vditor 的 toolbar 由内部异步渲染，分几次尝试避免错过绑定时机。
  nextTick(() => {
    setupFixedToolbar();
    window.setTimeout(setupFixedToolbar, 50);
    window.setTimeout(setupFixedToolbar, 250);
    window.setTimeout(setupFixedToolbar, 1000);
    window.setTimeout(setupFixedToolbar, 2000);
    setupInlineImagePreview();
    setupAttachmentPreview();
  });
});

onBeforeUnmount(() => {
  try {
    if (vditorInstance) {
      vditorInstance.destroy();
      vditorInstance = null;
    }
    if (fixedCleanup) {
      fixedCleanup();
      fixedCleanup = null;
    }
    if (panelCleanup) {
      panelCleanup();
      panelCleanup = null;
    }
    if (imagePreviewCleanup) {
      imagePreviewCleanup();
      imagePreviewCleanup = null;
    }
    if (attachmentPreviewCleanup) {
      attachmentPreviewCleanup();
      attachmentPreviewCleanup = null;
    }
  } catch (e) {
    console.warn('Vditor destroy error', e);
  }
});

defineExpose({
  clear: () => {
    if (vditorInstance) {
      vditorInstance.setValue('');
      emit("update:modelValue", '');
    }
  },
  insertValue: (val: string) => {
    if (vditorInstance) {
      vditorInstance.insertValue(val);
      emit("update:modelValue", vditorInstance.getValue());
    }
  },
  getValue: (): string => {
    return vditorInstance ? vditorInstance.getValue() : ''
  },
  setValue: (val: string) => {
    if (vditorInstance) {
      vditorInstance.setValue(val)
      emit("update:modelValue", vditorInstance.getValue())
    } else {
      emit("update:modelValue", val || '')
    }
  }
});

watch(() => props.theme, (newTheme) => {
  if (vditorInstance) {
    vditorInstance.setTheme(newTheme === 'dark' ? 'dark' : 'classic');
  }
});
</script>

<style>
.vditor-container {
  --publish-editor-font: "LXGW WenKai Screen";
  border-radius: 8px;
  margin-bottom: 12px;
  position: relative;
  overflow: visible;
  font-family: var(--publish-editor-font);
}
.vditor-content {
  position: relative;
  z-index: 1;
}

.vditor-container .editor-attachment-link,
.vditor-container .editor-attachment-link * {
  cursor: pointer !important;
}

.vditor-container .editor-attachment-link {
  text-decoration-style: dotted;
  text-underline-offset: 3px;
  -webkit-user-drag: none;
  pointer-events: auto;
}

.editor-attachment-fancybox-video {
  width: min(92vw, 960px);
  max-height: 82vh;
}

.editor-attachment-fancybox-video video {
  display: block;
  width: 100%;
  max-height: 82vh;
  background: #000;
  border-radius: 8px;
}

.editor-attachment-preview {
  margin: 6px 12px 10px;
  padding: 8px;
  border: 1px solid rgba(148, 163, 184, 0.28);
  border-radius: 10px;
  background: rgba(248, 250, 252, 0.92);
  box-shadow: 0 8px 22px rgba(15, 23, 42, 0.08);
}

.editor-attachment-preview__header {
  margin-bottom: 6px;
  color: rgba(71, 85, 105, 0.95);
  font-size: 12px;
  font-weight: 650;
  line-height: 1.35;
  word-break: break-all;
}

.editor-attachment-preview img,
.editor-attachment-preview video,
.editor-attachment-preview audio {
  display: block;
  width: 100%;
  max-width: 100%;
}

.editor-attachment-preview img {
  height: auto;
  border-radius: 8px;
}

.editor-attachment-preview audio {
  margin: 0;
}

html.dark .editor-attachment-preview,
.vditor--dark .editor-attachment-preview {
  border-color: rgba(148, 163, 184, 0.28);
  background: rgba(30, 41, 59, 0.78);
  box-shadow: 0 10px 24px rgba(2, 6, 23, 0.28);
}

html.dark .editor-attachment-preview__header,
.vditor--dark .editor-attachment-preview__header {
  color: rgba(226, 232, 240, 0.86);
}
.vditor-container:hover {
  border-color: #90a4ae;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
}
.vditor-reset ol {
  list-style-type: decimal;
  padding-left: 2em;
}

.vditor-reset ul {
  list-style-type: disc;
  padding-left: 2em;
}

.vditor-ir .vditor-ir__list {
  counter-reset: list-counter;
}

.vditor-ir .vditor-ir__list--ordered > .vditor-ir__list-item::before {
  content: counter(list-counter) ".";
  counter-increment: list-counter;
}
.vditor-toolbar {
  display: flex !important;
  flex-wrap: nowrap !important;
  align-items: center !important;
  justify-content: stretch;
  overflow: hidden !important;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  white-space: nowrap;
  scrollbar-width: none;
  -ms-overflow-style: none;
  background-color: #f8f9fab7;
  border-bottom: none;
  z-index: 100;
  box-sizing: border-box;
  gap: 2px;
  padding: 0 !important;
}

.vditor-toolbar > * {
  flex: 1 1 0 !important;
  min-width: 0 !important;
}

.vditor-toolbar__br {
  display: none !important;
}

.vditor-toolbar::-webkit-scrollbar {
  display: none; /* Chrome, Safari and Opera */
}

.vditor-toolbar--pin { padding:0 !important; background-color:#f8f9fa; border-bottom:none; z-index:101; }

/* 修改弹出面板样式 */
.vditor-panel--none {
  display: none !important;
}

.vditor-panel {
  position: fixed; /* 恢复为 fixed，避免被容器裁剪 */
  z-index: 10000;
  display: grid;
  gap: 4px;
  padding: 8px;
  background: var(--nw-floating-bg) !important;
  color: var(--nw-floating-text) !important;
  box-shadow: var(--nw-floating-shadow);
  border-radius: 12px;
  border: 1px solid var(--nw-floating-border);
  max-height: 50vh;
  overflow: auto;
}

.vditor-panel button,
.vditor-panel .vditor-menu,
.vditor-panel .vditor-toolbar__item {
  display: flex !important;
  align-items: center;
  justify-content: flex-start;
  gap: 8px;
  min-height: 32px;
  width: 100% !important;
  min-width: 106px !important;
  padding: 0 10px !important;
  border: 1px solid transparent !important;
  border-radius: 9px !important;
  background: transparent !important;
  color: inherit !important;
  font-size: 12px;
  font-weight: 650;
  line-height: 1;
  text-align: left;
}

.vditor-panel button:hover,
.vditor-panel button:focus-visible,
.vditor-panel .vditor-menu:hover,
.vditor-panel .vditor-menu:focus-visible,
.vditor-panel .vditor-toolbar__item:hover,
.vditor-panel .vditor-toolbar__item:focus-visible {
  outline: none;
  border-color: var(--nw-floating-hover-border) !important;
  background: var(--nw-floating-hover-bg) !important;
}

.vditor-panel .vditor-menu--current,
.vditor-panel .vditor-menu--active,
.vditor-panel [aria-selected="true"] {
  border-color: var(--nw-floating-selected-border) !important;
  background: var(--nw-floating-selected-bg) !important;
  color: var(--nw-floating-text) !important;
}
.vditor-hint {
  position: fixed;
  z-index: 10000;
  background: #fff;
  box-shadow: 0 8px 24px rgba(0,0,0,.16);
  border-radius: 8px;
  border: 1px solid #e9ecef;
  max-height: 50vh;
  overflow: auto;
}
.vditor-tip, .vditor-tooltip { position: fixed; z-index: 10000; }
.vditor-toolbar .vditor-tooltipped::after,
.vditor-toolbar .vditor-tooltipped::before {
  content: none !important;
  display: none !important;
}
.vditor-toolbar .vditor-tooltipped__s::before,
.vditor-toolbar .vditor-tooltipped__se::before,
.vditor-toolbar .vditor-tooltipped__sw::before,
.vditor-toolbar .vditor-tooltipped__n::before,
.vditor-toolbar .vditor-tooltipped__ne::before,
.vditor-toolbar .vditor-tooltipped__nw::before,
.vditor-toolbar .vditor-tooltipped__e::before,
.vditor-toolbar .vditor-tooltipped__w::before {
  border-color: transparent !important;
}
.vditor-toolbar__item {
  display: inline-flex !important;
  align-items: center !important;
  justify-content: center !important;
  flex: 1 1 0 !important;
  width: auto;
  min-width: 0;
  height: 34px;
  padding: 0 !important;
  margin: 0 !important;
  line-height: 1 !important;
  transition: all 0.2s ease;
}

.vditor-toolbar__item svg,
.vditor-toolbar__item .vditor-icon {
  display: block !important;
  width: 16px !important;
  height: 16px !important;
  margin: auto !important;
}

.vditor-toolbar__item:first-child {
  margin-left: 0 !important;
}

.vditor-toolbar__item:last-child {
  margin-left: 0 !important;
  margin-right: 0 !important;
}

.vditor-toolbar__item[data-type="|"],
.vditor-toolbar__item--divider,
.vditor-toolbar__divider {
  flex: 0 0 1px !important;
  width: 1px !important;
  min-width: 1px !important;
  max-width: 1px !important;
  padding: 0 !important;
  margin: 0 3px !important;
}

.vditor-toolbar__item:hover {
  background-color: var(--nw-floating-hover-bg) !important;
  border-color: var(--nw-floating-hover-border) !important;
  border-radius: 4px;
}

.vditor-container .vditor,
.vditor-container .vditor-content,
.vditor-container .vditor-ir,
.vditor-container .vditor-ir pre.vditor-reset,
.vditor-container .vditor-ir pre.vditor-reset *,
.vditor-container .vditor-ir__node,
.vditor-container .vditor-ir__node *,
.vditor-container .vditor-wysiwyg .vditor-reset,
.vditor-container .vditor-wysiwyg .vditor-reset *,
.vditor-container .vditor-sv .vditor-reset,
.vditor-container .vditor-sv .vditor-reset * {
  font-family: var(--publish-editor-font) !important;
  font-weight: 400 !important;
  letter-spacing: normal !important;
}

.vditor-ir pre.vditor-reset {
  padding: 8px 12px !important;
  color: #1a2634 !important;
  line-height: 1.5;
  font-size: 14px;
  min-height: 120px !important;
}

.vditor-container .vditor-ir pre.vditor-reset:empty::before,
.vditor-container .vditor-ir pre.vditor-reset[placeholder]:empty::before,
.vditor-container .vditor-wysiwyg pre.vditor-reset:empty::before,
.vditor-container .vditor-wysiwyg pre.vditor-reset[placeholder]:empty::before,
.vditor-container .vditor-sv:empty::before,
.vditor-container .vditor-sv[placeholder]:empty::before,
.vditor-container .vditor-ir__node--placeholder::before,
.vditor-container .vditor-wysiwyg__placeholder,
.vditor-container .vditor-placeholder {
  color: rgba(51, 65, 85, 0.58) !important;
  font-family: var(--publish-editor-font) !important;
  font-size: 14px !important;
  font-weight: 400 !important;
  line-height: 1.5 !important;
  letter-spacing: normal !important;
  font-style: normal !important;
}

.vditor-preview {
  background-color: rgba(231, 223, 223, 0.222) !important;
}

.vditor-reset {
  color: #111827 !important;
}

.vditor-container .vditor-reset img:not(.emoji):not(.xiaohongshu-render-image):not(.xhs-render-image):not(.rednote-render-image) {
  width: var(--inline-image-thumb-size) !important;
  height: var(--inline-image-thumb-size) !important;
  max-width: 100% !important;
  min-height: 0 !important;
  object-fit: cover;
  object-position: center;
  border-radius: 10px;
  cursor: zoom-in;
  display: inline-block;
  vertical-align: top;
}

.vditor-container .vditor-reset a > img:not(.emoji) {
  display: block;
}

.vditor-reset table {
  border-collapse: collapse;
}

.vditor-reset table th,
.vditor-reset table td {
  border: 1px solid rgba(148, 163, 184, 0.55);
  background: rgba(255, 255, 255, 0.95);
  color: #111827;
}

.vditor-reset table th {
  background: rgba(248, 250, 252, 0.98);
  font-weight: 600;
}

html.dark .vditor-container { background-color: #202a36; border: 1px solid rgba(255, 255, 255, 0.16); }

html.dark .vditor-toolbar { background-color: rgba(39, 50, 66, 0.68) !important; border-bottom: 1px solid rgba(255, 255, 255, 0.16) !important; }

html.dark .vditor-toolbar__item:hover {
  background-color: var(--nw-floating-hover-bg) !important;
  border-color: var(--nw-floating-hover-border) !important;
  color: #fff !important;
}

html.dark .vditor-ir pre.vditor-reset {
  color: #ffffff !important;
}

html.dark .vditor-container .vditor-ir pre.vditor-reset:empty::before,
html.dark .vditor-container .vditor-ir pre.vditor-reset[placeholder]:empty::before,
html.dark .vditor-container .vditor-wysiwyg pre.vditor-reset:empty::before,
html.dark .vditor-container .vditor-wysiwyg pre.vditor-reset[placeholder]:empty::before,
html.dark .vditor-container .vditor-sv:empty::before,
html.dark .vditor-container .vditor-sv[placeholder]:empty::before,
html.dark .vditor-wysiwyg .vditor-reset:empty:before,
html.dark .vditor-sv .vditor-reset:empty:before,
html.dark .vditor-ir__node--placeholder::before,
html.dark .vditor-wysiwyg__placeholder,
html.dark .vditor-placeholder {
  color: rgba(226, 232, 240, 0.82) !important;
}

html.dark .vditor-toolbar {
  color: #ffffff !important;
}

html.dark .vditor-reset {
  color: #e9ecef !important;
}

html.dark .vditor-reset table th,
html.dark .vditor-reset table td {
  border: 1px solid rgba(226, 232, 240, 0.22);
  background: rgba(39, 50, 66, 0.76);
  color: rgba(226, 232, 240, 0.96);
}

html.dark .vditor-reset table th { background: rgba(47, 59, 76, 0.88); }

html.dark .vditor-hint {
  background: #202a36;
  color: #ffffff;
  border-color: rgba(255, 255, 255, 0.1);
}

.vditor-heading-floating-menu.vditor-hint,
.vditor-heading-floating-menu.vditor-panel,
.vditor-heading-floating-menu.floating-control-menu {
  position: fixed !important;
  z-index: 5004 !important;
  box-sizing: border-box;
  display: grid !important;
  gap: 4px !important;
  min-width: 0 !important;
  width: auto !important;
  max-width: none !important;
  max-height: none !important;
  margin: 0 !important;
  padding: 8px !important;
  border: 1px solid var(--nw-floating-border) !important;
  border-radius: 12px !important;
  background: var(--nw-floating-bg) !important;
  color: var(--nw-floating-text) !important;
  box-shadow: var(--nw-floating-shadow) !important;
  opacity: 1 !important;
  line-height: 1 !important;
  list-style: none !important;
  overflow: visible !important;
  backdrop-filter: none !important;
  -webkit-backdrop-filter: none !important;
}

.vditor-heading-floating-menu.vditor-panel--arrow::before,
.vditor-heading-floating-menu::before,
.vditor-heading-floating-menu::after {
  content: none !important;
  display: none !important;
}

.vditor-heading-floating-menu.is-dark {
  --nw-floating-bg: #0f172a;
  --nw-floating-text: #f8fafc;
  --nw-floating-border: rgba(255, 255, 255, 0.18);
  --nw-floating-shadow: 0 18px 42px rgba(0, 0, 0, 0.42);
  --nw-floating-hover-bg: rgba(249, 115, 22, 0.26);
  --nw-floating-hover-border: rgba(249, 115, 22, 0.58);
  --nw-floating-selected-bg: rgba(249, 115, 22, 0.30);
  --nw-floating-selected-border: rgba(249, 115, 22, 0.70);
}

.vditor-heading-floating-menu button.floating-control-option,
.vditor-heading-floating-menu .floating-control-option {
  box-sizing: border-box;
  display: flex !important;
  align-items: center !important;
  justify-content: flex-start !important;
  gap: 8px !important;
  width: 100% !important;
  min-width: 0 !important;
  min-height: 32px !important;
  margin: 0 !important;
  padding: 0 8px !important;
  border: 1px solid transparent !important;
  border-radius: 9px !important;
  background: transparent !important;
  color: inherit !important;
  font-size: 12px !important;
  font-weight: 650 !important;
  line-height: 1 !important;
  text-align: left !important;
  white-space: nowrap !important;
}

.vditor-heading-floating-menu button.floating-control-option:hover,
.vditor-heading-floating-menu button.floating-control-option:focus-visible,
.vditor-heading-floating-menu .floating-control-option:hover,
.vditor-heading-floating-menu .floating-control-option:focus-visible {
  outline: none !important;
  border-color: var(--nw-floating-hover-border) !important;
  background: var(--nw-floating-hover-bg) !important;
}

.vditor-heading-floating-menu button.floating-control-option.is-selected,
.vditor-heading-floating-menu .floating-control-option.is-selected {
  border-color: var(--nw-floating-selected-border) !important;
  background: var(--nw-floating-selected-bg) !important;
  color: var(--nw-floating-text) !important;
}

html.dark .vditor-tooltip, html.dark .vditor-tip {
  color: #ffffff;
}


html.dark .vditor-preview { background-color: rgba(39, 50, 66, 0.68) !important; }

/* 全屏模式主题自适应 */
html.dark .vditor--fullscreen { background: #202a36 !important; }
html:not(.dark) .vditor--fullscreen { background: #ffffff !important; }
html.dark .vditor--fullscreen .vditor-toolbar { background: rgba(39, 50, 66, 0.68) !important; }
html:not(.dark) .vditor--fullscreen .vditor-toolbar { background: #f8f9fa !important; }
.vditor--fullscreen .vditor-ir pre.vditor-reset { font-size: 16px; line-height: 1.9; }

@media screen and (max-width: 520px) {
  .vditor-toolbar__item {
    padding: 4px !important;
  }
  
  .vditor-ir pre.vditor-reset {
    padding: 8px 12px !important;
    font-size: 13px;
  }
  .vditor-toolbar {
    overflow-x: auto;
    overflow-y: hidden;
    width: 100%;
    max-width: 100%;
    -webkit-overflow-scrolling: touch;
    touch-action: pan-x;
    overscroll-behavior-x: contain;
  }
}
</style>
