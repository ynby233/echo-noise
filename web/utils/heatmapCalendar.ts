export interface HeatItem { date: string; count: number }
export interface CalendarDay { date: string; count: number; level: number }

const formatDate = (date: Date) => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

// Use local calendar dates throughout, including across daylight-saving changes.
export const generateHeatmapCalendar = (items: readonly HeatItem[], now = new Date()): CalendarDay[][] => {
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  // Construct on day 1 so subtracting months cannot overflow at month end.
  const current = new Date(today.getFullYear(), today.getMonth() - 11, 1)
  current.setDate(current.getDate() - current.getDay())
  const end = new Date(today)
  end.setDate(end.getDate() + 6 - end.getDay())
  const todayKey = formatDate(today)
  const dateMap = new Map<string, number>()
  for (const item of items) {
    if (item && item.date && item.count !== undefined) dateMap.set(item.date, item.count)
  }

  const calendar: CalendarDay[][] = []
  while (current <= end) {
    const week: CalendarDay[] = []
    for (let day = 0; day < 7; day++) {
      const date = formatDate(current)
      const count = date <= todayKey ? dateMap.get(date) || 0 : 0
      week.push({ date, count, level: count ? Math.min(Math.ceil(count / 2), 5) : 0 })
      current.setDate(current.getDate() + 1)
    }
    calendar.push(week)
  }
  return calendar
}
