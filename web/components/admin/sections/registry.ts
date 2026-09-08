import type { Component } from 'vue'

export type AdminSectionKey =
  'dashboard' | 'user' | 'site' | 'notify' | 'attachments' | 'db' | 'version' | 'security' | 'access-logs' | 'site-visits' | 'login-audits' |
  'site-register' | 'site-announcement' | 'site-music' |
  'site-social-links' | 'site-ads' | 'site-feed' | 'site-rss' | 'hitokoto' | 'life-countdown' |
  'site-configs' | 'comments' | 'email' | 'admin-users' | 'registration-review' | 'widgets' |
  'storage' | 'authorization' | 'admin-audit' | 'notes' | 'recycle-bin' | 'comment-recycle-bin' |
  'system-push' | 'personal-notes' | 'personal-note-recycle-bin' | 'personal-interactions' | 'personal-interaction-recycle-bin'

export type AdminSectionModule = { default: Component }
export type AdminSectionLoader = () => Promise<AdminSectionModule>

export const adminSectionLoaders: Partial<Record<AdminSectionKey, AdminSectionLoader>> = {
  dashboard: () => import('./DashboardSection.vue'),
  user: () => import('./ProfileSection.vue'),
  'system-push': () => import('./SystemPushSection.vue'),
  notify: () => import('./NotificationConfigSection.vue'),
  email: () => import('./MailSection.vue'),
  'admin-users': () => import('./UsersSection.vue'),
  'registration-review': () => import('./RegistrationReviewSection.vue'),
  'access-logs': () => import('./AccessLogsSection.vue'),
  'site-visits': () => import('./SiteVisitsSection.vue'),
  'login-audits': () => import('./LoginAuditsSection.vue'),
  security: () => import('./SecuritySection.vue'),
  site: () => import('./WebsiteConfigSection.vue'),
  'site-music': () => import('./MusicSection.vue'),
  'site-configs': () => import('./SiteInfoSection.vue'),
  'site-ads': () => import('./AdsSection.vue'),
  'site-feed': () => import('./FeedSection.vue'),
  'site-rss': () => import('./RssSection.vue'),
  'site-register': () => import('./RegistrationSection.vue'),
  version: () => import('./VersionSection.vue'),
  'site-announcement': () => import('./AnnouncementSection.vue'),
  'site-social-links': () => import('./SocialLinksSection.vue'),
  widgets: () => import('./WidgetsSection.vue'),
  'personal-notes': () => import('./PersonalContentSection.vue'),
  'personal-note-recycle-bin': () => import('./PersonalContentSection.vue'),
  'personal-interactions': () => import('./PersonalContentSection.vue'),
  'personal-interaction-recycle-bin': () => import('./PersonalContentSection.vue'),
  authorization: () => import('./AuthorizationSection.vue'),
  'admin-audit': () => import('./AuditLogSection.vue'),
  notes: () => import('./ManagedContentSection.vue'),
  'recycle-bin': () => import('./ManagedContentSection.vue'),
  comments: () => import('./ManagedContentSection.vue'),
  'comment-recycle-bin': () => import('./ManagedContentSection.vue'),
  attachments: () => import('./AttachmentsSection.vue'),
  storage: () => import('./StorageSection.vue'),
  db: () => import('./DatabaseSection.vue'),
}
