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
    component.includes('v-if="isAdmin" class="admin-dashboard-group"') &&
    component.includes('v-for="item in dashboardOperationCards"') &&
    component.includes("label: '笔记总数'") &&
    component.includes("label: '全站反馈'") &&
    component.includes("label: '用户与注册'") &&
    component.includes("label: '存储方案'") &&
    component.includes('stats.totalCommentCount + stats.totalReplyCount + stats.totalGuestbookCount') &&
    component.includes('`评论 ${stats.totalCommentCount} / 回复 ${stats.totalReplyCount} / 留言 ${stats.totalGuestbookCount}`'),
  'administrator operation overview must use four cards and keep comments/replies/guestbook separately visible'
)

assert(
  component.includes('<span>系统信息</span>') &&
    component.includes('v-for="item in systemSummaryItems"') &&
    !component.includes('id="system-section"') &&
    !component.includes('admin-dashboard-panels-grid') &&
    !component.includes('admin-calendar-shell') &&
    !component.includes('日历时间'),
  'system information must be inside the dashboard card without the old calendar or a separate section'
)

assert(
  component.includes("{ label: '个人笔记'") &&
    component.includes("desc: '当前账户所发布的笔记总数'") &&
    component.includes("isAdmin.value ? '拥有管理员权限' : '拥有普通用户权限'") &&
    component.includes('status.auto_ban_enabled ?? status.autoBanEnabled ?? securityConfig.autoBanEnabled') &&
    component.includes('await Promise.all([refreshSecurity(), userStore.getStatus(true)])'),
  'system cards must use the requested role copy, personal note scope, and shared auto-ban summary'
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
  'total_guestbook?: number',
  'received_likes?: number',
  'received_guestbook?: number',
  'auto_ban_enabled?: boolean',
]) {
  assert(models.includes(field), `Status type must include ${field}`)
}

console.log('admin dashboard contract checks passed')
