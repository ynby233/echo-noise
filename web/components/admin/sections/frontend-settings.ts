import { useRuntimeConfig } from '#imports'

export type FrontendConfigEnvelope = {
  frontendSettings: Record<string, any>
  raw: Record<string, any>
}

export const loadFrontendSettings = async (): Promise<FrontendConfigEnvelope> => {
  const baseApi = useRuntimeConfig().public.baseApi || '/api'
  const response = await fetch(`${baseApi}/frontend/config?t=${Date.now()}`, {
    credentials: 'include',
    headers: { 'Cache-Control': 'no-cache', Pragma: 'no-cache' },
  })
  const body = await response.json().catch(() => ({}))
  if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '获取配置失败')
  return {
    frontendSettings: body.data?.frontendSettings && typeof body.data.frontendSettings === 'object' ? body.data.frontendSettings : {},
    raw: body.data && typeof body.data === 'object' ? body.data : {},
  }
}

export const saveFrontendSettings = async (fields: Record<string, any>) => {
  const baseApi = useRuntimeConfig().public.baseApi || '/api'
  const response = await fetch(`${baseApi}/settings`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ frontendSettings: fields }),
  })
  const body = await response.json().catch(() => ({}))
  if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '保存失败')
  if (typeof window !== 'undefined') window.dispatchEvent(new Event('frontend-config-updated'))
  return body
}

export const booleanSetting = (value: unknown, fallback = false) => {
  if (value === undefined || value === null) return fallback
  return value === true || value === 'true' || value === 1 || value === '1'
}
