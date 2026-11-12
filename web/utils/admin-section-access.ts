export const resolveAccessibleAdminSection = <T extends string>(active: T, accessible: T[], fallback: T): T => {
  if (accessible.includes(active)) return active
  if (accessible.includes(fallback)) return fallback
  return accessible[0] || fallback
}

export const backupSectionAliases = {
  'backup-data': 'data',
  'backup-attachments': 'attachments',
  'backup-server-config': 'server-config',
  'cloud-backup': 'cloud',
} as const

export type BackupTab = 'data' | 'attachments' | 'server-config' | 'cloud'

export const accessibleBackupTabs = (primary: boolean, can: (capability: string) => boolean): BackupTab[] => {
  const tabs: BackupTab[] = []
  if (primary || can('backup.data.view')) tabs.push('data')
  if (primary || can('backup.attachments.view')) tabs.push('attachments')
  if (primary) tabs.push('server-config', 'cloud')
  return tabs
}

export const resolveBackupTab = (requested: string, accessible: BackupTab[]): BackupTab | undefined =>
  accessible.find(tab => tab === requested) || accessible[0]
