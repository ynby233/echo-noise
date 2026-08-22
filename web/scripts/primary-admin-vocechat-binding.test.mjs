import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const panel = await readFile(new URL('../components/index/StatusPanel.vue', import.meta.url), 'utf8')
const notifications = await readFile(new URL('../components/index/UserNotificationCenter.vue', import.meta.url), 'utf8')

assert.match(panel, /v-if="isPrimaryAdmin"[\s\S]*?primaryVoceChatBindingEmail[\s\S]*?校验并绑定/, 'only the primary administrator should receive the editable binding control')
assert.match(panel, /putRequest<any>\('user\/vocechat\/bind', \{ email, password \}/, 'the binding control must submit both personal credentials to the dedicated endpoint')
assert.doesNotMatch(panel, /if \(isPrimaryAdmin\.value && voceChatConfig\.adminUsernameConfiguredValue\) return voceChatConfig\.adminUsernameConfiguredValue/, 'the management API email must not masquerade as the primary user binding')
assert.match(panel, /1号管理员的本站密码与 VoceChat 密码始终独立/, 'the UI must explain the password independence rule')
assert.match(panel, /联系人可见内容按私密处理/, 'the UI must explain the fail-closed contact visibility rule')
assert.match(notifications, /1号管理员的 VoceChat 账号信息已失效/, 'the notification center must explain invalid primary VoceChat credentials')
assert.match(notifications, /重新填写并校验.*VoceChat 邮箱和密码/, 'the notification must provide a concrete recovery action')

console.log('primary administrator VoceChat binding contract checks passed')
