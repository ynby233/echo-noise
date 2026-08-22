import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const panel = await readFile(new URL('../components/index/StatusPanel.vue', import.meta.url), 'utf8')
const notifications = await readFile(new URL('../components/index/UserNotificationCenter.vue', import.meta.url), 'utf8')

assert.match(panel, /v-if="isPrimaryAdmin"[\s\S]*?primaryVoceChatBindingEmail[\s\S]*?校验并保存/, 'only the primary administrator should receive the editable binding control')
assert.match(panel, /putRequest<any>\('user\/vocechat\/bind', \{ email, password \}/, 'the binding control must submit both personal credentials to the dedicated endpoint')
assert.doesNotMatch(panel, /if \(isPrimaryAdmin\.value && voceChatConfig\.adminUsernameConfiguredValue\) return voceChatConfig\.adminUsernameConfiguredValue/, 'the management API email must not masquerade as the primary user binding')
assert.match(panel, /1号管理员的本站密码与 VoceChat 密码始终独立/, 'the UI must explain the password independence rule')
assert.match(panel, /联系人可见内容按私密处理/, 'the UI must explain the fail-closed contact visibility rule')
assert.match(panel, /VoceChat 账号与推送/, 'the VoceChat binding and push preference must share one module')
assert.match(panel, /admin-vc-binding-form/, 'the primary administrator binding controls must use the spacious responsive form layout')
assert.match(panel, /admin-verification-input/, 'verification codes must use a compact field instead of consuming the full row')
assert.match(panel, /admin-email-bind-row[\s\S]*?userForm\.email[\s\S]*?发送验证码[\s\S]*?userForm\.emailCode[\s\S]*?立即绑定/, 'email and verification controls must share one responsive row')
assert.match(panel, /个性签名[\s\S]*?v-if="isPrimaryAdmin"[\s\S]*?API Token[\s\S]*?xl:col-span-2/, 'the primary administrator API token card must sit below the signature in the left column')
assert.match(panel, /v-if="!isPrimaryAdmin"[\s\S]*?API Token/, 'non-primary users must keep their API token card in the existing right column')
assert.match(panel, /权限与当前账号一致/, 'the API token description must state its real authorization scope')
assert.match(notifications, /1号管理员的 VoceChat 账号信息已失效/, 'the notification center must explain invalid primary VoceChat credentials')
assert.match(notifications, /重新填写并校验.*VoceChat 邮箱和密码/, 'the notification must provide a concrete recovery action')

console.log('primary administrator VoceChat binding contract checks passed')
