import { getRequest, putRequest } from '~/utils/api'

export type SecurityConfig = {
  autoBanEnabled: boolean
  autoBanWindowSeconds: number
  autoBanThreshold: number
  autoBanMinutes: number
  accessLogEnabled: boolean
  siteVisitLogEnabled: boolean
  attackLogRetentionDays: number
  accessLogRetentionDays: number
  siteVisitRetentionDays: number
  loginAuditRetentionDays: number
}

export const securityConfigDefaults: SecurityConfig = {
  autoBanEnabled: false,
  autoBanWindowSeconds: 600,
  autoBanThreshold: 10,
  autoBanMinutes: 60,
  accessLogEnabled: false,
  siteVisitLogEnabled: false,
  attackLogRetentionDays: 90,
  accessLogRetentionDays: 30,
  siteVisitRetentionDays: 90,
  loginAuditRetentionDays: 365,
}

export const loadSecurityConfig = async (): Promise<SecurityConfig> => {
  const response: any = await getRequest<any>('security/config', undefined, { credentials: 'include', silent: true })
  if (response?.code !== 1) throw new Error(response?.msg || '加载安全策略失败')
  const data = response.data || {}
  return {
    autoBanEnabled: !!data.autoBanEnabled,
    autoBanWindowSeconds: Number(data.autoBanWindowSeconds ?? 600),
    autoBanThreshold: Number(data.autoBanThreshold ?? 10),
    autoBanMinutes: Number(data.autoBanMinutes ?? 60),
    accessLogEnabled: !!data.accessLogEnabled,
    siteVisitLogEnabled: !!data.siteVisitLogEnabled,
    attackLogRetentionDays: Number(data.attackLogRetentionDays ?? 90),
    accessLogRetentionDays: Number(data.accessLogRetentionDays ?? 30),
    siteVisitRetentionDays: Number(data.siteVisitRetentionDays ?? 90),
    loginAuditRetentionDays: Number(data.loginAuditRetentionDays ?? 365),
  }
}

export const saveSecurityConfig = async (config: SecurityConfig) => {
  const response: any = await putRequest<any>('security/config', {
    autoBanEnabled: !!config.autoBanEnabled,
    autoBanWindowSeconds: Number(config.autoBanWindowSeconds || 0),
    autoBanThreshold: Number(config.autoBanThreshold || 0),
    autoBanMinutes: Number(config.autoBanMinutes || 0),
    accessLogEnabled: !!config.accessLogEnabled,
    siteVisitLogEnabled: !!config.siteVisitLogEnabled,
    attackLogRetentionDays: Number(config.attackLogRetentionDays ?? 90),
    accessLogRetentionDays: Number(config.accessLogRetentionDays ?? 30),
    siteVisitRetentionDays: Number(config.siteVisitRetentionDays ?? 90),
    loginAuditRetentionDays: Number(config.loginAuditRetentionDays ?? 365),
  }, { credentials: 'include' })
  if (response?.code !== 1) throw new Error(response?.msg || '保存失败')
  return response
}

export const retentionOptions = [
  { label: '永不按时间清理', value: 0 },
  { label: '保留 7 天', value: 7 },
  { label: '保留 30 天', value: 30 },
  { label: '保留 90 天', value: 90 },
  { label: '保留 180 天', value: 180 },
  { label: '保留 365 天', value: 365 },
  { label: '保留 730 天', value: 730 },
]

export const formatShanghai = (value: any) => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return String(value).replace('T', ' ').replace('Z', '')
  return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false, timeZone: 'Asia/Shanghai' }).format(date)
}
