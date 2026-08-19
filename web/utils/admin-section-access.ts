export const resolveAccessibleAdminSection = <T extends string>(active: T, accessible: T[], fallback: T): T => {
  if (accessible.includes(active)) return active
  if (accessible.includes(fallback)) return fallback
  return accessible[0] || fallback
}
