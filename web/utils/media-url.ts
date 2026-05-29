export const resolveMediaURL = (baseApi: string, raw: string) => {
  const value = String(raw || '').trim()
  if (!value) return ''
  if (/^https?:\/\//i.test(value) || value.startsWith('data:') || value.startsWith('blob:')) return value

  const base = String(baseApi || '/api').replace(/\/+$/, '') || '/api'
  const path = value.startsWith('/') ? value : `/${value}`

  // 后端上传接口返回的是 /api/images/...；当前前端 baseApi 默认也是 /api。
  // 避免拼成 /api/api/images/... 导致真实账号头像 404 后回落到随机头像。
  if (path.startsWith('/api/') && base.endsWith('/api')) {
    return `${base.slice(0, -4)}${path}` || path
  }

  return `${base}${path}`
}
