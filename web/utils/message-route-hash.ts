export const getMessageIdFromRouteHash = (hash: unknown) => {
  const value = String(hash || '').trim()
  const match = /^#\/messages\/(\d+)(?:[/?].*)?$/.exec(value)
  return match?.[1] || ''
}

export const isMessageRouteHash = (hash: unknown) => !!getMessageIdFromRouteHash(hash)
