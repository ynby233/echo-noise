export type FilteredLifecycleAction = 'trash' | 'restore' | 'permanent-delete'

export const filteredLifecycleConfirmationMessage = (action: FilteredLifecycleAction, count: number) => {
  const safeCount = Math.max(0, Math.trunc(Number(count) || 0))
  switch (action) {
    case 'trash':
      return `将把当前筛选结果中的 ${safeCount} 条笔记移入回收站，是否继续？`
    case 'restore':
      return `将恢复当前筛选结果中的 ${safeCount} 条笔记，是否继续？`
    case 'permanent-delete':
      return `将永久删除当前筛选结果中的 ${safeCount} 条回收站笔记，且不可恢复。是否继续？`
  }
}

export const runConfirmedFilteredLifecycle = async <T>(
  action: FilteredLifecycleAction,
  count: number,
  confirm: (message: string) => boolean,
  request: () => Promise<T>,
): Promise<T | undefined> => {
  if (!confirm(filteredLifecycleConfirmationMessage(action, count))) return undefined
  return request()
}
