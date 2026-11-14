export type BackupKind = 'data' | 'attachments' | 'server-config'
export type BackupTarget = 'local' | 'cloud' | 'both'
export type AttachmentMode = 'original' | 'local'
export type BackupPlan = { kind: BackupKind, enabled: boolean, time: string, timezone: string, keepLocal: number, keepCloud: number, target: BackupTarget, scope: string, lastRun?: string, nextRun?: string }
export type BackupRecord = { id: string, kind: BackupKind, createdAt: string, trigger: 'manual' | 'scheduled' | 'discovered', target: BackupTarget, status: string, size: number, local: boolean, cloud: boolean, error?: string }
export const backupCapability = (kind: BackupKind, action: string) => `backup.${kind === 'server-config' ? 'server_config' : kind}.${action}`
export const backupLabels = { data: '数据备份', attachments: '附件备份', 'server-config': '服务端配置备份' }
export const backupScopes = { data: '当前仅支持 SQLite 数据库快照，包含笔记、互动、账号及业务数据，不包含附件文件和服务端配置。MySQL、PostgreSQL 暂不支持此备份与恢复方式。', attachments: '附件文件及其存储信息，不包含业务数据库和服务端配置。', 'server-config': '备份 config/config.yaml 和 config/runtime.env，可能包含敏感连接信息。范围不含容器编排、宿主机其他文件或独立挂载的密钥。' }
export const backupRecordStatus = (status: string) => ({ running: '进行中', success: '成功', failed: '失败', canceled: '已取消', interrupted: '已中断' } as Record<string, string>)[status] || '未知状态'
export const backupPlanInput = (plan: BackupPlan) => {
  if (!/^(?:[01]\d|2[0-3]):[0-5]\d$/.test(plan.time) || !plan.timezone.trim() || ![plan.keepLocal, plan.keepCloud].every(value => Number.isInteger(value) && value >= 1 && value <= 1000)) throw new Error('请填写有效时间、时区和 1–1000 的保留数量')
  try { new Intl.DateTimeFormat('zh-CN', { timeZone: plan.timezone.trim() }) } catch { throw new Error('请填写有效时区，例如 Asia/Shanghai') }
  if (!['local', 'cloud', 'both'].includes(plan.target) || plan.scope === 'unknown' || (plan.kind === 'attachments' && !['local', 'all'].includes(plan.scope))) throw new Error('请选择有效的备份位置和范围')
  return { enabled: plan.enabled, time: plan.time, timezone: plan.timezone.trim(), keepLocal: plan.keepLocal, keepCloud: plan.keepCloud, target: plan.target, scope: plan.kind === 'attachments' ? plan.scope : 'default' }
}
export const confirmBackupRestore = (confirm: (message: string) => boolean, label: string, mode: AttachmentMode) => {
  if (!confirm(`确认恢复“${label}”？当前内容将被替换，恢复会暂存并在重启后生效。请先备份当前内容。`)) return false
  if (mode === 'original') return confirm('按备份中的原存储位置恢复附件。请确认原存储仍可访问；如无法访问，请取消并选择恢复到本地。')
  return confirm('确认将附件恢复到本地？附件存储位置会迁移到本机，请确认磁盘空间充足。')
}
export const backupResultMessage = (result: { message?: string, warnings?: string[] }) => [result.message || '恢复已暂存，请重启服务后生效。', ...(result.warnings || [])].filter(Boolean).join('；')
