<!-- Admin shell: owns navigation, authorization context and the bounded draft
store. Each selected section owns its data requests, form state and cleanup. -->
<template>
  <div class="admin-root fixed inset-0 h-full w-full overflow-hidden" :class="[theme.pageBg, theme.text, panelTheme === 'light' ? 'admin-theme-light' : 'dark admin-theme-dark']" :data-admin-theme="panelTheme">
    <div class="admin-dashboard-shell h-full w-full">
      <aside class="admin-sidebar-surface fixed left-0 top-0 z-40 flex h-full flex-col overflow-hidden border-r backdrop-blur-md transition-transform duration-300 md:transition-[width]" :class="sidebarClass">
        <div class="flex flex-col items-center gap-2 border-b px-4 py-4" :class="theme.border">
          <img :src="avatarSrc" class="h-14 w-14 rounded-full object-cover shadow-lg ring-2 ring-indigo-400/60" alt="avatar" @error="useFallbackAvatar" />
          <div class="w-full text-center transition-all" :class="sidebarCollapsed ? 'max-h-0 opacity-0 pointer-events-none' : 'max-h-20 opacity-100'">
            <div class="truncate text-base font-semibold">{{ displayUsername }}</div>
            <div class="text-xs" :class="theme.mutedText">{{ sidebarNoteLabel }} {{ sidebarNoteCount }}</div>
          </div>
        </div>
        <nav class="admin-sidebar-nav flex-1 space-y-2 overflow-y-auto px-2 py-3">
          <div v-for="group in adminNavGroups" :key="group.key" class="admin-nav-group">
            <button type="button" class="admin-nav-group-btn cursor-pointer" :class="[navGroupOpen[group.key] ? 'admin-nav-group-btn-open' : '', sidebarCollapsed ? 'justify-center' : '']" :aria-expanded="navGroupOpen[group.key]" @click="toggleGroup(group.key)">
              <span class="flex items-center gap-2"><UIcon :name="group.icon" class="h-5 w-5" /><span v-show="!sidebarCollapsed" class="text-sm font-semibold tracking-wide">{{ group.label }}</span></span>
              <UIcon v-show="!sidebarCollapsed" name="i-heroicons-chevron-down" class="admin-nav-group-chevron h-4 w-4" :class="navGroupOpen[group.key] ? 'admin-nav-group-chevron-open' : ''" />
            </button>
            <div v-if="!sidebarCollapsed && navGroupOpen[group.key]" class="mt-2 space-y-1">
              <button v-for="item in group.items" :key="item.key" class="admin-nav-item" :class="activeSection === item.key ? 'admin-nav-item-active' : ''" @click="setActive(item.key)"><span class="flex items-center gap-2"><UIcon :name="item.icon" class="h-4 w-4" /><span class="text-sm">{{ item.label }}</span></span></button>
            </div>
          </div>
        </nav>
        <div v-if="!sidebarCollapsed" class="border-t px-4 py-3" :class="theme.border"><div class="text-xs text-slate-400">当前版本: {{ versionInfo.currentVersion || '最新' }}</div><UButton size="sm" color="primary" variant="soft" class="admin-action mt-2" @click="setActive('version')">版本与更新</UButton></div>
      </aside>

      <main class="admin-main-surface flex h-full w-full flex-col overflow-hidden transition-[padding] duration-200" :class="sidebarCollapsed ? 'md:pl-20' : 'md:pl-60'">
        <header class="flex items-center justify-between gap-2 border-b px-3 py-3 md:hidden" :class="[theme.headerBg, theme.border]">
          <div class="flex min-w-0 items-center gap-2"><button class="rounded-lg p-2 shadow" :class="headerButtonClass" @click="sidebarOpen = !sidebarOpen"><UIcon name="i-heroicons-bars-3" class="h-5 w-5" /></button><span class="truncate font-semibold">系统管理面板</span></div>
          <div class="flex gap-1.5"><UButton icon="i-heroicons-home" size="sm" color="gray" variant="soft" class="admin-action" @click="router.push('/')" /><UButton icon="i-heroicons-power" size="sm" color="red" class="admin-action" @click="handleLogout" /></div>
        </header>
        <div v-if="sidebarOpen" class="fixed inset-0 z-30 bg-slate-950/45 backdrop-blur-[1px] md:hidden" @click="sidebarOpen = false" />
        <header class="admin-desktop-flex admin-topbar-surface sticky top-0 z-30 items-center justify-between gap-3 border-b px-5 py-4" :class="[theme.headerBg, theme.border]">
          <div class="flex min-w-0 items-center"><button class="admin-desktop-toggle-btn" :class="headerButtonClass" :aria-label="sidebarCollapsed ? '展开导航' : '收起导航'" @click="sidebarCollapsed = !sidebarCollapsed"><UIcon :name="sidebarCollapsed ? 'i-heroicons-chevron-double-right' : 'i-heroicons-chevron-double-left'" class="h-5 w-5" /></button><div class="mx-4 h-8 w-px" :class="theme.border" /><div><h1 class="truncate text-xl font-semibold">系统管理面板</h1><p class="mt-1 text-xs" :class="theme.mutedText">统一管理站点配置、内容能力与安全设置</p></div></div>
          <div class="flex items-center gap-2"><UButton size="sm" color="gray" :variant="panelTheme === 'light' ? 'soft' : 'solid'" class="admin-action" @click="router.push('/')">返回首页</UButton><UButton size="sm" icon="i-heroicons-power" color="red" class="admin-action" @click="handleLogout">退出登录</UButton></div>
        </header>

        <div ref="mainScroll" class="admin-content-scroll flex-1 overflow-y-auto">
          <div class="admin-form-shell w-full px-4 pb-20 pt-3 md:pt-4">
            <div v-if="adminCapabilitiesLoading" id="admin-capabilities-loading" :class="adminShellCardClass"><div class="flex min-h-56 items-center justify-center gap-2 px-4 py-6" :class="theme.mutedText"><UIcon name="i-heroicons-arrow-path" class="h-5 w-5 animate-spin" /><span>正在加载管理权限…</span></div></div>
            <KeepAlive v-else :key="draftAccount" :max="3">
              <AdminSectionHost
                v-if="activeSectionLoader"
                :key="activeSection"
                :loader="activeSectionLoader"
                :section-key="activeSection"
                :theme="theme"
                :admin-shell-card-class="adminShellCardClass"
                :admin-panel-card-class="adminPanelCardClass"
                :admin-subtle-card-class="adminSubtleCardClass"
                :panel-theme="panelTheme"
                :version-info="versionInfo"
                @update:panel-theme="panelTheme = $event"
                @save-admin-theme="saveAdminTheme"
                @restore-success="$emit('restore-success')"
              />
            </KeepAlive>
          </div>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { provide, computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRouter } from '#imports'
import { useUser } from '~/composables/useUser'
import { useUserStore } from '~/store/user'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { resolveAccessibleAdminSection } from '~/utils/admin-section-access'
import { resolveAdminDashboardPresentation } from '~/utils/admin-dashboard-policy'
import { resolveManagedAttachmentURL } from '~/utils/media-url'
import adminSectionCapabilities from '~/config/admin-section-capabilities.json'
import AdminSectionHost from '~/components/admin/sections/AdminSectionHost.vue'
import { adminSectionLoaders, type AdminSectionKey } from '~/components/admin/sections/registry'

import { adminDraftAccountKey, adminDraftsKey } from '~/components/admin/sections/config-draft'
const adminDrafts = new Map()
provide(adminDraftsKey, adminDrafts)

defineEmits<{ 'restore-success': [] }>()
type AdminTheme = 'dark' | 'midnight' | 'forest' | 'plum' | 'light'
type NavItem = { key: AdminSectionKey, label: string, icon: string }
type NavGroup = { key: string, label: string, icon: string, items: NavItem[] }
const userStore = useUserStore()
const draftAccount = computed(() => userStore.isLogin ? String((userStore.user as any)?.userid ?? (userStore.user as any)?.id ?? (userStore.user as any)?.ID ?? '') : '')
provide(adminDraftAccountKey, draftAccount)
watch(draftAccount, () => adminDrafts.clear(), { flush: 'sync' })
const router = useRouter()
const { logout } = useUser()
const { isPrimaryAdmin, isLoading: adminCapabilitiesLoading, isReady: adminCapabilitiesReady, can, refreshCapabilities } = useAdminCapabilities()
const isAdmin = computed(() => !!(userStore.isLogin && ((userStore.user as any)?.is_admin || (userStore.user as any)?.IsAdmin)))
const activeSection = ref<AdminSectionKey>('dashboard')
const activeSectionLoader = computed(() => adminSectionLoaders[activeSection.value])
const sectionCapabilities: Partial<Record<AdminSectionKey, string>> = adminSectionCapabilities
const canSection = (section: AdminSectionKey) => {
  if (!isAdmin.value) return ['dashboard', 'user', 'widgets', 'system-push', 'personal-notes', 'personal-note-recycle-bin', 'personal-interactions', 'personal-interaction-recycle-bin'].includes(section)
  if (section === 'site-rss') return isPrimaryAdmin.value
  const capability = sectionCapabilities[section]
  return !capability || can(capability)
}
const adminNavGroups = computed<NavGroup[]>(() => {
  const groups: NavGroup[] = [
  { key: 'overview', label: '概览', icon: 'i-heroicons-home', items: [{ key: 'dashboard', label: '仪表盘', icon: 'i-heroicons-squares-2x2' }, { key: 'user', label: '用户信息', icon: 'i-heroicons-user-circle' }] },
  { key: 'site-display', label: '站点与展示', icon: 'i-heroicons-wrench-screwdriver', items: [{ key: 'site', label: '网站配置', icon: 'i-heroicons-wrench-screwdriver' }, { key: 'site-configs', label: '站点信息', icon: 'i-heroicons-cog-6-tooth' }, { key: 'site-register', label: '注册配置', icon: 'i-heroicons-user-plus' }, { key: 'site-announcement', label: '公告', icon: 'i-heroicons-megaphone' }, { key: 'site-ads', label: '广告', icon: 'i-heroicons-photo' }, { key: 'site-feed', label: '信息流', icon: 'i-heroicons-rss' }, ...(isPrimaryAdmin.value ? [{ key: 'site-rss' as AdminSectionKey, label: 'RSS 订阅', icon: 'i-heroicons-rss' }] : []), { key: 'widgets', label: '小组件', icon: 'i-heroicons-squares-plus' }, { key: 'site-social-links', label: '社交链接', icon: 'i-heroicons-link' }] },
  { key: 'content-interaction', label: '内容与互动', icon: 'i-heroicons-puzzle-piece', items: [{ key: 'system-push', label: '系统推送', icon: 'i-heroicons-bell-alert' }, { key: 'personal-notes', label: '个人笔记', icon: 'i-heroicons-document-text' }, { key: 'personal-note-recycle-bin', label: '个人笔记回收站', icon: 'i-heroicons-trash' }, { key: 'personal-interactions', label: '个人互动', icon: 'i-heroicons-chat-bubble-left-right' }, { key: 'personal-interaction-recycle-bin', label: '个人互动回收站', icon: 'i-heroicons-archive-box' }, { key: 'comments', label: '互动管理', icon: 'i-heroicons-chat-bubble-left-right' }, { key: 'comment-recycle-bin', label: '互动回收站', icon: 'i-heroicons-archive-box' }, { key: 'notes', label: '笔记管理', icon: 'i-heroicons-document-text' }, { key: 'recycle-bin', label: '笔记回收站', icon: 'i-heroicons-trash' }, { key: 'site-music', label: '音乐配置', icon: 'i-heroicons-musical-note' }, { key: 'notify', label: '推送配置', icon: 'i-heroicons-bell-alert' }, { key: 'email', label: '邮件配置', icon: 'i-heroicons-envelope' }] },
  { key: 'account-security', label: '账号与安全', icon: 'i-heroicons-shield-check', items: [{ key: 'admin-users', label: '用户管理', icon: 'i-heroicons-user-group' }, { key: 'registration-review', label: '注册审核', icon: 'i-heroicons-user-plus' }, { key: 'access-logs', label: '访问日志', icon: 'i-heroicons-eye' }, { key: 'site-visits', label: '站点访问', icon: 'i-heroicons-home' }, { key: 'login-audits', label: '登录审计', icon: 'i-heroicons-clipboard-document-check' }, ...(isPrimaryAdmin.value ? [{ key: 'authorization' as AdminSectionKey, label: '管理员授权', icon: 'i-heroicons-key' }] : []), ...(can('audit.view') ? [{ key: 'admin-audit' as AdminSectionKey, label: '管理员审计', icon: 'i-heroicons-clipboard-document-list' }] : []), { key: 'security', label: '安全防护', icon: 'i-heroicons-shield-exclamation' }] },
  { key: 'storage-maintain', label: '存储与维护', icon: 'i-heroicons-circle-stack', items: [{ key: 'attachments', label: '附件管理', icon: 'i-heroicons-paper-clip' }, { key: 'storage', label: '存储方案', icon: 'i-heroicons-cloud' }, { key: 'db', label: '数据库管理', icon: 'i-heroicons-circle-stack' }, { key: 'version', label: '版本与更新', icon: 'i-heroicons-arrow-path' }] },
  ]
  return groups.map(group => ({ ...group, items: group.items.filter(item => canSection(item.key)) })).filter(group => group.items.length > 0)
})
const accessibleSectionKeys = computed(() => adminNavGroups.value.flatMap(group => group.items.map(item => item.key)))
const sectionGroupMap = computed(() => Object.fromEntries(adminNavGroups.value.flatMap(group => group.items.map(item => [item.key, group.key]))))
const navGroupOpen = reactive<Record<string, boolean>>({ overview: true, 'site-display': true, 'content-interaction': true, 'account-security': true, 'storage-maintain': true })
const sidebarOpen = ref(false)
const sidebarCollapsed = ref(false)
const mainScroll = ref<HTMLElement | null>(null)
const toggleGroup = (key: string) => { if (sidebarCollapsed.value) sidebarCollapsed.value = false; navGroupOpen[key] = !navGroupOpen[key] }

const panelTheme = ref<AdminTheme>('light')
const themeMap: Record<AdminTheme, any> = {
  light: { sidebarBg: 'bg-white', headerBg: 'bg-white/95', cardBg: 'bg-white/95', subtleBg: 'bg-[#f7f8fa]', border: 'border-[#e5e6eb]', text: 'text-[#1d2129]', sidebarText: 'text-[#1d2129]', mutedText: 'text-[#647080]', pageBg: 'bg-[#f2f3f5]' },
  dark: { sidebarBg: 'bg-[#1f2329]', headerBg: 'bg-[#23272e]/95', cardBg: 'bg-[#23272e]/92', subtleBg: 'bg-[#2a2f37]', border: 'border-[#3f444c]', text: 'text-[#f7f8fa]', sidebarText: 'text-[#f7f8fa]', mutedText: 'text-[#c9cdd4]', pageBg: 'bg-[#17171a]' },
  midnight: { sidebarBg: 'bg-[#111827]', headerBg: 'bg-[#1f2937]/95', cardBg: 'bg-[#1f2937]/92', subtleBg: 'bg-[#273449]', border: 'border-[#364152]', text: 'text-[#f3f4f6]', sidebarText: 'text-[#f3f4f6]', mutedText: 'text-[#9ca3af]', pageBg: 'bg-[#0f172a]' },
  forest: { sidebarBg: 'bg-[#10281f]', headerBg: 'bg-[#163328]/95', cardBg: 'bg-[#163328]/92', subtleBg: 'bg-[#1d4737]', border: 'border-[#2f5f4a]', text: 'text-[#ecfdf5]', sidebarText: 'text-[#ecfdf5]', mutedText: 'text-[#a7f3d0]', pageBg: 'bg-[#0b1f17]' },
  plum: { sidebarBg: 'bg-[#2a1144]', headerBg: 'bg-[#34155a]/95', cardBg: 'bg-[#34155a]/92', subtleBg: 'bg-[#44206d]', border: 'border-[#5d3590]', text: 'text-[#faf5ff]', sidebarText: 'text-[#faf5ff]', mutedText: 'text-[#d8b4fe]', pageBg: 'bg-[#1b0b2e]' },
}
const theme = computed(() => themeMap[panelTheme.value])
const sidebarClass = computed(() => [{ 'translate-x-0': sidebarOpen.value, '-translate-x-full md:translate-x-0': !sidebarOpen.value, 'md:w-20': sidebarCollapsed.value, 'md:w-60': !sidebarCollapsed.value }, theme.value.sidebarBg, theme.value.border, theme.value.sidebarText])
const headerButtonClass = computed(() => panelTheme.value === 'light' ? 'bg-gray-100 hover:bg-gray-200 text-slate-900' : 'bg-slate-800/70 hover:bg-slate-700/70 text-white')
const adminShellCardClass = computed(() => ['admin-card admin-shell-card border rounded-xl shadow-sm', theme.value.cardBg, theme.value.border])
const adminPanelCardClass = computed(() => ['admin-card admin-panel-card border rounded-xl shadow-sm', theme.value.cardBg, theme.value.border])
const adminSubtleCardClass = computed(() => ['admin-subcard rounded-lg border p-4', theme.value.subtleBg, theme.value.border])
const saveAdminTheme = () => { localStorage.setItem('adminTheme', panelTheme.value) }

const fallbackAvatar = 'data:image/svg+xml;charset=UTF-8,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" rx="32" fill="#9ca3af"/><circle cx="32" cy="24" r="12" fill="#e5e7eb"/><path d="M16 54c0-11 7-19 16-19s16 8 16 19" fill="#e5e7eb"/></svg>')
const avatarSrc = computed(() => resolveManagedAttachmentURL('/api', String((userStore.user as any)?.avatar_url || (userStore.user as any)?.AvatarURL || '')) || fallbackAvatar)
const useFallbackAvatar = (event: Event) => { (event.target as HTMLImageElement).src = fallbackAvatar }
const displayUsername = computed(() => String((userStore.user as any)?.username || (userStore.user as any)?.Username || (userStore.status as any)?.username || 'admin'))
const dashboardPresentation = computed(() => resolveAdminDashboardPresentation((userStore.status as any)?.admin_dashboard))
const sidebarNoteLabel = computed(() => dashboardPresentation.value.sidebarNoteLabel)
const sidebarNoteCount = computed(() => Number((userStore.status as any)?.admin_dashboard?.notes?.count ?? (userStore.status as any)?.personal_messages ?? 0))
const versionInfo = reactive({ currentVersion: '', latestVersion: '', hasUpdate: false })

const normalizeHash = (raw: string): AdminSectionKey | null => {
  let key = decodeURIComponent(String(raw || '')).replace(/^#/, '').replace(/-section$/, '').trim()
  if (key === 'panel-theme' || key === 'system') key = 'dashboard'
  if (['site-default-theme', 'site-pwa', 'site-github-card'].includes(key)) key = 'site'
  if (key === 'attachment-storage') key = 'storage'
  return accessibleSectionKeys.value.includes(key as AdminSectionKey) ? key as AdminSectionKey : null
}
const setActive = (section: AdminSectionKey, updateHash = true) => {
  if (!accessibleSectionKeys.value.includes(section)) section = resolveAccessibleAdminSection(section, accessibleSectionKeys.value, 'dashboard')
  activeSection.value = section
  const group = sectionGroupMap.value[section]
  if (group) navGroupOpen[group] = true
  mainScroll.value?.scrollTo({ top: 0, behavior: 'auto' })
  if (updateHash) { const url = new URL(location.href); url.hash = `${section}-section`; history.replaceState({}, document.title, url.toString()) }
  if (window.innerWidth < 768) sidebarOpen.value = false
}
const onHashChange = () => { const section = normalizeHash(location.hash); if (section) setActive(section, false) }
const handleLogout = async () => {
  await logout()
}
const syncRootTheme = () => document.documentElement.classList.toggle('dark', panelTheme.value !== 'light')
let previousRootDark = false

watch(panelTheme, syncRootTheme)
watch(sidebarCollapsed, value => localStorage.setItem('adminSidebarCollapsed', value ? '1' : '0'))
watch([adminCapabilitiesReady, accessibleSectionKeys], ([ready, keys]) => { if (!ready) return; const fromHash = normalizeHash(location.hash); setActive(fromHash || resolveAccessibleAdminSection(activeSection.value, keys, 'dashboard'), !fromHash) }, { immediate: true })
onMounted(async () => {
  previousRootDark = document.documentElement.classList.contains('dark')
  const savedTheme = localStorage.getItem('adminTheme') as AdminTheme | null
  if (savedTheme && savedTheme in themeMap) panelTheme.value = savedTheme
  sidebarCollapsed.value = localStorage.getItem('adminSidebarCollapsed') === '1'
  sidebarOpen.value = window.innerWidth >= 768
  syncRootTheme()
  window.addEventListener('hashchange', onHashChange)
  if (!userStore.isLogin) await userStore.getUser()
  await Promise.all([userStore.getStatus(true), refreshCapabilities(), fetch('/api/version', { credentials: 'include' }).then(response => response.json()).then(body => { if (body?.code === 1) versionInfo.currentVersion = String(body.data?.version || '') }).catch(() => undefined)])
  const section = normalizeHash(location.hash)
  if (section) setActive(section, false)
})
onUnmounted(() => { window.removeEventListener('hashchange', onHashChange); document.documentElement.classList.toggle('dark', previousRootDark); document.body.style.overflow = '' })
</script>

<style src="~/assets/css/admin.css"></style>
<style src="~/assets/css/admin-sections.css"></style>
<style scoped>
.admin-dashboard-shell { min-height: 100%; }
.admin-sidebar-surface { width: 15rem; }
.admin-content-scroll { scrollbar-gutter: stable; }
.admin-form-shell { max-width: 120rem; margin-inline: auto; }
.admin-desktop-flex { display: none; }
@media (min-width: 768px) { .admin-desktop-flex { display: flex; } }
</style>
