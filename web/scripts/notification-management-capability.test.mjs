import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const [statusPanel, notifyPanel, routes] = await Promise.all([
  readFile(join(webRoot, 'components/index/StatusPanel.vue'), 'utf8'),
  readFile(join(webRoot, 'components/index/NotifyPanel.vue'), 'utf8'),
  readFile(join(webRoot, '../internal/routers/routers.go'), 'utf8'),
])

assert.match(statusPanel, /const canManageNotifications = computed\(\(\) => can\('notifications\.manage'\)\)/, 'the status panel must resolve notification management independently from view access')
assert.match(statusPanel, /const canManageNotificationState = computed\(\(\) => canManageNotifications\.value && can\('site_settings\.manage'\)\)/, 'the global notification switch must follow both notification and site-setting write protection')
assert.match(statusPanel, /v-if="canManageNotificationState"[\s\S]*?<UToggle v-model="frontendConfig\.notifyEnabled"[\s\S]*?saveConfigItem\('notifyEnabled'\)/, 'the global notification switch and save action must be absent unless the backend write capabilities are present')
assert.match(statusPanel, /<NotifyPanel[\s\S]*?:readonly="!canManageNotifications"/, 'the notification editor must receive a read-only capability state')
assert.match(notifyPanel, /readonly\?: boolean/, 'the notification editor must expose a read-only interface')
assert.match(notifyPanel, /v-if="!isReadOnly"[\s\S]*?恢复默认[\s\S]*?保存配置/, 'restore and save actions must be absent in read-only mode')
assert.match(notifyPanel, /:disabled="isReadOnly \|\| props\.disabled"/, 'notification form controls must be disabled in read-only mode')
assert.match(notifyPanel, /v-if="!isReadOnly"[\s\S]*?测试当前渠道/, 'channel test actions must be absent in read-only mode')
assert.match(routes, /notify\.PUT\("\/config", middleware\.RequireCapability\(authorization\.CapabilityNotificationsManage\)/, 'notification writes must remain protected by notifications.manage on the backend')
assert.match(routes, /notify\.POST\("\/test", middleware\.RequireCapability\(authorization\.CapabilityNotificationsManage\)/, 'notification tests must remain protected by notifications.manage on the backend')

console.log('notification management capability checks passed')
