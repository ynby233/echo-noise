import type { BackupKind, BackupPlan, BackupRecord, BackupTarget } from './backup-contract'
export type RestoreResult = { message?: string, pendingRestart?: boolean, restartRequired?: boolean, warnings: string[] }
export type Inspection = { kind: BackupKind, legacy: boolean, components: string[], warnings: string[], attachmentCount: number, cloudAvailable?: boolean, fallbackRequired?: boolean }
export const inspectionHasAttachments = (inspection: Inspection) => inspection.components.some(component => ['attachments', 'attachment-blobs', 'images', 'video', 'audio'].includes(component))
export class BackupFallbackRequired extends Error {}
const object = (value: unknown): Record<string, unknown> => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('备份响应格式无效')
  return Object.fromEntries(Object.entries(value))
}
const text = (value: unknown): string => { if (typeof value !== 'string') throw new Error('备份响应缺少文本字段'); return value }
const number = (value: unknown): number => { if (typeof value !== 'number' || !Number.isFinite(value)) throw new Error('备份响应缺少数值字段'); return value }
const bool = (value: unknown): boolean => { if (typeof value !== 'boolean') throw new Error('备份响应缺少状态字段'); return value }
const list = (value: unknown): unknown[] => { if (!Array.isArray(value)) throw new Error('备份响应缺少列表字段'); return value }
const strings = (value: unknown) => value == null ? [] : list(value).map(text)
const optionalText = (value: unknown) => value == null ? undefined : text(value)
const optionalBool = (value: unknown) => value == null ? undefined : bool(value)
const kind = (value: unknown): BackupKind => { if (value !== 'data' && value !== 'attachments' && value !== 'server-config') throw new Error('未知备份类型'); return value }
const target = (value: unknown): BackupTarget => { if (value !== 'local' && value !== 'cloud' && value !== 'both') throw new Error('未知备份位置'); return value }
export const parseBackupEnvelope = (value: unknown, ok: boolean): unknown => {
  const body = object(value)
  if (!ok || body.code !== 1) {
    const message = typeof body.message === 'string' ? body.message : typeof body.msg === 'string' ? body.msg : '备份操作失败，请重试'
    if (body.data && object(body.data).fallbackRequired === true) throw new BackupFallbackRequired(message)
    throw new Error(message)
  }
  return body.data
}
export const parsePlan = (value: unknown): BackupPlan => {
  const p = object(value)
  return { kind: kind(p.kind), enabled: bool(p.enabled), time: text(p.time), timezone: text(p.timezone), keepLocal: number(p.keepLocal), keepCloud: number(p.keepCloud), target: target(p.target), scope: text(p.scope), lastRun: optionalText(p.lastRun), nextRun: optionalText(p.nextRun) }
}
export const parseRecord = (value: unknown): BackupRecord => {
  const p = object(value)
  if (p.trigger !== 'manual' && p.trigger !== 'scheduled' && p.trigger !== 'discovered') throw new Error('未知备份来源')
  return { id: text(p.id), kind: kind(p.kind), createdAt: text(p.createdAt), trigger: p.trigger, target: target(p.target), status: text(p.status), size: number(p.size), local: bool(p.local), cloud: bool(p.cloud), error: optionalText(p.error) }
}
export const parseStatus = (value: unknown) => {
  const p = object(value), rawDirectories = object(p.directories)
  const directories: Partial<Record<BackupKind, string>> = {}
  for (const key of ['data', 'attachments', 'server-config'] as const) if (rawDirectories[key] != null) directories[key] = text(rawDirectories[key])
  return { plans: list(p.plans).map(parsePlan), directories, pendingRestart: bool(p.pendingRestart), warnings: strings(p.warnings) }
}
export const parseHistory = (value: unknown) => { const p = object(value); return { records: list(p.records).map(parseRecord), pendingRestart: bool(p.pendingRestart), warnings: strings(p.warnings) } }
export const parseRestore = (value: unknown): RestoreResult => {
  const p = object(value)
  return { message: optionalText(p.message), pendingRestart: optionalBool(p.pendingRestart), restartRequired: optionalBool(p.restartRequired), warnings: strings(p.warnings) }
}
export const parseInspection = (value: unknown): Inspection => {
  const p = object(value)
  return { kind: kind(p.kind), legacy: bool(p.legacy), components: strings(p.components), warnings: strings(p.warnings), attachmentCount: number(p.attachmentCount), cloudAvailable: optionalBool(p.cloudAvailable), fallbackRequired: optionalBool(p.fallbackRequired) }
}
export const parseMessage = (value: unknown) => { const p = object(value); return { message: optionalText(p.message) } }
