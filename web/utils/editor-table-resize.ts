import { ref, type Ref } from 'vue'
import { applyTableTrackSize, resolveTableTrackResize, resolveTableTrailingScrollReserve, type TableTrackResizeSession } from './table-resize-session'

type ExpandedTableResizeDrag = TableTrackResizeSession & {
  type: 'row' | 'column'
  index: number
  active: boolean
  startTrailingScrollReserve: number
}
export type ExpandedTableResizeStart = Omit<ExpandedTableResizeDrag, 'active' | 'startTrailingScrollReserve'>


// Owns one expanded table drag and its global pointer listeners. Geometry uses
// the existing resize session; the editor supplies the table and sizing state.
export const createEditorTableResize = (options: {
  table: () => HTMLTableElement | null
  rowHeights: Ref<number[]>
  columnWidths: Ref<number[]>
  onColumnResizeEnd: () => void
  onResize: () => void
}) => {
  let expandedTableResizeDrag: ExpandedTableResizeDrag | null = null
  const expandedTableActiveResize = ref<Pick<ExpandedTableResizeDrag, 'type' | 'index'> | null>(null)
  const finishResize = (notify: boolean) => {
    const drag = expandedTableResizeDrag
    if (typeof window !== 'undefined') {
      window.removeEventListener('pointermove', onExpandedTableResizeMove, true)
      window.removeEventListener('pointerup', stopExpandedTableResize, true)
      window.removeEventListener('pointercancel', stopExpandedTableResize, true)
    }
    expandedTableResizeDrag = null
    expandedTableActiveResize.value = null
    if (typeof document !== 'undefined') {
      document.body.classList.remove('is-resizing-expanded-table-row', 'is-resizing-expanded-table-column')
    }
    if (notify) {
      if (drag?.type === 'column' && drag.active) options.onColumnResizeEnd()
      options.onResize()
    }
  }
  const stopExpandedTableResize = () => finishResize(true)

  const onExpandedTableResizeMove = (event: PointerEvent) => {
    const drag = expandedTableResizeDrag
    if (!drag) return
    event.preventDefault()
    event.stopPropagation()
    const scale = drag.scale || 1
    const pointer = (drag.type === 'row' ? event.clientY : event.clientX) / scale
    const resolved = resolveTableTrackResize(drag, pointer, drag.active)
    drag.active = resolved.active
    if (!resolved.active) return
    if (drag.type === 'row') {
      const nextHeight = resolved.size
      const heights = [...options.rowHeights.value]
      heights[drag.index] = nextHeight
      options.rowHeights.value = heights
      const table = options.table()
      if (table) {
        applyTableTrackSize(table, 'row', drag.index, nextHeight)
        table.style.marginBottom = `${resolveTableTrailingScrollReserve(drag.startTrailingScrollReserve, drag.startSize, nextHeight)}px`
      }
      options.onResize()
      return
    }
    const nextWidth = resolved.size
    const widths = [...options.columnWidths.value]
    widths[drag.index] = nextWidth
    options.columnWidths.value = widths
    const table = options.table()
    if (table) {
      applyTableTrackSize(table, 'column', drag.index, nextWidth)
      table.style.marginRight = `${resolveTableTrailingScrollReserve(drag.startTrailingScrollReserve, drag.startSize, nextWidth)}px`
    }
    options.onResize()
  }

  const startExpandedTableResize = (drag: ExpandedTableResizeStart, event: PointerEvent) => {
    if (typeof window === 'undefined') return
    stopExpandedTableResize()
    const table = options.table()
    const startTrailingScrollReserve = Number.parseFloat(drag.type === 'row' ? table?.style.marginBottom || '' : table?.style.marginRight || '')
    expandedTableResizeDrag = {
      ...drag,
      active: false,
      startTrailingScrollReserve: Number.isFinite(startTrailingScrollReserve) ? startTrailingScrollReserve : 0,
    }
    expandedTableActiveResize.value = { type: drag.type, index: drag.index }
    document.body.classList.add(drag.type === 'row' ? 'is-resizing-expanded-table-row' : 'is-resizing-expanded-table-column')
    event.currentTarget instanceof HTMLElement && event.currentTarget.setPointerCapture?.(event.pointerId)
    window.addEventListener('pointermove', onExpandedTableResizeMove, true)
    window.addEventListener('pointerup', stopExpandedTableResize, true)
    window.addEventListener('pointercancel', stopExpandedTableResize, true)
  }


  return { start: startExpandedTableResize, stop: stopExpandedTableResize, active: expandedTableActiveResize, dispose: () => finishResize(false) }
}
