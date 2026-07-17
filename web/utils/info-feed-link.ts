export const resolveInfoFeedLink = (raw: unknown, browserOrigin = '') => {
  const value = String(raw || '').trim()
  if (!value) return ''
  const origin = String(browserOrigin || '').trim()
  if (!origin) return value
  try {
    return new URL(value, origin).toString()
  } catch {
    return value
  }
}

export const getInfoFeedLinkHost = (raw: unknown, browserOrigin = '') => {
  const resolved = resolveInfoFeedLink(raw, browserOrigin)
  if (!resolved) return ''
  try {
    return new URL(resolved).host
  } catch {
    return resolved
  }
}
