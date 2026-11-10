export interface AuthViaRecord {
  auth_via?: string | null
  AuthVia?: string | null
}

export const authViaDescription = '认证来源依据请求使用的认证凭据记录，不代表真实操作者；网页会话也可能由自动化工具使用，不能据此证明人工操作。历史记录缺少来源时显示“未知”。'

export function authViaLabel(row: AuthViaRecord): string {
  switch (row.auth_via ?? row.AuthVia) {
    case 'session': return '网页会话'
    case 'token': return '普通API'
    case 'ai_token': return 'AI专用凭据'
    default: return '未知'
  }
}
