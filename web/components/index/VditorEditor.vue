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
  <Teleport to="body">
    <div
      v-if="showTableMenu"
      ref="tableMenuRef"
      :class="['floating-control-menu visibility-floating-menu nw-floating-menu vditor-table-menu', { 'is-dark': props.theme === 'dark' }]"
      :style="tableMenuStyle"
      role="dialog"
      aria-label="插入表格"
      @mousedown.prevent.stop
      @click.stop
    >
      <div class="table-menu-row">
        <span class="table-menu-label">行</span>
        <button type="button" class="table-stepper-btn nw-action-btn" aria-label="减少行" @click="adjustTableRows(-1)">-</button>
        <span class="table-menu-value">{{ tableRows }}</span>
        <button type="button" class="table-stepper-btn nw-action-btn" aria-label="增加行" @click="adjustTableRows(1)">+</button>
      </div>
      <div class="table-menu-row">
        <span class="table-menu-label">列</span>
        <button type="button" class="table-stepper-btn nw-action-btn" aria-label="减少列" @click="adjustTableCols(-1)">-</button>
        <span class="table-menu-value">{{ tableCols }}</span>
        <button type="button" class="table-stepper-btn nw-action-btn" aria-label="增加列" @click="adjustTableCols(1)">+</button>
      </div>
      <div class="table-size-grid" aria-label="快速选择表格尺寸">
        <button
          v-for="cell in tableGridCells"
          :key="`${cell.row}-${cell.col}`"
          type="button"
          :class="['table-size-cell', { 'is-active': cell.row <= tableRows && cell.col <= tableCols }]"
          :aria-label="`${cell.row} 行 ${cell.col} 列`"
          @mouseenter="previewTableSize(cell.row, cell.col)"
          @focus="previewTableSize(cell.row, cell.col)"
          @click="insertTable(cell.row, cell.col)"
        />
      </div>
      <button type="button" class="floating-control-option nw-floating-option table-insert-btn" @click="insertTable(tableRows, tableCols)">
        插入 {{ tableRows }} 行 {{ tableCols }} 列
      </button>
    </div>
  </Teleport>
  <Teleport to="body">
    <button
      v-if="showTableDeleteButton"
      type="button"
      class="editor-table-delete-button nw-action-btn nw-action-btn--danger nw-tooltip-anchor"
      data-tooltip="删除表格"
      :style="tableDeleteButtonStyle"
      aria-label="删除该表格"
      @pointerenter="cancelTableDeleteHide"
      @pointerleave="scheduleTableDeleteHide"
      @mousedown.prevent.stop
      @click.prevent.stop="confirmDeleteHoveredTable"
    />
  </Teleport>
  <Teleport to="body">
    <button
      v-if="showTableDeleteButton"
      type="button"
      class="editor-table-expand-button nw-action-btn nw-tooltip-anchor"
      data-tooltip="放大显示表格"
      :style="tableExpandButtonStyle"
      aria-label="放大显示该表格"
      @pointerenter="cancelTableDeleteHide"
      @pointerleave="scheduleTableDeleteHide"
      @mousedown.prevent.stop
      @click.prevent.stop="openHoveredTableExpand"
    >
      <span aria-hidden="true">⛶</span>
    </button>
  </Teleport>
  <Teleport to="body">
    <div
      v-if="showTableExpandDialog"
      :class="['editor-table-expand-overlay', { 'is-dark': props.theme === 'dark', 'is-closing': tableExpandClosing }]"
      @click.self="closeExpandedTable"
    >
      <section class="editor-table-expand-dialog" role="dialog" aria-modal="true" aria-label="放大显示表格" @click.stop>
        <header class="editor-table-expand-header">
          <div>
            <strong>放大显示表格</strong>
            <span>{{ expandedTableEditable ? '可直接编辑单元格内容' : '当前表格仅可预览' }}</span>
          </div>
          <button type="button" class="editor-table-expand-close nw-action-btn nw-tooltip-anchor" data-tooltip="关闭" aria-label="关闭放大表格" @click="closeExpandedTable">
            <span class="table-expand-close-icon" aria-hidden="true"></span>
          </button>
        </header>
        <div class="editor-table-expand-scroll">
          <table class="editor-table-expand-table">
            <tbody>
              <tr v-for="(row, rowIndex) in expandedTableRows" :key="`expanded-row-${rowIndex}`">
                <component
                  :is="'td'"
                  v-for="(_cell, cellIndex) in row"
                  :key="`expanded-cell-${rowIndex}-${cellIndex}`"
                >
                  <textarea
                    v-model="expandedTableRows[rowIndex][cellIndex]"
                    :readonly="!expandedTableEditable"
                    rows="1"
                    @input="syncExpandedTableToEditor"
                    @keydown.enter.exact="insertExpandedTableCellLineBreak(rowIndex, cellIndex, $event)"
                    @keydown.tab.prevent="focusNextExpandedTableCell(rowIndex, cellIndex, $event.shiftKey)"
                  />
                  <div v-if="expandedTableCellAttachments(rowIndex, cellIndex).length" class="editor-table-expand-attachments">
                    <a
                      v-for="attachment in expandedTableCellAttachments(rowIndex, cellIndex)"
                      :key="`${rowIndex}-${cellIndex}-${attachment.type}-${attachment.url}`"
                      :href="attachment.url"
                      class="editor-table-expand-attachment-tag editor-attachment-link"
                      :data-attachment-kind="attachment.type"
                      :data-attachment-url="attachment.url"
                      :aria-label="`预览${attachment.title}`"
                      @mousedown.prevent.stop
                      @click.prevent.stop="previewExpandedTableAttachment(attachment, $event)"
                    >
                      {{ attachment.title }}
                    </a>
                  </div>
                </component>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, nextTick } from "vue";
import { getFixedCoordinateScale, getFixedRect, positionFloatingMenu, scheduleFloatingMenuPosition } from '~/utils/floating-menu'
import { captureVideoFirstFrameFromSource, ensureFancyboxVideoThumbnail, getVideoPlaybackFrameForSource, normalizeMediaPreviewUrl } from '~/utils/fancybox-video-close'
import { createMediaFancyboxOptions } from '~/utils/media-fancybox'
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
let toolbarResizeObserver: ResizeObserver | null = null;
let fixedCleanup: (() => void) | null = null;
let panelCleanup: (() => void) | null = null;
let imagePreviewCleanup: (() => void) | null = null;
let attachmentPreviewCleanup: (() => void) | null = null;
let refreshAttachmentLinksFromEditor: () => void = () => {};
let lastEditorSelectionRange: Range | null = null;
let selectedEditorTable: HTMLTableElement | null = null;
let selectedEditorTableIndex = -1;
let hoveredEditorTable: HTMLTableElement | null = null;
let expandedEditorTableBlock: EditorTableSourceBlock | null = null;
let tableDeleteHideTimer: number | null = null;
let tableExpandCloseTimer: number | null = null;
const editorTableScrollPositions = new Map<string, number>();
const TABLE_DELETE_BUTTON_SIZE = 10;
const TABLE_EXPAND_BUTTON_SIZE = TABLE_DELETE_BUTTON_SIZE;
const TABLE_CELL_BREAK_RE = /<br\s*\/?\s*>/gi;
const isReady = ref(false);
const showHeadingMenu = ref(false);
const headingMenuRef = ref<HTMLElement | null>(null);
const headingMenuStyle = ref<Record<string, string>>({});
const selectedHeadingTag = ref('');
const headingTrigger = ref<HTMLElement | null>(null);
const nativeHeadingPanel = ref<HTMLElement | null>(null);
const showTableMenu = ref(false);
const tableMenuRef = ref<HTMLElement | null>(null);
const tableMenuStyle = ref<Record<string, string>>({});
const tableTrigger = ref<HTMLElement | null>(null);
const nativeTablePanel = ref<HTMLElement | null>(null);
const showTableDeleteButton = ref(false);
const tableDeleteButtonStyle = ref<Record<string, string>>({});
const tableExpandButtonStyle = ref<Record<string, string>>({});
const showTableExpandDialog = ref(false);
const tableExpandClosing = ref(false);
const expandedTableRows = ref<string[][]>([]);
const expandedTableEditable = ref(false);
const TABLE_SIZE_LIMIT = 10
const tableRows = ref(3);
const tableCols = ref(3);
const tableGridCells = Array.from({ length: TABLE_SIZE_LIMIT * TABLE_SIZE_LIMIT }, (_, index) => ({ row: Math.floor(index / TABLE_SIZE_LIMIT) + 1, col: (index % TABLE_SIZE_LIMIT) + 1 }));
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
const ATTACHMENT_MARKER_GLOBAL_RE = /\[(图片附件|视频附件|音频附件)：([^\]]+)\]\(([^)\s]+)\)/g
const ADJACENT_ATTACHMENT_MARKER_RE = /(\[(?:图片附件|视频附件|音频附件)：[^\]]+\]\([^)\s]+\))(\[(?:图片附件|视频附件|音频附件)：[^\]]+\]\([^)\s]+\))/g
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

const hasAttachmentMarker = (value: string) => {
  ATTACHMENT_MARKER_GLOBAL_RE.lastIndex = 0
  return ATTACHMENT_MARKER_GLOBAL_RE.test(String(value || ''))
}

const normalizeAdjacentAttachmentMarkers = (value: string) => String(value || '').replace(ADJACENT_ATTACHMENT_MARKER_RE, '$1 $2')

const normalizeAttachmentInsertValue = (value: string) => {
  const raw = String(value || '')
  if (!hasAttachmentMarker(raw)) return raw
  const normalized = normalizeAdjacentAttachmentMarkers(raw.trim())
  return `\n\n${normalized}\n\n`
}

const normalizeEditorAttachmentSource = () => {
  if (!vditorInstance?.getValue || !vditorInstance?.setValue) return false
  const value = vditorInstance.getValue()
  const normalized = normalizeAdjacentAttachmentMarkers(value)
  if (normalized === value) return false
  vditorInstance.setValue(normalized)
  emit('update:modelValue', vditorInstance.getValue?.() || normalized)
  window.setTimeout(() => refreshAttachmentLinksFromEditor(), 0)
  return true
}

const escapeAttachmentHtmlAttr = (value: string) => String(value || '')
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/\"/g, '&quot;')
  .replace(/'/g, '&#39;')

const buildAttachmentPreviewHtml = (info: EditorAttachmentInfo) => {
  const safeUrl = escapeAttachmentHtmlAttr(info.url)
  const safeName = escapeAttachmentHtmlAttr(info.name)
  if (info.type === 'image') {
    return `<img class="noise-attachment-image" src="${safeUrl}" alt="${safeName}" loading="lazy" decoding="async" />`
  }
  if (info.type === 'video') {
    return `<div class="noise-attachment-render noise-attachment-render--video"><video src="${safeUrl}" controls preload="metadata" playsinline style="width:100%;height:auto"></video></div>`
  }
  return `<div class="noise-attachment-render noise-attachment-render--audio"><audio src="${safeUrl}" controls preload="metadata"></audio></div>`
}

const transformAttachmentPreviewHtml = (html: string) => {
  if (typeof document === 'undefined' || !html) return html
  const holder = document.createElement('div')
  holder.innerHTML = html
  holder.querySelectorAll('a').forEach((node) => {
    const anchor = node as HTMLAnchorElement
    const info = attachmentInfoFromAnchor(anchor)
    if (!info) return
    const fragment = document.createElement('div')
    fragment.innerHTML = buildAttachmentPreviewHtml(info)
    const replacement = fragment.firstElementChild as HTMLElement | null
    if (!replacement) return
    const parent = anchor.parentElement
    const onlyAttachmentInParagraph = parent?.tagName.toLowerCase() === 'p'
      && parent.children.length === 1
      && (parent.textContent || '').trim() === (anchor.textContent || '').trim()
    if (onlyAttachmentInParagraph && info.type !== 'image') {
      parent?.replaceWith(replacement)
      return
    }
    anchor.replaceWith(replacement)
  })
  return holder.innerHTML
}

const getAttachmentImageFancyboxOptions = (startIndex = 0) => createMediaFancyboxOptions({ startIndex })

const getAttachmentVideoFancyboxOptions = (startIndex = 0) => createMediaFancyboxOptions({ startIndex, video: true })

const isImagePreviewSource = (src: string) => /^(data:image|blob:)/i.test(src) || /\.(png|jpe?g|gif|webp|bmp|svg)(?:[?#].*)?$/i.test(src)

const getVideoFirstFrameThumbnail = (url: string) => captureVideoFirstFrameFromSource(url)

const getProjectThumbnailTargetSize = () => {
  if (typeof document === 'undefined') return 72
  const galleryThumb = document.querySelector('.recommend-grid a, .recommend-image-box') as HTMLElement | null
  const rect = galleryThumb?.getBoundingClientRect?.()
  if (rect && rect.width > 16 && rect.height > 16) return Math.round(Math.min(rect.width, rect.height))
  return 72
}

const getPreviewProxyRect = (sourceEl: HTMLElement | null) => {
  if (!sourceEl || typeof document === 'undefined') {
    return { left: -9999, top: -9999, size: 1 }
  }
  const sourceRect = sourceEl.getBoundingClientRect()
  const size = getProjectThumbnailTargetSize()
  if (!sourceRect.width || !sourceRect.height) return { left: -9999, top: -9999, size }
  return {
    left: sourceRect.left + (sourceRect.width - size) / 2,
    top: sourceRect.top + (sourceRect.height - size) / 2,
    size
  }
}

const createFancyboxProxyNode = (item: EditorAttachmentInfo, thumbSrc: string, sourceEl: HTMLElement | null, group: string) => {
  const proxy = document.createElement('a')
  const previewUrl = item.type === 'video' ? normalizeMediaPreviewUrl(item.url) : item.url
  proxy.href = previewUrl
  proxy.dataset.fancybox = group
  proxy.dataset.src = previewUrl
  const proxyThumb = isImagePreviewSource(thumbSrc)
    ? thumbSrc
    : item.type === 'image'
      ? item.url
      : ''
  if (proxyThumb) proxy.dataset.thumbSrc = proxyThumb
  if (item.type === 'video') {
    proxy.dataset.type = 'html5video'
    if (proxyThumb) proxy.dataset.poster = proxyThumb
  }
  proxy.setAttribute('aria-hidden', 'true')
  proxy.tabIndex = -1
  const proxyRect = getPreviewProxyRect(sourceEl)
  Object.assign(proxy.style, {
    position: 'fixed',
    left: `${proxyRect.left}px`,
    top: `${proxyRect.top}px`,
    width: `${proxyRect.size}px`,
    height: `${proxyRect.size}px`,
    opacity: '0.001',
    pointerEvents: 'none',
    overflow: 'hidden',
    zIndex: '-1'
  })
  if (proxyThumb) {
    const img = document.createElement('img')
    img.src = proxyThumb
    img.alt = item.name || item.title
    Object.assign(img.style, {
      display: 'block',
      width: '100%',
      height: '100%',
      objectFit: 'cover'
    })
    proxy.appendChild(img)
  }
  document.body.appendChild(proxy)
  return proxy
}

const showAttachmentGallery = async (items: EditorAttachmentInfo[], current: EditorAttachmentInfo, triggerEl?: HTMLElement | null) => {
  if (typeof document === 'undefined') return
  const sameType = items.filter((item) => item.type === current.type)
  const galleryItems = sameType.length ? sameType : [current]
  const startIndex = Math.max(0, galleryItems.findIndex((item) => item.url === current.url && item.name === current.name))
  const thumbs = current.type === 'video'
    ? await Promise.all(galleryItems.map(async (item) => getVideoPlaybackFrameForSource(item.url) || await getVideoFirstFrameThumbnail(item.url)))
    : galleryItems.map((item) => item.url)
  const group = `editor-attachment-${Date.now()}-${Math.random().toString(36).slice(2)}`
  const sourceEl = triggerEl || null
  const nodes = galleryItems.map((item, index) => createFancyboxProxyNode(item, thumbs[index] || item.url, sourceEl, group))
  const options = current.type === 'video'
    ? getAttachmentVideoFancyboxOptions(startIndex)
    : getAttachmentImageFancyboxOptions(startIndex)
  const cleanup = () => {
    window.setTimeout(() => nodes.forEach((node) => node.remove()), 0)
  }
  const viewerOptions = {
    ...options,
    triggerEl: nodes[startIndex] || sourceEl || undefined,
    on: {
      ...(options as any).on,
      destroy: cleanup
    }
  }
  try {
    Fancybox.fromNodes(nodes, viewerOptions as any)
  } catch {
    try {
      ;(window as any).Fancybox?.fromNodes?.(nodes, viewerOptions)
    } catch {
      cleanup()
    }
  }
}

const showImageInProjectViewer = (info: EditorAttachmentInfo, triggerEl?: HTMLElement | null) => showAttachmentGallery([info], info, triggerEl)

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
    showImageInProjectViewer({ type: 'image', title: img.alt || '图片预览', name: img.alt || '图片预览', url: src }, img);
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

const attachmentInfoToMarkdownSource = (info: EditorAttachmentInfo) => `[${info.title}](${info.url})`

const replaceAttachmentNodesWithSourceText = (root: HTMLElement) => {
  root.querySelectorAll<HTMLElement>('[data-type="a"]').forEach((node) => {
    const info = attachmentInfoFromIrNode(node)
    if (info) node.replaceWith(document.createTextNode(attachmentInfoToMarkdownSource(info)))
  })
  root.querySelectorAll<HTMLAnchorElement>('a').forEach((anchor) => {
    const info = attachmentInfoFromAnchor(anchor)
    if (info) anchor.replaceWith(document.createTextNode(attachmentInfoToMarkdownSource(info)))
  })
}

const setupAttachmentPreview = () => {
  const root = editorContainer.value
  if (!root || attachmentPreviewCleanup) return

  const getEventElement = (event: Event) => {
    const target = event.target as Node | null
    if (target instanceof Element) return target
    return target?.parentElement || null
  }

  const captureEditorSelection = () => {
    if (typeof window === 'undefined') return
    const selection = window.getSelection()
    if (!selection || selection.rangeCount === 0) return
    const range = selection.getRangeAt(0)
    const node = range.commonAncestorContainer
    const element = node instanceof Element ? node : node.parentElement
    if (!element || !root.contains(element)) return
    if (!element.closest('.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset, .vditor-sv .vditor-reset')) return
    lastEditorSelectionRange = range.cloneRange()
  }

  const onEditorSelectionChange = () => {
    captureEditorSelection()
    scheduleCollapseIrAttachmentChrome()
  }

  const onEditorSelectionEvent = () => {
    captureEditorSelection()
    scheduleCollapseIrAttachmentChrome()
  }

  const onEditorInput = () => {
    captureEditorSelection()
    if (normalizeEditorAttachmentSource()) return
    scheduleRefreshAttachmentLinks()
  }

  const refreshAttachmentLinks = () => {
    root.querySelectorAll('a').forEach((node) => {
      const anchor = node as HTMLAnchorElement
      const info = attachmentInfoFromAnchor(anchor)
      anchor.classList.toggle('editor-attachment-link', !!info)
      if (!info) {
        anchor.style.cursor = ''
        anchor.removeAttribute('role')
        anchor.removeAttribute('aria-label')
        anchor.removeAttribute('data-attachment-kind')
        anchor.removeAttribute('data-attachment-url')
        return
      }
      anchor.setAttribute('role', 'button')
      anchor.setAttribute('aria-label', `预览${info.title}`)
      anchor.setAttribute('data-attachment-kind', info.type)
      anchor.setAttribute('data-attachment-url', info.url)
      anchor.removeAttribute('title')
      anchor.setAttribute('draggable', 'false')
      anchor.style.cursor = 'pointer'
    })

    root.querySelectorAll('[data-type="a"]').forEach((node) => {
      const marker = node as HTMLElement
      const label = marker.querySelector<HTMLElement>('.vditor-ir__link')
      const info = attachmentInfoFromIrNode(marker)
      marker.classList.toggle('editor-attachment-node', !!info)
      label?.classList.toggle('editor-attachment-link', !!info)
      if (info) {
        marker.classList.remove('vditor-ir__node--expand')
        marker.setAttribute('contenteditable', 'false')
        marker.setAttribute('data-attachment-kind', info.type)
        marker.setAttribute('data-attachment-url', info.url)
        marker.setAttribute('aria-label', `预览${info.title}`)
      } else {
        marker.removeAttribute('contenteditable')
        marker.removeAttribute('data-attachment-kind')
        marker.removeAttribute('data-attachment-url')
        marker.removeAttribute('aria-label')
      }
      if (!label) return
      if (!info) {
        label.style.cursor = ''
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
      label.style.cursor = 'pointer'
    })

    root.querySelectorAll<HTMLVideoElement>('.noise-attachment-render--video video').forEach((video) => {
      ensureFancyboxVideoThumbnail(video)
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
      showAttachmentGallery(getAttachmentInfosByType(info.type), info, target)
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
    const target = getEventElement(event) as HTMLElement | null
    if (!target || !root.contains(target)) return null

    const markerNode = target.closest<HTMLElement>('[data-type="a"].editor-attachment-node')
    if (markerNode && root.contains(markerNode)) {
      const markerInfo = attachmentInfoFromIrNode(markerNode)
      if (markerInfo) return { target: markerNode, info: markerInfo }
    }

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

  const collapseIrAttachmentChrome = () => {
    root.querySelectorAll<HTMLElement>('[data-type="a"].editor-attachment-node.vditor-ir__node--expand').forEach((marker) => {
      if (attachmentInfoFromIrNode(marker)) marker.classList.remove('vditor-ir__node--expand')
    })
  }

  const scheduleCollapseIrAttachmentChrome = () => {
    collapseIrAttachmentChrome()
    requestAnimationFrame(() => collapseIrAttachmentChrome())
    window.setTimeout(() => collapseIrAttachmentChrome(), 0)
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
    if (!hit) {
      scheduleCollapseIrAttachmentChrome()
      return
    }
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
  }

  const onAttachmentClick = (event: MouseEvent) => {
    const hit = attachmentTargetFromEvent(event)
    if (!hit) {
      scheduleCollapseIrAttachmentChrome()
      return
    }
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

  const isEditorSoftEnter = (event: KeyboardEvent) => {
    if (event.key !== 'Enter' || event.shiftKey || event.altKey || event.ctrlKey || event.metaKey || event.isComposing) return false
    const selection = window.getSelection()
    const anchorNode = selection?.anchorNode || null
    const anchorElement = anchorNode instanceof Element ? anchorNode : anchorNode?.parentElement || getEventElement(event)
    if (!anchorElement || !root.contains(anchorElement)) return false
    if (anchorElement.closest('.vditor-toolbar, .vditor-panel, .vditor-hint, [data-type="code-block"], .vditor-ir__marker--pre')) return false
    return !!anchorElement.closest('.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset, .vditor-sv .vditor-reset')
  }

  const moveCaretAfterAtomicAttachment = () => {
    const selection = window.getSelection()
    const anchorNode = selection?.anchorNode || null
    const anchorElement = anchorNode instanceof Element ? anchorNode : anchorNode?.parentElement || null
    const attachment = anchorElement?.closest<HTMLElement>('.editor-attachment-node')
    if (!attachment || !selection) return
    const range = document.createRange()
    range.setStartAfter(attachment)
    range.collapse(true)
    selection.removeAllRanges()
    selection.addRange(range)
  }

  const emitEditorSoftBreakInput = (event: Event) => {
    const target = getEventElement(event)
    const editable = target?.closest('.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset, .vditor-sv .vditor-reset') as HTMLElement | null
    editable?.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: '\n' }))
    scheduleRefreshAttachmentLinks()
    window.setTimeout(() => {
      if (vditorInstance?.getValue) emit('update:modelValue', vditorInstance.getValue())
    }, 0)
  }

  const insertEditorTableCellLineBreak = (event: Event) => {
    const cell = getCurrentEditorTableCell(event)
    if (!cell) return false
    const selection = window.getSelection()
    if (!selection) return false
    if (!selection.rangeCount) {
      const fallbackRange = document.createRange()
      fallbackRange.selectNodeContents(cell)
      fallbackRange.collapse(false)
      selection.addRange(fallbackRange)
    }
    let range = selection.getRangeAt(0)
    const rangeRoot = range.commonAncestorContainer instanceof Element ? range.commonAncestorContainer : range.commonAncestorContainer.parentElement
    if (!rangeRoot || !cell.contains(rangeRoot)) {
      range = document.createRange()
      range.selectNodeContents(cell)
      range.collapse(false)
      selection.removeAllRanges()
      selection.addRange(range)
    }
    let inserted = false
    try {
      inserted = document.execCommand('insertHTML', false, '<br>')
    } catch {
      inserted = false
    }
    if (!inserted) {
      range.deleteContents()
      const br = document.createElement('br')
      range.insertNode(br)
      range.setStartAfter(br)
      range.collapse(true)
      selection.removeAllRanges()
      selection.addRange(range)
      inserted = true
    }
    if (!inserted) return false
    if (!syncEditorTableCellDomToSource(cell)) syncEditorTableCellLineBreakToSource(cell)
    emitEditorSoftBreakInput(event)
    return true
  }

  const insertEditorSoftLineBreak = (event: Event) => {
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    moveCaretAfterAtomicAttachment()
    if (insertEditorTableCellLineBreak(event)) return
    let inserted = false
    try {
      inserted = document.execCommand('insertText', false, '\n')
    } catch {
      inserted = false
    }
    if (!inserted) {
      const selection = window.getSelection()
      if (selection?.rangeCount) {
        const range = selection.getRangeAt(0)
        range.deleteContents()
        const lineBreak = document.createTextNode('\n')
        range.insertNode(lineBreak)
        range.setStartAfter(lineBreak)
        range.collapse(true)
        selection.removeAllRanges()
        selection.addRange(range)
        inserted = true
      }
    }
    if (!inserted) return
    emitEditorSoftBreakInput(event)
  }

  const onPlainTextEnterKeydown = (event: KeyboardEvent) => {
    if (!isEditorSoftEnter(event)) return
    insertEditorSoftLineBreak(event)
  }

  const onEditorBeforeInput = (event: InputEvent) => {
    if (event.inputType !== 'insertParagraph' || event.isComposing) return
    const selection = window.getSelection()
    const anchorNode = selection?.anchorNode || null
    const anchorElement = anchorNode instanceof Element ? anchorNode : anchorNode?.parentElement || getEventElement(event)
    if (!anchorElement || !root.contains(anchorElement)) return
    if (anchorElement.closest('.vditor-toolbar, .vditor-panel, .vditor-hint, [data-type="code-block"], .vditor-ir__marker--pre')) return
    if (!anchorElement.closest('.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset, .vditor-sv .vditor-reset')) return
    insertEditorSoftLineBreak(event)
  }

  const scheduleTableEnhance = () => requestAnimationFrame(() => enhanceEditorTables(root))

  const tableFromEvent = (event: Event) => {
    const target = getEventElement(event)
    const table = target?.closest<HTMLTableElement>('.vditor-reset table.editor-deletable-table, .vditor-reset table') || null
    return table && root.contains(table) ? table : null
  }

  const onTablePointerMove = (event: PointerEvent) => {
    const table = tableFromEvent(event)
    if (!table) return
    showTableDeleteForTable(table)
  }

  const onTablePointerOut = (event: PointerEvent) => {
    const table = tableFromEvent(event)
    if (!table) return
    const next = event.relatedTarget instanceof Element ? event.relatedTarget : null
    if (next && table.contains(next)) return
    scheduleTableDeleteHide()
  }

  const onTablePointerDown = (event: PointerEvent) => {
    const table = tableFromEvent(event)
    if (!table) clearSelectedEditorTable()
  }

  const repositionVisibleTableDeleteButton = () => {
    if (!showTableDeleteButton.value || !hoveredEditorTable) return
    if (!root.contains(hoveredEditorTable) || !isEditorTableActionVisible(hoveredEditorTable)) {
      hideTableDeleteButton()
      return
    }
    positionTableDeleteButton(hoveredEditorTable)
  }

  let refreshQueued = false
  const scheduleRefreshAttachmentLinks = () => {
    if (refreshQueued) return
    refreshQueued = true
    requestAnimationFrame(() => {
      refreshQueued = false
      refreshAttachmentLinks()
      enhanceEditorTables(root)
    })
  }
  refreshAttachmentLinksFromEditor = scheduleRefreshAttachmentLinks

  refreshAttachmentLinks()
  scheduleTableEnhance()
  const previewObserver = new MutationObserver(() => scheduleRefreshAttachmentLinks())
  previewObserver.observe(root, { childList: true, subtree: true })
  root.addEventListener('input', onEditorInput, true)
  root.addEventListener('mouseup', onEditorSelectionEvent, true)
  root.addEventListener('keyup', onEditorSelectionEvent, true)
  document.addEventListener('selectionchange', onEditorSelectionChange, true)
  root.addEventListener('pointerdown', onTablePointerDown, true)
  root.addEventListener('pointermove', onTablePointerMove, true)
  root.addEventListener('pointerout', onTablePointerOut, true)
  root.addEventListener('pointerdown', preventAttachmentNavigation, true)
  root.addEventListener('mousedown', preventAttachmentNavigation, true)
  root.addEventListener('click', onAttachmentClick, true)
  root.addEventListener('keydown', onPlainTextEnterKeydown, true)
  root.addEventListener('beforeinput', onEditorBeforeInput as EventListener, true)
  root.addEventListener('keydown', onAttachmentKeydown, true)
  window.addEventListener('resize', repositionVisibleTableDeleteButton)
  window.addEventListener('scroll', repositionVisibleTableDeleteButton, { passive: true, capture: true })
  attachmentPreviewCleanup = () => {
    previewObserver.disconnect()
    root.removeEventListener('input', onEditorInput, true)
    root.removeEventListener('mouseup', onEditorSelectionEvent, true)
    root.removeEventListener('keyup', onEditorSelectionEvent, true)
    document.removeEventListener('selectionchange', onEditorSelectionChange, true)
    root.removeEventListener('pointerdown', onTablePointerDown, true)
    root.removeEventListener('pointermove', onTablePointerMove, true)
    root.removeEventListener('pointerout', onTablePointerOut, true)
    root.removeEventListener('pointerdown', preventAttachmentNavigation, true)
    root.removeEventListener('mousedown', preventAttachmentNavigation, true)
    root.removeEventListener('click', onAttachmentClick, true)
    root.removeEventListener('keydown', onPlainTextEnterKeydown, true)
    root.removeEventListener('beforeinput', onEditorBeforeInput as EventListener, true)
    root.removeEventListener('keydown', onAttachmentKeydown, true)
    window.removeEventListener('resize', repositionVisibleTableDeleteButton)
    window.removeEventListener('scroll', repositionVisibleTableDeleteButton, true)
    hideTableDeleteButton()
    root.querySelectorAll('.editor-attachment-preview').forEach((node) => node.remove())
    refreshAttachmentLinksFromEditor = () => {}
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
    transform: transformAttachmentPreviewHtml,
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

const positionTableMenu = () => {
  positionFloatingMenu(tableTrigger.value, tableMenuRef.value, tableMenuStyle, 324, 'above-align-left')
}

const closeTableMenu = () => {
  showTableMenu.value = false
  nativeTablePanel.value?.classList.add('vditor-panel--none')
  if (nativeTablePanel.value) nativeTablePanel.value.style.display = 'none'
}

const clampTableSize = (value: number) => Math.min(TABLE_SIZE_LIMIT, Math.max(1, Number(value) || 1))
const adjustTableRows = (delta: number) => { tableRows.value = clampTableSize(tableRows.value + delta) }
const adjustTableCols = (delta: number) => { tableCols.value = clampTableSize(tableCols.value + delta) }
const previewTableSize = (rows: number, cols: number) => {
  tableRows.value = clampTableSize(rows)
  tableCols.value = clampTableSize(cols)
}

const buildMarkdownTable = (rows: number, cols: number) => {
  const rowCount = clampTableSize(rows)
  const colCount = clampTableSize(cols)
  const header = Array.from({ length: colCount }, () => ' ')
  const divider = Array.from({ length: colCount }, () => '---')
  const bodyRows = Array.from({ length: Math.max(1, rowCount - 1) }, () => Array.from({ length: colCount }, () => ' '))
  const formatRow = (cells: string[]) => `| ${cells.join(' | ')} |`
  return `\n${[formatRow(header), formatRow(divider), ...bodyRows.map(formatRow)].join('\n')}\n`
}

const insertTable = (rows: number, cols: number) => {
  if (!vditorInstance) return
  vditorInstance.insertValue(buildMarkdownTable(rows, cols))
  emit('update:modelValue', vditorInstance.getValue?.() || '')
  closeTableMenu()
}

const getEditorTables = () => Array.from(editorContainer.value?.querySelectorAll<HTMLTableElement>('.vditor-reset table') || [])

const clearSelectedEditorTable = () => {
  selectedEditorTable?.classList.remove('editor-table-selected')
  selectedEditorTable = null
  selectedEditorTableIndex = -1
}

const selectEditorTable = (table: HTMLTableElement) => {
  const tables = getEditorTables()
  tables.forEach((item) => item.classList.toggle('editor-table-selected', item === table))
  selectedEditorTable = table
  selectedEditorTableIndex = tables.indexOf(table)
}

const cancelTableDeleteHide = () => {
  if (tableDeleteHideTimer !== null) {
    window.clearTimeout(tableDeleteHideTimer)
    tableDeleteHideTimer = null
  }
}

const hideTableDeleteButton = () => {
  cancelTableDeleteHide()
  showTableDeleteButton.value = false
  hoveredEditorTable = null
  clearSelectedEditorTable()
}

const scheduleTableDeleteHide = (delay: number | Event = 1800) => {
  cancelTableDeleteHide()
  const timeout = typeof delay === 'number' ? delay : 1800
  tableDeleteHideTimer = window.setTimeout(() => hideTableDeleteButton(), timeout)
}

const isEditorTableActionVisible = (table: HTMLTableElement) => {
  const rect = table.getBoundingClientRect()
  const width = window.innerWidth || document.documentElement.clientWidth || 0
  const height = window.innerHeight || document.documentElement.clientHeight || 0
  const editorRect = editorContainer.value?.getBoundingClientRect()
  const visibleTop = Math.max(0, editorRect?.top ?? 0)
  const visibleLeft = Math.max(0, editorRect?.left ?? 0)
  const visibleRight = Math.min(width, editorRect?.right ?? width)
  const visibleBottom = Math.min(height, editorRect?.bottom ?? height)
  return rect.top >= visibleTop && rect.left >= visibleLeft && rect.top < visibleBottom && rect.left < visibleRight
}

const positionTableDeleteButton = (table: HTMLTableElement) => {
  const scale = getFixedCoordinateScale()
  const rect = getFixedRect(table, scale)
  const deleteSize = TABLE_DELETE_BUTTON_SIZE
  const expandSize = TABLE_EXPAND_BUTTON_SIZE
  tableDeleteButtonStyle.value = {
    position: 'fixed',
    top: `${rect.top - deleteSize}px`,
    left: `${rect.left - deleteSize}px`,
    zIndex: '10020'
  }
  tableExpandButtonStyle.value = {
    position: 'fixed',
    top: `${rect.top - expandSize}px`,
    left: `${rect.left}px`,
    zIndex: '10020'
  }
}

const showTableDeleteForTable = (table: HTMLTableElement) => {
  if (!editorContainer.value?.contains(table)) return
  if (!isEditorTableActionVisible(table)) {
    hideTableDeleteButton()
    return
  }
  cancelTableDeleteHide()
  hoveredEditorTable = table
  positionTableDeleteButton(table)
  showTableDeleteButton.value = true
}

const confirmDeleteHoveredTable = () => {
  const table = hoveredEditorTable
  if (!table || !editorContainer.value?.contains(table)) {
    hideTableDeleteButton()
    return
  }
  const tableIndex = getEditorTables().indexOf(table)
  const confirmed = window.confirm('确定要删除该表格吗？')
  if (!confirmed) {
    scheduleTableDeleteHide()
    return
  }
  const deleted = deleteEditorTable(table, tableIndex)
  if (!deleted) {
    window.alert('未能定位到该表格，请先保存当前内容后重试。')
    scheduleTableDeleteHide()
    return
  }
  hideTableDeleteButton()
}

const stripTableBreakCode = (value: string) => String(value || '').replace(TABLE_CELL_BREAK_RE, ' ')
const tableCellSourceToEditorText = (value: string) => String(value || '').replace(TABLE_CELL_BREAK_RE, '\n').replace(/\\\|/g, '|').trim()
const editorTextToTableCellSource = (value: string) => {
  const text = String(value || '').replace(/\r\n?/g, '\n')
  const normalized = text
    .split('\n')
    .map((line) => line.replace(/\|/g, '\\|').trim())
    .join('<br />')
    .trim()
  return normalized || ' '
}

const escapeTableCellHtml = (value: string) => String(value || '')
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/\"/g, '&quot;')
  .replace(/'/g, '&#39;')

const editorTextToHtmlTableCellSource = (value: string) => {
  const text = String(value || '').replace(/\r\n?/g, '\n')
  const normalized = text
    .split('\n')
    .map((line) => escapeTableCellHtml(line).trim())
    .join('<br />')
    .trim()
  return normalized || '&nbsp;'
}

const htmlTableCellToEditorText = (cell: HTMLTableCellElement) => {
  const clone = cell.cloneNode(true) as HTMLElement
  clone.querySelectorAll('.editor-attachment-preview').forEach((node) => node.remove())
  replaceAttachmentNodesWithSourceText(clone)
  clone.querySelectorAll('br').forEach((br) => br.replaceWith(document.createTextNode('\n')))
  clone.querySelectorAll('p,div').forEach((block) => {
    if (block.nextSibling) block.after(document.createTextNode('\n'))
  })
  return String(clone.textContent || '').replace(/\u00a0/g, ' ').trim()
}

const replaceTableHeaderCells = (table: HTMLTableElement) => {
  table.querySelectorAll('th').forEach((headerCell) => {
    const cell = document.createElement('td')
    Array.from(headerCell.attributes).forEach((attr) => cell.setAttribute(attr.name, attr.value))
    while (headerCell.firstChild) cell.appendChild(headerCell.firstChild)
    headerCell.replaceWith(cell)
  })
}

const normalizeEditableHtmlTable = (table: HTMLTableElement) => {
  const thead = table.tHead
  if (thead) {
    const body = table.tBodies[0] || table.createTBody()
    Array.from(thead.rows).reverse().forEach((row) => body.insertBefore(row, body.firstChild))
    thead.remove()
  }
  replaceTableHeaderCells(table)
}

const tableScrollKeyFromBlock = (block?: EditorTableSourceBlock | null, fallback = '') => {
  if (block) return `${block.kind}:${block.start}:${block.end}`
  return fallback
}

const rememberEditorTableScroll = (table: HTMLTableElement) => {
  const key = table.dataset.editorTableScrollKey || table.dataset.editorTableIndex || ''
  if (!key) return
  editorTableScrollPositions.set(key, table.scrollLeft || 0)
}

const restoreEditorTableScroll = (table: HTMLTableElement) => {
  const key = table.dataset.editorTableScrollKey || table.dataset.editorTableIndex || ''
  if (!key) return
  const left = editorTableScrollPositions.get(key)
  if (typeof left !== 'number' || left <= 0) return
  table.scrollLeft = left
  requestAnimationFrame(() => { table.scrollLeft = left })
}

const parseAttachmentMarkersFromText = (text: string) => {
  const items: EditorAttachmentInfo[] = []
  const source = String(text || '')
  ATTACHMENT_MARKER_GLOBAL_RE.lastIndex = 0
  let match: RegExpExecArray | null
  while ((match = ATTACHMENT_MARKER_GLOBAL_RE.exec(source))) {
    const info = normalizeAttachmentInfo(match[1], match[2], match[3])
    if (info) items.push(info)
  }
  return items
}

const expandedTableCellAttachments = (rowIndex: number, cellIndex: number) => parseAttachmentMarkersFromText(expandedTableRows.value[rowIndex]?.[cellIndex] || '')

const previewExpandedTableAttachment = (attachment: EditorAttachmentInfo, event: MouseEvent) => {
  const target = event.currentTarget instanceof HTMLElement ? event.currentTarget : null
  if (attachment.type === 'audio') {
    const url = attachment.url
    window.open(url, '_blank', 'noopener,noreferrer')
    return
  }
  showAttachmentGallery([attachment], attachment, target)
}

const insertTextIntoTextarea = (textarea: HTMLTextAreaElement, value: string) => {
  const start = textarea.selectionStart ?? textarea.value.length
  const end = textarea.selectionEnd ?? start
  const nextValue = `${textarea.value.slice(0, start)}${value}${textarea.value.slice(end)}`
  textarea.value = nextValue
  const nextPos = start + value.length
  textarea.selectionStart = nextPos
  textarea.selectionEnd = nextPos
  textarea.dispatchEvent(new Event('input', { bubbles: true }))
}

const insertExpandedTableCellLineBreak = (rowIndex: number, cellIndex: number, event: KeyboardEvent) => {
  if (event.isComposing) return
  const textarea = event.target instanceof HTMLTextAreaElement ? event.target : null
  if (!textarea || !expandedTableEditable.value) return
  event.preventDefault()
  event.stopPropagation()
  insertTextIntoTextarea(textarea, '\n')
  expandedTableRows.value[rowIndex][cellIndex] = textarea.value
  syncExpandedTableToEditor()
}

const isMarkdownTableDivider = (line: string) => {
  const cells = String(line || '').trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((cell) => cell.trim())
  return cells.length > 1 && cells.every((cell) => /^:?-{3,}:?$/.test(cell))
}

type EditorTableSourceBlock = { start: number; end: number; lines: string[]; kind: 'markdown' | 'html' }

const getMarkdownTableBlocks = (content: string): EditorTableSourceBlock[] => {
  const lines = String(content || '').split('\n')
  const blocks: EditorTableSourceBlock[] = []
  for (let index = 0; index < lines.length - 1; index += 1) {
    if (!lines[index].includes('|') || !isMarkdownTableDivider(lines[index + 1])) continue
    let end = index + 2
    while (end < lines.length && lines[end].includes('|') && lines[end].trim() !== '') end += 1
    blocks.push({ start: index, end, lines: lines.slice(index, end), kind: 'markdown' })
    index = end - 1
  }
  return blocks
}

const getHtmlTableBlocks = (content: string): EditorTableSourceBlock[] => {
  const lines = String(content || '').split('\n')
  const blocks: EditorTableSourceBlock[] = []
  for (let index = 0; index < lines.length; index += 1) {
    if (!/<table\b/i.test(lines[index])) continue
    let end = index + 1
    while (end < lines.length && !/<\/table>/i.test(lines[end - 1])) end += 1
    blocks.push({ start: index, end, lines: lines.slice(index, end), kind: 'html' })
    index = end - 1
  }
  return blocks
}

const getEditorTableSourceBlocks = (content: string) => [
  ...getMarkdownTableBlocks(content),
  ...getHtmlTableBlocks(content)
].sort((left, right) => left.start - right.start)

const normalizeTableMatchText = (text: string) => String(stripTableBreakCode(text || '')).replace(/\s+/g, ' ').trim()

const getRenderedTableRows = (table: HTMLTableElement | null) => {
  if (!table) return [] as string[][]
  return Array.from(table.rows).map((row) => Array.from(row.cells).map((cell) => normalizeTableMatchText(cell.textContent || '')))
}

const createHtmlTableFromBlock = (block: EditorTableSourceBlock) => {
  if (block.kind !== 'html' || typeof document === 'undefined') return null
  const holder = document.createElement('div')
  holder.innerHTML = block.lines.join('\n').trim()
  const table = holder.querySelector('table') as HTMLTableElement | null
  if (table) normalizeEditableHtmlTable(table)
  return table
}

const parseMarkdownTableRow = (line: string) => String(line || '')
  .trim()
  .replace(/^\|/, '')
  .replace(/\|$/, '')
  .split('|')
  .map((cell) => normalizeTableMatchText(cell.replace(/\\\|/g, '|')))

const parseEditableMarkdownTableRow = (line: string) => String(line || '')
  .trim()
  .replace(/^\|/, '')
  .replace(/\|$/, '')
  .split('|')
  .map((cell) => tableCellSourceToEditorText(cell))

const getMarkdownTableRows = (lines: string[]) => {
  if (lines.length < 2) return [] as string[][]
  const rows = [parseMarkdownTableRow(lines[0])]
  lines.slice(2).forEach((line) => rows.push(parseMarkdownTableRow(line)))
  return rows
}

const editableRowsFromHtmlBlock = (block: EditorTableSourceBlock) => {
  const table = createHtmlTableFromBlock(block)
  if (!table) return [] as string[][]
  return Array.from(table.rows).map((row) => Array.from(row.cells).map((cell) => htmlTableCellToEditorText(cell as HTMLTableCellElement)))
}

const comparableRowsFromTableBlock = (block: EditorTableSourceBlock) => {
  const rows = block.kind === 'markdown' ? getMarkdownTableRows(block.lines) : editableRowsFromHtmlBlock(block)
  return rows.map((row) => row.map((cell) => normalizeTableMatchText(cell)))
}

const tableRowsHaveComparableContent = (rows: string[][]) => rows.some((row) => row.some((cell) => !!cell))

const sameTableRows = (left: string[][], right: string[][]) => {
  if (!left.length || left.length !== right.length) return false
  return left.every((row, rowIndex) => {
    const other = right[rowIndex] || []
    if (row.length !== other.length) return false
    return row.every((cell, cellIndex) => normalizeTableMatchText(cell) === normalizeTableMatchText(other[cellIndex]))
  })
}

const findMarkdownTableBlock = (
  blocks: EditorTableSourceBlock[],
  renderedRows: string[][],
  preferredIndex: number
) => {
  if (tableRowsHaveComparableContent(renderedRows)) {
    const matched = blocks.find((block) => sameTableRows(comparableRowsFromTableBlock(block), renderedRows))
    if (matched) return matched
  }
  return preferredIndex >= 0 ? blocks[preferredIndex] : undefined
}

const tableBlockFromDataset = (table: HTMLTableElement | null, blocks: EditorTableSourceBlock[]) => {
  if (!table) return undefined
  const start = Number(table.dataset.editorTableBlockStart)
  const end = Number(table.dataset.editorTableBlockEnd)
  if (!Number.isFinite(start) || !Number.isFinite(end)) return undefined
  return blocks.find((block) => block.start === start && block.end === end)
}

type EditorTableCellSourceTarget = {
  table: HTMLTableElement
  block: EditorTableSourceBlock
  lines: string[]
  lineIndex: number
  rowIndex: number
  cellIndex: number
  rowCells: string[]
}

const getEditorTableBlockForTable = (table: HTMLTableElement | null, preferredIndex = -1) => {
  const value = vditorInstance?.getValue?.() || ''
  const blocks = getEditorTableSourceBlocks(value)
  return tableBlockFromDataset(table, blocks) || findMarkdownTableBlock(blocks, getRenderedTableRows(table), preferredIndex)
}

const getEditorTableCellSourceTarget = (cell: HTMLTableCellElement | null): EditorTableCellSourceTarget | null => {
  if (!cell || !vditorInstance) return null
  const table = cell.closest('table') as HTMLTableElement | null
  const row = cell.parentElement as HTMLTableRowElement | null
  if (!table || !row || !editorContainer.value?.contains(table)) return null
  const rowIndex = row.rowIndex
  const cellIndex = cell.cellIndex
  if (rowIndex < 0 || cellIndex < 0) return null
  const value = vditorInstance.getValue?.() || ''
  const lines = value.split('\n')
  const blocks = getEditorTableSourceBlocks(value)
  const tableIndex = getEditorTables().indexOf(table)
  const block = tableBlockFromDataset(table, blocks) || findMarkdownTableBlock(blocks, getRenderedTableRows(table), tableIndex)
  if (!block) return null
  if (block.kind === 'html') {
    const sourceRows = editableRowsFromHtmlBlock(block)
    const rowCells = sourceRows[rowIndex]
    if (!rowCells) return null
    return { table, block, lines, lineIndex: -1, rowIndex, cellIndex, rowCells }
  }
  if (!isMarkdownTableDivider(lines[block.start + 1] || '')) return null
  const lineIndex = rowIndex === 0 ? block.start : block.start + rowIndex + 1
  if (lineIndex < block.start || lineIndex >= block.end || !lines[lineIndex]) return null
  const rowCells = parseEditableMarkdownTableRow(lines[lineIndex])
  return { table, block, lines, lineIndex, rowIndex, cellIndex, rowCells }
}

const serializeEditableHtmlTableBlock = (block: EditorTableSourceBlock, rows: string[][]) => {
  const table = createHtmlTableFromBlock(block)
  if (!table) return null
  table.querySelectorAll('thead,tbody,tfoot').forEach((section) => section.remove())
  const body = table.createTBody()
  rows.forEach((cells) => {
    const row = body.insertRow()
    cells.forEach((text) => {
      const targetCell = row.insertCell()
      targetCell.innerHTML = editorTextToHtmlTableCellSource(text)
    })
  })
  return table.outerHTML.split('\n')
}

const applyEditorTableCellSourceValue = (cell: HTMLTableCellElement | null, nextText: string) => {
  const target = getEditorTableCellSourceTarget(cell)
  if (!target || !vditorInstance) return false
  const rowCells = [...target.rowCells]
  while (rowCells.length <= target.cellIndex) rowCells.push('')
  rowCells[target.cellIndex] = nextText
  if (target.block.kind === 'html') {
    const rows = editableRowsFromHtmlBlock(target.block)
    if (!rows[target.rowIndex]) rows[target.rowIndex] = []
    rows[target.rowIndex] = rowCells
    const nextBlockLines = serializeEditableHtmlTableBlock(target.block, rows)
    if (!nextBlockLines) return false
    target.lines.splice(target.block.start, target.block.end - target.block.start, ...nextBlockLines)
  } else {
    target.lines[target.lineIndex] = formatEditableMarkdownTableRow(rowCells)
  }
  const nextValue = target.lines.join('\n')
  rememberEditorTableScroll(target.table)
  vditorInstance.setValue(nextValue)
  emit('update:modelValue', nextValue)
  window.setTimeout(() => refreshAttachmentLinksFromEditor(), 0)
  return true
}

const editorTableCellTextFromDom = (cell: HTMLTableCellElement) => {
  const clone = cell.cloneNode(true) as HTMLElement
  clone.querySelectorAll('.editor-attachment-preview').forEach((node) => node.remove())
  replaceAttachmentNodesWithSourceText(clone)
  clone.querySelectorAll('br').forEach((br) => br.replaceWith(document.createTextNode('\n')))
  return String(clone.textContent || '').replace(/\u00a0/g, ' ')
}

const syncEditorTableCellDomToSource = (cell: HTMLTableCellElement | null) => {
  const target = getEditorTableCellSourceTarget(cell)
  if (!target || !cell) return false
  const current = target.rowCells[target.cellIndex] || ''
  const nextText = editorTableCellTextFromDom(cell)
  if (hasAttachmentMarker(current) && !hasAttachmentMarker(nextText)) return false
  return applyEditorTableCellSourceValue(cell, nextText)
}

const syncEditorTableCellLineBreakToSource = (cell: HTMLTableCellElement | null) => {
  const target = getEditorTableCellSourceTarget(cell)
  if (!target) return false
  const current = target.rowCells[target.cellIndex] || ''
  return applyEditorTableCellSourceValue(cell, `${current}\n`)
}

const editableRowsFromMarkdownBlock = (block: EditorTableSourceBlock) => {
  if (block.kind !== 'markdown' || block.lines.length < 2) return [] as string[][]
  return [parseEditableMarkdownTableRow(block.lines[0]), ...block.lines.slice(2).map((line) => parseEditableMarkdownTableRow(line))]
}

const editableRowsFromTableBlock = (block: EditorTableSourceBlock) => block.kind === 'markdown'
  ? editableRowsFromMarkdownBlock(block)
  : editableRowsFromHtmlBlock(block)

const normalizeExpandedTableRows = (rows: string[][]) => {
  const colCount = Math.max(1, ...rows.map((row) => row.length))
  return rows.map((row) => Array.from({ length: colCount }, (_, index) => row[index] ?? ''))
}

const formatEditableMarkdownTableRow = (cells: string[]) => `| ${cells.map(editorTextToTableCellSource).join(' | ')} |`

const formatMarkdownDividerLine = (dividerLine: string, colCount: number) => {
  const cells = String(dividerLine || '')
    .trim()
    .replace(/^\|/, '')
    .replace(/\|$/, '')
    .split('|')
    .map((cell) => cell.trim())
  const normalized = Array.from({ length: colCount }, (_, index) => /^:?-{3,}:?$/.test(cells[index] || '') ? cells[index] : '---')
  return `| ${normalized.join(' | ')} |`
}

const syncExpandedTableToEditor = () => {
  if (!vditorInstance || !expandedEditorTableBlock || !expandedTableEditable.value) return false
  const value = vditorInstance.getValue?.() || ''
  const lines = value.split('\n')
  const blocks = getEditorTableSourceBlocks(value)
  const currentBlock = blocks.find((block) => block.start === expandedEditorTableBlock?.start && block.end === expandedEditorTableBlock?.end) || expandedEditorTableBlock
  if (!currentBlock) return false
  const rows = normalizeExpandedTableRows(expandedTableRows.value)
  const colCount = rows[0]?.length || 1
  let nextBlockLines: string[] | null = null
  if (currentBlock.kind === 'html') {
    nextBlockLines = serializeEditableHtmlTableBlock(currentBlock, rows)
  } else if (isMarkdownTableDivider(lines[currentBlock.start + 1] || '')) {
    nextBlockLines = [
      formatEditableMarkdownTableRow(rows[0] || Array.from({ length: colCount }, () => '')),
      formatMarkdownDividerLine(lines[currentBlock.start + 1] || currentBlock.lines[1] || '', colCount),
      ...rows.slice(1).map(formatEditableMarkdownTableRow)
    ]
  }
  if (!nextBlockLines) return false
  lines.splice(currentBlock.start, currentBlock.end - currentBlock.start, ...nextBlockLines)
  const nextValue = lines.join('\n')
  expandedEditorTableBlock = { ...currentBlock, end: currentBlock.start + nextBlockLines.length, lines: nextBlockLines }
  vditorInstance.setValue(nextValue)
  emit('update:modelValue', nextValue)
  window.setTimeout(() => refreshAttachmentLinksFromEditor(), 0)
  return true
}

const focusNextExpandedTableCell = (rowIndex: number, cellIndex: number, reverse = false) => {
  const cells = Array.from(document.querySelectorAll<HTMLTextAreaElement>('.editor-table-expand-dialog textarea'))
  const currentIndex = cells.findIndex((cell) => cell === document.activeElement)
  const fallback = expandedTableRows.value.slice(0, rowIndex).reduce((sum, row) => sum + row.length, 0) + cellIndex
  const baseIndex = currentIndex >= 0 ? currentIndex : fallback
  const nextIndex = reverse ? baseIndex - 1 : baseIndex + 1
  const target = cells[((nextIndex % cells.length) + cells.length) % cells.length]
  target?.focus()
  target?.select()
}

const closeExpandedTable = () => {
  if (!showTableExpandDialog.value || tableExpandClosing.value) return
  tableExpandClosing.value = true
  if (tableExpandCloseTimer !== null) window.clearTimeout(tableExpandCloseTimer)
  tableExpandCloseTimer = window.setTimeout(() => {
    showTableExpandDialog.value = false
    tableExpandClosing.value = false
    expandedTableRows.value = []
    expandedTableEditable.value = false
    expandedEditorTableBlock = null
    tableExpandCloseTimer = null
  }, 180)
}

const openHoveredTableExpand = () => {
  const table = hoveredEditorTable
  if (!table || !editorContainer.value?.contains(table)) return
  const tableIndex = getEditorTables().indexOf(table)
  const block = getEditorTableBlockForTable(table, tableIndex)
  const rows = block ? editableRowsFromTableBlock(block) : getRenderedTableRows(table)
  if (!rows.length) return
  if (tableExpandCloseTimer !== null) {
    window.clearTimeout(tableExpandCloseTimer)
    tableExpandCloseTimer = null
  }
  expandedTableRows.value = normalizeExpandedTableRows(rows)
  expandedTableEditable.value = !!block
  expandedEditorTableBlock = block || null
  tableExpandClosing.value = false
  showTableExpandDialog.value = true
  hideTableDeleteButton()
  nextTick(() => document.querySelector<HTMLTextAreaElement>('.editor-table-expand-dialog textarea')?.focus())
}

const replaceTableBreakTextNodes = (table: HTMLTableElement) => {
  table.querySelectorAll('td,th').forEach((cell) => {
    const walker = document.createTreeWalker(cell, NodeFilter.SHOW_TEXT, {
      acceptNode(node) {
        return /<br\s*\/?\s*>/i.test(node.textContent || '') ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT
      }
    })
    const nodes: Text[] = []
    while (walker.nextNode()) nodes.push(walker.currentNode as Text)
    nodes.forEach((textNode) => {
      const parts = String(textNode.textContent || '').split(TABLE_CELL_BREAK_RE)
      if (parts.length <= 1) return
      const fragment = document.createDocumentFragment()
      parts.forEach((part, index) => {
        if (part) fragment.appendChild(document.createTextNode(part))
        if (index < parts.length - 1) fragment.appendChild(document.createElement('br'))
      })
      textNode.parentNode?.replaceChild(fragment, textNode)
    })
  })
}

const syncEditorAfterDomTableRemoval = (table: HTMLTableElement | null) => {
  if (!table || !editorContainer.value?.contains(table)) return false
  const editable = table.closest('.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset, .vditor-sv .vditor-reset') as HTMLElement | null
  const removable = table.closest<HTMLElement>('[data-type="table"], .vditor-ir__node') || table
  removable.remove()
  editable?.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'deleteContent' }))
  window.setTimeout(() => {
    const nextValue = vditorInstance?.getValue?.() || ''
    emit('update:modelValue', nextValue)
    refreshAttachmentLinksFromEditor()
  }, 0)
  clearSelectedEditorTable()
  return true
}

const applyTableSourceDeletion = (value: string, block: EditorTableSourceBlock) => {
  const lines = value.split('\n')
  lines.splice(block.start, block.end - block.start)
  return lines.join('\n').replace(/\n{3,}/g, '\n\n').replace(/^\n+|\n+$/g, '')
}

const deleteEditorTable = (table: HTMLTableElement | null, preferredIndex = -1) => {
  if (!vditorInstance) return false
  const value = vditorInstance.getValue?.() || ''
  const blocks = getEditorTableSourceBlocks(value)
  const block = tableBlockFromDataset(table, blocks) || findMarkdownTableBlock(blocks, getRenderedTableRows(table), preferredIndex)
  if (!block) return syncEditorAfterDomTableRemoval(table)
  const nextValue = applyTableSourceDeletion(value, block)
  vditorInstance.setValue(nextValue)
  emit('update:modelValue', nextValue)
  clearSelectedEditorTable()
  window.setTimeout(() => refreshAttachmentLinksFromEditor(), 0)
  return true
}

const deleteSelectedEditorTable = () => {
  if (!vditorInstance) return false
  const tables = getEditorTables()
  const tableIndex = selectedEditorTable ? tables.indexOf(selectedEditorTable) : selectedEditorTableIndex
  if (tableIndex < 0) return false
  return deleteEditorTable(selectedEditorTable, tableIndex)
}

const enhanceEditorTables = (_root: HTMLElement) => {
  const blocks = getEditorTableSourceBlocks(vditorInstance?.getValue?.() || '')
  const usedBlocks = new Set<EditorTableSourceBlock>()
  getEditorTables().forEach((table, index) => {
    table.classList.add('editor-deletable-table')
    table.dataset.editorTableIndex = String(index)
    replaceTableBreakTextNodes(table)
    const renderedRows = getRenderedTableRows(table)
    const datasetBlock = tableBlockFromDataset(table, blocks)
    let block = datasetBlock && !usedBlocks.has(datasetBlock) ? datasetBlock : undefined
    if (!block && tableRowsHaveComparableContent(renderedRows)) {
      block = blocks.find((candidate) => !usedBlocks.has(candidate) && sameTableRows(comparableRowsFromTableBlock(candidate), renderedRows))
    }
    if (!block) block = blocks.find((candidate) => !usedBlocks.has(candidate))
    if (block) usedBlocks.add(block)
    const scrollKey = tableScrollKeyFromBlock(block, `index:${index}`)
    table.dataset.editorTableScrollKey = scrollKey
    table.onscroll = () => rememberEditorTableScroll(table)
    if (block) {
      table.dataset.editorTableBlockStart = String(block.start)
      table.dataset.editorTableBlockEnd = String(block.end)
    } else {
      delete table.dataset.editorTableBlockStart
      delete table.dataset.editorTableBlockEnd
    }
    restoreEditorTableScroll(table)
  })
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

  const toolbarAction = (item: HTMLElement | null) => {
    if (!item) return null
    return item.matches('[data-type]') ? item : item.querySelector<HTMLElement>('[data-type], [aria-label], [title]')
  }

  const isHeadingsItem = (item: HTMLElement | null) => {
    const action = toolbarAction(item)
    const type = action?.getAttribute('data-type') || ''
    const label = action?.getAttribute('aria-label') || action?.getAttribute('title') || ''
    return type === 'headings' || /标题|Heading|Headings/i.test(label)
  }

  const isTableItem = (item: HTMLElement | null) => {
    const action = toolbarAction(item)
    const type = action?.getAttribute('data-type') || ''
    const label = action?.getAttribute('aria-label') || action?.getAttribute('title') || ''
    return type === 'table' || /表格|Table/i.test(label)
  }

  const openHeadingMenu = async (item: HTMLElement) => {
    headingTrigger.value = item
    nativeHeadingPanel.value = item.querySelector<HTMLElement>('.vditor-hint, .vditor-panel')
    if (nativeHeadingPanel.value) {
      nativeHeadingPanel.value.classList.add('vditor-panel--none')
      nativeHeadingPanel.value.style.display = 'none'
    }
    closeTableMenu()
    selectedHeadingTag.value = getCurrentHeadingTag()
    showHeadingMenu.value = true
    await nextTick()
    scheduleFloatingMenuPosition(positionHeadingMenu)
  }

  const openTableMenu = async (item: HTMLElement) => {
    tableTrigger.value = item
    nativeTablePanel.value = item.querySelector<HTMLElement>('.vditor-hint, .vditor-panel')
    if (nativeTablePanel.value) {
      nativeTablePanel.value.classList.add('vditor-panel--none')
      nativeTablePanel.value.style.display = 'none'
    }
    closeHeadingMenu()
    showTableMenu.value = true
    await nextTick()
    scheduleFloatingMenuPosition(positionTableMenu)
  }

  const handleToolbarClick = (event: Event) => {
    const target = event.target instanceof Element ? event.target : null
    const item = target?.closest('.vditor-toolbar__item') as HTMLElement | null
    if (!item || !editorContainer.value?.contains(item)) return
    const isHeading = isHeadingsItem(item)
    const isTable = isTableItem(item)
    if (!isHeading && !isTable) return
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    if (isHeading) {
      if (showHeadingMenu.value && headingTrigger.value === item) {
        closeHeadingMenu()
        return
      }
      openHeadingMenu(item)
      return
    }
    if (showTableMenu.value && tableTrigger.value === item) {
      closeTableMenu()
      return
    }
    openTableMenu(item)
  }

  const handleDocumentPointer = (event: Event) => {
    if (!showHeadingMenu.value && !showTableMenu.value) return
    const target = event.target instanceof Element ? event.target : null
    if (target?.closest('.vditor-heading-menu, .vditor-table-menu')) return
    const toolbarItem = target?.closest('.vditor-toolbar__item')
    if (toolbarItem === headingTrigger.value || toolbarItem === tableTrigger.value) return
    closeHeadingMenu()
    closeTableMenu()
  }

  const handleFloatingReposition = () => {
    if (showHeadingMenu.value) scheduleFloatingMenuPosition(positionHeadingMenu)
    if (showTableMenu.value) scheduleFloatingMenuPosition(positionTableMenu)
  }
  const scrollContainers = Array.from(document.querySelectorAll('.center-col, .content-wrapper')) as HTMLElement[]
  const observedPanelRoots = [editorContainer.value, editorContainer.value?.querySelector('.vditor'), editorContainer.value?.querySelector('.vditor-content'), toolbarEl]
    .filter((el): el is HTMLElement => el instanceof HTMLElement)
  const panelResizeObserver = typeof ResizeObserver !== 'undefined'
    ? new ResizeObserver(() => handleFloatingReposition())
    : null
  observedPanelRoots.forEach((el) => panelResizeObserver?.observe(el))
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
    observedPanelRoots.forEach((el) => panelResizeObserver?.unobserve(el))
    panelResizeObserver?.disconnect()
    window.visualViewport?.removeEventListener('resize', handleFloatingReposition)
    window.visualViewport?.removeEventListener('scroll', handleFloatingReposition)
    closeHeadingMenu()
    closeTableMenu()
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

    const scheduleToolbarPositionUpdate = () => requestAnimationFrame(updateToolbarPosition)

    const scrollContainers = Array.from(document.querySelectorAll('.center-col, .content-wrapper')) as HTMLElement[];
    scrollContainers.forEach((el) => el.addEventListener('scroll', updateToolbarPosition, { passive: true }));
    window.addEventListener('resize', updateToolbarPosition);
    window.addEventListener('scroll', updateToolbarPosition, { passive: true });
    if (typeof ResizeObserver !== 'undefined') {
      toolbarResizeObserver = new ResizeObserver(scheduleToolbarPositionUpdate)
      toolbarResizeObserver.observe(root)
      const content = root.querySelector('.vditor-content') as HTMLElement | null
      if (content) toolbarResizeObserver.observe(content)
    }
    updateToolbarPosition();

    mutationObserver = new MutationObserver(() => updateToolbarPosition());
    mutationObserver.observe(root, { attributes: true, attributeFilter: ['class'] });

    fixedCleanup = () => {
      scrollContainers.forEach((el) => el.removeEventListener('scroll', updateToolbarPosition));
      window.removeEventListener('resize', updateToolbarPosition);
      window.removeEventListener('scroll', updateToolbarPosition);
      mutationObserver?.disconnect();
      mutationObserver = null;
      toolbarResizeObserver?.disconnect();
      toolbarResizeObserver = null;
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
    if (tableExpandCloseTimer !== null) {
      window.clearTimeout(tableExpandCloseTimer)
      tableExpandCloseTimer = null
    }
    showTableExpandDialog.value = false
    tableExpandClosing.value = false
  } catch (e) {
    console.warn('Vditor destroy error', e);
  }
});

const getEditorTableCellFromElement = (element: Element | null | undefined) => {
  if (!element || !editorContainer.value) return null as HTMLTableCellElement | null
  const cell = element.closest?.('td,th') as HTMLTableCellElement | null
  if (!cell || !editorContainer.value.contains(cell)) return null
  if (cell.closest('.vditor-ir table, .vditor-wysiwyg table, .vditor-reset table')) return cell
  return null
}

const getEditorTableCellFromRange = (range: Range | null) => {
  if (!range || !editorContainer.value) return null as HTMLTableCellElement | null
  const node = range.commonAncestorContainer
  const element = node instanceof Element ? node : node.parentElement
  return getEditorTableCellFromElement(element)
}

const getEditorTableCellFromEvent = (event?: Event) => {
  const target = event?.target as Node | null | undefined
  const element = target instanceof Element ? target : target?.parentElement
  return getEditorTableCellFromElement(element)
}

const getCurrentEditorTableCell = (event?: Event) => {
  if (typeof window === 'undefined') return null as HTMLTableCellElement | null
  const selection = window.getSelection()
  const range = selection && selection.rangeCount > 0 ? selection.getRangeAt(0) : null
  return getEditorTableCellFromRange(range) || getEditorTableCellFromEvent(event)
}

const restoreLastEditorSelection = () => {
  if (typeof window === 'undefined' || !lastEditorSelectionRange || !editorContainer.value) return false
  const cell = getEditorTableCellFromRange(lastEditorSelectionRange)
  if (!cell) return false
  const selection = window.getSelection()
  if (!selection) return false
  try {
    selection.removeAllRanges()
    selection.addRange(lastEditorSelectionRange.cloneRange())
    return true
  } catch {
    return false
  }
}

const normalizeTableCellInsertion = (value: string) => String(value || '')
  .replace(/\r?\n+/g, ' ')
  .trim()

const insertValueIntoCurrentTableCell = (value: string) => {
  if (!vditorInstance) return false
  const text = normalizeTableCellInsertion(value)
  if (!text) return false
  let cell = getCurrentEditorTableCell()
  if (!cell && restoreLastEditorSelection()) {
    cell = getCurrentEditorTableCell()
  }
  if (!cell) return false
  if (hasAttachmentMarker(text)) {
    const target = getEditorTableCellSourceTarget(cell)
    if (target) {
      const current = target.rowCells[target.cellIndex] || ''
      const separator = current && !/\s$/.test(current) ? ' ' : ''
      return applyEditorTableCellSourceValue(cell, `${current}${separator}${text}`)
    }
  }
  const selection = window.getSelection()
  if (!selection || selection.rangeCount === 0) return false
  const currentRange = selection.getRangeAt(0)
  const rangeRoot = currentRange.commonAncestorContainer instanceof Element
    ? currentRange.commonAncestorContainer
    : currentRange.commonAncestorContainer.parentElement
  if (!rangeRoot || !cell.contains(rangeRoot)) return false
  let inserted = false
  try {
    inserted = document.execCommand('insertText', false, text)
  } catch {
    inserted = false
  }
  if (!inserted) {
    const range = selection.getRangeAt(0)
    range.deleteContents()
    const textNode = document.createTextNode(text)
    range.insertNode(textNode)
    range.setStartAfter(textNode)
    range.collapse(true)
    selection.removeAllRanges()
    selection.addRange(range)
  }
  const editable = cell.closest('.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset, .vditor-sv .vditor-reset') as HTMLElement | null
  editable?.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: text }))
  syncEditorTableCellDomToSource(cell)
  const updatedSelection = window.getSelection()
  if (updatedSelection && updatedSelection.rangeCount > 0) lastEditorSelectionRange = updatedSelection.getRangeAt(0).cloneRange()
  refreshAttachmentLinksFromEditor()
  window.setTimeout(() => emit("update:modelValue", vditorInstance?.getValue?.() || ''), 0)
  return true
}

const insertAttachmentSourceValue = (value: string) => {
  if (!vditorInstance || !hasAttachmentMarker(value)) return false
  const normalized = normalizeAttachmentInsertValue(value).trim()
  if (!normalized) return false
  const currentValue = vditorInstance.getValue?.() || ''
  const base = currentValue.replace(/\s+$/g, '')
  const nextValue = base ? `${base}\n\n${normalized}` : normalized
  vditorInstance.setValue(nextValue)
  normalizeEditorAttachmentSource()
  refreshAttachmentLinksFromEditor()
  window.setTimeout(() => refreshAttachmentLinksFromEditor(), 0)
  emit("update:modelValue", nextValue)
  return true
}

defineExpose({
  clear: () => {
    if (vditorInstance) {
      vditorInstance.setValue('');
      emit("update:modelValue", '');
    }
  },
  insertValue: (val: string) => {
    if (vditorInstance) {
      if (insertValueIntoCurrentTableCell(val)) return
      if (insertAttachmentSourceValue(val)) return
      const nextValue = normalizeAttachmentInsertValue(val)
      vditorInstance.insertValue(nextValue);
      normalizeEditorAttachmentSource()
      refreshAttachmentLinksFromEditor()
      window.setTimeout(() => refreshAttachmentLinksFromEditor(), 0)
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

.vditor-container .editor-attachment-node {
  display: inline-block;
  max-width: 100%;
  vertical-align: baseline;
  user-select: none;
  -webkit-user-drag: none;
}

.vditor-container .editor-attachment-node .vditor-ir__link {
  display: inline-block;
  max-width: 100%;
  overflow-wrap: anywhere;
  word-break: break-word;
}

.vditor-container .editor-attachment-node .vditor-ir__marker,
.vditor-container .editor-attachment-node .vditor-ir__marker--link,
.vditor-container .editor-attachment-node .vditor-ir__marker--bracket,
.vditor-container .editor-attachment-node .vditor-ir__marker--open,
.vditor-container .editor-attachment-node .vditor-ir__marker--close,
.vditor-container .editor-attachment-node .vditor-ir__marker--paren {
  display: none !important;
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
.editor-attachment-preview video {
  display: block;
  width: 100%;
  max-width: 100%;
}

.editor-attachment-preview img {
  height: auto;
  border-radius: 8px;
}

.editor-attachment-preview audio {
  display: block;
  width: min(300px, 100%);
  max-width: 100%;
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

.vditor-container .vditor-reset table.editor-deletable-table {
  display: block;
  width: max-content;
  min-width: 100%;
  max-width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  border-collapse: collapse;
  scrollbar-width: thin;
  scrollbar-color: rgba(100, 116, 139, 0.62) rgba(148, 163, 184, 0.18);
}

.vditor-container .vditor-reset table.editor-deletable-table::-webkit-scrollbar {
  height: 9px;
}

.vditor-container .vditor-reset table.editor-deletable-table::-webkit-scrollbar-track {
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.18);
}

.vditor-container .vditor-reset table.editor-deletable-table::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgba(100, 116, 139, 0.62);
}

.vditor-container .vditor-reset table.editor-deletable-table::-webkit-scrollbar-thumb:hover {
  background: rgba(71, 85, 105, 0.82);
}

.vditor-container .vditor-reset table.editor-deletable-table th,
.vditor-container .vditor-reset table.editor-deletable-table td {
  min-width: 88px;
  max-width: 280px;
  white-space: pre-wrap;
  overflow-wrap: break-word;
  word-break: break-word;
  vertical-align: top;
}

.vditor-reset table th,
.vditor-reset table td {
  border: 1px solid rgba(148, 163, 184, 0.55);
  background: rgba(255, 255, 255, 0.95);
  color: #111827;
}

.vditor-reset table th {
  font-weight: 400;
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

.editor-table-delete-button {
  box-sizing: border-box;
  display: grid !important;
  place-items: center !important;
  position: fixed !important;
  width: 10px !important;
  min-width: 10px !important;
  height: 10px !important;
  min-height: 10px !important;
  padding: 0 !important;
  border: 0 !important;
  border-radius: 2px !important;
  background: #f97316 !important;
  color: #fff !important;
  box-shadow: 0 1px 2px rgba(154, 52, 18, 0.35);
  cursor: pointer;
  opacity: .96;
  transform-origin: 100% 100% !important;
}

.editor-table-delete-button::before,
.editor-table-delete-button::after {
  content: '';
  position: absolute;
  left: 50%;
  top: 50%;
  width: 7px;
  height: 1.5px;
  border-radius: 999px;
  background: currentColor;
  transform-origin: center;
}

.editor-table-delete-button::before {
  transform: translate(-50%, -50%) rotate(45deg);
}

.editor-table-delete-button::after {
  transform: translate(-50%, -50%) rotate(-45deg);
}

.editor-table-delete-button:hover,
.editor-table-delete-button:focus-visible {
  outline: none;
  background: #ea580c !important;
  color: #fff !important;
  box-shadow: 0 1px 3px rgba(154, 52, 18, 0.42);
  opacity: 1;
}

html.dark .editor-table-delete-button {
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.42);
}

html.dark .editor-table-delete-button:hover,
html.dark .editor-table-delete-button:focus-visible {
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.5);
}

.editor-table-expand-button {
  box-sizing: border-box;
  display: inline-flex !important;
  align-items: center !important;
  justify-content: center !important;
  position: fixed !important;
  width: 10px !important;
  min-width: 10px !important;
  height: 10px !important;
  min-height: 10px !important;
  padding: 0 !important;
  border-radius: 2px !important;
  border-color: rgba(148, 163, 184, 0.46) !important;
  background: rgba(255, 255, 255, 0.96) !important;
  color: rgba(51, 65, 85, 0.96) !important;
  font-size: 8px;
  line-height: 1;
  transform-origin: 0 100% !important;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.20);
}

.editor-table-expand-button:hover,
.editor-table-expand-button:focus-visible {
  border-color: rgba(100, 116, 139, 0.62) !important;
  background: rgba(241, 245, 249, 0.98) !important;
  color: rgba(15, 23, 42, 0.98) !important;
}

.editor-table-expand-button > span {
  display: block;
  line-height: 1;
  transform: none;
}

html.dark .editor-table-expand-button {
  border-color: rgba(148, 163, 184, 0.42) !important;
  background: rgba(30, 41, 59, 0.96) !important;
  color: rgba(226, 232, 240, 0.96) !important;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.42);
}

html.dark .editor-table-expand-button:hover,
html.dark .editor-table-expand-button:focus-visible {
  border-color: rgba(203, 213, 225, 0.46) !important;
  background: rgba(51, 65, 85, 0.98) !important;
  color: #fff !important;
}

.editor-table-expand-overlay {
  position: fixed;
  inset: 0;
  z-index: 10030;
  display: grid;
  place-items: center;
  padding: 12px;
  background: rgba(15, 23, 42, 0.38);
  backdrop-filter: blur(8px) saturate(115%);
  animation: editorTableOverlayIn 180ms ease both;
}

.editor-table-expand-overlay.is-closing {
  animation: editorTableOverlayOut 180ms ease both;
}

.editor-table-expand-dialog {
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
  animation: editorTableDialogIn 180ms cubic-bezier(.2, .85, .2, 1) both;
}

.editor-table-expand-overlay.is-closing .editor-table-expand-dialog {
  animation: editorTableDialogOut 180ms ease both;
}

.editor-table-expand-overlay.is-dark .editor-table-expand-dialog {
  border-color: rgba(255, 255, 255, 0.16);
  background: rgba(15, 23, 42, 0.96);
  color: #f8fafc;
  box-shadow: 0 26px 70px rgba(0, 0, 0, 0.48);
}

.editor-table-expand-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 14px 16px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.20);
}

.editor-table-expand-header > div {
  display: grid;
  gap: 2px;
}

.editor-table-expand-header strong {
  font-size: 15px;
  font-weight: 700;
}

.editor-table-expand-header span {
  font-size: 12px;
  color: rgba(71, 85, 105, 0.86);
}

.editor-table-expand-overlay.is-dark .editor-table-expand-header {
  border-bottom-color: rgba(255, 255, 255, 0.12);
}

.editor-table-expand-overlay.is-dark .editor-table-expand-header span {
  color: rgba(203, 213, 225, 0.78);
}

.editor-table-expand-close {
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

.editor-table-expand-scroll {
  min-width: 0;
  min-height: 0;
  overflow: auto;
  padding: 12px;
  scrollbar-width: thin;
  scrollbar-color: rgba(100, 116, 139, 0.62) rgba(148, 163, 184, 0.18);
}

.editor-table-expand-scroll::-webkit-scrollbar {
  width: 9px;
  height: 9px;
}

.editor-table-expand-scroll::-webkit-scrollbar-track {
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.18);
}

.editor-table-expand-scroll::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgba(100, 116, 139, 0.62);
}

.editor-table-expand-table {
  width: max-content;
  min-width: 100%;
  border-collapse: collapse;
}

.editor-table-expand-table th,
.editor-table-expand-table td {
  min-width: 72px;
  max-width: 220px;
  padding: 0;
  border: 1px solid rgba(148, 163, 184, 0.42);
  vertical-align: top;
  background: rgba(255, 255, 255, 0.94);
  font-weight: 400;
}

.editor-table-expand-overlay.is-dark .editor-table-expand-table th,
.editor-table-expand-overlay.is-dark .editor-table-expand-table td {
  border-color: rgba(226, 232, 240, 0.20);
  background: rgba(30, 41, 59, 0.74);
  font-weight: 400;
}

.editor-table-expand-table textarea {
  display: block;
  width: 100%;
  min-width: 72px;
  min-height: 42px;
  height: auto;
  padding: 9px 10px;
  border: 0;
  outline: none;
  resize: vertical;
  background: transparent;
  color: inherit;
  font: inherit;
  line-height: 1.45;
  white-space: pre-wrap;
  overflow-wrap: break-word;
}

.editor-table-expand-table textarea:focus {
  box-shadow: inset 0 0 0 2px rgba(249, 115, 22, 0.48);
}

.editor-table-expand-table textarea[readonly] {
  cursor: default;
}

.editor-table-expand-attachments {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 0 8px 8px;
}

.editor-table-expand-attachment-tag {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  min-height: 24px;
  padding: 0 8px;
  border: 1px solid rgba(249, 115, 22, 0.36);
  border-radius: 8px;
  background: rgba(249, 115, 22, 0.10);
  color: #ea580c;
  font-size: 11px;
  font-weight: 650;
  line-height: 1;
  text-decoration: none;
  cursor: zoom-in;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.editor-table-expand-overlay.is-dark .editor-table-expand-attachment-tag {
  border-color: rgba(251, 146, 60, 0.42);
  background: rgba(249, 115, 22, 0.18);
  color: #fed7aa;
}

.editor-table-expand-attachment-tag:hover,
.editor-table-expand-attachment-tag:focus-visible {
  outline: none;
  border-color: rgba(249, 115, 22, 0.68);
  background: rgba(249, 115, 22, 0.18);
  color: #c2410c;
}

@keyframes editorTableOverlayIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes editorTableOverlayOut {
  from { opacity: 1; }
  to { opacity: 0; }
}

@keyframes editorTableDialogIn {
  from { opacity: 0; transform: translate3d(0, 16px, 0) scale(.92); }
  to { opacity: 1; transform: translate3d(0, 0, 0) scale(1); }
}

@keyframes editorTableDialogOut {
  from { opacity: 1; transform: translate3d(0, 0, 0) scale(1); }
  to { opacity: 0; transform: translate3d(0, 12px, 0) scale(.92); }
}

html.dark .vditor-hint {
  background: #202a36;
  color: #ffffff;
  border-color: rgba(255, 255, 255, 0.1);
}

.vditor-table-menu {
  position: fixed !important;
  z-index: 5004 !important;
  box-sizing: border-box;
  display: grid !important;
  gap: 10px !important;
  width: 324px !important;
  min-width: 324px !important;
  max-width: min(324px, calc(100vw - 16px)) !important;
  padding: 12px 24px !important;
  border: 1px solid var(--nw-floating-border) !important;
  border-radius: 12px !important;
  background: var(--nw-floating-bg) !important;
  color: var(--nw-floating-text) !important;
  box-shadow: var(--nw-floating-shadow) !important;
}

.vditor-table-menu.is-dark {
  --nw-floating-bg: #0f172a;
  --nw-floating-text: #f8fafc;
  --nw-floating-border: rgba(255, 255, 255, 0.18);
  --nw-floating-shadow: 0 18px 42px rgba(0, 0, 0, 0.42);
  --nw-floating-hover-bg: rgba(249, 115, 22, 0.26);
  --nw-floating-hover-border: rgba(249, 115, 22, 0.58);
  --nw-floating-selected-bg: rgba(249, 115, 22, 0.30);
  --nw-floating-selected-border: rgba(249, 115, 22, 0.70);
}

.table-menu-row {
  display: grid;
  grid-template-columns: 1fr 28px 34px 28px;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.table-menu-label,
.table-menu-value {
  min-width: 0;
  color: inherit;
  font-size: 12px;
  font-weight: 650;
  line-height: 1;
}

.table-menu-value {
  text-align: center;
}

.table-stepper-btn,
.table-size-cell {
  box-sizing: border-box;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  min-width: 28px;
  border: 1px solid var(--nw-floating-border) !important;
  border-radius: 7px !important;
  background: transparent !important;
  color: inherit !important;
  font-size: 14px;
  font-weight: 700;
  line-height: 1;
}

.table-stepper-btn:hover,
.table-stepper-btn:focus-visible,
.table-size-cell:hover,
.table-size-cell:focus-visible,
.table-size-cell.is-active {
  outline: none !important;
  border-color: var(--nw-floating-hover-border) !important;
  background: var(--nw-floating-hover-bg) !important;
}

.table-size-grid {
  display: grid;
  grid-template-columns: repeat(10, 24px);
  gap: 4px;
  justify-content: center;
}

.table-size-cell {
  width: 24px !important;
  height: 24px !important;
  min-width: 24px !important;
  min-height: 24px !important;
  max-width: 24px !important;
  max-height: 24px !important;
  flex: 0 0 24px;
  border-radius: 6px !important;
  padding: 0 !important;
}

.table-insert-btn {
  width: 100% !important;
  justify-content: center !important;
  min-height: 32px !important;
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
