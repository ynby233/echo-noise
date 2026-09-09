import { nextTick, ref } from 'vue'
import { createRenderedTableEnhancer } from './rendered-table-enhancer'
import { applyTableTrackSize, getTableResizeZoomScale, resolveTableTrackResize, resolveTableTrailingScrollReserve, type TableTrackResizeSession } from './table-resize-session'

type RenderedTableDialogOptions = {
  root: () => HTMLElement | null
  enhance: (root: HTMLElement) => void
  cleanup: (root: HTMLElement) => void
}

const RENDERED_TABLE_MIN_COLUMN_WIDTH = 48
const RENDERED_TABLE_ATTACHMENT_CARD_WIDTH = 280
const RENDERED_TABLE_ATTACHMENT_VIDEO_WIDTH = 240
const RENDERED_TABLE_ATTACHMENT_IMAGE_WIDTH = 200
const RENDERED_TABLE_MIN_ROW_HEIGHT = 38
const RENDERED_TABLE_CELL_HORIZONTAL_PADDING = 18
const RENDERED_TABLE_SCROLL_OVERFLOW_TOLERANCE = 2
type RenderedTableResizeDrag = TableTrackResizeSession & {
  type: 'row' | 'column'
  index: number
  active: boolean
  startTrailingScrollReserve: number
}
type RenderedTableResizeStart = Omit<RenderedTableResizeDrag, 'active' | 'startTrailingScrollReserve'>
const estimateRenderedTableLineWidth = (line: string) => {
  const text = String(line || '') || ' '
  return Array.from(text).reduce((width, char) => {
    if (/\s/.test(char)) return width + 4
    if (/[^\x00-\xff]/.test(char)) return width + 14
    return width + 7
  }, RENDERED_TABLE_CELL_HORIZONTAL_PADDING)
}

const estimateRenderedTableCellAttachmentWidth = (cell: HTMLTableCellElement | undefined) => {
  if (!cell) return 0
  let width = 0
  if (cell.querySelector('.site-attachment-file, .site-attachment-audio, [data-site-audio-player], .site-table-audio-trigger')) {
    width = Math.max(width, RENDERED_TABLE_ATTACHMENT_CARD_WIDTH)
  }
  if (cell.querySelector('.site-attachment-render--video, video')) {
    width = Math.max(width, RENDERED_TABLE_ATTACHMENT_VIDEO_WIDTH)
  }
  if (cell.querySelector('.site-attachment-paragraph, .site-attachment-image, img')) {
    width = Math.max(width, RENDERED_TABLE_ATTACHMENT_IMAGE_WIDTH)
  }
  return width
}

export const adaptiveRenderedTableColumnWidths = (table: HTMLTableElement, availableWidth: number, minWidth = RENDERED_TABLE_MIN_COLUMN_WIDTH) => {
  const rows = Array.from(table.rows)
  const columnCount = rows.reduce((max, row) => Math.max(max, row.cells.length), 0)
  if (!columnCount) return [] as number[]
  const safeAvailable = Math.max(minWidth * columnCount, Math.floor(availableWidth || 0))
  const average = safeAvailable / columnCount
  const natural = Array.from({ length: columnCount }, (_, columnIndex) => {
    const maxLine = rows.reduce((max, row) => {
      const cell = row.cells[columnIndex]
      const text = String(cell?.textContent || '').replace(/\u00a0/g, ' ')
      const textWidth = Math.max(...text.split('\n').map((line) => estimateRenderedTableLineWidth(line)))
      const attachmentWidth = estimateRenderedTableCellAttachmentWidth(cell)
      return Math.max(max, textWidth, attachmentWidth)
    }, minWidth)
    return Math.max(minWidth, Math.ceil(maxLine))
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

const applyAdaptiveRenderedTableColumns = (table: HTMLTableElement, availableWidth: number, manualWidths: number[] = []) => {
  const widths = adaptiveRenderedTableColumnWidths(table, availableWidth).map((width, index) => Math.max(
    RENDERED_TABLE_MIN_COLUMN_WIDTH,
    Math.ceil(manualWidths[index] || width)
  ))
  if (!widths.length) return
  table.querySelector('colgroup')?.remove()
  const colgroup = document.createElement('colgroup')
  widths.forEach((width) => {
    const col = document.createElement('col')
    col.style.width = `${width}px`
    colgroup.appendChild(col)
  })
  table.insertBefore(colgroup, table.firstChild)
}

const measureRenderedTableAutoRowHeights = (table: HTMLTableElement) => {
  const rows = Array.from(table.rows)
  const previousRowHeights = rows.map((row) => row.style.height)
  const previousCellHeights = rows.map((row) => Array.from(row.cells).map((cell) => (cell as HTMLElement).style.height))
  rows.forEach((row) => {
    row.style.height = 'auto'
    Array.from(row.cells).forEach((cell) => { (cell as HTMLElement).style.height = 'auto' })
  })
  const heights = rows.map((row) => {
    const maxCellHeight = Array.from(row.cells).reduce((max, cell) => Math.max(max, Math.ceil((cell as HTMLElement).scrollHeight)), RENDERED_TABLE_MIN_ROW_HEIGHT)
    return Math.max(RENDERED_TABLE_MIN_ROW_HEIGHT, maxCellHeight)
  })
  rows.forEach((row, rowIndex) => {
    row.style.height = previousRowHeights[rowIndex] || ''
    Array.from(row.cells).forEach((cell, cellIndex) => {
      ;(cell as HTMLElement).style.height = previousCellHeights[rowIndex]?.[cellIndex] || ''
    })
  })
  return heights
}

const applyRenderedTableRowHeights = (table: HTMLTableElement, manualHeights: number[] = []) => {
  const autoHeights = measureRenderedTableAutoRowHeights(table)
  Array.from(table.rows).forEach((row, rowIndex) => {
    const height = Math.max(
      RENDERED_TABLE_MIN_ROW_HEIGHT,
      Math.ceil(autoHeights[rowIndex] || 0),
      Math.ceil(manualHeights[rowIndex] || 0)
    )
    row.style.height = `${height}px`
    Array.from(row.cells).forEach((cell) => { (cell as HTMLElement).style.height = `${height}px` })
  })
  return autoHeights
}

// Owns the expanded table, resize state, event listeners and scheduled work.
// Mount before opening; update after preview changes; dispose before dropping the root.
// Sizing algorithms are moved unchanged from the renderer.
export const createRenderedTableDialog = (options: RenderedTableDialogOptions) => {
  const renderedTableExpandBody = ref<HTMLDivElement | null>(null)
  const showRenderedTableExpandDialog = ref(false)
  const renderedTableExpandClosing = ref(false)
  const renderedTableExpandHtml = ref('')
  const renderedTableExpandDark = ref(false)
  let renderedTableExpandCloseTimer: ReturnType<typeof setTimeout> | null = null
  let renderedTableScrollOverflowFrame: number | null = null
  let renderedTableResizeDrag: RenderedTableResizeDrag | null = null
  let renderedTableManualRowHeights: number[] = []
  let renderedTableManualColumnWidths: number[] = []
  let generation = 0
  let mounted = false
const renderedTableExpandTable = () => renderedTableExpandBody.value?.querySelector<HTMLTableElement>('.rendered-table-expanded-table') || null

const renderedTableExpandAvailableWidth = () => {
  const scroll = renderedTableExpandBody.value
  const fallback = Math.min(1680, Math.max(320, window.innerWidth - 48)) - 24
  return Math.max(160, Math.floor((scroll?.clientWidth || fallback) - 24))
}

const syncRenderedTableScrollOverflowState = () => {
  const scroll = renderedTableExpandBody.value
  if (!scroll) return
  const horizontalOverflow = scroll.scrollWidth - scroll.clientWidth > RENDERED_TABLE_SCROLL_OVERFLOW_TOLERANCE
  const verticalOverflow = scroll.scrollHeight - scroll.clientHeight > RENDERED_TABLE_SCROLL_OVERFLOW_TOLERANCE
  scroll.classList.toggle('has-real-horizontal-overflow', horizontalOverflow)
  scroll.classList.toggle('has-real-vertical-overflow', verticalOverflow)
}

const scheduleRenderedTableScrollOverflowState = () => {
  if (typeof window === 'undefined') return
  if (renderedTableScrollOverflowFrame !== null) return
  renderedTableScrollOverflowFrame = window.requestAnimationFrame(() => {
    renderedTableScrollOverflowFrame = null
    syncRenderedTableScrollOverflowState()
  })
}

const stopRenderedTableResize = () => {
  const drag = renderedTableResizeDrag
  window.removeEventListener('pointermove', onRenderedTableResizeMove, true)
  window.removeEventListener('pointerup', stopRenderedTableResize, true)
  window.removeEventListener('pointercancel', stopRenderedTableResize, true)
  const table = renderedTableExpandTable()
  table?.querySelectorAll('.rendered-table-expand-row-resize-handle.is-resizing, .rendered-table-expand-column-resize-handle.is-resizing')
    .forEach((handle) => handle.classList.remove('is-resizing'))
  renderedTableResizeDrag = null
  document.body.classList.remove('is-resizing-rendered-table-row', 'is-resizing-rendered-table-column')
  if (table && drag?.type === 'column' && drag.active) {
    applyRenderedTableRowHeights(table, renderedTableManualRowHeights)
  }
  scheduleRenderedTableScrollOverflowState()
}

const syncRenderedTableExpandLayout = (options: { rebuildHandles?: boolean } = {}) => {
  const table = renderedTableExpandTable()
  if (!table) return
  applyAdaptiveRenderedTableColumns(table, renderedTableExpandAvailableWidth(), renderedTableManualColumnWidths)
  const autoRowHeights = applyRenderedTableRowHeights(table, renderedTableManualRowHeights)
  if (options.rebuildHandles !== false) ensureRenderedTableResizeHandles(table, autoRowHeights)
  scheduleRenderedTableScrollOverflowState()
}

const onRenderedTableExpandViewportResize = () => syncRenderedTableExpandLayout()

const onRenderedTableResizeMove = (event: PointerEvent) => {
  const drag = renderedTableResizeDrag
  if (!drag) return
  const table = renderedTableExpandTable()
  if (!table) return
  event.preventDefault()
  event.stopPropagation()
  const scale = drag.scale || 1
  const pointer = (drag.type === 'row' ? event.clientY : event.clientX) / scale
  const resolved = resolveTableTrackResize(drag, pointer, drag.active)
  drag.active = resolved.active
  if (!resolved.active) return
  if (drag.type === 'row') {
    const nextHeight = resolved.size
    renderedTableManualRowHeights[drag.index] = nextHeight
    applyTableTrackSize(table, 'row', drag.index, nextHeight)
    table.style.marginBottom = `${resolveTableTrailingScrollReserve(drag.startTrailingScrollReserve, drag.startSize, nextHeight)}px`
    scheduleRenderedTableScrollOverflowState()
    return
  }
  const nextWidth = resolved.size
  renderedTableManualColumnWidths[drag.index] = nextWidth
  applyTableTrackSize(table, 'column', drag.index, nextWidth)
  table.style.marginRight = `${resolveTableTrailingScrollReserve(drag.startTrailingScrollReserve, drag.startSize, nextWidth)}px`
  scheduleRenderedTableScrollOverflowState()
}

const startRenderedTableResize = (drag: RenderedTableResizeStart, event: PointerEvent) => {
  stopRenderedTableResize()
  const table = renderedTableExpandTable()
  if (!table) return
  const startTrailingScrollReserve = Number.parseFloat(drag.type === 'row' ? table.style.marginBottom : table.style.marginRight)
  renderedTableResizeDrag = {
    ...drag,
    active: false,
    startTrailingScrollReserve: Number.isFinite(startTrailingScrollReserve) ? startTrailingScrollReserve : 0,
  }
  const handleClass = drag.type === 'row'
    ? 'rendered-table-expand-row-resize-handle'
    : 'rendered-table-expand-column-resize-handle'
  table?.querySelectorAll(`.${handleClass}[data-resize-index="${drag.index}"]`)
    .forEach((handle) => handle.classList.add('is-resizing'))
  document.body.classList.add(drag.type === 'row' ? 'is-resizing-rendered-table-row' : 'is-resizing-rendered-table-column')
  event.currentTarget instanceof HTMLElement && event.currentTarget.setPointerCapture?.(event.pointerId)
  window.addEventListener('pointermove', onRenderedTableResizeMove, true)
  window.addEventListener('pointerup', stopRenderedTableResize, true)
  window.addEventListener('pointercancel', stopRenderedTableResize, true)
}

const ensureRenderedTableResizeHandles = (table: HTMLTableElement, autoRowHeights: number[] = []) => {
  table.querySelectorAll('.rendered-table-expand-row-resize-handle, .rendered-table-expand-column-resize-handle').forEach((handle) => handle.remove())
  const rows = Array.from(table.rows)
  rows.forEach((row, rowIndex) => {
    Array.from(row.cells).forEach((cell, cellIndex) => {
      const cellElement = cell as HTMLElement
      const rowHandle = document.createElement('span')
      rowHandle.className = 'rendered-table-expand-row-resize-handle'
      rowHandle.dataset.resizeIndex = String(rowIndex)
      rowHandle.setAttribute('aria-hidden', 'true')
      rowHandle.addEventListener('pointerdown', (event) => {
        event.preventDefault()
        event.stopPropagation()
        const scale = getTableResizeZoomScale()
        startRenderedTableResize({
          type: 'row',
          index: rowIndex,
          startPointer: event.clientY / scale,
          startSize: row.getBoundingClientRect().height / scale,
          minSize: Math.max(RENDERED_TABLE_MIN_ROW_HEIGHT, autoRowHeights[rowIndex] || 0),
          scale,
        }, event)
      })
      cellElement.appendChild(rowHandle)

      const columnHandle = document.createElement('span')
      columnHandle.className = 'rendered-table-expand-column-resize-handle'
      columnHandle.dataset.resizeIndex = String(cellIndex)
      columnHandle.setAttribute('aria-hidden', 'true')
      columnHandle.addEventListener('pointerdown', (event) => {
        event.preventDefault()
        event.stopPropagation()
        const scale = getTableResizeZoomScale()
        startRenderedTableResize({
          type: 'column',
          index: cellIndex,
          startPointer: event.clientX / scale,
          startSize: cellElement.getBoundingClientRect().width / scale,
          minSize: RENDERED_TABLE_MIN_COLUMN_WIDTH,
          scale,
        }, event)
      })
      cellElement.appendChild(columnHandle)
    })
  })
}

const openRenderedTableExpand = async (table: HTMLTableElement) => {
  if (!mounted || !table) return
  const currentGeneration = ++generation
  if (renderedTableExpandBody.value) options.cleanup(renderedTableExpandBody.value)
  renderedTableManualRowHeights = []
  renderedTableManualColumnWidths = []
  const clone = table.cloneNode(true) as HTMLTableElement
  enhancer.prepare(clone)
  clone.classList.add('site-scrollable-table', 'rendered-table-expanded-table')
  const availableWidth = Math.min(1680, Math.max(320, window.innerWidth - 48)) - 24
  applyAdaptiveRenderedTableColumns(clone, availableWidth, renderedTableManualColumnWidths)
  clone.querySelectorAll('button:not(.site-rendered-table-expand-button):not(.site-table-audio-trigger)').forEach((button) => button.remove())
  clone.querySelectorAll('.site-rendered-table-expand-button').forEach((button) => button.remove())
  renderedTableExpandHtml.value = clone.outerHTML
  if (renderedTableExpandCloseTimer) {
    clearTimeout(renderedTableExpandCloseTimer)
    renderedTableExpandCloseTimer = null
  }
  renderedTableExpandClosing.value = false
  showRenderedTableExpandDialog.value = true
  await nextTick()
  if (!mounted || currentGeneration !== generation) return
  if (renderedTableExpandBody.value) {
    renderedTableExpandBody.value.querySelectorAll<HTMLTableElement>('table').forEach(enhancer.prepare)
    options.enhance(renderedTableExpandBody.value)
    syncRenderedTableExpandLayout()
  }
}

const closeRenderedTableExpand = () => {
  if (!showRenderedTableExpandDialog.value || renderedTableExpandClosing.value) return
  generation += 1
  if (renderedTableExpandBody.value) options.cleanup(renderedTableExpandBody.value)
  renderedTableExpandClosing.value = true
  if (renderedTableExpandCloseTimer) clearTimeout(renderedTableExpandCloseTimer)
  renderedTableExpandCloseTimer = setTimeout(() => {
    showRenderedTableExpandDialog.value = false
    renderedTableExpandClosing.value = false
    renderedTableExpandHtml.value = ''
    renderedTableManualRowHeights = []
    renderedTableManualColumnWidths = []
    stopRenderedTableResize()
    renderedTableExpandCloseTimer = null
  }, 180)
}


  const enhancer = createRenderedTableEnhancer({
    root: options.root,
    open: (table) => { void openRenderedTableExpand(table) },
  })
  const update = () => enhancer.update()
  const mount = () => {
    if (mounted) return
    mounted = true
    enhancer.mount()
    window.addEventListener('resize', onRenderedTableExpandViewportResize, { passive: true })
  }
  const dispose = () => {
    mounted = false
    generation += 1
    enhancer.dispose()
    window.removeEventListener('resize', onRenderedTableExpandViewportResize)
    if (renderedTableExpandCloseTimer) clearTimeout(renderedTableExpandCloseTimer)
    renderedTableExpandCloseTimer = null
    stopRenderedTableResize()
    if (renderedTableScrollOverflowFrame !== null) window.cancelAnimationFrame(renderedTableScrollOverflowFrame)
    renderedTableScrollOverflowFrame = null
    if (renderedTableExpandBody.value) options.cleanup(renderedTableExpandBody.value)
    renderedTableExpandBody.value = null
    showRenderedTableExpandDialog.value = false
    renderedTableExpandClosing.value = false
    renderedTableExpandHtml.value = ''
  }
  return {
    body: renderedTableExpandBody,
    visible: showRenderedTableExpandDialog,
    closing: renderedTableExpandClosing,
    html: renderedTableExpandHtml,
    dark: renderedTableExpandDark,
    open: openRenderedTableExpand,
    close: closeRenderedTableExpand,
    mount, update, dispose,
  }
}
