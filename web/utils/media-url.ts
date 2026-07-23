const MANAGED_ATTACHMENT_PATH_RE = /(?:^|\/)(?:api\/)?(?:images|video|audio|files|attachments|cloud-attachments)\//i
const ABSOLUTE_MANAGED_ATTACHMENT_PATH_RE = /(?:^|\/)api\/(?:images|video|audio|files|attachments|cloud-attachments)\//i

const managedAttachmentPath = (raw: string) => {
  const value = String(raw || '').trim()
  if (!value || value.startsWith('data:') || value.startsWith('blob:')) return ''
  try {
    const parsed = new URL(value, 'http://managed-attachment.invalid')
    const absolute = /^(?:https?:)?\/\//i.test(value)
    const match = (absolute ? ABSOLUTE_MANAGED_ATTACHMENT_PATH_RE : MANAGED_ATTACHMENT_PATH_RE).exec(parsed.pathname)
    if (!match || match.index < 0) return ''
    const start = match.index + (match[0].startsWith('/') ? 1 : 0)
    const path = `/${parsed.pathname.slice(start).replace(/^\/+/, '')}`
    return `${path}${parsed.search}${parsed.hash}`
  } catch {
    return ''
  }
}

export const isManagedAttachmentURL = (raw: string) => !!managedAttachmentPath(raw)

export const resolveMediaURL = (baseApi: string, raw: string) => {
  const value = String(raw || '').trim()
  if (!value) return ''
  if (/^https?:\/\//i.test(value) || value.startsWith('data:') || value.startsWith('blob:')) return value

  const base = String(baseApi || '/api').replace(/\/+$/, '') || '/api'
  const path = value.startsWith('/') ? value : `/${value}`

  const legacyApiMediaPrefixes = ['/images/', '/video/', '/audio/', '/files/', '/attachments/', '/cloud-attachments/']
  const isLegacyApiMediaPath = legacyApiMediaPrefixes.some((prefix) => path.startsWith(prefix))

  // Root-relative site assets (for example /favicon.svg) already address the
  // web origin. Only legacy media paths without /api need the API base added.
  if (value.startsWith('/') && !path.startsWith('/api/') && !isLegacyApiMediaPath) {
    return path
  }

  // 后端上传接口返回的是 /api/images/...；当前前端 baseApi 默认也是 /api。
  // 避免拼成 /api/api/images/... 导致真实账号头像 404 后回落到随机头像。
  if (path.startsWith('/api/') && base.endsWith('/api')) {
    return `${base.slice(0, -4)}${path}` || path
  }

  return `${base}${path}`
}

// Attachment references belong to this deployment, not to the host name that
// happened to be active when a note was published. Convert both current and
// legacy absolute attachment URLs back to their canonical API path, then let
// the configured API base decide the runtime origin and optional path prefix.
export const resolveManagedAttachmentURL = (baseApi: string, raw: string) => {
  const value = String(raw || '').trim()
  if (!value) return ''
  const portablePath = managedAttachmentPath(value)
  return resolveMediaURL(baseApi, portablePath || value)
}
