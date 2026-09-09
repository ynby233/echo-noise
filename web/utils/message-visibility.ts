import type { MessageVisibility } from '~/types/models'

// Canonicalizes legacy visibility spellings at the display/edit seam. Callers
// use these helpers instead of repeating the compatibility rules in each view.
export const messageVisibilityOptions: ReadonlyArray<{ value: MessageVisibility; label: string; icon: string }> = [
  { value: 'public', label: '公开', icon: 'i-mdi-earth' },
  { value: 'users', label: '成员', icon: 'i-mdi-account-group-outline' },
  { value: 'contacts', label: '联系人', icon: 'i-mdi-account-multiple-check-outline' },
  { value: 'private', label: '私密', icon: 'i-mdi-lock-outline' },
]

export const normalizeMessageVisibility = (value: unknown, fallbackPrivate = false): MessageVisibility => {
  const raw = String(value || '').trim().toLowerCase()
  if (raw === 'users' || raw === 'members' || raw === 'member' || raw === 'logged_in' || raw === 'logged-in') return 'users'
  if (raw === 'contacts') return 'contacts'
  if (raw === 'private') return 'private'
  if (raw === 'public') return 'public'
  return fallbackPrivate ? 'private' : 'public'
}

export const messageVisibility = (message: any): MessageVisibility => normalizeMessageVisibility(message?.visibility, !!message?.private)
export const messageVisibilityRequiresPrivate = (value: MessageVisibility) => value !== 'public'
export const messageVisibilityLabel = (value: unknown) => messageVisibilityOptions.find((option) => option.value === normalizeMessageVisibility(value))?.label || '公开'
export const messageVisibilityIcon = (value: unknown) => messageVisibilityOptions.find((option) => option.value === normalizeMessageVisibility(value))?.icon || 'i-mdi-earth'
