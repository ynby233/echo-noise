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
            <span>{{ expandedTableEditable ? '可直接编辑单元格内容' : '正在同步表格内容' }}</span>
          </div>
          <button type="button" class="editor-table-expand-close nw-action-btn nw-tooltip-anchor" data-tooltip="关闭" aria-label="关闭放大表格" @click="closeExpandedTable">
            <span class="table-expand-close-icon" aria-hidden="true"></span>
          </button>
        </header>
        <div class="editor-table-expand-scroll">
          <table class="editor-table-expand-table">
            <colgroup v-if="expandedTableColumnWidths.length">
              <col
                v-for="(width, columnIndex) in expandedTableColumnWidths"
                :key="`expanded-column-${columnIndex}`"
                :style="{ width: `${width}px` }"
              />
            </colgroup>
            <tbody>
              <tr
                v-for="(row, rowIndex) in expandedTableRows"
                :key="`expanded-row-${rowIndex}`"
                :style="{ height: `${expandedTableRowHeight(rowIndex)}px` }"
              >
                <component
                  :is="'td'"
                  v-for="(_cell, cellIndex) in row"
                  :key="`expanded-cell-${rowIndex}-${cellIndex}`"
                  :style="{ width: `${expandedTableColumnWidths[cellIndex] || EXPANDED_TABLE_MIN_COLUMN_WIDTH}px`, height: `${expandedTableRowHeight(rowIndex)}px` }"
                >
                  <div class="editor-table-expand-cell" :style="{ height: `${expandedTableRowHeight(rowIndex)}px` }">
                    <textarea
                      :value="expandedTableCellEditorText(rowIndex, cellIndex)"
                      :readonly="!expandedTableEditable"
                      rows="1"
                      @input="updateExpandedTableCellText(rowIndex, cellIndex, $event)"
                      @keydown.enter.exact="insertExpandedTableCellLineBreak(rowIndex, cellIndex, $event)"
                      @keydown.tab.prevent="focusNextExpandedTableCell(rowIndex, cellIndex, $event.shiftKey)"
                    />
                    <div v-if="expandedTableCellAttachments(rowIndex, cellIndex).length" class="editor-table-expand-attachments">
                      <a
                        v-for="attachment in expandedTableCellAttachments(rowIndex, cellIndex)"
                        :key="`${rowIndex}-${cellIndex}-${attachment.type}-${attachment.url}`"
                        :href="attachment.url"
                        class="editor-table-expand-attachment-tag editor-attachment-link"
                        role="button"
                        tabindex="0"
                        draggable="false"
                        :data-attachment-kind="attachment.type"
                        :data-attachment-url="attachment.url"
                        :aria-label="`预览${attachment.title}`"
                        @pointerdown.stop
                        @click.prevent.stop="previewExpandedTableAttachment(attachment, $event)"
                        @keydown.enter.prevent.stop="previewExpandedTableAttachment(attachment, $event)"
                        @keydown.space.prevent.stop="previewExpandedTableAttachment(attachment, $event)"
                      >
                        {{ attachment.title }}
                      </a>
                    </div>
                  </div>
                  <span
                    :class="['editor-table-expand-row-resize-handle', { 'is-table-edge': rowIndex === expandedTableRows.length - 1 }]"
                    aria-hidden="true"
                    @pointerdown.prevent.stop="startExpandedTableRowResize(rowIndex, $event)"
                  ></span>
                  <span
                    :class="['editor-table-expand-column-resize-handle', { 'is-table-edge': cellIndex === row.length - 1 }]"
                    aria-hidden="true"
                    @pointerdown.prevent.stop="startExpandedTableColumnResize(cellIndex, $event)"
                  ></span>
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
import { computed, ref, onMounted, onBeforeUnmount, watch, nextTick } from "vue";
import { useToast } from '#imports'
import { getFixedCoordinateScale, getFixedRect, positionFloatingMenu, scheduleFloatingMenuPosition } from '~/utils/floating-menu'
import { captureVideoFirstFrameFromSource, ensureFancyboxVideoThumbnail, getVideoPlaybackFrameForSource, normalizeMediaPreviewUrl } from '~/utils/fancybox-video-close'
import { createMediaFancyboxOptions } from '~/utils/media-fancybox'
import { MARKDOWN_BLANK_LINE_SENTINEL, encodeMarkdownExtraBlankLines, isMarkdownBlankLineSentinel, markMarkdownPreservedBlankLineElements } from '~/utils/markdown-blank-lines'
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
const toast = useToast()

type PendingEditorTableCellSync = { tableIndex: number; rowIndex: number; cellIndex: number; text: string }
type EditorTableCellPosition = Pick<PendingEditorTableCellSync, 'tableIndex' | 'rowIndex' | 'cellIndex'>
type EditorTableCompositionCommitKey = EditorTableCellPosition & { key: 'Space' | 'Enter'; expiresAt: number }
type EditorTableCompositionCaretTarget = EditorTableCellPosition & { offset: number; expiresAt: number }
type ExpandedTableResizeDrag = { type: 'row' | 'column'; index: number; startClient: number; startSize: number }

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
let lastEditorTableSelectionRange: Range | null = null;
let lastEditorTableSelectionState: { editable: HTMLElement; tableIndex: number; rowIndex: number; cellIndex: number } | null = null;
let selectedEditorTable: HTMLTableElement | null = null;
let selectedEditorTableIndex = -1;
let hoveredEditorTable: HTMLTableElement | null = null;
let expandedEditorTableBlock: EditorTableSourceBlock | null = null;
let expandedEditorTableElement: HTMLTableElement | null = null;
let tableDeleteHideTimer: number | null = null;
let tableExpandCloseTimer: number | null = null;
let editorTableDomStabilizeTimer: number | null = null;
let pendingEditorTableCellSync: PendingEditorTableCellSync | null = null;
let editorTableCompositionActive = false;
let editorTableCompositionTarget: PendingEditorTableCellSync | null = null;
let editorTableCompositionSnapshot: string[][] | null = null;
let editorTableCompositionStartText = '';
let editorTableCompositionStartPrefix = '';
let editorTableCompositionCommitKey: EditorTableCompositionCommitKey | null = null;
let editorTableCompositionCaretTarget: EditorTableCompositionCaretTarget | null = null;
let editorTableCompositionSettlingUntil = 0;
let inlineEditorTableTextarea: HTMLTextAreaElement | null = null;
let inlineEditorTableTextareaBottomShield: HTMLDivElement | null = null;
type InlineEditorTableTextareaStyleSnapshot = {
  color: string;
  fontFamily: string;
  fontSize: string;
  fontStyle: string;
  fontVariant: string;
  fontWeight: string;
  letterSpacing: string;
  lineHeight: string;
  overflowWrap: string;
  padding: string;
  tabSize: string;
  textAlign: string;
  textIndent: string;
  textTransform: string;
  whiteSpace: string;
  wordBreak: string;
  wordSpacing: string;
};
let inlineEditorTableTextareaState: {
  cell: HTMLTableCellElement;
  baseText: string;
  minCellHeight: number;
  dirty: boolean;
  editorHeight: number;
  restoreStyle: {
    color: string;
    caretColor: string;
    height: string;
    minHeight: string;
    textShadow: string;
  };
  editorStyle: InlineEditorTableTextareaStyleSnapshot;
} | null = null;
const editorTableScrollPositions = new Map<string, number>();
let expandedTableResizeDrag: ExpandedTableResizeDrag | null = null;
let expandedTableRowHeightMeasureTimer: number | null = null;
const TABLE_DELETE_BUTTON_SIZE = 10;
const TABLE_EXPAND_BUTTON_SIZE = TABLE_DELETE_BUTTON_SIZE;
const INLINE_TABLE_CELL_BOTTOM_EDGE_SHIELD_MIN_PX = 8;
const INLINE_TABLE_CELL_EDGE_GUARD_PX = 8;
const TABLE_CELL_BREAK_RE = /<br\s*\/?\s*>/gi;
const TABLE_CELL_BREAK_PLACEHOLDER = '%%NW_TABLE_BR%%';
const TABLE_CELL_BREAK_SOURCE_RE = /(?:<br\s*\/?\s*>|%%NW_TABLE_BR%%)/gi;
const TABLE_CELL_BREAK_TEXT_RE = /^<br\s*\/?\s*>$/i;
const TABLE_CELL_CARET_ANCHOR = '\u200b';
const TABLE_CELL_CARET_ANCHOR_RE = /\u200b/g;
const PRESERVED_BLANK_LINE_DOM_ANCHOR = '\u200b';
const MARKDOWN_EMPTY_TABLE_CELL = '';
const MARKDOWN_EMPTY_TABLE_CELL_RE = /^(?:&nbsp;|&#160;|&#xA0;|\u00a0)$/i;
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
const expandedTableDirty = ref(false);
const expandedTableAvailableWidth = ref(0);
const expandedTableAutoRowHeights = ref<number[]>([]);
const expandedTableManualRowHeights = ref<number[]>([]);
const expandedTableManualColumnWidths = ref<number[]>([]);
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

type EditorAttachmentInfo = { type: 'image' | 'video' | 'audio' | 'file'; title: string; name: string; url: string }
const ATTACHMENT_MARKER_RE = /!?\[(图片附件|视频附件|音频附件|文件附件)：([^\]]+)\]\(([^)\s]+)\)/
const ATTACHMENT_MARKER_GLOBAL_RE = /!?\[(图片附件|视频附件|音频附件|文件附件)：([^\]]+)\]\(([^)\s]+)\)/g
const ADJACENT_ATTACHMENT_MARKER_RE = /(!?\[(?:图片附件|视频附件|音频附件|文件附件)：[^\]]+\]\([^)\s]+\))(!?\[(?:图片附件|视频附件|音频附件|文件附件)：[^\]]+\]\([^)\s]+\))/g
const RAW_ATTACHMENT_ANCHOR_RE = /<a\b[^>]*(?:data-attachment-url|href)=["']([^"']+)["'][^>]*>\s*(图片附件|视频附件|音频附件|文件附件)：([^<]+?)\s*<\/a>/gi
const ATTACHMENT_ANCHOR_LABEL_RE = /^(图片附件|视频附件|音频附件|文件附件)：(.+)$/
const EXPANDED_TABLE_MIN_COLUMN_WIDTH = 48
const EXPANDED_TABLE_MIN_ROW_HEIGHT = 38
const EXPANDED_TABLE_CELL_HORIZONTAL_PADDING = 18
const EXPANDED_TABLE_SCROLL_OVERFLOW_TOLERANCE = 2

const estimateTableLineWidth = (line: string) => {
  const text = stripAttachmentMarkersFromEditorText(String(line || '').replace(/\r\n?/g, '\n')) || ' '
  return Array.from(text).reduce((width, char) => {
    if (/\s/.test(char)) return width + 4
    if (/[^\x00-\xff]/.test(char)) return width + 14
    return width + 7
  }, EXPANDED_TABLE_CELL_HORIZONTAL_PADDING)
}

const calculateAdaptiveTableColumnWidths = (rows: string[][], availableWidth: number, minWidth = EXPANDED_TABLE_MIN_COLUMN_WIDTH) => {
  const columnCount = rows.reduce((max, row) => Math.max(max, row.length), 0)
  if (!columnCount) return [] as number[]
  const safeAvailable = Math.max(minWidth * columnCount, Math.floor(availableWidth || 0))
  const average = safeAvailable / columnCount
  const natural = Array.from({ length: columnCount }, (_, columnIndex) => {
    const maxLineWidth = rows.reduce((max, row) => {
      const value = String(row[columnIndex] || '')
      const lines = value.replace(/<br\s*\/?\s*>/gi, '\n').split('\n')
      return Math.max(max, ...lines.map((line) => estimateTableLineWidth(line)))
    }, minWidth)
    return Math.max(minWidth, Math.ceil(maxLineWidth))
  })
  if (natural.every((width) => width <= average)) {
    const base = Math.floor(average)
    const remainder = safeAvailable - base * columnCount
    return Array.from({ length: columnCount }, (_, index) => base + (index < remainder ? 1 : 0))
  }
  let widths = natural.map((width) => Math.max(minWidth, width))
  const total = widths.reduce((sum, width) => sum + width, 0)
  if (total < safeAvailable) {
    const extra = safeAvailable - total
    const share = Math.floor(extra / columnCount)
    const remainder = extra - share * columnCount
    widths = widths.map((width, index) => width + share + (index < remainder ? 1 : 0))
  }
  return widths.map((width) => Math.max(minWidth, Math.ceil(width)))
}

const syncExpandedTableScrollOverflowState = () => {
  if (typeof document === 'undefined') return
  const scroll = document.querySelector<HTMLElement>('.editor-table-expand-scroll')
  if (!scroll) return
  const horizontalOverflow = scroll.scrollWidth - scroll.clientWidth > EXPANDED_TABLE_SCROLL_OVERFLOW_TOLERANCE
  const verticalOverflow = scroll.scrollHeight - scroll.clientHeight > EXPANDED_TABLE_SCROLL_OVERFLOW_TOLERANCE
  scroll.classList.toggle('has-real-horizontal-overflow', horizontalOverflow)
  scroll.classList.toggle('has-real-vertical-overflow', verticalOverflow)
}

const scheduleExpandedTableScrollOverflowState = () => {
  if (typeof window === 'undefined') return
  window.requestAnimationFrame(syncExpandedTableScrollOverflowState)
}

const normalizeAttachmentInfo = (kindLabel: string, name: string, url: string): EditorAttachmentInfo | null => {
  const href = String(url || '').trim()
  if (!href) return null
  const type = kindLabel === '图片附件' ? 'image' : (kindLabel === '视频附件' ? 'video' : (kindLabel === '音频附件' ? 'audio' : 'file'))
  const cleanName = String(name || '').trim() || '未命名附件'
  return { type, title: `${kindLabel}：${cleanName}`, name: cleanName, url: href }
}

const browserPreviewableAttachmentUrl = (url: string) => /\.(pdf|txt|text|csv|json|xml|html?)(?:[?#].*)?$/i.test(String(url || ''))

const openFileAttachment = (info: EditorAttachmentInfo) => {
  if (browserPreviewableAttachmentUrl(info.url)) {
    window.open(info.url, '_blank', 'noopener,noreferrer')
    return
  }
  toast.add({
    title: '暂不支持预览',
    description: info.name || '该附件类型无法在浏览器中直接预览',
    color: 'orange',
    timeout: 2200,
  })
}

const attachmentInfoFromText = (text: string) => {
  const match = String(text || '').match(ATTACHMENT_MARKER_RE)
  if (!match) return null
  return normalizeAttachmentInfo(match[1], match[2], match[3])
}

const hasAttachmentMarker = (value: string) => {
  ATTACHMENT_MARKER_GLOBAL_RE.lastIndex = 0
  return ATTACHMENT_MARKER_GLOBAL_RE.test(normalizeAttachmentSourceText(value))
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
  emitEditorValue(normalized)
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
  if (info.type === 'file') {
    return `<a class="noise-attachment-file" href="${safeUrl}" target="_blank" rel="noopener noreferrer">${safeName}</a>`
  }
  return `<div class="noise-attachment-render noise-attachment-render--audio"><audio src="${safeUrl}" controls preload="metadata"></audio></div>`
}

const transformAttachmentPreviewHtml = (html: string) => {
  if (typeof document === 'undefined' || !html) return html
  const holder = document.createElement('div')
  holder.innerHTML = html
  markMarkdownPreservedBlankLineElements(holder)
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

const normalizeAttachmentSourceText = (value: string) => {
  RAW_ATTACHMENT_ANCHOR_RE.lastIndex = 0
  return String(value || '').replace(
    RAW_ATTACHMENT_ANCHOR_RE,
    (_match, url, kindLabel, name) => {
      const info = normalizeAttachmentInfo(kindLabel, name, url)
      return info ? attachmentInfoToMarkdownSource(info) : _match
    }
  )
}

const stripAttachmentMarkersFromEditorText = (value: string) => {
  const source = normalizeAttachmentSourceText(value).replace(/\r\n?/g, '\n')
  return source
    .split('\n')
    .map((line) => {
      ATTACHMENT_MARKER_GLOBAL_RE.lastIndex = 0
      const stripped = line
        .replace(ATTACHMENT_MARKER_GLOBAL_RE, '')
        .replace(/[ \t]+$/g, '')
      ATTACHMENT_MARKER_GLOBAL_RE.lastIndex = 0
      return ATTACHMENT_MARKER_GLOBAL_RE.test(line) && !stripped.trim() ? null : stripped
    })
    .filter((line): line is string => line !== null)
    .join('\n')
}

const mergeExpandedCellEditorText = (currentValue: string, editorText: string) => {
  const attachments = parseAttachmentMarkersFromText(currentValue).map(attachmentInfoToMarkdownSource)
  const text = String(editorText || '').replace(/\r\n?/g, '\n')
  if (!attachments.length) return text
  return text ? [text, ...attachments].join('\n') : attachments.join('\n')
}

const materializeEditorPreservedBlankLineBlocks = (root: HTMLElement) => {
  root.querySelectorAll<HTMLElement>('p[data-block], div[data-block]').forEach((block) => {
    if (block.closest('table, [data-type="code-block"], .vditor-ir__marker--pre')) return
    const rawText = block.textContent || ''
    if (!rawText.includes(MARKDOWN_BLANK_LINE_SENTINEL) || !isMarkdownBlankLineSentinel(rawText)) return
    setPlainBlankLineBlock(block)
  })
}

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

const createEditorAttachmentAnchor = (info: EditorAttachmentInfo) => {
  const anchor = document.createElement('a')
  anchor.href = info.url
  anchor.textContent = info.title
  anchor.className = 'editor-attachment-link'
  anchor.setAttribute('role', 'button')
  anchor.setAttribute('aria-label', `预览${info.title}`)
  anchor.setAttribute('data-attachment-kind', info.type)
  anchor.setAttribute('data-attachment-url', info.url)
  anchor.setAttribute('draggable', 'false')
  anchor.setAttribute('contenteditable', 'false')
  anchor.style.cursor = 'pointer'
  return anchor
}

const renderAttachmentMarkersInEditableRoot = (root: HTMLElement) => {
  if (typeof document === 'undefined') return false
  const textNodes: Text[] = []
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      const text = node.textContent || ''
      ATTACHMENT_MARKER_GLOBAL_RE.lastIndex = 0
      if (!ATTACHMENT_MARKER_GLOBAL_RE.test(text)) return NodeFilter.FILTER_REJECT
      const parent = node.parentElement
      if (!parent) return NodeFilter.FILTER_REJECT
      if (parent.closest('a, [data-type="a"], .editor-attachment-preview, textarea, code, [data-type="code-block"], .vditor-ir__marker--pre')) {
        return NodeFilter.FILTER_REJECT
      }
      return NodeFilter.FILTER_ACCEPT
    }
  })
  while (walker.nextNode()) textNodes.push(walker.currentNode as Text)
  let changed = false
  textNodes.forEach((textNode) => {
    const source = textNode.textContent || ''
    ATTACHMENT_MARKER_GLOBAL_RE.lastIndex = 0
    let match: RegExpExecArray | null
    let lastIndex = 0
    const fragment = document.createDocumentFragment()
    while ((match = ATTACHMENT_MARKER_GLOBAL_RE.exec(source))) {
      if (match.index > lastIndex) fragment.appendChild(document.createTextNode(source.slice(lastIndex, match.index)))
      const info = normalizeAttachmentInfo(match[1], match[2], match[3])
      fragment.appendChild(info ? createEditorAttachmentAnchor(info) : document.createTextNode(match[0]))
      lastIndex = ATTACHMENT_MARKER_GLOBAL_RE.lastIndex
    }
    if (lastIndex === 0) return
    if (lastIndex < source.length) fragment.appendChild(document.createTextNode(source.slice(lastIndex)))
    textNode.parentNode?.replaceChild(fragment, textNode)
    changed = true
  })
  return changed
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
    if (getEditorTableCellFromRange(range)) {
      storeLastEditorTableSelection(range)
    } else {
      lastEditorTableSelectionRange = null
      lastEditorTableSelectionState = null
    }
  }

  const onEditorSelectionChange = () => {
    if (document.activeElement === inlineEditorTableTextarea) return
    flushPendingEditorTableCellSourceSyncIfMoved(getCurrentEditorTableCell())
    captureEditorSelection()
    scheduleCollapseIrAttachmentChrome()
  }

  const onEditorSelectionEvent = () => {
    flushPendingEditorTableCellSourceSyncIfMoved(getCurrentEditorTableCell())
    captureEditorSelection()
    scheduleCollapseIrAttachmentChrome()
  }

  const commitEditorTableCellDomEdit = (cell: HTMLTableCellElement, options: { emit?: boolean; stabilize?: boolean; renderAttachments?: boolean } = {}) => {
    if (options.renderAttachments !== false) renderAttachmentMarkersInEditableRoot(cell)
    markEditorTableCellSourceDirty(cell)
    captureEditorSelection()
    if (options.stabilize !== false) scheduleStabilizePendingEditorTableCellDom()
    if (options.renderAttachments !== false) scheduleRefreshAttachmentLinks()
    if (options.emit === false) return
    const emitSafeValue = () => emitEditorValue()
    emitSafeValue()
    window.setTimeout(emitSafeValue, 0)
  }

  const handleEditorTableBeforeInput = (event: Event) => {
    if (isInlineEditorTableTextareaEvent(event)) {
      event.stopPropagation()
      return true
    }
    const inputEvent = event as InputEvent
    const cell = getCurrentEditorTableCell(event) || getEditorTableCellForCompositionInput(event)
    if (!cell) return false
    const inputType = inputEvent.inputType || ''
    const isLineBreakInput = inputType === 'insertLineBreak' || inputType === 'insertParagraph'
    const isCompositionTextInput = inputType === 'insertCompositionText'
    const pastedText = inputType === 'insertFromPaste'
      ? (inputEvent.dataTransfer?.getData('text/plain') || inputEvent.data || '')
      : ''
    const text = inputType === 'insertText' || inputType === 'insertCompositionText'
      ? (inputEvent.data || '')
      : pastedText
    if (shouldSuppressEditorTableCompositionCommitArtifact(cell, inputType, text)) {
      event.preventDefault()
      event.stopPropagation()
      ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
      return true
    }
    if (isLineBreakInput) return false
    if (editorTableCompositionActive || isCompositionTextInput || (inputEvent.isComposing && !isLineBreakInput)) return false
    if (!text && !isLineBreakInput) return false
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    const applied = isLineBreakInput
      ? insertLineBreakIntoCellDom(cell)
      : insertTextIntoCellDom(cell, text)
    if (!applied) return true
    commitEditorTableCellDomEdit(cell)
    return true
  }


  const onEditorInput = (event: Event) => {
    if (isInlineEditorTableTextareaEvent(event)) {
      event.stopPropagation()
      return
    }
    const cell = getCurrentEditorTableCell(event) || getEditorTableCellForCompositionInput(event)
    if (cell) {
      event.stopPropagation()
      ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
      commitEditorTableCellDomEdit(cell, {
        emit: !editorTableCompositionActive,
        stabilize: !editorTableCompositionActive,
        renderAttachments: !editorTableCompositionActive,
      })
      return
    }
    flushPendingEditorTableCellSourceSyncIfMoved(cell)
    captureEditorSelection()
    if (getEditorTables().length) {
      event.stopPropagation()
      ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
      const emitSafeValue = () => emitEditorValue()
      emitSafeValue()
      window.setTimeout(emitSafeValue, 0)
      return
    }
    if (normalizeEditorAttachmentSource()) return
    scheduleRefreshAttachmentLinks()
  }

  const onEditorFocusOut = () => {
    window.setTimeout(() => flushPendingEditorTableCellSourceSyncIfMoved(getCurrentEditorTableCell()), 0)
  }

  const onEditorCompositionStart = (event: CompositionEvent) => {
    if (isInlineEditorTableTextareaEvent(event)) {
      event.stopPropagation()
      return
    }
    const compositionCell = getCurrentEditorTableCell(event)
    if (!compositionCell) {
      preparePlainBlankLineInput(event)
      return
    }
    editorTableCompositionActive = true
    editorTableCompositionCommitKey = null
    editorTableCompositionCaretTarget = null
    editorTableCompositionStartText = ''
    editorTableCompositionStartPrefix = ''
    if (clearEditorTableEmptyPlaceholder(compositionCell)) placeCaretAtStartOfEditorTableCell(compositionCell)
    rememberEditorTableCompositionCell(compositionCell)
  }

  const onEditorCompositionUpdate = (event: CompositionEvent) => {
    if (isInlineEditorTableTextareaEvent(event)) {
      event.stopPropagation()
      return
    }
    if (getCurrentEditorTableCell(event) || getEditorTableCellForCompositionInput(event)) {
      stopEditorTablePropagation(event)
    }
  }

  const onEditorCompositionEnd = (event: CompositionEvent) => {
    if (isInlineEditorTableTextareaEvent(event)) {
      event.stopPropagation()
      return
    }
    editorTableCompositionActive = false
    const rememberedCell = editorTableCompositionTarget
      ? (getEditorTables()[editorTableCompositionTarget.tableIndex]?.rows[editorTableCompositionTarget.rowIndex]?.cells[editorTableCompositionTarget.cellIndex] as HTMLTableCellElement | undefined) || null
      : null
    const cell = rememberedCell || getCurrentEditorTableCell(event) || getPendingEditorTableCell()
    if (cell) {
      stopEditorTableNativeEvent(event)
      clearVditorCompositionLock()
      cleanupEditorTableCompositionDrift(event.data || '')
      commitEditorTableCellDomEdit(cell)
      rememberEditorTableCompositionCaretTarget(cell, event.data || '')
      markEditorTableCompositionSettling()
      scheduleRestoreEditorTableCompositionCaret()
    }
    editorTableCompositionTarget = null
    editorTableCompositionSnapshot = null
    editorTableCompositionStartText = ''
    editorTableCompositionStartPrefix = ''
  }

  const refreshAttachmentLinks = () => {
    materializeEditorPreservedBlankLineBlocks(root)
    root.querySelectorAll<HTMLElement>('td,th').forEach((cell) => renderAttachmentMarkersInEditableRoot(cell))
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

    if (info.type === 'file') {
      root.querySelectorAll('.editor-attachment-preview').forEach((node) => node.remove())
      openFileAttachment(info)
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

  const currentPlainEditorBlock = (event?: Event) => {
    const selection = window.getSelection()
    const anchorNode = selection?.anchorNode || null
    return getPlainEditorBlock(anchorNode) || getPlainEditorBlock(event?.target as Node | null | undefined)
  }

  const isEditorBlankLineEnter = (event: KeyboardEvent) => {
    if (event.key !== 'Enter' || event.shiftKey || event.altKey || event.ctrlKey || event.metaKey || event.isComposing) return false
    const block = currentPlainEditorBlock(event)
    const anchorElement = block || getEventElement(event)
    if (!anchorElement || !root.contains(anchorElement)) return false
    if (anchorElement.closest('.vditor-toolbar, .vditor-panel, .vditor-hint, [data-type="code-block"], .vditor-ir__marker--pre')) return false
    if (getCurrentEditorTableCell(event)) return false
    return isPlainBlankLineBlock(block)
  }

  const emitEditorSoftBreakInput = (_event: Event) => {
    scheduleRefreshAttachmentLinks()
    window.setTimeout(() => {
      if (vditorInstance?.getValue) emitEditorValue(getEditorDomContentFallback() || vditorInstance.getValue())
    }, 0)
  }

  const insertEditorSoftLineBreak = (event: Event) => {
    const block = currentPlainEditorBlock(event)
    if (!isPlainBlankLineBlock(block)) return false
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    setPlainBlankLineBlock(block!)
    insertPreservedBlankLineAfter(block!)
    emitEditorSoftBreakInput(event)
    return true
  }

  const insertEditorLeadingBlankLine = (event: KeyboardEvent) => {
    if (event.key !== 'Enter' || event.shiftKey || event.altKey || event.ctrlKey || event.metaKey || event.isComposing || editorTableCompositionActive) return false
    const block = currentPlainEditorBlock(event)
    if (!block || isPlainBlankLineBlock(block) || !isCaretAtStartOfPlainBlock(block)) return false
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    insertPreservedBlankLineBefore(block)
    placeCaretInPlainBlock(block)
    emitEditorSoftBreakInput(event)
    return true
  }

  const preparePlainBlankLineInput = (event: InputEvent | CompositionEvent) => {
    const block = currentPlainEditorBlock(event)
    if (!isPlainBlankLineBlock(block)) return false
    return clearPlainBlankLineForInput(block!)
  }

  const encodePlainEditorPasteValue = (pastedText: string) => {
    const source = String(pastedText || '').replace(/\r\n?/g, '\n')
    const tableBlocks = getMarkdownTableBlocks(source)
    if (!tableBlocks.length) return encodeMarkdownExtraBlankLines(source)
    const sourceLines = source.split('\n')
    let cursor = 0
    let output = ''
    const append = (value: string, options: { blockBoundary?: boolean } = {}) => {
      if (!value) return
      if (options.blockBoundary && output && !output.endsWith('\n')) output += '\n'
      output += value
    }
    tableBlocks.forEach((block) => {
      append(encodeMarkdownExtraBlankLines(sourceLines.slice(cursor, block.start).join('\n')))
      append(block.lines.map((line) => line.replace(TABLE_CELL_BREAK_RE, TABLE_CELL_BREAK_PLACEHOLDER)).join('\n'), { blockBoundary: true })
      cursor = block.end
    })
    append(encodeMarkdownExtraBlankLines(sourceLines.slice(cursor).join('\n')), { blockBoundary: true })
    return output
  }

  const insertPlainEditorPastedTextWithBlankLines = (event: Event, pastedText: string) => {
    if (!/\n{3,}/.test(pastedText.replace(/\r\n?/g, '\n'))) return false
    const anchorElement = getEventElement(event)
    if (!anchorElement || !root.contains(anchorElement)) return false
    if (anchorElement.closest('.vditor-toolbar, .vditor-panel, .vditor-hint, table, [data-type="code-block"], .vditor-ir__marker--pre')) return false
    if (!vditorInstance?.insertValue) return false
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    vditorInstance.insertValue(encodePlainEditorPasteValue(pastedText))
    window.setTimeout(() => {
      materializeEditorPreservedBlankLineBlocks(root)
      enhanceEditorTables(root)
      emitEditorValue()
    }, 0)
    return true
  }

  const handlePlainEditorPasteWithBlankLines = (event: InputEvent) => {
    if (event.inputType !== 'insertFromPaste') return false
    const pastedText = event.dataTransfer?.getData('text/plain') || event.data || ''
    return insertPlainEditorPastedTextWithBlankLines(event, pastedText)
  }

  const onEditorPaste = (event: ClipboardEvent) => {
    const pastedText = event.clipboardData?.getData('text/plain') || ''
    insertPlainEditorPastedTextWithBlankLines(event, pastedText)
  }

  const onPlainBlankLineMouseDown = (event: MouseEvent) => {
    const block = getPlainEditorBlock(event.target as Node | null)
    if (!isPlainBlankLineBlock(block)) return
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    setPlainBlankLineBlock(block!)
    placeCaretInPlainBlock(block!)
  }

  const onEditorTableMouseDown = (event: MouseEvent) => {
    const targetNode = event.target as Node | null
    const target = targetNode instanceof Element ? targetNode : targetNode?.parentElement
    if (target?.closest('a, button, .editor-attachment-preview')) return
    const cell = getEditorTableCellFromEvent(event)
    if (!cell) return
    editorTableCompositionCommitKey = null
    editorTableCompositionCaretTarget = null
    if (openInlineEditorTableCellTextarea(cell, event)) {
      event.preventDefault()
      event.stopPropagation()
      ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    }
  }

  const onPlainEditorBlankAreaMouseDown = (event: MouseEvent) => {
    if (getEditorTableCellFromEvent(event)) return
    if (getPlainEditorBlock(event.target as Node | null)) return
    editorTableCompositionCommitKey = null
    editorTableCompositionCaretTarget = null
    if (placeCaretInPlainEditorVisualBlankLine(event)) {
      event.preventDefault()
      event.stopPropagation()
      ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    }
  }

  const handleEditorBackspaceKeydown = (event: KeyboardEvent) => {
    if (event.key !== 'Backspace' || event.shiftKey || event.altKey || event.ctrlKey || event.metaKey || event.isComposing) return false
    const cell = getCurrentEditorTableCell(event)
    if (cell && handleEditorTableBackspaceAtLineBoundary(event, cell)) {
      commitEditorTableCellDomEdit(cell)
      return true
    }
    const block = currentPlainEditorBlock(event)
    if (block && handlePlainEditorBackspaceAtLineBoundary(event, block)) return true
    return false
  }

  const onPlainTextEnterKeydown = (event: KeyboardEvent) => {
    if (isInlineEditorTableTextareaEvent(event)) {
      event.stopPropagation()
      return
    }
    if (handleEditorBackspaceKeydown(event)) return
    const cell = getCurrentEditorTableCell(event) || getEditorTableCellAtPosition(editorTableCompositionTarget)
    // Windows Pinyin can leave the line-break Enter event marked as composing
    // after Space commits the candidate, so trust our composition lifecycle flag.
    const isPlainEnter = (event.key === 'Enter' || event.code === 'Enter' || event.code === 'NumpadEnter')
      && !event.shiftKey && !event.altKey && !event.ctrlKey && !event.metaKey
    if (cell && editorTableCompositionActive && (event.key === ' ' || event.code === 'Space')) {
      rememberEditorTableCompositionCommitKey(cell, 'Space')
      stopEditorTablePropagation(event)
      return
    }
    if (isPlainEnter) {
      if (cell && editorTableCompositionActive) {
        rememberEditorTableCompositionCommitKey(cell, 'Enter')
        stopEditorTablePropagation(event)
        return
      }
      if (!editorTableCompositionActive && cell && insertEditorTableCellLineBreak(event, cell)) return
    }
    if (insertEditorLeadingBlankLine(event)) return
    if (!isEditorBlankLineEnter(event)) return
    insertEditorSoftLineBreak(event)
  }

  const onEditorBeforeInput = (event: InputEvent) => {
    if (handleEditorTableBeforeInput(event)) return
    if (handlePlainEditorPasteWithBlankLines(event)) return
    if (/^(insertText|insertCompositionText|insertFromPaste)$/.test(event.inputType || '')) {
      preparePlainBlankLineInput(event)
      return
    }
    if (event.inputType !== 'insertParagraph' || event.isComposing) return
    const selection = window.getSelection()
    const anchorNode = selection?.anchorNode || null
    const anchorElement = anchorNode instanceof Element ? anchorNode : anchorNode?.parentElement || getEventElement(event)
    if (!anchorElement || !root.contains(anchorElement)) return
    if (anchorElement.closest('.vditor-toolbar, .vditor-panel, .vditor-hint, [data-type="code-block"], .vditor-ir__marker--pre')) return
    if (getCurrentEditorTableCell(event)) return
    if (isPlainBlankLineBlock(currentPlainEditorBlock(event))) {
      insertEditorSoftLineBreak(event)
    }
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
  const previewObserver = new MutationObserver(() => {
    if (pendingEditorTableCellSync) scheduleStabilizePendingEditorTableCellDom()
    scheduleRefreshAttachmentLinks()
  })
  previewObserver.observe(root, { childList: true, subtree: true })
  document.addEventListener('beforeinput', onEditorBeforeInput, true)
  root.addEventListener('beforeinput', onEditorBeforeInput, true)
  document.addEventListener('paste', onEditorPaste, true)
  root.addEventListener('paste', onEditorPaste, true)
  root.addEventListener('input', onEditorInput, true)
  root.addEventListener('compositionstart', onEditorCompositionStart, true)
  root.addEventListener('compositionupdate', onEditorCompositionUpdate, true)
  root.addEventListener('compositionend', onEditorCompositionEnd, true)
  root.addEventListener('focusout', onEditorFocusOut, true)
  root.addEventListener('mousedown', onEditorTableMouseDown, true)
  root.addEventListener('mousedown', onPlainBlankLineMouseDown, true)
  root.addEventListener('mousedown', onPlainEditorBlankAreaMouseDown, true)
  root.addEventListener('mouseup', onEditorSelectionEvent, true)
  root.addEventListener('keyup', onEditorSelectionEvent, true)
  document.addEventListener('selectionchange', onEditorSelectionChange, true)
  root.addEventListener('pointerdown', onTablePointerDown, true)
  root.addEventListener('pointermove', onTablePointerMove, true)
  root.addEventListener('pointerout', onTablePointerOut, true)
  root.addEventListener('pointerdown', preventAttachmentNavigation, true)
  root.addEventListener('mousedown', preventAttachmentNavigation, true)
  root.addEventListener('click', onAttachmentClick, true)
  document.addEventListener('keydown', onPlainTextEnterKeydown, true)
  root.addEventListener('keydown', onPlainTextEnterKeydown, true)
  root.addEventListener('keydown', onAttachmentKeydown, true)
  window.addEventListener('resize', repositionVisibleTableDeleteButton)
  window.addEventListener('resize', updateExpandedTableAvailableWidth)
  window.addEventListener('resize', repositionInlineEditorTableTextarea)
  window.addEventListener('scroll', repositionVisibleTableDeleteButton, { passive: true, capture: true })
  window.addEventListener('scroll', repositionInlineEditorTableTextarea, { passive: true, capture: true })
  attachmentPreviewCleanup = () => {
    previewObserver.disconnect()
    document.removeEventListener('beforeinput', onEditorBeforeInput, true)
    root.removeEventListener('beforeinput', onEditorBeforeInput, true)
    document.removeEventListener('paste', onEditorPaste, true)
    root.removeEventListener('paste', onEditorPaste, true)
    root.removeEventListener('input', onEditorInput, true)
    root.removeEventListener('compositionstart', onEditorCompositionStart, true)
    root.removeEventListener('compositionupdate', onEditorCompositionUpdate, true)
    root.removeEventListener('compositionend', onEditorCompositionEnd, true)
    root.removeEventListener('focusout', onEditorFocusOut, true)
    root.removeEventListener('mousedown', onEditorTableMouseDown, true)
    root.removeEventListener('mousedown', onPlainBlankLineMouseDown, true)
    root.removeEventListener('mousedown', onPlainEditorBlankAreaMouseDown, true)
    root.removeEventListener('mouseup', onEditorSelectionEvent, true)
    root.removeEventListener('keyup', onEditorSelectionEvent, true)
    document.removeEventListener('selectionchange', onEditorSelectionChange, true)
    root.removeEventListener('pointerdown', onTablePointerDown, true)
    root.removeEventListener('pointermove', onTablePointerMove, true)
    root.removeEventListener('pointerout', onTablePointerOut, true)
    root.removeEventListener('pointerdown', preventAttachmentNavigation, true)
    root.removeEventListener('mousedown', preventAttachmentNavigation, true)
    root.removeEventListener('click', onAttachmentClick, true)
    document.removeEventListener('keydown', onPlainTextEnterKeydown, true)
    root.removeEventListener('keydown', onPlainTextEnterKeydown, true)
    root.removeEventListener('keydown', onAttachmentKeydown, true)
    window.removeEventListener('resize', repositionVisibleTableDeleteButton)
    window.removeEventListener('resize', updateExpandedTableAvailableWidth)
    window.removeEventListener('resize', repositionInlineEditorTableTextarea)
    window.removeEventListener('scroll', repositionVisibleTableDeleteButton, true)
    window.removeEventListener('scroll', repositionInlineEditorTableTextarea, true)
    hideTableDeleteButton()
    closeInlineEditorTableTextarea()
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
    // Vditor's native cache stores raw getValue() output before our table serializer can
    // normalize in-cell line breaks, so persisted drafts must stay in AddForm's safe path.
    enable: false,
    id: "vue-vditor",
  },
  input: (content: string) => {
    const needsSafeTableValue = !!pendingEditorTableCellSync || !!getEditorTables().length || hasUnsafeMarkdownTableStructure(content)
    if (needsSafeTableValue) {
      const emitSafeValue = () => emitEditorValue()
      emitSafeValue()
      window.setTimeout(emitSafeValue, 0)
      window.setTimeout(emitSafeValue, 48)
      window.setTimeout(emitSafeValue, 160)
      return
    }
    emitEditorValue(content)
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
  positionFloatingMenu(tableTrigger.value, tableMenuRef.value, tableMenuStyle, 272, 'above-align-left')
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
  const tableRows = Array.from({ length: rowCount }, () => Array.from({ length: colCount }, () => MARKDOWN_EMPTY_TABLE_CELL))
  const divider = Array.from({ length: colCount }, () => '---')
  const formatRow = (cells: string[]) => `| ${cells.join(' | ')} |`
  return `\n${[formatRow(tableRows[0] || []), formatRow(divider), ...tableRows.slice(1).map(formatRow)].join('\n')}\n\n`
}

const insertTable = (rows: number, cols: number) => {
  if (!vditorInstance) return
  if (pendingEditorTableCellSync) flushPendingEditorTableCellSourceSync()
  vditorInstance.insertValue(buildMarkdownTable(rows, cols))
  emitEditorValue()
  scheduleNormalizeEditorTableSource()
  closeTableMenu()
}

const getEditorRootElement = () => {
  if (editorContainer.value) return editorContainer.value
  if (typeof document === 'undefined') return null
  return document.querySelector<HTMLElement>('.editor-box .vditor, .vditor-container.vditor, .vditor')
}

const isInsideEditorRoot = (node: Node | null) => {
  const root = getEditorRootElement()
  return !!node && !!root && root.contains(node)
}

const getEditorEditableElement = () => {
  const root = getEditorRootElement()
  if (!root) return null
  const candidates = Array.from(root.querySelectorAll<HTMLElement>('.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset, .vditor-sv .vditor-reset'))
  return candidates.find((node) => !!node.querySelector('table'))
    || candidates.find((node) => node.offsetParent !== null || node.getClientRects().length > 0)
    || candidates[0]
    || null
}

const getEditorTables = () => Array.from(getEditorRootElement()?.querySelectorAll<HTMLTableElement>('.vditor-reset table') || [])

const getPlainEditorBlock = (node: Node | null | undefined) => {
  const element = node instanceof Element ? node : node?.parentElement
  const block = element?.closest?.('p[data-block], div[data-block]') as HTMLElement | null
  const editable = getEditorEditableElement()
  if (!block || !editable?.contains(block) || block.closest('table')) return null
  return block
}

const isPlainBlankLineBlock = (block: HTMLElement | null | undefined) => {
  if (!block || block.closest('table')) return false
  const normalized = String(block.textContent || '')
    .replace(/[\u200b\u200c\ufeff]/g, '')
    .replace(/\u00a0/g, ' ')
    .trim()
  if (normalized !== '') return false
  return block.matches('p[data-block], div[data-block]')
}

const setPlainBlankLineBlock = (block: HTMLElement) => {
  block.innerHTML = ''
  block.appendChild(document.createTextNode(PRESERVED_BLANK_LINE_DOM_ANCHOR))
  block.classList.add('vditor-preserved-blank-line')
}

const placeCaretInPlainBlock = (block: HTMLElement, atEnd = false) => {
  const selection = window.getSelection()
  if (!selection) return false
  getEditorEditableFromNode(block)?.focus({ preventScroll: true })
  const range = document.createRange()
  const text = block.firstChild && block.firstChild.nodeType === Node.TEXT_NODE
    ? block.firstChild
    : block.appendChild(document.createTextNode(''))
  range.setStart(text, atEnd ? String(text.textContent || '').length : 0)
  range.collapse(true)
  selection.removeAllRanges()
  selection.addRange(range)
  lastEditorSelectionRange = range.cloneRange()
  return true
}

const insertPreservedBlankLineAfter = (block: HTMLElement) => {
  const nextBlock = document.createElement(block.tagName.toLowerCase())
  nextBlock.setAttribute('data-block', block.getAttribute('data-block') || '0')
  setPlainBlankLineBlock(nextBlock)
  block.after(nextBlock)
  placeCaretInPlainBlock(nextBlock)
  return nextBlock
}

const insertPreservedBlankLineBefore = (block: HTMLElement) => {
  const previousBlock = document.createElement(block.tagName.toLowerCase())
  previousBlock.setAttribute('data-block', block.getAttribute('data-block') || '0')
  setPlainBlankLineBlock(previousBlock)
  block.before(previousBlock)
  placeCaretInPlainBlock(previousBlock)
  return previousBlock
}

const createPlainPreservedBlankBlock = () => {
  const block = document.createElement('p')
  block.setAttribute('data-block', '0')
  setPlainBlankLineBlock(block)
  return block
}

const placeCaretInPlainEditorVisualBlankLine = (event: MouseEvent) => {
  const editable = getEditorEditableFromNode(event.target as Node | null)
  if (!editable) return false
  const editableRect = editable.getBoundingClientRect()
  if (event.clientY < editableRect.top || event.clientY > editableRect.bottom) return false
  const children = Array.from(editable.children).filter((child) => child instanceof HTMLElement) as HTMLElement[]
  const lastChild = children[children.length - 1] || null
  const lineHeight = getEditorElementLineHeight(editable)
  const targetBottom = lastChild?.getBoundingClientRect().bottom ?? (editableRect.top + getEditorElementPaddingTop(editable))
  if (event.clientY < targetBottom) return false
  const extraLines = Math.max(1, Math.floor((event.clientY - targetBottom) / lineHeight) + 1)
  let anchor = lastChild
  let targetBlock: HTMLElement | null = null
  for (let index = 0; index < extraLines; index += 1) {
    if (index === 0 && anchor && getPlainEditorBlock(anchor) && isPlainBlankLineBlock(anchor)) {
      targetBlock = anchor
    } else {
      const block = createPlainPreservedBlankBlock()
      if (anchor) anchor.after(block)
      else editable.appendChild(block)
      anchor = block
      targetBlock = block
    }
  }
  if (!targetBlock) return false
  lastEditorTableSelectionRange = null
  lastEditorTableSelectionState = null
  placeCaretInPlainBlock(targetBlock)
  emitEditorValue(getEditorDomContentFallback())
  return true
}

const isCaretAtStartOfPlainBlock = (block: HTMLElement) => {
  const selection = window.getSelection()
  if (!selection?.rangeCount || !selection.isCollapsed || !block.contains(selection.anchorNode)) return false
  const range = selection.getRangeAt(0)
  const prefixRange = document.createRange()
  prefixRange.selectNodeContents(block)
  try {
    prefixRange.setEnd(range.startContainer, range.startOffset)
  } catch {
    return false
  }
  const prefixText = String(prefixRange.cloneContents().textContent || '')
    .replace(/[\u200b\u200c\ufeff]/g, '')
    .replace(/\u00a0/g, ' ')
    .trim()
  return prefixText === ''
}

const clearPlainBlankLineForInput = (block: HTMLElement) => {
  if (!isPlainBlankLineBlock(block)) return false
  block.innerHTML = ''
  block.classList.remove('vditor-preserved-blank-line')
  placeCaretInPlainBlock(block)
  return true
}

const handlePlainEditorBackspaceAtLineBoundary = (event: KeyboardEvent, block: HTMLElement) => {
  const selection = typeof window === 'undefined' ? null : window.getSelection()
  if (!selection?.rangeCount || !selection.isCollapsed) return false
  if (isPlainBlankLineBlock(block)) {
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    const nextBlock = getPlainEditorBlock(block.nextElementSibling)
    const previousBlock = getPlainEditorBlock(block.previousElementSibling)
    block.remove()
    if (nextBlock) placeCaretInPlainBlock(nextBlock)
    else if (previousBlock) placeCaretInPlainBlock(previousBlock, true)
    return true
  }
  if (!isCaretAtStartOfPlainBlock(block)) return false
  const previousBlock = getPlainEditorBlock(block.previousElementSibling)
  if (!isPlainBlankLineBlock(previousBlock)) return false
  event.preventDefault()
  event.stopPropagation()
  ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
  previousBlock!.remove()
  placeCaretInPlainBlock(block)
  return true
}

const placeCaretAtStartOfEditorTableCell = (cell: HTMLTableCellElement) => {
  const selection = typeof window === 'undefined' ? null : window.getSelection()
  if (!selection) return false
  getEditorEditableFromNode(cell)?.focus({ preventScroll: true })
  const range = document.createRange()
  const textNode = cell.firstChild && cell.firstChild.nodeType === Node.TEXT_NODE
    ? cell.firstChild
    : cell.insertBefore(document.createTextNode(''), cell.firstChild)
  range.setStart(textNode, 0)
  range.collapse(true)
  selection.removeAllRanges()
  selection.addRange(range)
  lastEditorSelectionRange = range.cloneRange()
  storeLastEditorTableSelection(range)
  return true
}

const getEditorElementLineHeight = (element: HTMLElement) => {
  const style = window.getComputedStyle(element)
  const parsed = Number.parseFloat(style.lineHeight || '')
  if (Number.isFinite(parsed) && parsed > 0) return parsed
  const fontSize = Number.parseFloat(style.fontSize || '')
  return Number.isFinite(fontSize) && fontSize > 0 ? fontSize * 1.5 : 21
}

const getEditorElementPaddingTop = (element: HTMLElement) => {
  const parsed = Number.parseFloat(window.getComputedStyle(element).paddingTop || '')
  return Number.isFinite(parsed) ? parsed : 0
}

const isEditorTableStructurallyEmptyCell = (cell: HTMLTableCellElement) => {
  const clone = cell.cloneNode(true) as HTMLElement
  clone.querySelectorAll('.editor-attachment-preview').forEach((node) => node.remove())
  clone.querySelectorAll('br').forEach((br) => br.remove())
  return String(clone.textContent || '')
    .replace(TABLE_CELL_CARET_ANCHOR_RE, '')
    .replace(/[\u200b\u200c\ufeff]/g, '')
    .replace(/\u00a0/g, ' ')
    .trim() === ''
}

const getEditorTableCellLines = (cell: HTMLTableCellElement) => {
  const text = editorTableCellTextFromDom(cell)
  return text ? text.split('\n') : ['']
}

const getEditorTableCellVisualLineIndex = (cell: HTMLTableCellElement, clientY: number) => {
  const rect = cell.getBoundingClientRect()
  const lineHeight = getEditorElementLineHeight(cell)
  const top = rect.top + getEditorElementPaddingTop(cell)
  return Math.max(0, Math.floor((clientY - top) / lineHeight))
}

const getEditorElementPaddingLeft = (element: HTMLElement) => {
  const parsed = Number.parseFloat(window.getComputedStyle(element).paddingLeft || '')
  return Number.isFinite(parsed) ? parsed : 0
}

const getEditorElementPaddingBottom = (element: HTMLElement) => {
  const parsed = Number.parseFloat(window.getComputedStyle(element).paddingBottom || '')
  return Number.isFinite(parsed) ? parsed : 0
}

const getEditorTableCellLineStartOffset = (lines: string[], lineIndex: number) => {
  let offset = 0
  for (let index = 0; index < lineIndex; index += 1) offset += (lines[index] || '').length + 1
  return offset
}

const measureEditorTableCellLineColumnOffset = (cell: HTMLTableCellElement, line: string, clientX: number) => {
  if (!line) return 0
  const chars = Array.from(line)
  if (!chars.length) return 0
  const style = window.getComputedStyle(cell)
  const probe = document.createElement('span')
  probe.setAttribute('aria-hidden', 'true')
  probe.style.position = 'fixed'
  probe.style.left = '-10000px'
  probe.style.top = '0'
  probe.style.visibility = 'hidden'
  probe.style.pointerEvents = 'none'
  probe.style.whiteSpace = 'pre'
  probe.style.font = style.font
  probe.style.fontKerning = style.fontKerning
  probe.style.letterSpacing = style.letterSpacing
  document.body.appendChild(probe)
  const widths = [0]
  for (let index = 1; index <= chars.length; index += 1) {
    probe.textContent = chars.slice(0, index).join('')
    widths.push(probe.getBoundingClientRect().width)
  }
  probe.remove()
  const targetX = Math.max(0, clientX - (cell.getBoundingClientRect().left + getEditorElementPaddingLeft(cell)))
  let charOffset = chars.length
  for (let index = 0; index < chars.length; index += 1) {
    const midpoint = (widths[index] + widths[index + 1]) / 2
    if (targetX < midpoint) {
      charOffset = index
      break
    }
  }
  return chars.slice(0, charOffset).join('').length
}

const createEditorTableCaretAnchorNode = () => document.createTextNode(TABLE_CELL_CARET_ANCHOR)

const removeEditorTableCaretAnchors = (root: HTMLElement) => {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      return (node.textContent || '').includes(TABLE_CELL_CARET_ANCHOR) ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT
    }
  })
  const nodes: Text[] = []
  while (walker.nextNode()) nodes.push(walker.currentNode as Text)
  nodes.forEach((node) => {
    const text = node.textContent || ''
    const cleaned = text.replace(TABLE_CELL_CARET_ANCHOR_RE, '')
    if (!cleaned) node.parentNode?.removeChild(node)
    else if (cleaned !== text) node.textContent = cleaned
  })
}

const selectEditorTableRange = (cell: HTMLTableCellElement, range: Range) => {
  const selection = typeof window === 'undefined' ? null : window.getSelection()
  if (!selection) return false
  getEditorEditableFromNode(cell)?.focus({ preventScroll: true })
  range.collapse(true)
  selection.removeAllRanges()
  selection.addRange(range)
  lastEditorSelectionRange = range.cloneRange()
  storeLastEditorTableSelection(range)
  storeLastEditorTableCell(cell)
  return true
}

const getNativeEditorTableCaretRangeFromPoint = (cell: HTMLTableCellElement, clientX: number, clientY: number) => {
  const doc = document as Document & {
    caretRangeFromPoint?: (x: number, y: number) => Range | null
    caretPositionFromPoint?: (x: number, y: number) => { offsetNode: Node; offset: number } | null
  }
  let range = doc.caretRangeFromPoint?.(clientX, clientY) || null
  if (!range && doc.caretPositionFromPoint) {
    const position = doc.caretPositionFromPoint(clientX, clientY)
    if (position) {
      range = document.createRange()
      range.setStart(position.offsetNode, position.offset)
      range.collapse(true)
    }
  }
  if (!range) return null
  const container = range.startContainer
  const element = container instanceof Element ? container : container.parentElement
  return element && cell.contains(element) ? range : null
}

const placeCaretAtEditorTableCellLineStart = (cell: HTMLTableCellElement, lineIndex: number) => {
  const selection = typeof window === 'undefined' ? null : window.getSelection()
  if (!selection) return false
  getEditorEditableFromNode(cell)?.focus({ preventScroll: true })
  const range = document.createRange()
  normalizeEditorTableBreakCodeMarkers(cell)
  if (lineIndex <= 0) {
    const textNode = cell.firstChild && cell.firstChild.nodeType === Node.TEXT_NODE
      ? cell.firstChild
      : cell.insertBefore(document.createTextNode(''), cell.firstChild)
    range.setStart(textNode, 0)
  } else {
    const breaks = Array.from(cell.querySelectorAll('br'))
    const lineBreak = breaks[lineIndex - 1]
    if (!lineBreak) return false
    removeEditorTableCaretAnchors(cell)
    const caretNode = createEditorTableCaretAnchorNode()
    lineBreak.after(caretNode)
    range.setStart(caretNode, 0)
  }
  range.collapse(true)
  selection.removeAllRanges()
  selection.addRange(range)
  lastEditorSelectionRange = range.cloneRange()
  storeLastEditorTableSelection(range)
  return true
}

const placeCaretAtEditorTableCellTextOffset = (cell: HTMLTableCellElement, offset: number) => {
  const selection = typeof window === 'undefined' ? null : window.getSelection()
  if (!selection) return false
  getEditorEditableFromNode(cell)?.focus({ preventScroll: true })
  normalizeEditorTableBreakCodeMarkers(cell)
  const range = document.createRange()
  let remaining = Math.max(0, offset)
  let placed = false
  const invisibleText = /[\u200b\u200c\ufeff]/
  const setRange = (node: Node, nodeOffset: number) => {
    range.setStart(node, nodeOffset)
    range.collapse(true)
    placed = true
  }
  const visit = (node: Node): boolean => {
    if (placed) return true
    if (node.nodeType === Node.TEXT_NODE) {
      const text = node.textContent || ''
      let visibleLength = 0
      for (let index = 0; index < text.length; index += 1) {
        if (visibleLength === remaining) {
          setRange(node, index)
          return true
        }
        if (!invisibleText.test(text[index] || '')) visibleLength += 1
      }
      if (visibleLength === remaining) {
        setRange(node, text.length)
        return true
      }
      remaining -= visibleLength
      return false
    }
    if (node instanceof HTMLBRElement) {
      if (remaining === 0) {
        range.setStartBefore(node)
        range.collapse(true)
        placed = true
        return true
      }
      remaining -= 1
      if (remaining === 0) {
        removeEditorTableCaretAnchors(cell)
        const caretNode = createEditorTableCaretAnchorNode()
        node.after(caretNode)
        setRange(caretNode, 0)
        return true
      }
      return false
    }
    Array.from(node.childNodes).some((child) => visit(child))
    return placed
  }
  visit(cell)
  if (!placed) range.selectNodeContents(cell)
  if (!placed) range.collapse(false)
  selection.removeAllRanges()
  selection.addRange(range)
  lastEditorSelectionRange = range.cloneRange()
  storeLastEditorTableSelection(range)
  return true
}

const placeCaretInEditorTableCellVisualLine = (cell: HTMLTableCellElement, event: MouseEvent) => {
  const lines = getEditorTableCellLines(cell)
  const targetLine = getEditorTableCellVisualLineIndex(cell, event.clientY)
  const existingLineIndex = Math.min(targetLine, Math.max(0, lines.length - 1))
  clearEditorTableEmptyPlaceholder(cell)
  normalizeEditorTableBreakCodeMarkers(cell)
  const lineText = lines[existingLineIndex] || ''
  const lineOffset = getEditorTableCellLineStartOffset(lines, existingLineIndex)
  const nativeRange = targetLine < lines.length && String(lineText).trim()
    ? getNativeEditorTableCaretRangeFromPoint(cell, event.clientX, event.clientY)
    : null
  const placed = nativeRange
    ? selectEditorTableRange(cell, nativeRange)
    : targetLine >= lines.length
      ? placeCaretAtEditorTableCellTextOffset(cell, editorTableCellTextFromDom(cell).length)
      : String(lineText).trim()
        ? placeCaretAtEditorTableCellTextOffset(cell, lineOffset + measureEditorTableCellLineColumnOffset(cell, lineText, event.clientX))
    : placeCaretAtEditorTableCellLineStart(cell, targetLine)
  if (placed) {
    storeLastEditorTableCell(cell)
  }
  return placed
}

const getEditorTableTextBeforeRange = (cell: HTMLTableCellElement, range: Range) => {
  const prefixRange = document.createRange()
  prefixRange.selectNodeContents(cell)
  try {
    prefixRange.setEnd(range.startContainer, range.startOffset)
  } catch {
    return ''
  }
  const holder = document.createElement('div')
  holder.appendChild(prefixRange.cloneContents())
  holder.querySelectorAll('br').forEach((br) => br.replaceWith(document.createTextNode('\n')))
  return stripEditorTableCaretAnchors(holder.textContent || '').replace(/[\u200b\u200c\ufeff]/g, '').replace(/\u00a0/g, ' ')
}

const isNodeBeforeOrAtRangeStart = (node: Node, range: Range) => {
  const nodeRange = document.createRange()
  try {
    nodeRange.selectNode(node)
    return nodeRange.compareBoundaryPoints(Range.END_TO_START, range) <= 0
  } catch {
    return false
  }
}

const removeAdjacentTableCaretAnchors = (node: Node) => {
  const cleanTextNode = (candidate: Node | null) => {
    if (!candidate || candidate.nodeType !== Node.TEXT_NODE) return
    const text = candidate.textContent || ''
    const cleaned = text.replace(TABLE_CELL_CARET_ANCHOR_RE, '')
    if (!cleaned) candidate.parentNode?.removeChild(candidate)
    else if (cleaned !== text) candidate.textContent = cleaned
  }
  cleanTextNode(node.previousSibling)
  cleanTextNode(node.nextSibling)
}

const removePreviousEditorTableLineBreak = (cell: HTMLTableCellElement, range: Range) => {
  const breaks = Array.from(cell.querySelectorAll('br')).filter((br) => isNodeBeforeOrAtRangeStart(br, range))
  const lineBreak = breaks[breaks.length - 1]
  if (!lineBreak?.parentNode) return false
  const caretNode = document.createTextNode('')
  lineBreak.parentNode.replaceChild(caretNode, lineBreak)
  removeAdjacentTableCaretAnchors(caretNode)
  const selection = window.getSelection()
  if (!selection) return true
  const nextRange = document.createRange()
  nextRange.setStart(caretNode, 0)
  nextRange.collapse(true)
  selection.removeAllRanges()
  selection.addRange(nextRange)
  lastEditorSelectionRange = nextRange.cloneRange()
  storeLastEditorTableSelection(nextRange)
  return true
}

const handleEditorTableBackspaceAtLineBoundary = (event: KeyboardEvent, cell: HTMLTableCellElement) => {
  const selection = typeof window === 'undefined' ? null : window.getSelection()
  if (!selection?.rangeCount || !selection.isCollapsed) return false
  const range = selection.getRangeAt(0)
  if (getEditorTableCellFromRange(range) !== cell) return false
  const prefix = getEditorTableTextBeforeRange(cell, range)
  if (!prefix) {
    event.preventDefault()
    event.stopPropagation()
    ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
    placeCaretAtStartOfEditorTableCell(cell)
    return true
  }
  if (!prefix.endsWith('\n')) return false
  event.preventDefault()
  event.stopPropagation()
  ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
  return removePreviousEditorTableLineBreak(cell, range)
}

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

const stripEditorTableCaretAnchors = (value: string) => String(value || '').replace(TABLE_CELL_CARET_ANCHOR_RE, '')
const stripTableBreakCode = (value: string) => stripEditorTableCaretAnchors(value).replace(TABLE_CELL_BREAK_SOURCE_RE, ' ')
const decodeMarkdownTablePipeEntities = (value: string) => String(value || '').replace(/&#(?:124|x7c);|&vert;/gi, '|')
const stripOuterTableCellHorizontalPadding = (value: string) => String(value || '').replace(/^[ \t]+|[ \t]+$/g, '')
const normalizeEditorTableCellTextEdges = (value: string) => {
  const text = stripOuterTableCellHorizontalPadding(
    normalizeAttachmentSourceText(stripEditorTableCaretAnchors(value).replace(/\r\n?/g, '\n'))
      .replace(/[\u200b\u200c\ufeff]/g, '')
      .replace(/\u00a0/g, ' ')
  )
  if (MARKDOWN_EMPTY_TABLE_CELL_RE.test(text.trim())) return ''
  if (!text.includes('\n') && text.trim() === '') return ''
  return text
}
const tableCellSourceToEditorText = (value: string) => {
  const text = decodeMarkdownTablePipeEntities(String(value || '').replace(TABLE_CELL_BREAK_SOURCE_RE, '\n').replace(/\\\|/g, '|'))
  return normalizeEditorTableCellTextEdges(text)
}
const escapeTableCellHtml = (value: string) => String(value || '')
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/\"/g, '&quot;')
  .replace(/'/g, '&#39;')

const escapeHtmlAttribute = (value: string) => escapeTableCellHtml(value).replace(/`/g, '&#96;')

const editorTextLineToHtmlTableCellSource = (value: string) => {
  const source = normalizeAttachmentSourceText(value).trim()
  if (!source) return ''
  let output = ''
  let lastIndex = 0
  ATTACHMENT_MARKER_GLOBAL_RE.lastIndex = 0
  let match: RegExpExecArray | null
  while ((match = ATTACHMENT_MARKER_GLOBAL_RE.exec(source))) {
    output += escapeTableCellHtml(source.slice(lastIndex, match.index))
    const info = normalizeAttachmentInfo(match[1], match[2], match[3])
    output += info
      ? `<a href="${escapeHtmlAttribute(info.url)}">${escapeTableCellHtml(info.title)}</a>`
      : escapeTableCellHtml(match[0])
    lastIndex = ATTACHMENT_MARKER_GLOBAL_RE.lastIndex
  }
  output += escapeTableCellHtml(source.slice(lastIndex))
  return output
}

const editorTextToHtmlTableCellSource = (value: string) => {
  const text = stripEditorTableCaretAnchors(value).replace(/\r\n?/g, '\n')
  const normalized = text
    .split('\n')
    .map((line) => editorTextLineToHtmlTableCellSource(line))
    .join('<br />')
    .trim()
  return normalized || '&nbsp;'
}

const editorTextToMarkdownTableCellSource = (value: string) => {
  const text = normalizeAttachmentSourceText(stripEditorTableCaretAnchors(value).replace(/\r\n?/g, '\n'))
  const normalized = text
    .split('\n')
    .map((line) => normalizeAttachmentSourceText(line).replace(/\|/g, () => '&#124;').trim())
    .join('<br />')
    .trim()
  return normalized || MARKDOWN_EMPTY_TABLE_CELL
}

const editorTextToTabTableCellSource = (value: string) => {
  const text = normalizeAttachmentSourceText(stripEditorTableCaretAnchors(value).replace(/\r\n?/g, '\n'))
  const normalized = text
    .split('\n')
    .map((line) => normalizeAttachmentSourceText(line).replace(/\t/g, ' ').trim())
    .join('<br />')
    .trim()
  return normalized || ' '
}

const htmlTableCellToEditorText = (cell: HTMLTableCellElement) => {
  const clone = cell.cloneNode(true) as HTMLElement
  clone.querySelectorAll('.editor-attachment-preview').forEach((node) => node.remove())
  replaceAttachmentNodesWithSourceText(clone)
  const startsWithBreak = /^\s*<br\s*\/?\s*>/i.test(clone.innerHTML || '')
  const endsWithBreak = /<br\s*\/?\s*>\s*$/i.test(clone.innerHTML || '')
  clone.querySelectorAll('br').forEach((br) => br.replaceWith(document.createTextNode('\n')))
  clone.querySelectorAll('p,div').forEach((block) => {
    if (block.nextSibling) block.after(document.createTextNode('\n'))
  })
  let text = normalizeEditorTableCellTextEdges(clone.textContent || '')
  if (text && startsWithBreak && !text.startsWith('\n')) text = `\n${text}`
  if (text && endsWithBreak && !text.endsWith('\n')) text = `${text}\n`
  return text
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

const removeMarkdownTableDividerRow = (table: HTMLTableElement, block: EditorTableSourceBlock | null) => {
  if (block?.kind !== 'markdown') return
  const expectedRows = editableRowsFromMarkdownBlock(block).length
  const dividerRow = table.rows[1]
  if (!dividerRow || table.rows.length !== expectedRows + 1) return
  dividerRow.remove()
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
  const source = normalizeAttachmentSourceText(text)
  ATTACHMENT_MARKER_GLOBAL_RE.lastIndex = 0
  let match: RegExpExecArray | null
  while ((match = ATTACHMENT_MARKER_GLOBAL_RE.exec(source))) {
    const info = normalizeAttachmentInfo(match[1], match[2], match[3])
    if (info) items.push(info)
  }
  return items
}

const expandedTableCellEditorText = (rowIndex: number, cellIndex: number) => stripAttachmentMarkersFromEditorText(expandedTableRows.value[rowIndex]?.[cellIndex] || '')
const expandedTableBaseColumnWidths = computed(() => calculateAdaptiveTableColumnWidths(expandedTableRows.value, expandedTableAvailableWidth.value))
const expandedTableColumnWidths = computed(() => expandedTableBaseColumnWidths.value.map((width, index) => Math.max(
  EXPANDED_TABLE_MIN_COLUMN_WIDTH,
  Math.ceil(expandedTableManualColumnWidths.value[index] || width)
)))
const expandedTableRowHeights = computed(() => expandedTableRows.value.map((_, index) => Math.max(
  EXPANDED_TABLE_MIN_ROW_HEIGHT,
  Math.ceil(expandedTableAutoRowHeights.value[index] || 0),
  Math.ceil(expandedTableManualRowHeights.value[index] || 0)
)))
const expandedTableRowHeight = (rowIndex: number) => expandedTableRowHeights.value[rowIndex] || EXPANDED_TABLE_MIN_ROW_HEIGHT

const measureExpandedTableTextareaContentHeight = (textarea: HTMLTextAreaElement) => {
  if (typeof document === 'undefined') return EXPANDED_TABLE_MIN_ROW_HEIGHT
  const rect = textarea.getBoundingClientRect()
  const styles = window.getComputedStyle(textarea)
  const probe = document.createElement('textarea')
  probe.value = textarea.value
  probe.rows = 1
  probe.setAttribute('aria-hidden', 'true')
  probe.style.position = 'fixed'
  probe.style.left = '-10000px'
  probe.style.top = '0'
  probe.style.width = `${Math.max(1, rect.width)}px`
  probe.style.height = '0'
  probe.style.minHeight = '0'
  probe.style.maxHeight = 'none'
  probe.style.padding = styles.padding
  probe.style.border = '0'
  probe.style.boxSizing = styles.boxSizing
  probe.style.font = styles.font
  probe.style.lineHeight = styles.lineHeight
  probe.style.letterSpacing = styles.letterSpacing
  probe.style.whiteSpace = styles.whiteSpace
  probe.style.overflowWrap = styles.overflowWrap
  probe.style.wordBreak = styles.wordBreak
  probe.style.visibility = 'hidden'
  probe.style.overflow = 'hidden'
  probe.style.resize = 'none'
  probe.style.pointerEvents = 'none'
  document.body.appendChild(probe)
  const height = Math.ceil(probe.scrollHeight)
  probe.remove()
  return Math.max(EXPANDED_TABLE_MIN_ROW_HEIGHT, height)
}

const measureExpandedTableAutoRowHeights = () => {
  if (typeof document === 'undefined' || !showTableExpandDialog.value) return
  const rows = Array.from(document.querySelectorAll<HTMLTableRowElement>('.editor-table-expand-table tbody tr'))
  const heights = rows.map((row) => {
    const cells = Array.from(row.cells)
    const maxCellHeight = cells.reduce((max, cell) => {
      const textarea = cell.querySelector<HTMLTextAreaElement>('textarea')
      const attachments = cell.querySelector<HTMLElement>('.editor-table-expand-attachments')
      const textareaHeight = textarea ? measureExpandedTableTextareaContentHeight(textarea) : EXPANDED_TABLE_MIN_ROW_HEIGHT
      const attachmentHeight = attachments ? Math.ceil(attachments.offsetHeight) : 0
      return Math.max(max, textareaHeight + attachmentHeight)
    }, EXPANDED_TABLE_MIN_ROW_HEIGHT)
    return Math.max(EXPANDED_TABLE_MIN_ROW_HEIGHT, maxCellHeight)
  })
  expandedTableAutoRowHeights.value = heights
  nextTick(() => scheduleExpandedTableScrollOverflowState())
}

const scheduleMeasureExpandedTableAutoRowHeights = () => {
  if (typeof window === 'undefined') return
  if (expandedTableRowHeightMeasureTimer !== null) window.cancelAnimationFrame(expandedTableRowHeightMeasureTimer)
  expandedTableRowHeightMeasureTimer = window.requestAnimationFrame(() => {
    expandedTableRowHeightMeasureTimer = null
    measureExpandedTableAutoRowHeights()
  })
}

const updateExpandedTableAvailableWidth = () => {
  if (typeof window === 'undefined') return
  const scroll = document.querySelector<HTMLElement>('.editor-table-expand-scroll')
  const fallback = Math.min(1680, Math.max(320, window.innerWidth - 48)) - 24
  expandedTableAvailableWidth.value = Math.max(160, Math.floor((scroll?.clientWidth || fallback) - 24))
  scheduleMeasureExpandedTableAutoRowHeights()
  nextTick(() => scheduleExpandedTableScrollOverflowState())
}

const updateExpandedTableCellText = (rowIndex: number, cellIndex: number, event: Event) => {
  if (!expandedTableEditable.value) return
  const textarea = event.target instanceof HTMLTextAreaElement ? event.target : null
  if (!textarea) return
  const row = expandedTableRows.value[rowIndex]
  if (!row) return
  row[cellIndex] = mergeExpandedCellEditorText(row[cellIndex] || '', textarea.value)
  expandedTableDirty.value = true
  nextTick(() => scheduleMeasureExpandedTableAutoRowHeights())
}

const expandedTableCellAttachments = (rowIndex: number, cellIndex: number) => parseAttachmentMarkersFromText(expandedTableRows.value[rowIndex]?.[cellIndex] || '')

const expandedTableAttachmentsByType = (type: EditorAttachmentInfo['type']) => {
  const seen = new Set<string>()
  const items: EditorAttachmentInfo[] = []
  expandedTableRows.value.forEach((row) => {
    row.forEach((cell) => {
      parseAttachmentMarkersFromText(cell).forEach((item) => {
        if (item.type !== type) return
        const key = `${item.type}\n${item.url}\n${item.name}`
        if (seen.has(key)) return
        seen.add(key)
        items.push(item)
      })
    })
  })
  return items
}

const previewExpandedTableAttachment = (attachment: EditorAttachmentInfo, event: MouseEvent | KeyboardEvent) => {
  const target = event.currentTarget instanceof HTMLElement ? event.currentTarget : null
  if (attachment.type === 'audio') {
    const url = attachment.url
    window.open(url, '_blank', 'noopener,noreferrer')
    return
  }
  if (attachment.type === 'file') {
    openFileAttachment(attachment)
    return
  }
  showAttachmentGallery(expandedTableAttachmentsByType(attachment.type), attachment, target)
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
  const row = expandedTableRows.value[rowIndex]
  if (!row) return
  row[cellIndex] = mergeExpandedCellEditorText(row[cellIndex] || '', textarea.value)
  expandedTableDirty.value = true
  nextTick(() => scheduleMeasureExpandedTableAutoRowHeights())
}

const isMarkdownTableDivider = (line: string) => {
  const cells = String(line || '').trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((cell) => cell.trim())
  return cells.length > 1 && cells.every((cell) => /^:?-{3,}:?$/.test(cell))
}

type EditorTableSourceBlock = { start: number; end: number; lines: string[]; kind: 'markdown' | 'html' | 'tab' }

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

const getTabTableBlocks = (content: string): EditorTableSourceBlock[] => {
  const lines = String(content || '').split('\n')
  const blocks: EditorTableSourceBlock[] = []
  for (let index = 0; index < lines.length; index += 1) {
    if (!lines[index].includes('\t')) continue
    let end = index + 1
    while (end < lines.length && lines[end].includes('\t')) end += 1
    const blockLines = lines.slice(index, end)
    const maxColumns = Math.max(0, ...blockLines.map((line) => line.split('\t').length))
    if (blockLines.length >= 2 && maxColumns > 1) {
      blocks.push({ start: index, end, lines: blockLines, kind: 'tab' })
      index = end - 1
    }
  }
  return blocks
}

const getEditorTableSourceBlocks = (content: string) => [
  ...getMarkdownTableBlocks(content),
  ...getHtmlTableBlocks(content),
  ...getTabTableBlocks(content)
].sort((left, right) => left.start - right.start)

const splitMarkdownTableRowCells = (line: string) => String(line || '')
  .trim()
  .replace(/^\|/, '')
  .replace(/\|$/, '')
  .split('|')

const markdownTableRowCellCount = (line: string) => splitMarkdownTableRowCells(line).length

const collapseMarkdownTableRowCells = (cells: string[], expected: number) => {
  if (expected <= 0) return cells
  if (cells.length < expected) return [...cells, ...Array.from({ length: expected - cells.length }, () => '')]
  if (cells.length === expected) return cells
  const overflow = cells.length - expected
  return [cells.slice(0, overflow + 1).join('|'), ...cells.slice(overflow + 1)]
}

const editableCellsFromPossiblyBrokenMarkdownTableRow = (line: string, expected: number) =>
  collapseMarkdownTableRowCells(splitMarkdownTableRowCells(line), expected).map((cell) => tableCellSourceToEditorText(cell))

const normalizeMarkdownTableEmptyCellEntities = (content: string) => {
  const lines = String(content || '').split('\n')
  const blocks = getMarkdownTableBlocks(content)
  if (!blocks.length) return content || ''
  const replacements = blocks
    .map((block) => {
      const rows = editableRowsFromMarkdownBlock(block)
      if (!rows.length) return null
      const nextLines = serializeEditableMarkdownTableBlock(block, rows)
      return { start: block.start, end: block.end, lines: nextLines }
    })
    .filter((replacement): replacement is { start: number; end: number; lines: string[] } => !!replacement)
    .sort((left, right) => right.start - left.start)
  replacements.forEach((replacement) => {
    lines.splice(replacement.start, replacement.end - replacement.start, ...replacement.lines)
  })
  return lines.join('\n')
}

const looksLikeMarkdownTableRowFragment = (line: string) => {
  const trimmed = String(line || '').trim()
  if (!trimmed || isMarkdownTableDivider(trimmed)) return false
  if (trimmed.startsWith('|') || trimmed.endsWith('|')) return trimmed.includes('|')
  return (trimmed.match(/\|/g) || []).length >= 2
}

const looksLikeCompleteMarkdownTableRow = (line: string, expected: number) => {
  const trimmed = String(line || '').trim()
  if (!trimmed || !trimmed.startsWith('|') || isMarkdownTableDivider(trimmed)) return false
  return markdownTableRowCellCount(trimmed) >= expected
}

const hasUnsafeMarkdownTableStructure = (content: string) => {
  const lines = String(content || '').split('\n')
  const blocks = getMarkdownTableBlocks(content)
  const covered = new Set<number>()
  blocks.forEach((block) => {
    for (let index = block.start; index < block.end; index += 1) covered.add(index)
  })
  if (blocks.length && lines.some((line, index) => !covered.has(index) && looksLikeMarkdownTableRowFragment(line))) return true
  return blocks.some((block) => {
    const expected = markdownTableRowCellCount(block.lines[0] || '')
    if (expected <= 1) return false
    return block.lines.some((line, index) => {
      if (index === 1 && isMarkdownTableDivider(line)) return false
      return markdownTableRowCellCount(line) !== expected
    })
  })
}

const repairUnsafeMarkdownTableCellBreaks = (content: string) => {
  const lines = String(content || '').split('\n')
  const output: string[] = []
  for (let index = 0; index < lines.length; index += 1) {
    const header = lines[index] || ''
    const divider = lines[index + 1] || ''
    if (!header.includes('|') || !isMarkdownTableDivider(divider)) {
      output.push(header)
      continue
    }
    const expected = markdownTableRowCellCount(header)
    const safeHeader = formatEditableMarkdownTableRow(editableCellsFromPossiblyBrokenMarkdownTableRow(header, expected))
    output.push(safeHeader, formatMarkdownDividerLine('', expected))
    index += 2
    while (index < lines.length) {
      const current = lines[index] || ''
      if (!current.trim()) {
        output.push(current)
        break
      }
      if (!looksLikeMarkdownTableRowFragment(current) && !current.includes('|')) {
        index -= 1
        break
      }
      let merged = current
      while (markdownTableRowCellCount(merged) < expected && index + 1 < lines.length) {
        const next = lines[index + 1] || ''
        if (!next.trim() || isMarkdownTableDivider(next) || looksLikeCompleteMarkdownTableRow(next, expected)) break
        merged = `${merged}<br />${next.trim()}`
        index += 1
      }
      output.push(formatEditableMarkdownTableRow(editableCellsFromPossiblyBrokenMarkdownTableRow(merged, expected)))
      index += 1
    }
  }
  return output.join('\n')
}

const ensureSafeEditorTableMarkdown = (content: string) => normalizeMarkdownTableEmptyCellEntities(repairUnsafeMarkdownTableCellBreaks(content))

const normalizeTableMatchText = (text: string) => {
  ATTACHMENT_MARKER_GLOBAL_RE.lastIndex = 0
  const source = String(stripTableBreakCode(normalizeAttachmentSourceText(text) || ''))
    .replace(/\u00a0/g, ' ')
    .trim()
  if (MARKDOWN_EMPTY_TABLE_CELL_RE.test(source)) return ''
  return source
    .replace(ATTACHMENT_MARKER_GLOBAL_RE, '$1：$2')
    .replace(/\s+/g, ' ')
    .trim()
}

const getRenderedTableRows = (table: HTMLTableElement | null) => {
  if (!table) return [] as string[][]
  return Array.from(table.rows).map((row) => Array.from(row.cells).map((cell) => normalizeTableMatchText(cell.textContent || '')))
}

const editableRowsFromRenderedTable = (table: HTMLTableElement | null) => {
  if (!table) return [] as string[][]
  return Array.from(table.rows).map((row) => Array.from(row.cells).map((cell) => htmlTableCellToEditorText(cell as HTMLTableCellElement)))
}

const countEdgeLineBreaks = (value: string, edge: 'start' | 'end') => {
  const match = edge === 'start' ? String(value || '').match(/^\n+/) : String(value || '').match(/\n+$/)
  return match?.[0].length || 0
}

const mergeRenderedTableCellEdgeBreaks = (sourceText: string, renderedText: string) => {
  const source = String(sourceText || '')
  const rendered = String(renderedText || '')
  if (!rendered) return source
  const sourceCore = source.replace(/^\n+|\n+$/g, '')
  const renderedCore = rendered.replace(/^\n+|\n+$/g, '')
  if (normalizeTableMatchText(sourceCore) !== normalizeTableMatchText(renderedCore)) return source
  const leading = Math.max(countEdgeLineBreaks(source, 'start'), countEdgeLineBreaks(rendered, 'start'))
  const trailing = Math.max(countEdgeLineBreaks(source, 'end'), countEdgeLineBreaks(rendered, 'end'))
  return `${'\n'.repeat(leading)}${sourceCore}${'\n'.repeat(trailing)}`
}

const mergeRenderedTableEdgeBreaks = (sourceRows: string[][], renderedRows: string[][]) =>
  sourceRows.map((row, rowIndex) => row.map((cell, cellIndex) => mergeRenderedTableCellEdgeBreaks(cell, renderedRows[rowIndex]?.[cellIndex] || '')))

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
  .map((cell) => normalizeTableMatchText(decodeMarkdownTablePipeEntities(cell.replace(/\\\|/g, '|'))))

const parseEditableMarkdownTableRow = (line: string) => String(line || '')
  .trim()
  .replace(/^\|/, '')
  .replace(/\|$/, '')
  .split('|')
  .map((cell) => tableCellSourceToEditorText(cell))

const parseEditableTabTableRow = (line: string) => String(line || '')
  .split('\t')
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

const editableRowsFromTabBlock = (block: EditorTableSourceBlock) => block.kind === 'tab'
  ? block.lines.map((line) => parseEditableTabTableRow(line))
  : [] as string[][]

const comparableRowsFromTableBlock = (block: EditorTableSourceBlock) => {
  const rows = block.kind === 'markdown'
    ? getMarkdownTableRows(block.lines)
    : (block.kind === 'tab' ? editableRowsFromTabBlock(block) : editableRowsFromHtmlBlock(block))
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
  if (preferredIndex >= 0 && preferredIndex < blocks.length) return blocks[preferredIndex]
  return blocks.length === 1 ? blocks[0] : undefined
}

const tableBlockFromDataset = (table: HTMLTableElement | null, blocks: EditorTableSourceBlock[]) => {
  if (!table) return undefined
  const start = Number(table.dataset.editorTableBlockStart)
  const end = Number(table.dataset.editorTableBlockEnd)
  const exactBlock = Number.isFinite(start) && Number.isFinite(end)
    ? blocks.find((block) => block.start === start && block.end === end)
    : undefined
  if (exactBlock) return exactBlock
  const sourceIndex = Number(table.dataset.editorTableSourceIndex)
  if (Number.isFinite(sourceIndex) && sourceIndex >= 0 && sourceIndex < blocks.length) return blocks[sourceIndex]
  return undefined
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
  const tableIndex = preferredIndex >= 0 ? preferredIndex : (table ? getEditorTables().indexOf(table) : -1)
  return tableBlockFromDataset(table, blocks) || findMarkdownTableBlock(blocks, getRenderedTableRows(table), tableIndex)
}

const getEditorTableCellSourceTarget = (cell: HTMLTableCellElement | null): EditorTableCellSourceTarget | null => {
  if (!cell || !vditorInstance) return null
  const table = cell.closest('table') as HTMLTableElement | null
  const row = cell.parentElement as HTMLTableRowElement | null
  if (!table || !row || !isInsideEditorRoot(table)) return null
  const rowIndex = row.rowIndex
  const cellIndex = cell.cellIndex
  if (rowIndex < 0 || cellIndex < 0) return null
  const value = vditorInstance.getValue?.() || ''
  const lines = value.split('\n')
  const tableIndex = getEditorTables().indexOf(table)
  const block = getEditorTableBlockForTable(table, tableIndex)
  if (!block) return null
  if (block.kind === 'html') {
    const sourceRows = editableRowsFromHtmlBlock(block)
    const rowCells = sourceRows[rowIndex]
    if (!rowCells) return null
    return { table, block, lines, lineIndex: -1, rowIndex, cellIndex, rowCells }
  }
  if (block.kind === 'tab') {
    const lineIndex = block.start + rowIndex
    if (lineIndex < block.start || lineIndex >= block.end || !lines[lineIndex]) return null
    const rowCells = parseEditableTabTableRow(lines[lineIndex])
    return { table, block, lines, lineIndex, rowIndex, cellIndex, rowCells }
  }
  if (!isMarkdownTableDivider(lines[block.start + 1] || '')) return null
  const lineIndex = rowIndex === 0 ? block.start : block.start + rowIndex + 1
  if (lineIndex < block.start || lineIndex >= block.end || !lines[lineIndex]) return null
  const rowCells = parseEditableMarkdownTableRow(lines[lineIndex])
  return { table, block, lines, lineIndex, rowIndex, cellIndex, rowCells }
}

const serializeEditableRowsToHtmlTableBlock = (rows: string[][], sourceTable?: HTMLTableElement | null) => {
  if (typeof document === 'undefined') return null
  const table = sourceTable ? sourceTable.cloneNode(false) as HTMLTableElement : document.createElement('table')
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

const serializeEditableHtmlTableBlock = (block: EditorTableSourceBlock, rows: string[][]) => {
  const table = createHtmlTableFromBlock(block)
  return serializeEditableRowsToHtmlTableBlock(rows, table)
}

const formatEditableMarkdownTableRow = (cells: string[]) => `| ${cells.map(editorTextToMarkdownTableCellSource).join(' | ')} |`
const formatEditableTabTableRow = (cells: string[]) => cells.map(editorTextToTabTableCellSource).join('\t')

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

const serializeEditableMarkdownTableBlock = (block: EditorTableSourceBlock, rows: string[][]) => {
  const normalizedRows = normalizeExpandedTableRows(rows)
  if (!normalizedRows.length) return [] as string[]
  const colCount = normalizedRows[0]?.length || 1
  return [
    formatEditableMarkdownTableRow(normalizedRows[0] || []),
    formatMarkdownDividerLine(block.lines[1] || '', colCount),
    ...normalizedRows.slice(1).map(formatEditableMarkdownTableRow)
  ]
}

const serializeEditableTabTableBlock = (rows: string[][]) => normalizeExpandedTableRows(rows).map(formatEditableTabTableRow)

const serializeEditableTableBlock = (block: EditorTableSourceBlock, rows: string[][]) => {
  if (block.kind === 'html') return serializeEditableHtmlTableBlock(block, rows)
  if (block.kind === 'tab') return serializeEditableTabTableBlock(rows)
  return serializeEditableMarkdownTableBlock(block, rows)
}

const normalizeEditorTableSource = () => false

const scheduleNormalizeEditorTableSource = () => {
  window.setTimeout(() => {
    if (editorContainer.value) enhanceEditorTables(editorContainer.value)
    refreshAttachmentLinksFromEditor()
  }, 0)
}

const buildEditorTableCellSourceValue = (cell: HTMLTableCellElement | null, nextText: string) => {
  const target = getEditorTableCellSourceTarget(cell)
  if (!target) return null
  const rowCells = [...target.rowCells]
  while (rowCells.length <= target.cellIndex) rowCells.push('')
  rowCells[target.cellIndex] = nextText
  const rows = editableRowsFromTableBlock(target.block)
  if (!rows[target.rowIndex]) rows[target.rowIndex] = []
  rows[target.rowIndex] = rowCells
  const nextBlockLines = serializeEditableTableBlock(target.block, rows)
  if (!nextBlockLines) return null
  target.lines.splice(target.block.start, target.block.end - target.block.start, ...nextBlockLines)
  return { table: target.table, value: target.lines.join('\n') }
}

const getEditorTableCellAtPosition = (position: Pick<PendingEditorTableCellSync, 'tableIndex' | 'rowIndex' | 'cellIndex'> | null) => {
  if (!position) return null as HTMLTableCellElement | null
  const table = getEditorTables()[position.tableIndex]
  return (table?.rows[position.rowIndex]?.cells[position.cellIndex] as HTMLTableCellElement | undefined) || null
}

const applyEditorTableCellSourceValue = (
  cell: HTMLTableCellElement | null,
  nextText: string,
  options: { restoreCaret?: boolean } = {}
) => {
  if (!vditorInstance) return false
  const position = options.restoreCaret ? getEditorTableCellPosition(cell) : null
  const result = buildEditorTableCellSourceValue(cell, nextText)
  if (!result) return false
  rememberEditorTableScroll(result.table)
  vditorInstance.setValue(result.value)
  if (position) {
    const restoreAppliedCell = () => {
      const nextCell = getEditorTableCellAtPosition(position)
      if (!nextCell) return
      normalizeEditorTableBreakCodeMarkers(nextCell)
      placeCaretAtEndOfEditorTableCell(nextCell)
    }
    restoreAppliedCell()
    window.requestAnimationFrame(restoreAppliedCell)
    window.setTimeout(restoreAppliedCell, 0)
  }
  emitEditorValue(result.value)
  window.setTimeout(() => refreshAttachmentLinksFromEditor(), 0)
  return true
}

const editorTableContentTextFromElement = (element: HTMLElement) => {
  const clone = element.cloneNode(true) as HTMLElement
  clone.querySelectorAll('.editor-attachment-preview').forEach((node) => node.remove())
  clone.querySelectorAll<HTMLElement>('[data-type="html-inline"], .vditor-ir__node').forEach((node) => {
    if (isEditorTableBreakCodeMarker(node)) node.replaceWith(document.createTextNode('\n'))
  })
  replaceAttachmentNodesWithSourceText(clone)
  clone.querySelectorAll('br').forEach((br) => br.replaceWith(document.createTextNode('\n')))
  return normalizeAttachmentSourceText(stripEditorTableCaretAnchors(clone.textContent || '').replace(/[\u200b\u200c\ufeff]/g, '').replace(/\u00a0/g, ' '))
}

const editorTableCellTextFromDom = (cell: HTMLTableCellElement) => editorTableContentTextFromElement(cell)

const isEditorTableBreakCodeMarker = (node: HTMLElement) => {
  const text = stripEditorTableCaretAnchors(node.textContent || '').trim()
  if (TABLE_CELL_BREAK_TEXT_RE.test(text)) return true
  const html = stripEditorTableCaretAnchors(node.innerHTML || '').trim()
  return /^<br\s*\/?\s*>$/i.test(html) || /^<code\b[^>]*>\s*<br\s*\/?\s*>\s*<\/code>$/i.test(html)
}

const normalizeEditorTableBreakCodeMarkers = (cell: HTMLTableCellElement) => {
  const replacements: HTMLElement[] = []
  cell.querySelectorAll<HTMLElement>('[data-type="html-inline"], .vditor-ir__node').forEach((node) => {
    if (isEditorTableBreakCodeMarker(node)) replacements.push(node)
  })
  replacements.forEach((node) => {
    node.replaceWith(document.createElement('br'))
  })
}

const hasEditorTableBreakCodeMarker = (cell: HTMLTableCellElement) => Array.from(cell.querySelectorAll<HTMLElement>('[data-type="html-inline"], .vditor-ir__node'))
  .some((node) => isEditorTableBreakCodeMarker(node))

const serializeEditorTableDomAsMarkdown = (table: HTMLTableElement) => {
  const rows = Array.from(table.rows).map((row) => Array.from(row.cells).map((cell) => editorTableCellTextFromDom(cell as HTMLTableCellElement)))
  if (!rows.length) return ''
  const colCount = Math.max(1, ...rows.map((row) => row.length))
  const normalizedRows = rows.map((row) => Array.from({ length: colCount }, (_, index) => row[index] ?? ''))
  const header = normalizedRows[0] || []
  const body = normalizedRows.slice(1)
  return [
    formatEditableMarkdownTableRow(header),
    formatMarkdownDividerLine('', colCount),
    ...body.map(formatEditableMarkdownTableRow)
  ].join('\n')
}

const serializePlainEditorBlockText = (block: Element) => {
  const clone = block.cloneNode(true) as HTMLElement
  clone.querySelectorAll('.editor-table-delete-button, .editor-table-expand-button, .editor-attachment-preview').forEach((node) => node.remove())
  clone.querySelectorAll('br').forEach((br) => br.replaceWith(document.createTextNode('\n')))
  const rawText = String(clone.textContent || '').replace(/[\u200b\u200c\ufeff]/g, '')
  if (block.classList.contains('vditor-preserved-blank-line') || (rawText.includes(MARKDOWN_BLANK_LINE_SENTINEL) && isMarkdownBlankLineSentinel(rawText))) return MARKDOWN_BLANK_LINE_SENTINEL
  return normalizeAttachmentSourceText(rawText.replace(/\u00a0/g, ' ')).replace(/[ \t]+$/g, '')
}

const serializePlainEditorLinesAsMarkdown = (lines: string[]) => {
  let output = ''
  const appendText = (line: string) => {
    if (!output) output = line
    else output += `${output.endsWith('\n\n') ? '' : '\n'}${line}`
  }
  const appendPreservedBlankLine = () => {
    output = output.replace(/\n+$/g, '')
    if (output) output += '\n\n'
    output += `${MARKDOWN_BLANK_LINE_SENTINEL}\n\n`
  }
  lines.forEach((line) => {
    if (line === MARKDOWN_BLANK_LINE_SENTINEL || line === '') appendPreservedBlankLine()
    else appendText(line)
  })
  return output.replace(/^\n+|\n+$/g, '')
}

const serializePlainEditorDomAsMarkdown = (editable: HTMLElement) => {
  const pieces: string[] = []
  Array.from(editable.childNodes).forEach((node) => {
    if (node instanceof Element && node.matches('p[data-block], div[data-block]') && !node.closest('table')) {
      pieces.push(serializePlainEditorBlockText(node))
      return
    }
    const text = String(node.textContent || '').replace(/[\u200b\u200c\ufeff]/g, '').replace(/\u00a0/g, ' ')
    if (text) pieces.push(normalizeAttachmentSourceText(text))
  })
  return serializePlainEditorLinesAsMarkdown(pieces)
}

const serializeEditorDomAsMarkdown = (editable: HTMLElement) => {
  const segments: string[] = []
  let plainLines: string[] = []
  const flushPlainLines = () => {
    if (!plainLines.length) return
    const text = serializePlainEditorLinesAsMarkdown(plainLines)
    if (text) segments.push(text)
    plainLines = []
  }
  Array.from(editable.childNodes).forEach((node) => {
    if (node instanceof HTMLTableElement) {
      flushPlainLines()
      const markdown = serializeEditorTableDomAsMarkdown(node)
      if (markdown) segments.push(markdown)
      return
    }
    if (node instanceof Element && node.matches('p[data-block], div[data-block]') && !node.closest('table')) {
      plainLines.push(serializePlainEditorBlockText(node))
      return
    }
    if (node instanceof Element && node.querySelector('table')) {
      flushPlainLines()
      const clone = node.cloneNode(true) as HTMLElement
      clone.querySelectorAll('.editor-table-delete-button, .editor-table-expand-button, .editor-attachment-preview').forEach((control) => control.remove())
      clone.querySelectorAll('table').forEach((table) => {
        const markdown = serializeEditorTableDomAsMarkdown(table as HTMLTableElement)
        table.replaceWith(document.createTextNode(markdown ? `\n${markdown}\n` : ''))
      })
      const text = normalizeAttachmentSourceText(String(clone.textContent || '').replace(/[\u200b\u200c\ufeff]/g, '').replace(/\u00a0/g, ' ')).trim()
      if (text) segments.push(text)
      return
    }
    const text = String(node.textContent || '').replace(/[\u200b\u200c\ufeff]/g, '').replace(/\u00a0/g, ' ')
    if (text) plainLines.push(normalizeAttachmentSourceText(text))
  })
  flushPlainLines()
  return encodeMarkdownExtraBlankLines(segments.join('\n\n')).replace(/^\n+|\n+$/g, '')
}

const getEditorDomContentFallback = () => {
  if (typeof document === 'undefined') return ''
  const editable = getEditorEditableElement()
  if (!editable) return ''
  return editable.querySelector('table') ? serializeEditorDomAsMarkdown(editable) : serializePlainEditorDomAsMarkdown(editable)
}

const hasEditorSoftBreakDom = () => {
  const editable = getEditorEditableElement()
  return !!editable?.querySelector('br, .vditor-preserved-blank-line')
}

const hasEditorPlainBlockDom = () => {
  const editable = getEditorEditableElement()
  if (!editable) return false
  const plainBlocks = Array.from(editable.children).filter((node) => node instanceof Element && node.matches('p[data-block], div[data-block]') && !node.closest('table'))
  return plainBlocks.length > 1
}

const getEditorVisibleDomTableSafeValue = () => {
  const fallbackValue = getEditorTables().length ? getEditorDomContentFallback() : ''
  return fallbackValue || getEditorValueWithPendingTableSync()
}

const getSafeOutgoingEditorValue = (sourceValue?: string) => {
  const source = typeof sourceValue === 'string' ? sourceValue : (vditorInstance?.getValue?.() || '')
  const needsDomFallback = getEditorTables().length || hasEditorSoftBreakDom() || hasEditorPlainBlockDom()
  if (typeof sourceValue === 'string' && !needsDomFallback) {
    const trustedSource = ensureSafeEditorTableMarkdown(encodeMarkdownExtraBlankLines(source))
    if (!hasUnsafeMarkdownTableStructure(trustedSource)) return trustedSource
  }
  const fallbackValue = needsDomFallback ? getEditorDomContentFallback() : ''
  if (fallbackValue) return fallbackValue
  const syncedValue = getEditorValueWithDomTableSync(source)
  const repairedValue = ensureSafeEditorTableMarkdown(encodeMarkdownExtraBlankLines(syncedValue || source))
  return repairedValue || syncedValue || source
}

const emitEditorValue = (sourceValue?: string) => {
  emit("update:modelValue", getSafeOutgoingEditorValue(sourceValue))
}

const syncEditorDomToVditorValueForPreview = () => {
  if (!vditorInstance?.getValue || !vditorInstance?.setValue) return false
  closeInlineEditorTableTextarea()
  flushPendingEditorTableCellSourceSync()
  const domValue = getEditorVisibleDomTableSafeValue()
  const nextValue = ensureSafeEditorTableMarkdown(encodeMarkdownExtraBlankLines(domValue || vditorInstance.getValue()))
  if (!nextValue || nextValue === vditorInstance.getValue()) return false
  vditorInstance.setValue(nextValue)
  emitEditorValue(nextValue)
  window.setTimeout(() => {
    if (!editorContainer.value) return
    materializeEditorPreservedBlankLineBlocks(editorContainer.value)
    enhanceEditorTables(editorContainer.value)
  }, 0)
  return true
}

const getEditorValueWithDomTableSync = (sourceValue = vditorInstance?.getValue?.() || '') => {
  const tables = getEditorTables()
  if (!tables.length) return ensureSafeEditorTableMarkdown(sourceValue)
  const fallbackValue = getEditorDomContentFallback()
  const blocks = getEditorTableSourceBlocks(sourceValue)
  const replacements: { start: number; end: number; lines: string[] }[] = []
  tables.forEach((table, tableIndex) => {
    const markdown = serializeEditorTableDomAsMarkdown(table)
    if (!markdown) return
    const block = tableBlockFromDataset(table, blocks) || findMarkdownTableBlock(blocks, getRenderedTableRows(table), tableIndex)
    if (!block) return
    replacements.push({ start: block.start, end: block.end, lines: markdown.split('\n') })
  })
  if (!replacements.length) return fallbackValue || sourceValue
  const lines = sourceValue.split('\n')
  replacements
    .sort((left, right) => right.start - left.start)
    .forEach((replacement, index, sorted) => {
      const previous = sorted[index - 1]
      if (previous && replacement.end > previous.start) return
      lines.splice(replacement.start, replacement.end - replacement.start, ...replacement.lines)
    })
  const syncedValue = lines.join('\n')
  if (hasUnsafeMarkdownTableStructure(syncedValue)) return fallbackValue || ensureSafeEditorTableMarkdown(syncedValue)
  return syncedValue || fallbackValue
}

const getEditorTableCellPosition = (cell: HTMLTableCellElement | null): PendingEditorTableCellSync | null => {
  const table = cell?.closest('table') as HTMLTableElement | null
  const row = cell?.parentElement as HTMLTableRowElement | null
  if (!cell || !table || !row) return null
  const tableIndex = getEditorTables().indexOf(table)
  const rowIndex = row.rowIndex
  const cellIndex = cell.cellIndex
  if (tableIndex < 0 || rowIndex < 0 || cellIndex < 0) return null
  return { tableIndex, rowIndex, cellIndex, text: editorTableCellTextFromDom(cell) }
}

const isSameEditorTableCellPosition = (cell: HTMLTableCellElement | null, position: Pick<PendingEditorTableCellSync, 'tableIndex' | 'rowIndex' | 'cellIndex'> | null | undefined) => {
  if (!cell || !position) return false
  const current = getEditorTableCellPosition(cell)
  return !!current && current.tableIndex === position.tableIndex && current.rowIndex === position.rowIndex && current.cellIndex === position.cellIndex
}

const isSamePendingEditorTableCell = (cell: HTMLTableCellElement | null, pending = pendingEditorTableCellSync) => {
  if (!pending) return false
  if (!isSameEditorTableCellPosition(cell, pending)) return false
  const position = getEditorTableCellPosition(cell)
  return !!position
}

const getPendingEditorTableCell = (pending = pendingEditorTableCellSync) => {
  if (!pending) return null as HTMLTableCellElement | null
  return getEditorTableCellAtPosition(pending)
}

const inlineEditorTableTextareaTextForCell = (cell: HTMLTableCellElement | null | undefined) => {
  if (!cell || !inlineEditorTableTextarea || inlineEditorTableTextareaState?.cell !== cell) return null
  return inlineEditorTableTextarea.value
}

const inlineEditorTableCellBaseText = (cell: HTMLTableCellElement) => {
  if (isEditorTableStructurallyEmptyCell(cell)) return ''
  return editorTableCellTextFromDom(cell)
}

const placeCaretAtEndOfEditorTableCell = (cell: HTMLTableCellElement | null) => {
  if (!cell || typeof window === 'undefined') return false
  const selection = window.getSelection()
  if (!selection) return false
  const range = document.createRange()
  range.selectNodeContents(cell)
  range.collapse(false)
  selection.removeAllRanges()
  selection.addRange(range)
  lastEditorSelectionRange = range.cloneRange()
  storeLastEditorTableSelection(range)
  return true
}

const stabilizePendingEditorTableCellDom = () => {
  if (!pendingEditorTableCellSync) return false
  const cell = getPendingEditorTableCell()
  if (!cell) return false
  const inlineText = inlineEditorTableTextareaTextForCell(cell)
  if (inlineText !== null) {
    pendingEditorTableCellSync.text = inlineText
    return true
  }
  const expectedText = pendingEditorTableCellSync.text
  const currentText = editorTableCellTextFromDom(cell)
  normalizeEditorTableBreakCodeMarkers(cell)
  if (currentText === expectedText) return true
  const needsCaretAnchor = /\n$/.test(expectedText)
  setEditorTableDomCellText(cell, expectedText, needsCaretAnchor)
  storeLastEditorTableCell(cell)
  return true
}

const emitPendingEditorTableSafeValue = () => {
  if (!pendingEditorTableCellSync) return
  emitEditorValue()
}

const scheduleStabilizePendingEditorTableCellDom = () => {
  if (typeof window === 'undefined') return
  if (editorTableDomStabilizeTimer !== null) window.clearTimeout(editorTableDomStabilizeTimer)
  const run = () => {
    if (stabilizePendingEditorTableCellDom()) emitPendingEditorTableSafeValue()
  }
  window.requestAnimationFrame(() => {
    run()
    window.requestAnimationFrame(run)
  })
  window.setTimeout(run, 0)
  window.setTimeout(run, 48)
  editorTableDomStabilizeTimer = window.setTimeout(() => {
    editorTableDomStabilizeTimer = null
    run()
  }, 160)
}

const refreshPendingEditorTableCellText = (cell?: HTMLTableCellElement | null) => {
  if (!pendingEditorTableCellSync) return
  const target = cell && isSamePendingEditorTableCell(cell) ? cell : getPendingEditorTableCell()
  if (target) pendingEditorTableCellSync.text = inlineEditorTableTextareaTextForCell(target) ?? editorTableCellTextFromDom(target)
}

const markEditorTableCellSourceDirty = (cell: HTMLTableCellElement, text = editorTableCellTextFromDom(cell)) => {
  const position = getEditorTableCellPosition(cell)
  if (!position) return false
  storeLastEditorTableCell(cell)
  pendingEditorTableCellSync = { ...position, text }
  return true
}

const getInlineEditorTableTextareaElement = (node: Node | null | undefined) => {
  const element = node instanceof Element ? node : node?.parentElement
  return element?.closest?.('.editor-inline-table-cell-textarea') as HTMLTextAreaElement | null
}

const isInlineEditorTableTextareaEvent = (event?: Event) => !!getInlineEditorTableTextareaElement(event?.target as Node | null | undefined)

const textOffsetCandidates = (value: string) => {
  const offsets = [0]
  let offset = 0
  Array.from(String(value || '')).forEach((char) => {
    offset += char.length
    offsets.push(offset)
  })
  return offsets
}

const captureInlineEditorTextareaStyle = (source: HTMLElement): InlineEditorTableTextareaStyleSnapshot => {
  const style = window.getComputedStyle(source)
  return {
    color: style.color,
    fontFamily: style.fontFamily,
    fontSize: style.fontSize,
    fontStyle: style.fontStyle,
    fontVariant: style.fontVariant,
    fontWeight: style.fontWeight,
    letterSpacing: style.letterSpacing,
    lineHeight: style.lineHeight,
    overflowWrap: style.overflowWrap,
    padding: style.padding,
    tabSize: style.tabSize,
    textAlign: style.textAlign,
    textIndent: style.textIndent,
    textTransform: style.textTransform,
    whiteSpace: style.whiteSpace,
    wordBreak: style.wordBreak,
    wordSpacing: style.wordSpacing,
  }
}

const applyInlineEditorTextareaStyle = (target: HTMLElement, style: InlineEditorTableTextareaStyleSnapshot) => {
  target.style.color = style.color
  target.style.fontFamily = style.fontFamily
  target.style.fontSize = style.fontSize
  target.style.fontStyle = style.fontStyle
  target.style.fontVariant = style.fontVariant
  target.style.fontWeight = style.fontWeight
  target.style.letterSpacing = style.letterSpacing
  target.style.lineHeight = style.lineHeight
  target.style.overflowWrap = style.overflowWrap || 'break-word'
  target.style.padding = style.padding
  target.style.textAlign = style.textAlign
  target.style.textIndent = style.textIndent
  target.style.textTransform = style.textTransform
  target.style.whiteSpace = style.whiteSpace && style.whiteSpace !== 'normal' ? style.whiteSpace : 'pre-wrap'
  target.style.wordBreak = style.wordBreak
  target.style.wordSpacing = style.wordSpacing
  if (style.tabSize) target.style.tabSize = style.tabSize
}

const applyInlineEditorTextareaCellBoxStyle = (target: HTMLElement, state: NonNullable<typeof inlineEditorTableTextareaState>) => {
  applyInlineEditorTextareaStyle(target, state.editorStyle)
  const cellStyle = window.getComputedStyle(state.cell)
  const paddingTop = parseEditorCssPixelValue(cellStyle.paddingTop) + parseEditorCssPixelValue(cellStyle.borderTopWidth)
  const paddingRight = parseEditorCssPixelValue(cellStyle.paddingRight) + parseEditorCssPixelValue(cellStyle.borderRightWidth)
  const paddingBottom = parseEditorCssPixelValue(cellStyle.paddingBottom) + parseEditorCssPixelValue(cellStyle.borderBottomWidth)
  const paddingLeft = parseEditorCssPixelValue(cellStyle.paddingLeft) + parseEditorCssPixelValue(cellStyle.borderLeftWidth)
  target.style.padding = `${paddingTop}px ${paddingRight}px ${paddingBottom}px ${paddingLeft}px`
}

const copyInlineEditorTextareaFontStyles = (target: HTMLElement, source: HTMLElement) => {
  applyInlineEditorTextareaStyle(target, captureInlineEditorTextareaStyle(source))
}

const measureInlineEditorTextareaCaretCandidates = (textarea: HTMLTextAreaElement, value: string) => {
  const textareaRect = textarea.getBoundingClientRect()
  const textareaStyle = window.getComputedStyle(textarea)
  const offsets = textOffsetCandidates(value)
  return offsets.map((offset) => {
    const mirror = document.createElement('div')
    mirror.setAttribute('aria-hidden', 'true')
    mirror.style.position = 'fixed'
    mirror.style.left = '-10000px'
    mirror.style.top = '0'
    mirror.style.visibility = 'hidden'
    mirror.style.pointerEvents = 'none'
    mirror.style.boxSizing = 'border-box'
    mirror.style.width = `${Math.max(1, textareaRect.width)}px`
    mirror.style.minHeight = `${Math.max(1, textareaRect.height)}px`
    mirror.style.padding = textareaStyle.padding
    mirror.style.border = '0'
    mirror.style.whiteSpace = 'pre-wrap'
    mirror.style.overflowWrap = textareaStyle.overflowWrap || 'break-word'
    mirror.style.wordBreak = textareaStyle.wordBreak
    copyInlineEditorTextareaFontStyles(mirror, textarea)
    const marker = document.createElement('span')
    marker.textContent = TABLE_CELL_CARET_ANCHOR
    mirror.append(
      document.createTextNode(value.slice(0, offset)),
      marker,
      document.createTextNode(value.slice(offset) || TABLE_CELL_CARET_ANCHOR)
    )
    document.body.appendChild(mirror)
    const markerRect = marker.getBoundingClientRect()
    const mirrorRect = mirror.getBoundingClientRect()
    const result = {
      offset,
      left: markerRect.left - mirrorRect.left,
      top: markerRect.top - mirrorRect.top,
      bottom: markerRect.bottom - mirrorRect.top,
      height: markerRect.height || getEditorElementLineHeight(textarea),
    }
    mirror.remove()
    return result
  })
}

const inlineEditorTextareaRenderedLineCount = (textarea: HTMLTextAreaElement, value: string) => {
  const style = window.getComputedStyle(textarea)
  const lineHeight = getEditorElementLineHeight(textarea)
  const paddingTop = Number.parseFloat(style.paddingTop || '') || 0
  const lines = new Set<number>()
  measureInlineEditorTextareaCaretCandidates(textarea, value).forEach((candidate) => {
    lines.add(Math.max(0, Math.round((candidate.top - paddingTop) / lineHeight)))
  })
  return Math.max(1, lines.size)
}

const parseEditorCssPixelValue = (value: string | null | undefined) => {
  const parsed = Number.parseFloat(value || '')
  return Number.isFinite(parsed) ? parsed : 0
}

const inlineEditorTableCellMinimumHeight = (cell: HTMLTableCellElement) => {
  const style = window.getComputedStyle(cell)
  return Math.max(
    1,
    getEditorElementLineHeight(cell)
      + parseEditorCssPixelValue(style.paddingTop)
      + parseEditorCssPixelValue(style.paddingBottom)
      + parseEditorCssPixelValue(style.borderTopWidth)
      + parseEditorCssPixelValue(style.borderBottomWidth)
  )
}

const inlineEditorTextareaVerticalMetrics = (textarea: HTMLTextAreaElement) => {
  const rect = textarea.getBoundingClientRect()
  const style = window.getComputedStyle(textarea)
  const lineHeight = getEditorElementLineHeight(textarea)
  const paddingTop = parseEditorCssPixelValue(style.paddingTop)
  const paddingBottom = parseEditorCssPixelValue(style.paddingBottom)
  const visualLineCount = Math.max(1, Math.floor(Math.max(0, rect.height - paddingTop - paddingBottom) / lineHeight))
  const lineAreaBottom = rect.top + paddingTop + visualLineCount * lineHeight
  const shieldHeight = Math.min(rect.height, Math.max(INLINE_TABLE_CELL_BOTTOM_EDGE_SHIELD_MIN_PX, rect.bottom - lineAreaBottom))
  return {
    rect,
    lineHeight,
    paddingTop,
    paddingBottom,
    visualLineCount,
    maxLine: visualLineCount - 1,
    shieldTop: Math.max(rect.top, rect.bottom - shieldHeight),
    shieldHeight,
  }
}

const inlineEditorTextareaEdgeGuard = (metrics: ReturnType<typeof inlineEditorTextareaVerticalMetrics>) =>
  Math.max(0, Math.min(INLINE_TABLE_CELL_EDGE_GUARD_PX, metrics.rect.height / 2 - 1))

const inlineEditorTextareaGuardedClientY = (
  event: MouseEvent,
  metrics: ReturnType<typeof inlineEditorTextareaVerticalMetrics>
) => {
  const guard = inlineEditorTextareaEdgeGuard(metrics)
  if (guard <= 0) return event.clientY
  const minY = metrics.rect.top + guard
  const maxY = Math.max(minY, metrics.rect.bottom - guard - metrics.lineHeight / 2)
  return Math.max(minY, Math.min(maxY, event.clientY))
}

const inlineEditorTextareaLineFromPoint = (
  textarea: HTMLTextAreaElement,
  event: MouseEvent,
  metrics = inlineEditorTextareaVerticalMetrics(textarea)
) => {
  const guardedY = inlineEditorTextareaGuardedClientY(event, metrics)
  const rawLine = Math.floor((guardedY - metrics.rect.top - metrics.paddingTop) / metrics.lineHeight)
  return Math.max(0, Math.min(metrics.maxLine, rawLine))
}

const inlineEditorTextareaIsBottomShieldPoint = (textarea: HTMLTextAreaElement, event: MouseEvent) => {
  const metrics = inlineEditorTextareaVerticalMetrics(textarea)
  return event.clientY >= metrics.shieldTop
}

const inlineEditorTextareaCaretFromPoint = (
  textarea: HTMLTextAreaElement,
  cell: HTMLTableCellElement,
  baseText: string,
  event?: MouseEvent
) => {
  if (!event) return { value: baseText, offset: baseText.length }
  const metrics = inlineEditorTextareaVerticalMetrics(textarea)
  const { rect: textareaRect, lineHeight } = metrics
  const targetLine = inlineEditorTextareaLineFromPoint(textarea, event, metrics)
  if (!baseText && targetLine <= 0) return { value: '', offset: 0 }
  const renderedLineCount = inlineEditorTextareaRenderedLineCount(textarea, baseText)
  if (targetLine >= renderedLineCount) {
    const value = `${baseText}${'\n'.repeat(targetLine - renderedLineCount + 1)}`
    return { value, offset: value.length }
  }
  const targetX = event.clientX - textareaRect.left
  const targetY = Math.min(inlineEditorTextareaGuardedClientY(event, metrics), metrics.shieldTop - 0.5) - textareaRect.top
  const candidates = measureInlineEditorTextareaCaretCandidates(textarea, baseText)
  const best = candidates.reduce((current, candidate) => {
    const currentScore = Math.abs((current.top + current.height / 2) - targetY) * 1000 + Math.abs(current.left - targetX)
    const candidateScore = Math.abs((candidate.top + candidate.height / 2) - targetY) * 1000 + Math.abs(candidate.left - targetX)
    return candidateScore < currentScore ? candidate : current
  }, candidates[0] || { offset: 0, left: 0, top: 0, bottom: 0, height: lineHeight })
  return { value: baseText, offset: Math.max(0, Math.min(baseText.length, best.offset)) }
}

const stopInlineEditorTableTextareaBottomShieldEvent = (event: Event) => {
  event.preventDefault()
  event.stopPropagation()
  ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
  inlineEditorTableTextarea?.focus({ preventScroll: true })
}

const ensureInlineEditorTableTextareaBottomShield = () => {
  if (inlineEditorTableTextareaBottomShield) return inlineEditorTableTextareaBottomShield
  const shield = document.createElement('div')
  shield.className = 'editor-inline-table-cell-bottom-shield'
  ;['pointerdown', 'mousedown', 'mouseup', 'click', 'dblclick'].forEach((eventName) => {
    shield.addEventListener(eventName, stopInlineEditorTableTextareaBottomShieldEvent)
  })
  document.body.appendChild(shield)
  inlineEditorTableTextareaBottomShield = shield
  return shield
}

const removeInlineEditorTableTextareaBottomShield = () => {
  inlineEditorTableTextareaBottomShield?.remove()
  inlineEditorTableTextareaBottomShield = null
}

const positionInlineEditorTableTextareaBottomShield = () => {
  const textarea = inlineEditorTableTextarea
  const state = inlineEditorTableTextareaState
  if (!textarea || !state || !editorContainer.value?.contains(state.cell)) {
    removeInlineEditorTableTextareaBottomShield()
    return false
  }
  const metrics = inlineEditorTextareaVerticalMetrics(textarea)
  const shield = ensureInlineEditorTableTextareaBottomShield()
  shield.style.position = 'fixed'
  shield.style.zIndex = '10025'
  shield.style.display = metrics.shieldHeight > 0 ? 'block' : 'none'
  shield.style.left = `${metrics.rect.left}px`
  shield.style.top = `${metrics.shieldTop}px`
  shield.style.width = `${Math.max(1, metrics.rect.width)}px`
  shield.style.height = `${Math.max(0, metrics.shieldHeight)}px`
  return true
}

const positionInlineEditorTableTextarea = (options: { fitContent?: boolean } = {}) => {
  const textarea = inlineEditorTableTextarea
  const state = inlineEditorTableTextareaState
  if (!textarea || !state || !editorContainer.value?.contains(state.cell)) return false
  const cell = state.cell
  const scale = getFixedCoordinateScale()
  const rect = getFixedRect(cell, scale)
  textarea.style.position = 'fixed'
  textarea.style.zIndex = '10024'
  textarea.style.boxSizing = 'border-box'
  textarea.style.display = 'block'
  textarea.style.left = `${rect.left}px`
  textarea.style.top = `${rect.top}px`
  textarea.style.width = `${Math.max(1, rect.width)}px`
  textarea.style.height = `${Math.max(1, state.editorHeight || rect.height)}px`
  textarea.style.margin = '0'
  textarea.style.border = '0'
  textarea.style.outline = 'none'
  textarea.style.resize = 'none'
  textarea.style.overflow = 'hidden'
  textarea.style.background = 'transparent'
  textarea.style.boxShadow = 'none'
  applyInlineEditorTextareaCellBoxStyle(textarea, state)
  if (options.fitContent !== false) resizeInlineEditorTableTextareaToContent()
  else positionInlineEditorTableTextareaBottomShield()
  return true
}

const restoreInlineEditorTableTextareaCellLayout = (state: NonNullable<typeof inlineEditorTableTextareaState>) => {
  state.cell.style.height = state.restoreStyle.height
  state.cell.style.minHeight = state.restoreStyle.minHeight
}

const updateInlineEditorTableTextareaCellLayoutMirror = () => {
  const textarea = inlineEditorTableTextarea
  const state = inlineEditorTableTextareaState
  if (!textarea || !state || !state.dirty || !editorContainer.value?.contains(state.cell)) return false
  const value = textarea.value
  restoreInlineEditorTableTextareaCellLayout(state)
  setEditorTableDomCellText(state.cell, value, /\n$/.test(value))
  markEditorTableCellSourceDirty(state.cell, value)
  return true
}

const resizeInlineEditorTableTextareaToContent = () => {
  const textarea = inlineEditorTableTextarea
  const state = inlineEditorTableTextareaState
  if (!textarea || !state || !editorContainer.value?.contains(state.cell)) return false
  updateInlineEditorTableTextareaCellLayoutMirror()
  const minHeight = Math.max(1, state.minCellHeight)
  textarea.style.height = `${minHeight}px`
  const scrollHeight = textarea.scrollHeight || 0
  let rect = getFixedRect(state.cell, getFixedCoordinateScale())
  const requiredHeight = Math.max(minHeight, scrollHeight, rect.height)
  if (requiredHeight > rect.height + 0.5) {
    state.cell.style.height = `${requiredHeight}px`
    state.cell.style.minHeight = `${requiredHeight}px`
    rect = getFixedRect(state.cell, getFixedCoordinateScale())
  } else {
    restoreInlineEditorTableTextareaCellLayout(state)
    rect = getFixedRect(state.cell, getFixedCoordinateScale())
  }
  const nextEditorHeight = Math.max(1, requiredHeight, rect.height)
  const changed = Math.abs((state.editorHeight || 0) - nextEditorHeight) > 0.5
  state.editorHeight = nextEditorHeight
  textarea.style.height = `${nextEditorHeight}px`
  if (changed) positionInlineEditorTableTextarea({ fitContent: false })
  else positionInlineEditorTableTextareaBottomShield()
  return true
}

const repositionInlineEditorTableTextarea = () => {
  positionInlineEditorTableTextarea()
}

const syncInlineEditorTableTextareaToCell = (options: { emit?: boolean; reposition?: boolean } = {}) => {
  const textarea = inlineEditorTableTextarea
  const state = inlineEditorTableTextareaState
  if (!textarea || !state || !editorContainer.value?.contains(state.cell)) return false
  const value = textarea.value
  setEditorTableDomCellText(state.cell, value, /\n$/.test(value))
  markEditorTableCellSourceDirty(state.cell, value)
  if (options.emit !== false) emitEditorValue()
  if (options.reposition !== false) positionInlineEditorTableTextarea()
  return true
}

const scheduleInlineEditorTableTextareaSync = () => {
  const textarea = inlineEditorTableTextarea
  const state = inlineEditorTableTextareaState
  if (!textarea || !state || !editorContainer.value?.contains(state.cell)) return
  resizeInlineEditorTableTextareaToContent()
}

const closeInlineEditorTableTextarea = () => {
  const textarea = inlineEditorTableTextarea
  const state = inlineEditorTableTextareaState
  if (textarea && state?.dirty) syncInlineEditorTableTextareaToCell({ reposition: false })
  if (state) {
    state.cell.classList.remove('editor-inline-table-cell-editing')
    state.cell.style.color = state.restoreStyle.color
    state.cell.style.caretColor = state.restoreStyle.caretColor
    state.cell.style.height = state.restoreStyle.height
    state.cell.style.minHeight = state.restoreStyle.minHeight
    state.cell.style.textShadow = state.restoreStyle.textShadow
  }
  textarea?.remove()
  removeInlineEditorTableTextareaBottomShield()
  inlineEditorTableTextarea = null
  inlineEditorTableTextareaState = null
}

const onInlineEditorTableTextareaInput = () => {
  if (!inlineEditorTableTextareaState) return
  inlineEditorTableTextareaState.dirty = true
  scheduleInlineEditorTableTextareaSync()
}

const onInlineEditorTableTextareaKeydown = (event: KeyboardEvent) => {
  event.stopPropagation()
  if (event.key === 'Escape') {
    event.preventDefault()
    closeInlineEditorTableTextarea()
  }
}

const setInlineEditorTextareaCaretFromMouseEvent = (event: MouseEvent) => {
  const textarea = inlineEditorTableTextarea
  const state = inlineEditorTableTextareaState
  if (!textarea || !state || !editorContainer.value?.contains(state.cell)) return false
  const caret = inlineEditorTextareaCaretFromPoint(textarea, state.cell, textarea.value, event)
  if (textarea.value !== caret.value) {
    textarea.value = caret.value
    state.dirty = true
    markEditorTableCellSourceDirty(state.cell, textarea.value)
    resizeInlineEditorTableTextareaToContent()
  }
  textarea.focus({ preventScroll: true })
  textarea.setSelectionRange(caret.offset, caret.offset)
  return true
}

const onInlineEditorTextareaMouseDown = (event: MouseEvent) => {
  event.stopPropagation()
  const textarea = getInlineEditorTableTextareaElement(event.target as Node | null | undefined)
  if (!textarea || event.button !== 0 || !inlineEditorTextareaIsBottomShieldPoint(textarea, event)) return
  event.preventDefault()
  setInlineEditorTextareaCaretFromMouseEvent(event)
}

const stopInlineEditorTextareaEventPropagation = (event: Event) => {
  event.stopPropagation()
}

const ensureInlineEditorTableTextarea = () => {
  if (inlineEditorTableTextarea) return inlineEditorTableTextarea
  const textarea = document.createElement('textarea')
  textarea.className = 'editor-inline-table-cell-textarea'
  textarea.rows = 1
  textarea.spellcheck = false
  textarea.autocapitalize = 'off'
  textarea.autocomplete = 'off'
  textarea.wrap = 'soft'
  textarea.addEventListener('input', onInlineEditorTableTextareaInput)
  textarea.addEventListener('keydown', onInlineEditorTableTextareaKeydown)
  textarea.addEventListener('beforeinput', stopInlineEditorTextareaEventPropagation)
  textarea.addEventListener('compositionstart', stopInlineEditorTextareaEventPropagation)
  textarea.addEventListener('compositionupdate', stopInlineEditorTextareaEventPropagation)
  textarea.addEventListener('compositionend', stopInlineEditorTextareaEventPropagation)
  textarea.addEventListener('mousedown', onInlineEditorTextareaMouseDown)
  textarea.addEventListener('click', stopInlineEditorTextareaEventPropagation)
  textarea.addEventListener('blur', () => window.setTimeout(() => {
    if (document.activeElement !== inlineEditorTableTextarea) closeInlineEditorTableTextarea()
  }, 0))
  document.body.appendChild(textarea)
  inlineEditorTableTextarea = textarea
  return textarea
}

const openInlineEditorTableCellTextarea = (cell: HTMLTableCellElement, event?: MouseEvent) => {
  if (!editorContainer.value?.contains(cell)) return false
  if (inlineEditorTableTextareaState?.cell !== cell) closeInlineEditorTableTextarea()
  const textarea = ensureInlineEditorTableTextarea()
  const baseText = inlineEditorTableCellBaseText(cell)
  const editorStyle = captureInlineEditorTextareaStyle(cell)
  const cellHeight = Math.max(1, getFixedRect(cell, getFixedCoordinateScale()).height)
  const minCellHeight = inlineEditorTableCellMinimumHeight(cell)
  inlineEditorTableTextareaState = {
    cell,
    baseText,
    minCellHeight,
    dirty: false,
    editorHeight: cellHeight,
    restoreStyle: {
      color: cell.style.color,
      caretColor: cell.style.caretColor,
      height: cell.style.height,
      minHeight: cell.style.minHeight,
      textShadow: cell.style.textShadow,
    },
    editorStyle,
  }
  storeLastEditorTableCell(cell)
  textarea.style.visibility = 'hidden'
  textarea.value = baseText
  positionInlineEditorTableTextarea()
  const caret = inlineEditorTextareaCaretFromPoint(textarea, cell, baseText, event)
  textarea.value = caret.value
  cell.classList.add('editor-inline-table-cell-editing')
  cell.style.color = 'transparent'
  cell.style.caretColor = 'transparent'
  cell.style.textShadow = 'none'
  textarea.style.visibility = 'visible'
  textarea.focus({ preventScroll: true })
  textarea.setSelectionRange(caret.offset, caret.offset)
  return true
}

const getEditorValueWithPendingTableSync = () => {
  const currentValue = vditorInstance?.getValue?.() || ''
  if (getEditorTables().length) {
    refreshPendingEditorTableCellText()
    const fallbackValue = getEditorDomContentFallback()
    if (fallbackValue) return fallbackValue
  }
  const syncedValue = getEditorValueWithDomTableSync(currentValue)
  const fallbackValue = syncedValue.trim() ? syncedValue : getEditorDomContentFallback()
  if (!pendingEditorTableCellSync) return fallbackValue || syncedValue || currentValue
  refreshPendingEditorTableCellText()
  const cell = getPendingEditorTableCell()
  const result = buildEditorTableCellSourceValue(cell, pendingEditorTableCellSync.text)
  if (result?.value && !hasUnsafeMarkdownTableStructure(result.value)) return result.value
  return fallbackValue || result?.value || syncedValue || currentValue
}

const flushPendingEditorTableCellSourceSync = () => {
  if (!pendingEditorTableCellSync) return true
  refreshPendingEditorTableCellText()
  const pending = pendingEditorTableCellSync
  const cell = getPendingEditorTableCell()
  if (!cell) return false
  pendingEditorTableCellSync = null
  const applied = applyEditorTableCellSourceValue(cell, pending.text)
  if (!applied) pendingEditorTableCellSync = pending
  return applied
}

const flushPendingEditorTableCellSourceSyncIfMoved = (currentCell?: HTMLTableCellElement | null) => {
  if (!pendingEditorTableCellSync) return true
  if (editorTableCompositionActive || isEditorTableCompositionSettling()) {
    refreshPendingEditorTableCellText(currentCell || getPendingEditorTableCell())
    return true
  }
  if (currentCell && isSamePendingEditorTableCell(currentCell)) {
    refreshPendingEditorTableCellText(currentCell)
    return true
  }
  return flushPendingEditorTableCellSourceSync()
}

const syncEditorTableCellDomToSource = (
  cell: HTMLTableCellElement | null,
  options: { restoreCaret?: boolean } = {}
) => {
  const target = getEditorTableCellSourceTarget(cell)
  if (!target || !cell) return false
  const current = target.rowCells[target.cellIndex] || ''
  const nextText = editorTableCellTextFromDom(cell)
  if (hasAttachmentMarker(current) && !hasAttachmentMarker(nextText)) return false
  const applied = applyEditorTableCellSourceValue(cell, nextText, options)
  if (applied && isSamePendingEditorTableCell(cell)) pendingEditorTableCellSync = null
  return applied
}

const editableRowsFromMarkdownBlock = (block: EditorTableSourceBlock) => {
  if (block.kind !== 'markdown' || block.lines.length < 2) return [] as string[][]
  return [parseEditableMarkdownTableRow(block.lines[0]), ...block.lines.slice(2).map((line) => parseEditableMarkdownTableRow(line))]
}

const editableRowsFromTableBlock = (block: EditorTableSourceBlock) => block.kind === 'markdown'
  ? editableRowsFromMarkdownBlock(block)
  : (block.kind === 'tab' ? editableRowsFromTabBlock(block) : editableRowsFromHtmlBlock(block))

const normalizeExpandedTableRows = (rows: string[][]) => {
  const colCount = Math.max(1, ...rows.map((row) => row.length))
  return rows.map((row) => Array.from({ length: colCount }, (_, index) => row[index] ?? ''))
}

const stopEditorTableNativeEvent = (event: Event) => {
  event.preventDefault()
  event.stopPropagation()
  ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
}

const stopEditorTablePropagation = (event: Event) => {
  event.stopPropagation()
  ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
}

const clearVditorCompositionLock = () => {
  const instance = vditorInstance as unknown as {
    currentMode?: 'ir' | 'wysiwyg' | 'sv'
    ir?: { composingLock?: boolean }
    wysiwyg?: { composingLock?: boolean }
    sv?: { composingLock?: boolean }
  } | null
  if (!instance) return
  const mode = instance.currentMode
  if (mode && instance[mode]) instance[mode].composingLock = false
}

const setEditorTableDomCellText = (cell: HTMLTableCellElement, value: string, withCaretAnchor = false) => {
  cell.innerHTML = editorTextToHtmlTableCellSource(value)
  if (withCaretAnchor) cell.appendChild(createEditorTableCaretAnchorNode())
}

const getEditorTableTextMatrix = (table: HTMLTableElement | null) => table
  ? Array.from(table.rows).map((row) => Array.from(row.cells).map((cell) => editorTableCellTextFromDom(cell as HTMLTableCellElement)))
  : []

const normalizeEditorTableCompositionText = (value: string) => stripEditorTableCaretAnchors(String(value || ''))
  .replace(/[\u200b\u200c\ufeff]/g, '')
  .replace(/\u00a0/g, ' ')

const rememberEditorTableCompositionCell = (cell: HTMLTableCellElement | null) => {
  const position = getEditorTableCellPosition(cell)
  if (!position) {
    editorTableCompositionTarget = null
    editorTableCompositionSnapshot = null
    editorTableCompositionStartText = ''
    editorTableCompositionStartPrefix = ''
    return
  }
  const table = getEditorTables()[position.tableIndex]
  editorTableCompositionTarget = position
  editorTableCompositionSnapshot = getEditorTableTextMatrix(table || null)
  const range = getRangeInsideEditorTableCell(cell!)
  editorTableCompositionStartText = editorTableCellTextFromDom(cell!)
  editorTableCompositionStartPrefix = range ? getEditorTableTextBeforeRange(cell!, range) : ''
  storeLastEditorTableCell(cell)
}

const clearEditorTableCompositionCommitKeyLater = (commitKey: EditorTableCompositionCommitKey) => {
  window.setTimeout(() => {
    if (editorTableCompositionCommitKey === commitKey) editorTableCompositionCommitKey = null
  }, Math.max(0, commitKey.expiresAt - Date.now()))
}

const rememberEditorTableCompositionCommitKey = (cell: HTMLTableCellElement | null, key: EditorTableCompositionCommitKey['key']) => {
  const position = getEditorTableCellPosition(cell) || editorTableCompositionTarget
  if (!position) return false
  const commitKey = {
    tableIndex: position.tableIndex,
    rowIndex: position.rowIndex,
    cellIndex: position.cellIndex,
    key,
    expiresAt: Date.now() + 450,
  }
  editorTableCompositionCommitKey = commitKey
  clearEditorTableCompositionCommitKeyLater(commitKey)
  return true
}

const markEditorTableCompositionSettling = () => {
  editorTableCompositionSettlingUntil = Date.now() + 240
  window.setTimeout(() => {
    if (Date.now() >= editorTableCompositionSettlingUntil) editorTableCompositionSettlingUntil = 0
  }, 260)
}

const isEditorTableCompositionSettling = () => Date.now() < editorTableCompositionSettlingUntil

const getEditorTableCellForCompositionInput = (event?: Event) => {
  if (!editorTableCompositionActive && !isEditorTableCompositionSettling() && !editorTableCompositionCommitKey) return null as HTMLTableCellElement | null
  const position = editorTableCompositionTarget || editorTableCompositionCommitKey
  const cell = getEditorTableCellAtPosition(position)
  if (!cell) return null as HTMLTableCellElement | null
  const eventEditable = getEditorEditableFromNode(event?.target as Node | null | undefined)
  const cellEditable = getEditorEditableFromNode(cell)
  if (eventEditable && cellEditable && eventEditable !== cellEditable) return null as HTMLTableCellElement | null
  return cell
}

const shouldSuppressEditorTableCompositionCommitArtifact = (cell: HTMLTableCellElement, inputType: string, text = '') => {
  const commitKey = editorTableCompositionCommitKey
  if (!commitKey || Date.now() > commitKey.expiresAt) {
    editorTableCompositionCommitKey = null
    return false
  }
  if (!isSameEditorTableCellPosition(cell, commitKey)) return false
  const isSpaceArtifact = commitKey.key === 'Space' && inputType === 'insertText' && text === ' '
  const isEnterArtifact = commitKey.key === 'Enter' && (inputType === 'insertParagraph' || inputType === 'insertLineBreak' || text === '\n')
  if (!isSpaceArtifact && !isEnterArtifact) return false
  editorTableCompositionCommitKey = null
  return true
}

const getEditorTableCompositionCaretOffset = (cell: HTMLTableCellElement, data = '') => {
  const before = normalizeEditorTableCompositionText(editorTableCompositionStartText)
  const after = normalizeEditorTableCompositionText(editorTableCellTextFromDom(cell))
  const prefix = normalizeEditorTableCompositionText(editorTableCompositionStartPrefix)
  const committed = normalizeEditorTableCompositionText(data)
  if (committed) {
    const directIndex = after.slice(prefix.length, prefix.length + committed.length) === committed
      ? prefix.length
      : after.indexOf(committed, Math.max(0, prefix.length - 1))
    if (directIndex >= 0) return directIndex + committed.length
  }
  let start = 0
  while (start < before.length && start < after.length && before[start] === after[start]) start += 1
  let beforeEnd = before.length
  let afterEnd = after.length
  while (beforeEnd > start && afterEnd > start && before[beforeEnd - 1] === after[afterEnd - 1]) {
    beforeEnd -= 1
    afterEnd -= 1
  }
  return afterEnd
}

const rememberEditorTableCompositionCaretTarget = (cell: HTMLTableCellElement | null, data = '') => {
  const position = getEditorTableCellPosition(cell)
  if (!position || !cell) {
    editorTableCompositionCaretTarget = null
    return false
  }
  editorTableCompositionCaretTarget = {
    tableIndex: position.tableIndex,
    rowIndex: position.rowIndex,
    cellIndex: position.cellIndex,
    offset: getEditorTableCompositionCaretOffset(cell, data),
    expiresAt: Date.now() + 300,
  }
  return true
}

const restoreEditorTableCompositionCaret = () => {
  const target = editorTableCompositionCaretTarget
  if (!target || Date.now() > target.expiresAt) {
    editorTableCompositionCaretTarget = null
    return false
  }
  const cell = getEditorTableCellAtPosition(target)
  if (!cell) return false
  const currentCell = getCurrentEditorTableCell()
  if (currentCell && !isSameEditorTableCellPosition(currentCell, target)) return false
  return placeCaretAtEditorTableCellTextOffset(cell, target.offset)
}

const scheduleRestoreEditorTableCompositionCaret = () => {
  restoreEditorTableCompositionCaret()
  window.requestAnimationFrame(() => restoreEditorTableCompositionCaret())
  window.setTimeout(() => restoreEditorTableCompositionCaret(), 0)
  window.setTimeout(() => {
    if (restoreEditorTableCompositionCaret()) editorTableCompositionCaretTarget = null
  }, 80)
}

const isEmptyEditorTableText = (text: string) => !String(text || '').replace(/[\u200b\u200c\ufeff\s]/g, '')

const cleanupEditorTableCompositionDrift = (data = '') => {
  const targetInfo = editorTableCompositionTarget
  if (!targetInfo) return false
  const table = getEditorTables()[targetInfo.tableIndex]
  const target = table?.rows[targetInfo.rowIndex]?.cells[targetInfo.cellIndex] as HTMLTableCellElement | undefined
  if (!table || !target) return false
  const targetText = editorTableCellTextFromDom(target)
  const normalizedData = String(data || '').trim()
  const targetLines = targetText.split('\n').map((line) => line.trim()).filter(Boolean)
  let changed = false
  Array.from(table.rows).forEach((row, rowIndex) => {
    Array.from(row.cells).forEach((rawCell, cellIndex) => {
      const cell = rawCell as HTMLTableCellElement
      if (rowIndex === targetInfo.rowIndex && cellIndex === targetInfo.cellIndex) return
      const before = editorTableCompositionSnapshot?.[rowIndex]?.[cellIndex] ?? ''
      const after = editorTableCellTextFromDom(cell)
      const afterTrimmed = after.trim()
      if (!isEmptyEditorTableText(before) || isEmptyEditorTableText(after) || after === before) return
      const duplicatedCurrentComposition = !!normalizedData && afterTrimmed === normalizedData && targetText.includes(normalizedData)
      const duplicatedTargetLine = targetLines.includes(afterTrimmed)
      if (!duplicatedCurrentComposition && !duplicatedTargetLine) return
      setEditorTableDomCellText(cell, before)
      changed = true
    })
  })
  if (changed) {
    markEditorTableCellSourceDirty(target)
    scheduleStabilizePendingEditorTableCellDom()
    emitEditorValue()
  }
  return changed
}

const dispatchEditorTableDomInput = (table: HTMLTableElement) => {
  const editable = table.closest('.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset, .vditor-sv .vditor-reset') as HTMLElement | null
  editable?.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertReplacementText' }))
  window.setTimeout(() => {
    const nextValue = getEditorValueWithPendingTableSync()
    emitEditorValue(nextValue)
    refreshAttachmentLinksFromEditor()
    if (editorContainer.value) enhanceEditorTables(editorContainer.value)
  }, 0)
}

const syncExpandedTableDomToEditor = () => {
  const table = expandedEditorTableElement
  if (!table || !editorContainer.value?.contains(table)) return false
  const rows = normalizeExpandedTableRows(expandedTableRows.value)
  if (!rows.length) return false
  rememberEditorTableScroll(table)
  const body = table.tBodies[0] || table.createTBody()
  while (table.rows.length > rows.length) table.deleteRow(table.rows.length - 1)
  rows.forEach((cells, rowIndex) => {
    const row = table.rows[rowIndex] || body.insertRow()
    while (row.cells.length < cells.length) row.insertCell()
    while (row.cells.length > cells.length) row.deleteCell(row.cells.length - 1)
    cells.forEach((text, cellIndex) => {
      setEditorTableDomCellText(row.cells[cellIndex] as HTMLTableCellElement, text)
    })
  })
  replaceTableBreakTextNodes(table)
  dispatchEditorTableDomInput(table)
  expandedTableDirty.value = false
  return true
}

const syncExpandedTableToEditor = () => {
  if (!vditorInstance || !expandedTableEditable.value) return false
  if (!expandedEditorTableBlock) return syncExpandedTableDomToEditor()
  const value = vditorInstance.getValue?.() || ''
  const lines = value.split('\n')
  const blocks = getEditorTableSourceBlocks(value)
  const currentBlock = blocks.find((block) => block.start === expandedEditorTableBlock?.start && block.end === expandedEditorTableBlock?.end) || expandedEditorTableBlock
  if (!currentBlock) return false
  const rows = normalizeExpandedTableRows(expandedTableRows.value)
  const nextBlockLines = serializeEditableTableBlock(currentBlock, rows)
  if (!nextBlockLines) return false
  lines.splice(currentBlock.start, currentBlock.end - currentBlock.start, ...nextBlockLines)
  const nextValue = lines.join('\n')
  expandedEditorTableBlock = { ...currentBlock, end: currentBlock.start + nextBlockLines.length, lines: nextBlockLines }
  vditorInstance.setValue(nextValue)
  emitEditorValue(nextValue)
  window.setTimeout(() => refreshAttachmentLinksFromEditor(), 0)
  expandedTableDirty.value = false
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

const stopExpandedTableResize = () => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('pointermove', onExpandedTableResizeMove, true)
    window.removeEventListener('pointerup', stopExpandedTableResize, true)
    window.removeEventListener('pointercancel', stopExpandedTableResize, true)
  }
  expandedTableResizeDrag = null
  if (typeof document !== 'undefined') {
    document.body.classList.remove('is-resizing-expanded-table-row', 'is-resizing-expanded-table-column')
  }
}

const onExpandedTableResizeMove = (event: PointerEvent) => {
  const drag = expandedTableResizeDrag
  if (!drag) return
  event.preventDefault()
  event.stopPropagation()
  if (drag.type === 'row') {
    const nextHeight = Math.max(
      expandedTableAutoRowHeights.value[drag.index] || EXPANDED_TABLE_MIN_ROW_HEIGHT,
      drag.startSize + event.clientY - drag.startClient
    )
    const heights = [...expandedTableManualRowHeights.value]
    heights[drag.index] = Math.ceil(nextHeight)
    expandedTableManualRowHeights.value = heights
    scheduleExpandedTableScrollOverflowState()
    return
  }
  const nextWidth = Math.max(EXPANDED_TABLE_MIN_COLUMN_WIDTH, drag.startSize + event.clientX - drag.startClient)
  const widths = [...expandedTableManualColumnWidths.value]
  widths[drag.index] = Math.ceil(nextWidth)
  expandedTableManualColumnWidths.value = widths
  scheduleMeasureExpandedTableAutoRowHeights()
  scheduleExpandedTableScrollOverflowState()
}

const startExpandedTableResize = (drag: ExpandedTableResizeDrag, event: PointerEvent) => {
  if (typeof window === 'undefined') return
  stopExpandedTableResize()
  expandedTableResizeDrag = drag
  document.body.classList.add(drag.type === 'row' ? 'is-resizing-expanded-table-row' : 'is-resizing-expanded-table-column')
  event.currentTarget instanceof HTMLElement && event.currentTarget.setPointerCapture?.(event.pointerId)
  window.addEventListener('pointermove', onExpandedTableResizeMove, true)
  window.addEventListener('pointerup', stopExpandedTableResize, true)
  window.addEventListener('pointercancel', stopExpandedTableResize, true)
}

const startExpandedTableRowResize = (rowIndex: number, event: PointerEvent) => {
  if (rowIndex < 0 || rowIndex >= expandedTableRows.value.length) return
  startExpandedTableResize({
    type: 'row',
    index: rowIndex,
    startClient: event.clientY,
    startSize: expandedTableRowHeight(rowIndex)
  }, event)
}

const startExpandedTableColumnResize = (columnIndex: number, event: PointerEvent) => {
  if (columnIndex < 0 || columnIndex >= expandedTableColumnWidths.value.length) return
  startExpandedTableResize({
    type: 'column',
    index: columnIndex,
    startClient: event.clientX,
    startSize: expandedTableColumnWidths.value[columnIndex] || EXPANDED_TABLE_MIN_COLUMN_WIDTH
  }, event)
}

const closeExpandedTable = () => {
  if (!showTableExpandDialog.value || tableExpandClosing.value) return
  if (expandedTableDirty.value && !syncExpandedTableToEditor()) {
    window.alert('未能同步放大表格内容，请先复制当前编辑内容后再关闭。')
    return
  }
  tableExpandClosing.value = true
  if (tableExpandCloseTimer !== null) window.clearTimeout(tableExpandCloseTimer)
  tableExpandCloseTimer = window.setTimeout(() => {
    showTableExpandDialog.value = false
    tableExpandClosing.value = false
    expandedTableRows.value = []
    expandedTableAutoRowHeights.value = []
    expandedTableManualRowHeights.value = []
    expandedTableManualColumnWidths.value = []
    stopExpandedTableResize()
    expandedTableEditable.value = false
    expandedTableDirty.value = false
    expandedEditorTableBlock = null
    expandedEditorTableElement = null
    tableExpandCloseTimer = null
  }, 180)
}

const openHoveredTableExpand = () => {
  let table = hoveredEditorTable
  if (!table || !editorContainer.value?.contains(table)) return
  const preferredIndex = getEditorTables().indexOf(table)
  closeInlineEditorTableTextarea()
  flushPendingEditorTableCellSourceSync()
  enhanceEditorTables(editorContainer.value)
  table = getEditorTables()[preferredIndex] || table
  if (!table || !editorContainer.value.contains(table)) return
  const tableIndex = getEditorTables().indexOf(table)
  const block = getEditorTableBlockForTable(table, tableIndex)
  const renderedRows = editableRowsFromRenderedTable(table)
  const rows = block ? mergeRenderedTableEdgeBreaks(editableRowsFromTableBlock(block), renderedRows) : renderedRows
  if (!rows.length) return
  if (tableExpandCloseTimer !== null) {
    window.clearTimeout(tableExpandCloseTimer)
    tableExpandCloseTimer = null
  }
  expandedTableRows.value = normalizeExpandedTableRows(rows)
  expandedTableAutoRowHeights.value = []
  expandedTableManualRowHeights.value = []
  expandedTableManualColumnWidths.value = []
  expandedTableEditable.value = !!block || !!table
  expandedTableDirty.value = false
  expandedEditorTableBlock = block || null
  expandedEditorTableElement = table
  tableExpandClosing.value = false
  showTableExpandDialog.value = true
  hideTableDeleteButton()
  nextTick(() => {
    updateExpandedTableAvailableWidth()
    scheduleMeasureExpandedTableAutoRowHeights()
    scheduleExpandedTableScrollOverflowState()
    document.querySelector<HTMLTextAreaElement>('.editor-table-expand-dialog textarea')?.focus()
  })
}

const replaceTableBreakTextNodes = (table: HTMLTableElement) => {
  table.querySelectorAll('td,th').forEach((cell) => {
    normalizeEditorTableBreakCodeMarkers(cell as HTMLTableCellElement)
    const walker = document.createTreeWalker(cell, NodeFilter.SHOW_TEXT, {
      acceptNode(node) {
        return /(?:<br\s*\/?\s*>|%%NW_TABLE_BR%%)/i.test(node.textContent || '') ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT
      }
    })
    const nodes: Text[] = []
    while (walker.nextNode()) nodes.push(walker.currentNode as Text)
    nodes.forEach((textNode) => {
      const parts = String(textNode.textContent || '').split(TABLE_CELL_BREAK_SOURCE_RE)
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
    const nextValue = getEditorValueWithPendingTableSync()
    emitEditorValue(nextValue)
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
  emitEditorValue(nextValue)
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

const syncEditorTableScrollEdgeGap = (table: HTMLTableElement, root: HTMLElement) => {
  if (typeof window === 'undefined') return
  const viewport = table.closest<HTMLElement>('.vditor-reset') || root
  const tableRect = table.getBoundingClientRect()
  const viewportRect = viewport.getBoundingClientRect()
  const leftGap = Math.max(0, Math.round(tableRect.left - viewportRect.left))
  const rightGap = Math.max(0, Math.round(viewportRect.right - tableRect.right))
  const edgeGap = Number.isFinite(leftGap) && Number.isFinite(rightGap) ? Math.max(0, leftGap - rightGap) + 1 : 1
  table.style.setProperty('--editor-table-scroll-edge-gap', `${edgeGap}px`)
}

const enhanceEditorTables = (root: HTMLElement) => {
  const blocks = getEditorTableSourceBlocks(vditorInstance?.getValue?.() || '')
  const usedBlocks = new Set<EditorTableSourceBlock>()
  getEditorTables().forEach((table, index) => {
    table.classList.add('editor-deletable-table')
    syncEditorTableScrollEdgeGap(table, root)
    table.dataset.editorTableIndex = String(index)
    const renderedRows = getRenderedTableRows(table)
    const datasetBlock = tableBlockFromDataset(table, blocks)
    let block = datasetBlock && !usedBlocks.has(datasetBlock) ? datasetBlock : undefined
    if (!block && tableRowsHaveComparableContent(renderedRows)) {
      block = blocks.find((candidate) => !usedBlocks.has(candidate) && sameTableRows(comparableRowsFromTableBlock(candidate), renderedRows))
    }
    if (!block && index >= 0 && index < blocks.length && !usedBlocks.has(blocks[index])) block = blocks[index]
    if (!block) block = blocks.find((candidate) => !usedBlocks.has(candidate))
    if (block) usedBlocks.add(block)
    // Do not structurally mutate Vditor's live editable table DOM here.
    // Vditor IR owns the table model; replacing the live thead/th tree or removing
    // its generated divider row can make getValue() return empty content and can
    // collapse the table during later typing/publishing. First-row parity is kept
    // visually through CSS while source/detached HTML paths may still normalize.
    replaceTableBreakTextNodes(table)
    const scrollKey = tableScrollKeyFromBlock(block, `index:${index}`)
    table.dataset.editorTableScrollKey = scrollKey
    table.onscroll = () => rememberEditorTableScroll(table)
    if (block) {
      const sourceIndex = blocks.indexOf(block)
      table.dataset.editorTableBlockStart = String(block.start)
      table.dataset.editorTableBlockEnd = String(block.end)
      if (sourceIndex >= 0) table.dataset.editorTableSourceIndex = String(sourceIndex)
      else delete table.dataset.editorTableSourceIndex
    } else {
      delete table.dataset.editorTableBlockStart
      delete table.dataset.editorTableBlockEnd
      delete table.dataset.editorTableSourceIndex
    }
    restoreEditorTableScroll(table)
  })
}

const applyHeadingFallback = (option: typeof headingOptions[number]) => {
  if (!vditorInstance) return
  const value = vditorInstance.getValue?.() || ''
  if (!value.trim()) {
    vditorInstance.setValue(option.value)
    emitEditorValue(option.value)
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
  emitEditorValue(nextValue)
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

  const isPreviewItem = (item: HTMLElement | null) => {
    const action = toolbarAction(item)
    const type = action?.getAttribute('data-type') || ''
    const label = action?.getAttribute('aria-label') || action?.getAttribute('title') || ''
    return type === 'preview' || /预览|Preview/i.test(label)
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
    const item = target?.closest('.vditor-toolbar__item, button[data-type], [role="button"][data-type]') as HTMLElement | null
    if (!item || !editorContainer.value?.contains(item)) return
    if (isPreviewItem(item)) {
      syncEditorDomToVditorValueForPreview()
      return
    }
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
      vditorInstance?.setValue(ensureSafeEditorTableMarkdown(encodeMarkdownExtraBlankLines(props.modelValue)));
      vditorInstance?.setTheme(props.theme === 'dark' ? 'dark' : 'classic');
      isReady.value = true;
      emit("ready");
      nextTick(() => {
        scheduleNormalizeEditorTableSource()
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
    if (editorTableDomStabilizeTimer !== null) {
      window.clearTimeout(editorTableDomStabilizeTimer)
      editorTableDomStabilizeTimer = null
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
    closeInlineEditorTableTextarea()
    if (tableExpandCloseTimer !== null) {
      window.clearTimeout(tableExpandCloseTimer)
      tableExpandCloseTimer = null
    }
    if (expandedTableRowHeightMeasureTimer !== null) {
      window.cancelAnimationFrame(expandedTableRowHeightMeasureTimer)
      expandedTableRowHeightMeasureTimer = null
    }
    stopExpandedTableResize()
    showTableExpandDialog.value = false
    tableExpandClosing.value = false
    expandedTableDirty.value = false
    pendingEditorTableCellSync = null
    editorTableCompositionActive = false
    editorTableCompositionTarget = null
    editorTableCompositionSnapshot = null
    editorTableCompositionStartText = ''
    editorTableCompositionStartPrefix = ''
    editorTableCompositionCommitKey = null
    editorTableCompositionCaretTarget = null
    editorTableCompositionSettlingUntil = 0
    lastEditorTableSelectionRange = null
    lastEditorTableSelectionState = null
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

const getEditorTableCellFromNode = (node: Node | null | undefined) => {
  const element = node instanceof Element ? node : node?.parentElement
  return getEditorTableCellFromElement(element)
}

const getEditorTableCellFromRange = (range: Range | null) => {
  if (!range || !editorContainer.value) return null as HTMLTableCellElement | null
  return getEditorTableCellFromNode(range.startContainer)
    || getEditorTableCellFromNode(range.endContainer)
    || getEditorTableCellFromNode(range.commonAncestorContainer)
}

const getEditorTableCellFromEvent = (event?: Event) => {
  const target = event?.target as Node | null | undefined
  const element = target instanceof Element ? target : target?.parentElement
  return getEditorTableCellFromElement(element)
}

const EDITOR_EDITABLE_SELECTOR = '.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset, .vditor-sv .vditor-reset'

const getEditorEditableFromNode = (node: Node | null | undefined) => {
  const element = node instanceof Element ? node : node?.parentElement
  const editable = element?.closest?.(EDITOR_EDITABLE_SELECTOR) as HTMLElement | null
  return editable && editorContainer.value?.contains(editable) ? editable : null
}

const storeLastEditorTableCell = (cell: HTMLTableCellElement | null | undefined) => {
  const editable = getEditorEditableFromNode(cell)
  const table = cell?.closest('table') as HTMLTableElement | null
  if (!cell || !editable || !table) return
  const tables = Array.from(editable.querySelectorAll('table'))
  const tableIndex = tables.indexOf(table)
  if (tableIndex < 0 || !cell.parentElement) return
  lastEditorTableSelectionState = {
    editable,
    tableIndex,
    rowIndex: (cell.parentElement as HTMLTableRowElement).rowIndex,
    cellIndex: cell.cellIndex,
  }
}

const storeLastEditorTableSelection = (range: Range) => {
  const cell = getEditorTableCellFromRange(range)
  storeLastEditorTableCell(cell)
  if (cell) lastEditorTableSelectionRange = range.cloneRange()
}

const getStoredEditorTableCell = (editable: HTMLElement | null) => {
  if (!editable || !lastEditorTableSelectionState) return null as HTMLTableCellElement | null
  const sameEditable = lastEditorTableSelectionState.editable === editable
  const previousEditableDetached = !editorContainer.value?.contains(lastEditorTableSelectionState.editable)
  if (!sameEditable && !previousEditableDetached) return null
  const table = editable.querySelectorAll<HTMLTableElement>('table')[lastEditorTableSelectionState.tableIndex]
  const row = table?.rows[lastEditorTableSelectionState.rowIndex]
  return (row?.cells[lastEditorTableSelectionState.cellIndex] as HTMLTableCellElement | undefined) || null
}

const getCurrentEditorTableCell = (event?: Event, options: { allowStoredFallback?: boolean } = {}) => {
  if (typeof window === 'undefined') return null as HTMLTableCellElement | null
  const selection = window.getSelection()
  const range = selection && selection.rangeCount > 0 ? selection.getRangeAt(0) : null
  const currentCell = getEditorTableCellFromRange(range) || getEditorTableCellFromEvent(event)
  if (currentCell) return currentCell
  if (!options.allowStoredFallback) return null
  const eventEditable = getEditorEditableFromNode(event?.target as Node | null | undefined)
  const rangeEditable = getEditorEditableFromNode(range?.commonAncestorContainer)
  const currentEditable = eventEditable || rangeEditable
  if (range && rangeEditable && !getEditorTableCellFromRange(range)) return null
  const rangeCell = getEditorTableCellFromRange(lastEditorTableSelectionRange)
  const lastCell = rangeCell && getEditorEditableFromNode(rangeCell) === currentEditable
    ? rangeCell
    : getStoredEditorTableCell(currentEditable)
  const pendingCell = getPendingEditorTableCell()
  const fallbackCell = lastCell || (pendingCell && getEditorEditableFromNode(pendingCell) === currentEditable ? pendingCell : null)
  if (!fallbackCell || getEditorEditableFromNode(fallbackCell) !== currentEditable) return null
  return fallbackCell
}

const clearEditorTableEmptyPlaceholder = (cell: HTMLTableCellElement) => {
  if (!isEditorTableStructurallyEmptyCell(cell)) return false
  cell.textContent = ''
  return true
}

const getRangeInsideEditorTableCell = (cell: HTMLTableCellElement) => {
  const selection = typeof window === 'undefined' ? null : window.getSelection()
  if (!selection?.rangeCount) return null as Range | null
  const currentRange = selection.getRangeAt(0)
  if (getEditorTableCellFromRange(currentRange) === cell) return currentRange
  return null
}

const insertTextIntoCellDom = (cell: HTMLTableCellElement, text: string) => {
  const selection = typeof window === 'undefined' ? null : window.getSelection()
  if (!selection || !text) return false
  let range: Range | null = getRangeInsideEditorTableCell(cell)
  if (clearEditorTableEmptyPlaceholder(cell) || !range) {
    range = document.createRange()
    range.selectNodeContents(cell)
    range.collapse(false)
  }
  range.deleteContents()
  const textNode = document.createTextNode(text)
  range.insertNode(textNode)
  range.setStartAfter(textNode)
  range.collapse(true)
  removeAdjacentTableCaretAnchors(textNode)
  selection.removeAllRanges()
  selection.addRange(range)
  lastEditorSelectionRange = range.cloneRange()
  storeLastEditorTableSelection(range)
  return true
}

const insertLineBreakIntoCellDom = (cell: HTMLTableCellElement) => {
  const selection = typeof window === 'undefined' ? null : window.getSelection()
  let range: Range | null = getRangeInsideEditorTableCell(cell)
  if (clearEditorTableEmptyPlaceholder(cell) || !range) {
    range = document.createRange()
    range.selectNodeContents(cell)
    range.collapse(false)
  }
  range.deleteContents()
  const lineBreak = document.createElement('br')
  const caretNode = createEditorTableCaretAnchorNode()
  range.insertNode(lineBreak)
  lineBreak.after(caretNode)
  range.setStart(caretNode, 0)
  range.collapse(true)
  selection?.removeAllRanges()
  selection?.addRange(range)
  lastEditorSelectionRange = range.cloneRange()
  storeLastEditorTableSelection(range)
  return true
}

const insertEditorTableCellLineBreak = (event: KeyboardEvent, cell: HTMLTableCellElement) => {
  const position = getEditorTableCellPosition(cell)
  if (!position) return false
  event.preventDefault()
  event.stopPropagation()
  ;(event as Event & { stopImmediatePropagation?: () => void }).stopImmediatePropagation?.()
  if (!insertLineBreakIntoCellDom(cell)) return false
  pendingEditorTableCellSync = { ...position, text: editorTableCellTextFromDom(cell) }
  scheduleStabilizePendingEditorTableCellDom()
  emitEditorValue()
  refreshAttachmentLinksFromEditor()
  return true
}

const restoreLastEditorSelection = () => {
  if (typeof window === 'undefined' || !editorContainer.value || (!lastEditorTableSelectionRange && !lastEditorTableSelectionState)) return false
  const rangeCell = getEditorTableCellFromRange(lastEditorTableSelectionRange)
  const cell = rangeCell && getEditorEditableFromNode(rangeCell)
    ? rangeCell
    : getStoredEditorTableCell(lastEditorTableSelectionState?.editable || null)
  if (!cell) return false
  const selection = window.getSelection()
  if (!selection) return false
  try {
    selection.removeAllRanges()
    if (rangeCell && getEditorEditableFromNode(rangeCell) && lastEditorTableSelectionRange) {
      selection.addRange(lastEditorTableSelectionRange.cloneRange())
    } else {
      const range = document.createRange()
      range.selectNodeContents(cell)
      range.collapse(false)
      selection.addRange(range)
      lastEditorTableSelectionRange = range.cloneRange()
    }
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
  const shouldRestoreTableSelection = hasAttachmentMarker(text)
  let cell = getCurrentEditorTableCell(undefined, { allowStoredFallback: shouldRestoreTableSelection })
  if (!cell && shouldRestoreTableSelection && restoreLastEditorSelection()) {
    cell = getCurrentEditorTableCell(undefined, { allowStoredFallback: true })
  }
  if (!cell) return false
  if (hasAttachmentMarker(text)) {
    const target = getEditorTableCellSourceTarget(cell)
    if (target) {
      const current = target.rowCells[target.cellIndex] || ''
      const separator = current && !/\s$/.test(current) ? ' ' : ''
      return applyEditorTableCellSourceValue(cell, `${current}${separator}${text}`, { restoreCaret: true })
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
  syncEditorTableCellDomToSource(cell, { restoreCaret: true })
  const updatedSelection = window.getSelection()
  if (updatedSelection && updatedSelection.rangeCount > 0) {
    lastEditorSelectionRange = updatedSelection.getRangeAt(0).cloneRange()
    storeLastEditorTableSelection(updatedSelection.getRangeAt(0))
  }
  refreshAttachmentLinksFromEditor()
  window.setTimeout(() => emitEditorValue(), 0)
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
  emitEditorValue(nextValue)
  return true
}

defineExpose({
  clear: () => {
    pendingEditorTableCellSync = null
    editorTableCompositionActive = false
    editorTableCompositionTarget = null
    editorTableCompositionCommitKey = null
    editorTableCompositionCaretTarget = null
    if (vditorInstance) {
      vditorInstance.setValue('');
      emit("update:modelValue", '');
    }
  },
  insertValue: (val: string) => {
    if (vditorInstance) {
      if (insertValueIntoCurrentTableCell(val)) return
      if (pendingEditorTableCellSync) flushPendingEditorTableCellSourceSync()
      if (insertAttachmentSourceValue(val)) return
      const nextValue = normalizeAttachmentInsertValue(val)
      vditorInstance.insertValue(nextValue);
      normalizeEditorAttachmentSource()
      refreshAttachmentLinksFromEditor()
      window.setTimeout(() => refreshAttachmentLinksFromEditor(), 0)
      emitEditorValue();
    }
  },
  getValue: (): string => {
    return vditorInstance ? getEditorValueWithPendingTableSync() : ''
  },
  setValue: (val: string) => {
    pendingEditorTableCellSync = null
    editorTableCompositionActive = false
    editorTableCompositionTarget = null
    editorTableCompositionCommitKey = null
    editorTableCompositionCaretTarget = null
    const safeValue = ensureSafeEditorTableMarkdown(encodeMarkdownExtraBlankLines(val))
    if (vditorInstance) {
      vditorInstance.setValue(safeValue)
      emitEditorValue()
    } else {
      emit("update:modelValue", safeValue || '')
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

.vditor-container .vditor-ir pre.vditor-reset > p[data-block],
.vditor-container .vditor-ir pre.vditor-reset > div[data-block],
.vditor-container .vditor-wysiwyg .vditor-reset > p[data-block],
.vditor-container .vditor-wysiwyg .vditor-reset > div[data-block] {
  margin-block: 0 !important;
  min-height: 1.5em;
}

.vditor-container .vditor-preserved-blank-line {
  margin-block: 0 !important;
  min-height: 1.5em;
}

.vditor-container .vditor-preview .markdown-preserved-blank-line {
  min-height: 1.5em;
  margin-block: 0 !important;
  white-space: pre-wrap;
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
  position: relative;
  box-sizing: border-box;
  display: block;
  width: max-content;
  min-width: 100%;
  max-width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  border-collapse: collapse;
  padding-inline-end: var(--editor-table-scroll-edge-gap, 0px);
  scroll-padding-inline-end: var(--editor-table-scroll-edge-gap, 0px);
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
  font-weight: 400 !important;
  text-align: left;
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
  scrollbar-gutter: stable;
  scrollbar-width: thin;
  scrollbar-color: rgba(100, 116, 139, 0.62) rgba(148, 163, 184, 0.18);
}

.editor-table-expand-scroll:not(.has-real-horizontal-overflow) {
  overflow-x: hidden;
}

.editor-table-expand-scroll:not(.has-real-vertical-overflow) {
  overflow-y: hidden;
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
  min-width: 0;
  border-collapse: collapse;
  table-layout: fixed;
}

.editor-table-expand-table th,
.editor-table-expand-table td {
  box-sizing: border-box;
  position: relative;
  min-width: 48px;
  padding: 0;
  border: 1px solid rgba(148, 163, 184, 0.42);
  vertical-align: top;
  background: rgba(255, 255, 255, 0.94);
  font-weight: 400;
}

.editor-table-expand-cell {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  min-height: 38px;
}

.editor-table-expand-overlay.is-dark .editor-table-expand-table th,
.editor-table-expand-overlay.is-dark .editor-table-expand-table td {
  border-color: rgba(226, 232, 240, 0.20);
  background: rgba(30, 41, 59, 0.74);
  font-weight: 400;
}

.editor-table-expand-table textarea {
  box-sizing: border-box;
  display: block;
  width: 100%;
  flex: 1 1 auto;
  min-width: 44px;
  min-height: 38px;
  height: 100%;
  padding: 7px 8px;
  border: 0;
  outline: none;
  resize: none;
  overflow: hidden;
  background: transparent;
  color: inherit;
  font: inherit;
  line-height: 1.45;
  white-space: pre-wrap;
  overflow-wrap: break-word;
}

.editor-table-expand-row-resize-handle,
.editor-table-expand-column-resize-handle {
  position: absolute;
  z-index: 4;
  display: block;
  background: transparent;
  touch-action: none;
}

.editor-table-expand-row-resize-handle {
  left: 0;
  right: 0;
  bottom: -4px;
  height: 8px;
  cursor: row-resize;
}

.editor-table-expand-column-resize-handle {
  top: 0;
  right: -4px;
  bottom: 0;
  width: 8px;
  cursor: col-resize;
}

.editor-table-expand-row-resize-handle::after,
.editor-table-expand-column-resize-handle::after {
  content: '';
  position: absolute;
  border-radius: 999px;
  background: rgba(249, 115, 22, 0.72);
  opacity: 0;
  transition: opacity .12s ease;
}

.editor-table-expand-row-resize-handle::after {
  left: 0;
  right: 0;
  top: 50%;
  height: 2px;
  transform: translateY(-50%);
}

.editor-table-expand-column-resize-handle::after {
  top: 0;
  bottom: 0;
  left: 50%;
  width: 2px;
  transform: translateX(-50%);
}

.editor-table-expand-row-resize-handle:hover::after,
.editor-table-expand-row-resize-handle:focus-visible::after,
.editor-table-expand-column-resize-handle:hover::after,
.editor-table-expand-column-resize-handle:focus-visible::after,
body.is-resizing-expanded-table-row .editor-table-expand-row-resize-handle::after,
body.is-resizing-expanded-table-column .editor-table-expand-column-resize-handle::after {
  opacity: 1;
}

body.is-resizing-expanded-table-row {
  cursor: row-resize !important;
}

body.is-resizing-expanded-table-column {
  cursor: col-resize !important;
}

.editor-table-expand-table textarea:focus {
  box-shadow: inset 0 0 0 2px rgba(249, 115, 22, 0.48);
}

.editor-table-expand-table textarea[readonly] {
  cursor: default;
}

:global(.editor-inline-table-cell-textarea) {
  position: fixed;
  z-index: 10024;
  box-sizing: border-box;
  display: block;
  min-width: 1px;
  min-height: 1px;
  margin: 0;
  border: 0;
  outline: none;
  resize: none;
  overflow: hidden;
  white-space: pre-wrap;
  overflow-wrap: break-word;
  caret-color: auto;
  border-radius: 0;
  box-shadow: none;
}

:global(.editor-inline-table-cell-bottom-shield) {
  position: fixed;
  z-index: 10025;
  display: none;
  min-width: 1px;
  min-height: 0;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: default;
  pointer-events: auto;
}

.vditor-container :deep(td.editor-inline-table-cell-editing),
.vditor-container :deep(th.editor-inline-table-cell-editing) {
  color: transparent !important;
  caret-color: transparent !important;
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
  gap: 8px !important;
  width: 324px !important;
  min-width: 324px !important;
  max-width: min(324px, calc(100vw - 16px)) !important;
  padding: 10px 24px 12px !important;
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
