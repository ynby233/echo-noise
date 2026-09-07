export type HomePagePosition = { tab: 'latest' | 'personal' | 'feed'; page: number }
const key = 'home-page-position'

export const parseHomePagePosition = (raw: string | null): HomePagePosition | null => {
  try {
    const value = JSON.parse(raw || 'null')
    if (!value || !['latest', 'personal', 'feed'].includes(value.tab) || !Number.isSafeInteger(value.page) || value.page < 1) return null
    return { tab: value.tab, page: value.page }
  } catch { return null }
}

export const readReloadPagePosition = (): HomePagePosition | null => {
  if (typeof window === 'undefined') return null
  try {
    const navigation = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined
    if (navigation?.type !== 'reload') return null
    const url = new URL(window.location.href)
    if (url.hash || url.searchParams.has('message_id') || url.searchParams.has('notification_id')) return null
    const saved = parseHomePagePosition(sessionStorage.getItem(key))
    if (url.searchParams.has('tab') && url.searchParams.get('tab') !== saved?.tab) return null
    return saved
  } catch { return null }
}

export const savePagePosition = (position: HomePagePosition | null) => {
  try {
    if (position) sessionStorage.setItem(key, JSON.stringify(position))
    else sessionStorage.removeItem(key)
  } catch { /* Storage restrictions must not prevent navigation. */ }
}
