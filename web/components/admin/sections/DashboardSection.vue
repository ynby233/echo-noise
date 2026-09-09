<template>
  <section id="dashboard-section" :class="adminShellCardClass">
    <AdminModuleHeader title="仪表盘" icon="i-heroicons-squares-2x2" description="查看当前账号可访问的互动统计、运营概览与系统信息。" :theme="theme">
      <template #actions>
        <span class="text-xs" :class="theme.mutedText">后台配色</span>
        <div class="flex items-center gap-2">
          <button
            v-for="option in panelThemeOptions"
            :key="option.value"
            :aria-label="`使用${option.label}配色`"
            :aria-pressed="panelTheme === option.value"
            type="button"
            class="theme-dot-btn"
            :class="panelTheme === option.value ? 'theme-dot-btn-active' : ''"
            :style="{ background: panelThemeDotColorMap[option.value] }"
            @click="$emit('update:panelTheme', option.value)"
          />
        </div>
        <UButton size="sm" color="primary" class="admin-action" @click="$emit('save-admin-theme')">保存配色</UButton>
      </template>
    </AdminModuleHeader>
    <div class="admin-dashboard-content">
      <section class="admin-dashboard-group" aria-labelledby="dashboard-interaction-title">
        <div id="dashboard-interaction-title" class="admin-dashboard-group-title" :class="theme.text">
          <UIcon name="i-heroicons-hand-thumb-up" class="w-4 h-4" />
          <span>互动统计</span>
        </div>
        <div class="admin-dashboard-interaction-grid" :class="isPrimaryAdmin ? 'admin-dashboard-interaction-grid--admin' : 'admin-dashboard-interaction-grid--user'">
          <div v-for="item in dashboardInteractionCards" :key="item.label" class="admin-dashboard-metric-card" :class="theme.subtleBg">
            <div class="admin-dashboard-card-label" :class="theme.mutedText"><UIcon :name="item.icon" class="w-4 h-4" /><span>{{ item.label }}</span></div>
            <div class="admin-dashboard-metric-value" :class="theme.text">{{ item.value }}</div>
          </div>
        </div>
      </section>

      <section v-if="dashboardOperationCards.length > 0" class="admin-dashboard-group" aria-labelledby="dashboard-operation-title">
        <div id="dashboard-operation-title" class="admin-dashboard-group-title" :class="theme.text"><UIcon name="i-heroicons-presentation-chart-line" class="w-4 h-4" /><span>运营概览</span></div>
        <div class="admin-dashboard-operation-grid">
          <div v-for="item in dashboardOperationCards" :key="item.label" class="admin-dashboard-detail-card" :class="theme.subtleBg">
            <div class="admin-dashboard-card-label" :class="theme.mutedText"><UIcon :name="item.icon" class="w-4 h-4" /><span>{{ item.label }}</span></div>
            <div class="admin-dashboard-detail-value" :class="theme.text">{{ item.value }}</div>
            <div class="admin-dashboard-card-desc" :class="theme.mutedText">{{ item.desc }}</div>
          </div>
        </div>
      </section>

      <section class="admin-dashboard-group" aria-labelledby="dashboard-system-title">
        <div id="dashboard-system-title" class="admin-dashboard-group-title" :class="theme.text"><UIcon name="i-heroicons-cpu-chip" class="w-4 h-4" /><span>系统信息</span></div>
        <div class="admin-system-summary-grid">
          <div v-for="item in systemSummaryItems" :key="item.label" class="admin-dashboard-detail-card" :class="theme.subtleBg">
            <div class="admin-dashboard-card-label" :class="theme.mutedText">{{ item.label }}</div>
            <div class="admin-dashboard-detail-value" :class="theme.text">{{ item.value }}</div>
            <div class="admin-dashboard-card-desc" :class="theme.mutedText">{{ item.desc }}</div>
          </div>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onDeactivated, onActivated, toRefs } from 'vue'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { useUserStore } from '~/store/user'
import { resolveAdminDashboardPresentation } from '~/utils/admin-dashboard-policy'

type AdminTheme = 'dark' | 'midnight' | 'forest' | 'plum' | 'light'
const props = defineProps<{
  theme: any
  adminShellCardClass: any
  panelTheme: AdminTheme
  versionInfo: { currentVersion?: string, latestVersion?: string, hasUpdate?: boolean }
}>()
const { theme, adminShellCardClass } = toRefs(props)
defineEmits<{ 'update:panelTheme': [value: AdminTheme], 'save-admin-theme': [] }>()

const userStore = useUserStore()
const { isPrimaryAdmin } = useAdminCapabilities()
const isLogin = computed(() => userStore.isLogin)
const isAdmin = computed(() => !!(userStore.isLogin && (userStore.user as any)?.is_admin))
const registerEnabled = ref(true)
const panelThemeOptions: Array<{ label: string, value: AdminTheme }> = [
  { label: '暗黑', value: 'dark' }, { label: '深蓝', value: 'midnight' }, { label: '墨绿', value: 'forest' },
  { label: '紫夜', value: 'plum' }, { label: '明亮', value: 'light' },
]
const panelThemeDotColorMap: Record<AdminTheme, string> = {
  dark: '#111827', midnight: '#1e1b4b', forest: '#0f3d2e', plum: '#4c1d95', light: '#e2e8f0',
}
const toCount = (value: unknown) => {
  const number = Number(value)
  return Number.isFinite(number) && number >= 0 ? Math.floor(number) : 0
}
const dashboardStats = computed(() => {
  const status: any = userStore.status || {}
  return {
    personalMessageCount: toCount(status.personal_messages ?? status.personalMessages),
    receivedLikeCount: toCount(status.received_likes ?? status.receivedLikes),
    receivedCommentCount: toCount(status.received_comments ?? status.receivedComments),
    receivedReplyCount: toCount(status.received_replies ?? status.receivedReplies),
    receivedGuestbookCount: toCount(status.received_guestbook ?? status.receivedGuestbook),
  }
})
const dashboardInteractionCards = computed(() => {
  const stats = dashboardStats.value
  const interactionCards = [
    { label: '收到点赞', value: stats.receivedLikeCount, icon: 'i-heroicons-hand-thumb-up' },
    { label: '收到评论', value: stats.receivedCommentCount, icon: 'i-heroicons-chat-bubble-left-right' },
    { label: '收到回复', value: stats.receivedReplyCount, icon: 'i-heroicons-arrow-uturn-left' },
  ]
  if (isPrimaryAdmin.value) interactionCards.push({ label: '收到留言', value: stats.receivedGuestbookCount, icon: 'i-heroicons-inbox-arrow-down' })
  return interactionCards
})
const adminDashboardPresentation = computed(() => resolveAdminDashboardPresentation((userStore.status as any)?.admin_dashboard))
const dashboardOperationCards = computed(() => adminDashboardPresentation.value.operationCards)
const systemSummaryItems = computed(() => {
  const status: any = userStore.status || {}
  const autoBanEnabled = !!(status.auto_ban_enabled ?? status.autoBanEnabled)
  return [
    { label: '系统管理员', value: status.username || '未设置', desc: '后台默认管理账号' },
    { label: '当前用户', value: isLogin.value ? (userStore.user?.username || '已登录') : '未登录', desc: isPrimaryAdmin.value ? '站长账号' : (isAdmin.value ? '受托管理员账号' : '普通用户账号') },
    { label: '个人笔记', value: `${dashboardStats.value.personalMessageCount} 条`, desc: '当前账户所发布的笔记总数' },
    { label: '系统版本', value: props.versionInfo.currentVersion || '最新', desc: props.versionInfo.hasUpdate && props.versionInfo.latestVersion ? `可更新到 ${props.versionInfo.latestVersion}` : '当前版本状态正常' },
    { label: '注册状态', value: registerEnabled.value ? '开放注册' : '关闭注册', desc: registerEnabled.value ? '允许新用户创建账户' : '仅限已有账户登录' },
    { label: '安全策略', value: autoBanEnabled ? '自动封禁中' : '手动防护', desc: autoBanEnabled ? '系统已启用自动封禁' : '系统使用手动防护' },
  ]
})
const loadDashboard = async () => {
  const [config] = await Promise.allSettled([
    fetch('/api/frontend/config', { credentials: 'include' }).then(response => response.json()),
    userStore.getStatus(true),
  ])
  if (config.status === 'fulfilled' && config.value?.code === 1 && typeof config.value.data?.allowRegistration === 'boolean') {
    registerEnabled.value = config.value.data.allowRegistration
  }
}
const refreshDashboardAfterCapabilityChange = () => { void userStore.getStatus(true) }
let needsRefresh = false
onDeactivated(() => { needsRefresh = true })
onActivated(() => { if (needsRefresh) { needsRefresh = false; void loadDashboard() } })
onMounted(() => {
  void loadDashboard()
  window.addEventListener('admin-capabilities-invalidated', refreshDashboardAfterCapabilityChange)
  window.addEventListener('admin-capabilities-updated', refreshDashboardAfterCapabilityChange)
})
onUnmounted(() => {
  window.removeEventListener('admin-capabilities-invalidated', refreshDashboardAfterCapabilityChange)
  window.removeEventListener('admin-capabilities-updated', refreshDashboardAfterCapabilityChange)
})
</script>

<style scoped>
.admin-dashboard-content { padding: 0 1rem 1rem; }
.admin-dashboard-group { padding: 1rem 0; }
.admin-dashboard-group + .admin-dashboard-group { border-top: 1px solid rgb(148 163 184 / 0.22); }
.admin-dashboard-group-title { display: flex; align-items: center; gap: .5rem; margin-bottom: .75rem; font-size: .9rem; font-weight: 650; }
.admin-dashboard-group-title::before { content: ''; width: .2rem; height: 1rem; border-radius: 999px; background: rgb(99 102 241); }
.admin-dashboard-interaction-grid, .admin-dashboard-operation-grid, .admin-system-summary-grid { display: grid; gap: .75rem; }
.admin-dashboard-metric-card, .admin-dashboard-detail-card { min-width: 0; border-radius: .85rem; padding: .85rem; text-align: center; }
.admin-dashboard-card-label { display: flex; align-items: center; justify-content: center; gap: .35rem; font-size: .75rem; }
.admin-dashboard-metric-value { margin-top: .35rem; font-size: 1.5rem; font-weight: 700; }
.admin-dashboard-detail-value { margin-top: .35rem; font-size: 1rem; font-weight: 650; overflow-wrap: anywhere; }
.admin-dashboard-card-desc { margin-top: .25rem; font-size: .72rem; line-height: 1.4; }
@media (min-width: 768px) {
  .admin-dashboard-operation-grid, .admin-system-summary-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .admin-dashboard-interaction-grid--admin { grid-template-columns: repeat(4, minmax(0, 1fr)); }
  .admin-dashboard-interaction-grid--user { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}
</style>
