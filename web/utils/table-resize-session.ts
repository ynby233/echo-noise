export type TableTrackResizeSession = {
  startPointer: number
  startBoundary: number
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
    size: Math.max(session.minSize, session.startSize + pointer - session.startBoundary),
  }
}

export const resolveTableTrailingScrollReserve = (
  startReserve: number,
  startTrackSize: number,
  nextTrackSize: number,
) => Math.max(0, startReserve + startTrackSize - nextTrackSize)
