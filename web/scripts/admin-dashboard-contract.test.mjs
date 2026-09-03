import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const component = await readFile(join(root, 'components/index/StatusPanel.vue'), 'utf8')
const models = await readFile(join(root, 'types/models.ts'), 'utf8')

assert(
  component.includes('<span>互动统计</span>') &&
    component.includes('v-for="item in dashboardInteractionCards"') &&
    component.includes("label: '收到点赞'") &&
    component.includes("label: '收到评论'") &&
    component.includes("label: '收到回复'") &&
    component.includes("label: '收到留言'") &&
    component.includes("receivedGuestbookCount") &&
    component.includes("isPrimaryAdmin ?") &&
    component.includes("if (isPrimaryAdmin.value)") &&
    component.includes("interactionCards.push"),
  'dashboard interaction section must show likes/comments/replies and add guestbook only for the primary administrator'
)

assert(
  component.includes('<span>运营概览</span>') &&
    component.includes('v-if="dashboardOperationCards.length > 0" class="admin-dashboard-group"') &&
    component.includes('v-for="item in dashboardOperationCards"') &&
    component.includes('resolveAdminDashboardPresentation') &&
    component.includes('(userStore.status as any)?.admin_dashboard'),
  'operation overview must render only capability-filtered dashboard cards from the backend response'
)

assert(
  component.includes('<span>系统信息</span>') &&
    component.includes('v-for="item in systemSummaryItems"') &&
    component.includes("{ label: '系统管理员'") &&
    component.includes("{ label: '当前用户'") &&
    component.includes("{ label: '个人笔记'") &&
    component.includes("{ label: '系统版本'") &&
    component.includes("{ label: '注册状态'") &&
    component.includes("{ label: '安全策略'") &&
    !component.includes('id="system-section"') &&
    !component.includes('admin-dashboard-panels-grid') &&
    !component.includes('admin-calendar-shell') &&
    !component.includes('日历时间'),
  'all six public system-information cards must remain inside the dashboard without the old calendar or a separate section'
)

assert(
  component.includes("{ label: '个人笔记'") &&
    component.includes("desc: '当前账户所发布的笔记总数'") &&
    component.includes("isPrimaryAdmin.value ? '站长账号' : (isAdmin.value ? '受托管理员账号' : '普通用户账号')") &&
    component.includes('status.auto_ban_enabled ?? status.autoBanEnabled ?? securityConfig.autoBanEnabled') &&
    component.includes("autoBanEnabled ? '系统已启用自动封禁' : '系统使用手动防护'") &&
    !component.includes("isAdmin.value ? '可在下方安全面板配置'") &&
    component.includes('await Promise.all([refreshSecurity(), userStore.getStatus(true)])'),
  'the six public system cards must preserve personal scope, exact role copy, and shared auto-ban summary'
)

assert(
  component.includes("window.addEventListener('admin-capabilities-invalidated', refreshDashboardAfterCapabilityChange)") &&
    component.includes("window.removeEventListener('admin-capabilities-invalidated', refreshDashboardAfterCapabilityChange)") &&
    component.includes('void userStore.getStatus(true)'),
  'dashboard statistics must refresh when a capability snapshot is invalidated'
)

assert(
  component.includes('admin-dashboard-interaction-grid--admin') &&
    component.includes('admin-dashboard-interaction-grid--user') &&
    component.includes('grid-template-columns: repeat(4, minmax(0, 1fr));') &&
    component.includes('grid-template-columns: repeat(3, minmax(0, 1fr));'),
  'desktop dashboard grids must keep administrator and ordinary-user metrics on one row'
)

assert(
  component.includes('>使用站长头像信息</UButton>') &&
    component.includes('Number(it?.id ?? it?.ID) === 1') &&
    component.includes('!!(it?.is_admin ?? it?.IsAdmin)'),
  'welcome settings must only use the valid ID 1 site owner'
)

for (const field of [
  'personal_messages?: number',
  'received_likes?: number',
  'received_guestbook?: number',
  'auto_ban_enabled?: boolean',
  'admin_dashboard?: {',
]) {
  assert(models.includes(field), `Status type must include ${field}`)
}

console.log('admin dashboard contract checks passed')
