<!-- Vditor shell: owns options, theme and component lifecycle. DOM input,
selection, tables and attachments belong to editor-dom-session; mount and
dispose must remain paired. -->
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
          <table ref="expandedTableElement" class="editor-table-expand-table">
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
                :key="`expanded-row-${expandedTableCellEditorRenderKey}-${rowIndex}`"
                :style="{ height: `${expandedTableRowHeight(rowIndex)}px` }"
              >
                <component
                  :is="'td'"
                  v-for="(_cell, cellIndex) in row"
                  :key="`expanded-cell-${expandedTableCellEditorRenderKey}-${rowIndex}-${cellIndex}`"
                  :style="{ width: `${expandedTableColumnWidths[cellIndex] || EXPANDED_TABLE_MIN_COLUMN_WIDTH}px`, height: `${expandedTableRowHeight(rowIndex)}px` }"
                >
                  <div class="editor-table-expand-cell" :style="{ height: `${expandedTableRowHeight(rowIndex)}px` }">
                    <div
                      :ref="(el) => registerExpandedTableCellEditor(el, rowIndex, cellIndex)"
                      class="editor-table-expand-cell-editor"
                      :contenteditable="expandedTableEditable ? 'true' : 'false'"
                      role="textbox"
                      aria-multiline="true"
                      spellcheck="false"
                      :data-expanded-row="rowIndex"
                      :data-expanded-cell="cellIndex"
                      @input="updateExpandedTableCellText(rowIndex, cellIndex, $event)"
                      @keydown.enter.exact="insertExpandedTableCellLineBreak(rowIndex, cellIndex, $event)"
                      @keydown.tab.prevent="focusNextExpandedTableCell(rowIndex, cellIndex, $event.shiftKey)"
                      @keydown.delete="removeExpandedTableCellAttachmentMarker(rowIndex, cellIndex, $event)"
                      @paste.prevent="pasteIntoExpandedTableCell(rowIndex, cellIndex, $event)"
                      @mousedown="onExpandedTableCellMarkerPointerDown"
                      @click="onExpandedTableCellMarkerClick"
                      @keydown.space="onExpandedTableCellMarkerKeyActivate"
                    />
                  </div>
                  <span
                    :class="['editor-table-expand-row-resize-handle', {
                      'is-resizing': expandedTableActiveResize?.type === 'row' && expandedTableActiveResize.index === rowIndex,
                    }]"
                    aria-hidden="true"
                    @pointerdown.prevent.stop="startExpandedTableRowResize(rowIndex, $event)"
                  ></span>
                  <span
                    :class="['editor-table-expand-column-resize-handle', {
                      'is-resizing': expandedTableActiveResize?.type === 'column' && expandedTableActiveResize.index === cellIndex,
                    }]"
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
import { ref, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { useToast } from '#imports'
import Vditor from 'vditor'
import 'vditor/dist/index.css'
import { createEditorDomSession } from '~/utils/editor-dom-session'
import { createStickyEditorToolbar } from '~/utils/sticky-editor-toolbar'
import { createVditorLifecycle } from '~/utils/vditor-lifecycle'

const props = defineProps({
  modelValue: { type: String, default: '' },
  theme: { type: String, default: 'classic' },
})
const emit = defineEmits(['update:modelValue', 'ready'])
const toast = useToast()
const editorContainer = ref<HTMLElement>()
const stickyToolbar = createStickyEditorToolbar({
  container: () => editorContainer.value || null,
  onReady: () => editorSession.update(),
})
const editorSession = createEditorDomSession({
  root: () => editorContainer.value || null,
  toolbar: () => stickyToolbar.element(),
  onChange: value => emit('update:modelValue', value),
  onPreviewError: () => toast.add({ title: '预览加载失败', description: '请再次点击附件重试。', color: 'red' }),
})
const {
  showHeadingMenu,
  headingMenuRef,
  headingMenuStyle,
  headingOptions,
  selectedHeadingTag,
  selectHeading,
  showTableMenu,
  tableMenuRef,
  tableMenuStyle,
  tableRows,
  tableCols,
  adjustTableRows,
  adjustTableCols,
  tableGridCells,
  previewTableSize,
  insertTable,
  showTableDeleteButton,
  tableDeleteButtonStyle,
  tableExpandButtonStyle,
  cancelTableDeleteHide,
  scheduleTableDeleteHide,
  confirmDeleteHoveredTable,
  openHoveredTableExpand,
  showTableExpandDialog,
  tableExpandClosing,
  closeExpandedTable,
  expandedTableEditable,
  expandedTableElement,
  expandedTableColumnWidths,
  expandedTableRows,
  expandedTableCellEditorRenderKey,
  expandedTableRowHeight,
  EXPANDED_TABLE_MIN_COLUMN_WIDTH,
  registerExpandedTableCellEditor,
  updateExpandedTableCellText,
  insertExpandedTableCellLineBreak,
  focusNextExpandedTableCell,
  removeExpandedTableCellAttachmentMarker,
  pasteIntoExpandedTableCell,
  onExpandedTableCellMarkerPointerDown,
  onExpandedTableCellMarkerClick,
  onExpandedTableCellMarkerKeyActivate,
  expandedTableActiveResize,
  startExpandedTableRowResize,
  startExpandedTableColumnResize
} = editorSession.controls

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
  input: (content: string) => editorSession.input(content),
  preview: {
    hljs: {
      style: "native",
    },
    markdown: {  
      listStyle: true,
      mark: true,
    },
    transform: (html: string) => editorSession.preview(html),
    actions: [],
  },
  placeholder: "灵感记录~"
};


const vditorLifecycle = createVditorLifecycle({
  container: () => editorContainer.value || null,
  create: (container, options) => new Vditor(container, options),
  options: () => ({
    ...editorOptions,
    theme: props.theme === 'dark' ? 'dark' : 'classic',
    preview: {
      ...editorOptions.preview,
      hljs: { style: props.theme === 'dark' ? 'native' : 'github' },
    },
  }),
  onInstance: instance => { if (instance) editorSession.mount(instance) },
  onReady: (instance) => {
    editorSession.update(props.modelValue)
    instance.setTheme(props.theme === 'dark' ? 'dark' : 'classic')
    emit('ready')
    nextTick(() => {
      if (vditorLifecycle.ready()) stickyToolbar.mount()
    })
  },
})

onMounted(() => {
  vditorLifecycle.mount()
  nextTick(() => {
    if (editorContainer.value?.isConnected) stickyToolbar.mount()
  })
})
onBeforeUnmount(() => {
  editorSession.dispose()
  stickyToolbar.dispose()
  vditorLifecycle.dispose()
})
defineExpose(editorSession.api)
watch(() => props.theme, newTheme => {
  vditorLifecycle.setTheme(newTheme === 'dark' ? 'dark' : 'classic')
})
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

.vditor-container .vditor-preview .vditor-reset {
  font-family: inherit;
}

.vditor-container .editor-attachment-link,
.vditor-container .editor-attachment-link * {
  cursor: pointer !important;
}

.vditor-container .editor-attachment-link {
  display: inline;
  max-width: 100%;
  color: var(--ir-bracket-color, #0000ff);
  text-decoration: underline;
  text-underline-offset: 2px;
  overflow-wrap: anywhere;
  word-break: break-word;
  -webkit-user-drag: none;
  pointer-events: auto;
}

.vditor-container .editor-table-attachment-marker {
  display: inline;
  max-width: 100%;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: var(--ir-bracket-color, #0000ff);
  line-height: inherit;
  text-decoration: underline;
  text-underline-offset: 2px;
  vertical-align: baseline;
  white-space: normal;
  overflow-wrap: anywhere;
  word-break: break-word;
  user-select: none;
  cursor: pointer;
}

html.dark .vditor-container .editor-table-attachment-marker,
.vditor--dark .editor-table-attachment-marker {
  border-color: transparent;
  background: transparent;
  color: var(--ir-bracket-color, #93c5fd);
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

.vditor-container .vditor-preserved-blank-line,
.vditor-container .vditor-plain-empty-line {
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
  min-width: 0;
  min-height: 38px;
}

.editor-table-expand-overlay.is-dark .editor-table-expand-table th,
.editor-table-expand-overlay.is-dark .editor-table-expand-table td {
  border-color: rgba(226, 232, 240, 0.20);
  background: rgba(30, 41, 59, 0.74);
  font-weight: 400;
}

.editor-table-expand-cell-editor {
  box-sizing: border-box;
  display: block;
  width: 100%;
  flex: 1 1 auto;
  min-width: 0;
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
  word-break: break-word;
  caret-color: auto;
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
  bottom: -0.5px;
  height: 1px;
  cursor: var(--table-row-resize-cursor);
}

.editor-table-expand-column-resize-handle {
  top: 0;
  right: -0.5px;
  bottom: 0;
  width: 1px;
  cursor: var(--table-column-resize-cursor);
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
.editor-table-expand-row-resize-handle.is-resizing::after,
.editor-table-expand-column-resize-handle.is-resizing::after {
  opacity: 1;
}

body.is-resizing-expanded-table-row,
body.is-resizing-expanded-table-row * {
  cursor: var(--table-row-resize-cursor) !important;
  user-select: none !important;
}

body.is-resizing-expanded-table-column,
body.is-resizing-expanded-table-column * {
  cursor: var(--table-column-resize-cursor) !important;
  user-select: none !important;
}

.editor-table-expand-cell-editor:focus {
  box-shadow: inset 0 0 0 2px rgba(249, 115, 22, 0.48);
}

.editor-table-expand-cell-editor[contenteditable="false"] {
  cursor: default;
}

.editor-inline-table-cell-textarea {
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

.editor-inline-table-cell-atomic-editor {
  position: fixed;
  z-index: 10024;
  box-sizing: border-box;
  display: block;
  min-width: 1px;
  min-height: 1px;
  margin: 0;
  border: 0;
  outline: none;
  overflow: hidden;
  white-space: pre-wrap;
  overflow-wrap: break-word;
  word-break: break-word;
  caret-color: auto;
  border-radius: 0;
  box-shadow: none;
}

.editor-inline-table-cell-atomic-editor .editor-table-attachment-marker {
  display: inline;
  max-width: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--ir-bracket-color, #0000ff);
  line-height: inherit;
  text-decoration: underline;
  text-underline-offset: 2px;
  vertical-align: baseline;
  white-space: normal;
  overflow-wrap: anywhere;
  word-break: break-word;
  user-select: none;
  cursor: pointer;
}

html.dark .editor-inline-table-cell-atomic-editor .editor-table-attachment-marker {
  color: var(--ir-bracket-color, #93c5fd);
}

.vditor-container :deep(td.editor-inline-table-cell-editing),
.vditor-container :deep(th.editor-inline-table-cell-editing) {
  color: transparent !important;
  caret-color: transparent !important;
}

.vditor-container :deep(td.editor-inline-table-cell-editing .editor-table-attachment-marker),
.vditor-container :deep(th.editor-inline-table-cell-editing .editor-table-attachment-marker) {
  color: transparent !important;
  text-decoration-color: transparent !important;
  pointer-events: none !important;
}

.editor-table-expand-cell-editor .editor-table-attachment-marker {
  display: inline;
  max-width: 100%;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: var(--ir-bracket-color, #0000ff);
  font: inherit;
  line-height: inherit;
  text-decoration: underline;
  text-underline-offset: 2px;
  vertical-align: baseline;
  white-space: normal;
  overflow-wrap: anywhere;
  word-break: break-word;
  user-select: none;
  cursor: pointer;
}

.editor-table-expand-overlay.is-dark .editor-table-expand-cell-editor .editor-table-attachment-marker {
  color: var(--ir-bracket-color, #93c5fd);
}

.editor-table-expand-cell-editor .editor-table-attachment-marker:hover,
.editor-table-expand-cell-editor .editor-table-attachment-marker:focus-visible {
  outline: none;
  text-decoration-thickness: 2px;
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
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes editorTableDialogOut {
  from { opacity: 1; }
  to { opacity: 0; }
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
