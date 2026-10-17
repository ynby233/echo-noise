import assert from 'node:assert/strict'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'esbuild'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const result = await build({
  entryPoints: [join(webRoot, 'utils/heatmapCalendar.ts')],
  bundle: true,
  write: false,
  format: 'esm',
  platform: 'node',
  target: 'node18'
})
const moduleURL = `data:text/javascript;base64,${Buffer.from(result.outputFiles[0].text).toString('base64')}`
const { generateHeatmapCalendar } = await import(moduleURL)
const localDate = (year, month, day) => new Date(year, month - 1, day, 15, 30)
const dayByDate = (calendar, date) => calendar.flat().find(day => day.date === date)

const assertWindow = (calendar, start, end, today) => {
  assert.equal(calendar[0][0].date, start, `window start for ${today}`)
  assert.equal(calendar.at(-1).at(-1).date, end, `window end for ${today}`)
  assert.ok(dayByDate(calendar, today), `today ${today} must be included`)
  assert.equal(new Date(`${start}T00:00:00Z`).getUTCDay(), 0)
  assert.equal(new Date(`${end}T00:00:00Z`).getUTCDay(), 6)
  let previous
  for (const week of calendar) {
    assert.equal(week.length, 7)
    for (const day of week) {
      const timestamp = Date.parse(`${day.date}T00:00:00Z`)
      if (previous !== undefined) assert.equal(timestamp - previous, 86400000, 'calendar dates must be contiguous')
      previous = timestamp
    }
  }
}

// The empty window includes today and the rest of its week, even late in a month.
const now = localDate(2026, 10, 18)
const empty = generateHeatmapCalendar([], now)
assertWindow(empty, '2025-10-26', '2026-10-24', '2026-10-18')
assert.ok(empty.flat().every(day => day.count === 0 && day.level === 0))

// Historical rows never expand or shift the fixed window.
const oldOnly = generateHeatmapCalendar([{ date: '2000-01-01', count: 12 }], now)
assert.deepEqual(oldOnly, empty)
const recentItems = [
  { date: '2026-10-16', count: 1 },
  { date: '2026-10-17', count: 4 },
  { date: '2026-10-18', count: 15 }
]
const recent = generateHeatmapCalendar(recentItems, now)
assert.deepEqual(generateHeatmapCalendar([
  ...recentItems,
  { date: '1900-01-01', count: 1000 }
], now), recent, 'adding ancient notes must leave the entire calendar unchanged')
assertWindow(recent, '2025-10-26', '2026-10-24', '2026-10-18')
assert.deepEqual(dayByDate(recent, '2026-10-16'), { date: '2026-10-16', count: 1, level: 1 })
assert.deepEqual(dayByDate(recent, '2026-10-17'), { date: '2026-10-17', count: 4, level: 2 })
assert.deepEqual(dayByDate(recent, '2026-10-18'), { date: '2026-10-18', count: 15, level: 5 })
assert.deepEqual(dayByDate(recent, '2026-10-15'), { date: '2026-10-15', count: 0, level: 0 })

const future = generateHeatmapCalendar([
  ...recentItems,
  { date: '2026-10-19', count: 3 },
  { date: '2026-10-24', count: 20 },
  { date: '2099-12-31', count: 100 }
], now)
assert.deepEqual(future, recent, 'future notes must neither contribute counts nor change the window')

// Month-end construction must set day 1 before subtracting months.
for (const [year, month, day, start, end, today] of [
  [2024, 3, 31, '2023-03-26', '2024-04-06', '2024-03-31'],
  [2024, 2, 29, '2023-02-26', '2024-03-02', '2024-02-29'],
  [2025, 1, 31, '2024-01-28', '2025-02-01', '2025-01-31'],
  [2025, 12, 31, '2024-12-29', '2026-01-03', '2025-12-31'],
  [2024, 8, 31, '2023-08-27', '2024-08-31', '2024-08-31']
]) {
  assertWindow(generateHeatmapCalendar([], localDate(year, month, day)), start, end, today)
}

const leapNow = localDate(2024, 3, 31)
const leap = generateHeatmapCalendar([{ date: '2024-02-29', count: 3 }], leapNow)
assert.deepEqual(dayByDate(leap, '2024-02-29'), { date: '2024-02-29', count: 3, level: 2 })
assert.equal(dayByDate(leap, '2024-03-01').count, 0)
assert.equal(leapNow.getDate(), 31, 'generation must not mutate the supplied clock')

// Leading days complete the first Sunday-based week using the same count rules.
const leading = generateHeatmapCalendar([{ date: '2025-10-26', count: 2 }], now)
assert.deepEqual(leading[0][0], { date: '2025-10-26', count: 2, level: 1 })
assert.deepEqual(generateHeatmapCalendar([{ date: '2025-10-25', count: 2 }], now), empty)

console.log('heatmap calendar behavior tests passed')
