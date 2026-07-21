export type TableTrackResizeSession = {
  startPointer: number
  startSize: number
  minSize: number
}

export type TableTrackResizeResult = {
  active: boolean
  size: number
}

export const TABLE_RESIZE_ACTIVATION_DISTANCE = 1

export const resolveTableTrackResize = (
  session: TableTrackResizeSession,
  pointer: number,
  alreadyActive: boolean,
): TableTrackResizeResult => {
  const active = alreadyActive || Math.abs(pointer - session.startPointer) >= TABLE_RESIZE_ACTIVATION_DISTANCE
  if (!active) return { active: false, size: session.startSize }
  return {
    active: true,
    size: Math.max(session.minSize, session.startSize + pointer - session.startPointer),
  }
}

export type TableTrackAxis = 'row' | 'column'

export const applyTableTrackSize = (
  table: HTMLTableElement,
  axis: TableTrackAxis,
  index: number,
  size: number,
) => {
  if (!Number.isFinite(size) || index < 0) return
  const value = `${size}px`
  if (axis === 'row') {
    const row = table.rows[index]
    if (!row) return
    row.style.height = value
    Array.from(row.cells).forEach((cell) => {
      cell.style.height = value
    })
    return
  }

  const column = Array.from(table.querySelectorAll<HTMLTableColElement>('colgroup col'))[index]
  if (column) column.style.width = value
  Array.from(table.rows).forEach((row) => {
    const cell = row.cells[index]
    if (cell) cell.style.width = value
  })
}

export const resolveTableTrailingScrollReserve = (
  startReserve: number,
  startTrackSize: number,
  nextTrackSize: number,
) => Math.max(0, startReserve + startTrackSize - nextTrackSize)
