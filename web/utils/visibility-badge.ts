export type VisibilityBadgeContext = {
  visibility: unknown
  isAdmin: boolean
  isAuthenticated: boolean
  isOwner: boolean
}

const normalizeVisibility = (visibility: unknown) => {
  const value = String(visibility || 'public').trim().toLowerCase()
  if (value === 'users' || value === 'contacts' || value === 'private') return value
  return 'public'
}

export const shouldShowVisibilityBadge = ({
  visibility,
  isAdmin,
  isAuthenticated,
  isOwner,
}: VisibilityBadgeContext) => {
  if (isAdmin) return true
  const normalizedVisibility = normalizeVisibility(visibility)
  if (normalizedVisibility === 'public') return true
  return isAuthenticated && isOwner
}
