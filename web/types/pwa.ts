import type { Ref } from 'vue'

export type WebPushPreferences = {
  enabled: boolean
  comment_enabled: boolean
  reply_enabled: boolean
  guestbook_enabled: boolean
  like_enabled: boolean
  announcement_enabled: boolean
  account_security_enabled: boolean
  show_preview: boolean
}

export type PwaManagerState = {
  enabled: Ref<boolean>
  supported: Ref<boolean>
  secureContext: Ref<boolean>
  online: Ref<boolean>
  standalone: Ref<boolean>
  ios: Ref<boolean>
  installable: Ref<boolean>
  installed: Ref<boolean>
  needRefresh: Ref<boolean>
  offlineReady: Ref<boolean>
  registrationError: Ref<boolean>
  workerRegistered: Ref<boolean>
  notificationPermission: Ref<NotificationPermission | 'unsupported'>
  pushConfigured: Ref<boolean>
  pushSubscribed: Ref<boolean>
  pushBusy: Ref<boolean>
  preferences: Ref<WebPushPreferences>
}

export type PwaInstallResult = 'installed' | 'ios-guide' | 'already-installed' | 'unsupported' | 'dismissed'

export type PwaManager = PwaManagerState & {
  refreshConfiguration: () => Promise<void>
  install: () => Promise<PwaInstallResult>
  applyUpdate: () => Promise<void>
  dismissUpdate: () => void
  loadPushConfig: () => Promise<void>
  refreshPushState: () => Promise<void>
  enableNotifications: () => Promise<void>
  disableNotifications: () => Promise<void>
  savePreferences: (preferences: WebPushPreferences) => Promise<void>
  sendTestNotification: () => Promise<void>
  syncBadge: (unreadCount: number) => Promise<void>
}
