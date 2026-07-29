import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const source = await readFile(join(root, 'components/index/UserNotificationCenter.vue'), 'utf8')

assert.match(
  source,
  /type\s+NotificationTargetStatus\s*=\s*'available'\s*\|\s*'message_deleted'\s*\|\s*'unavailable'\s*\|\s*'load_error'/,
  'notification items should use the backend target status instead of guessing from missing content'
)

assert.match(source, /原始笔记已被删除/, 'deleted messages should use the explicit deleted-note title')
assert.match(source, /这条通知对应的笔记已不存在，无法继续查看。/, 'deleted messages should explain that navigation is unavailable')
assert.match(source, /关联内容暂不可用/, 'comment, reply, guestbook, and permission failures should share a neutral title')
assert.match(source, /暂时无法查看这条通知对应的内容。/, 'content unavailability should not disclose deletion or permission details')
assert.match(source, /暂时无法加载关联内容，请稍后重试。/, 'technical failures should explicitly suggest retrying later')

assert.match(
  source,
  /v-if="isTargetUnavailable\(item\)"[\s\S]*?class="notification-target-card notification-target-card--unavailable"[\s\S]*?@click="markUnavailableRead\(item\)"[\s\S]*?v-else[\s\S]*?@click="jumpToTarget\(item\)"/,
  'unavailable targets should render a dedicated non-jumping branch while available targets keep their jump action'
)

assert.match(
  source,
  /const\s+canReply\s*=\s*\(item:\s*UserNotification\)\s*=>\s*\{[\s\S]*?!isTargetUnavailable\(item\)/,
  'unavailable notifications must not expose inline reply controls'
)

assert.match(
  source,
  /const\s+markUnavailableRead\s*=\s*async\s*\(item:\s*UserNotification\)\s*=>\s*\{\s*await\s+markRead\(item\)\s*\}/,
  'clicking an unavailable target should only mark that notification as read'
)

assert.match(
  source,
  /const\s+jumpToTarget\s*=\s*async\s*\(item:\s*UserNotification\)\s*=>\s*\{\s*if\s*\(isTargetUnavailable\(item\)\)\s*\{\s*await\s+markUnavailableRead\(item\)\s*return\s*\}/,
  'the jump handler should defensively refuse unavailable targets'
)

assert.doesNotMatch(
  source,
  /notification-target-card--unavailable[\s\S]{0,500}notification-target-jumping/,
  'unavailable target markup must not render jumping feedback'
)

console.log('notification unavailable state contract tests passed')
