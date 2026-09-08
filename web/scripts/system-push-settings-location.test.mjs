import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { readAdminPanelSource } from './admin-panel-source.mjs'

const [panel, userSection, systemPushSection, notifications, pushSettings, runtimeNotices] = await Promise.all([
  readAdminPanelSource(),
  readFile(new URL('../components/admin/sections/ProfileSection.vue', import.meta.url), 'utf8'),
  readFile(new URL('../components/admin/sections/SystemPushSection.vue', import.meta.url), 'utf8'),
  readFile(new URL('../components/index/UserNotificationCenter.vue', import.meta.url), 'utf8'),
  readFile(new URL('../components/index/PwaPushSettings.vue', import.meta.url), 'utf8'),
  readFile(new URL('../components/index/PwaRuntimeNotices.vue', import.meta.url), 'utf8'),
])

assert.match(userSection, /id="user-section"/, 'the user information section must still exist')
assert.match(systemPushSection, /id="system-push-section"/, 'the account backend must expose a dedicated system push section')
const contentNav = panel.slice(
  panel.indexOf("key: 'content-interaction'"),
  panel.indexOf("key: 'account-security'")
)

assert.match(panel, /'system-push'\s*\|/, 'system push must be a first-class backend section key')
assert.match(
  panel,
  /if \(!isAdmin\.value\)[^\n]*'system-push'/,
  'ordinary signed-in users must be able to open system push settings'
)
assert.ok(
  contentNav.indexOf("{ key: 'system-push', label: '系统推送'") >= 0
    && contentNav.indexOf("{ key: 'system-push', label: '系统推送'") < contentNav.indexOf("{ key: 'personal-notes', label: '个人笔记'"),
  'system push must be the first item above personal notes in content and interaction'
)
assert.equal((systemPushSection.match(/<PwaPushSettings\b/g) || []).length, 1, 'system push controls must have one backend owner')
assert.match(systemPushSection, /<PwaPushSettings\s+:dark="panelTheme !== 'light'"\s+embedded\s+title="浏览器系统推送"\s*\/>/)
assert.match(systemPushSection, /本站邮箱[\s\S]*?VoceChat 账号与推送/)
assert.doesNotMatch(userSection, /本站邮箱|VoceChat 账号与推送|PwaPushSettings/)
assert.doesNotMatch(notifications, /PwaPushSettings|push-settings/)
assert.match(systemPushSection, /import PwaPushSettings from '~\/components\/index\/PwaPushSettings\.vue'/)
assert.match(pushSettings, /embedded\?: boolean[\s\S]*?title\?: string/)
assert.match(pushSettings, /'is-embedded': embedded/)
assert.match(pushSettings, /--push-accent:\s*#165dff/)
assert.match(pushSettings, /@media \(max-width:640px\)[\s\S]*?\.push-settings\.is-embedded \{ padding:0; \}/)
assert.match(runtimeNotices, /后台“内容与互动”→“系统推送”/)

console.log('system push settings location checks passed')
