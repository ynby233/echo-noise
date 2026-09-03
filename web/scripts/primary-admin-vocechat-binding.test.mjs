import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const panel = await readFile(new URL('../components/index/StatusPanel.vue', import.meta.url), 'utf8')
const notifications = await readFile(new URL('../components/index/UserNotificationCenter.vue', import.meta.url), 'utf8')

assert.match(panel, /v-if="isPrimaryAdmin"[\s\S]*?primaryVoceChatBindingEmail[\s\S]*?校验并保存/, 'only the primary administrator should receive the editable binding control')
assert.match(panel, /putRequest<any>\('user\/vocechat\/bind', \{ email, password \}/, 'the binding control must submit both personal credentials to the dedicated endpoint')
assert.doesNotMatch(panel, /if \(isPrimaryAdmin\.value && voceChatConfig\.adminUsernameConfiguredValue\) return voceChatConfig\.adminUsernameConfiguredValue/, 'the management API email must not masquerade as the primary user binding')
assert.match(panel, /联系人可见内容按私密处理/, 'the UI must explain the fail-closed contact visibility rule')
assert.match(panel, /本站邮箱[\s\S]*?用于接收本站邮件通知、验证码和账号安全提醒/, 'the site email module must explain its own purpose')
assert.match(panel, /VoceChat 账号与推送[\s\S]*?用于校验联系人可见范围/, 'the VoceChat module must explain its own purpose')
assert.doesNotMatch(panel, /邮箱和 VoceChat 账号用于接收系统通知/, 'the redundant combined account-binding description must be removed')
assert.doesNotMatch(panel, />账号绑定</, 'site email and VoceChat must no longer sit under a redundant combined title')
assert.match(panel, /v-if="isPrimaryAdmin" class="admin-vc-binding-panel border"[\s\S]*?本站密码不会同步修改此密码/, 'the primary-admin-only password independence note must stay inside the primary binding panel')
assert.match(panel, /userForm\.description[\s\S]*?:rows="4"[\s\S]*?admin-description-textarea/, 'the signature editor must request exactly four visible rows')
assert.match(panel, /admin-vc-binding-form/, 'the primary administrator binding controls must use the spacious responsive form layout')
assert.match(panel, /admin-verification-input/, 'verification codes must use a compact field instead of consuming the full row')
assert.match(panel, /admin-email-bind-row[\s\S]*?userForm\.email[\s\S]*?发送验证码[\s\S]*?userForm\.emailCode[\s\S]*?立即绑定/, 'email and verification controls must share one responsive row')
assert.match(panel, /个性签名[\s\S]*?v-if="isPrimaryAdmin"[\s\S]*?API Token[\s\S]*?xl:col-span-2/, 'the primary administrator API token card must sit below the signature in the left column')
assert.match(panel, /v-if="!isPrimaryAdmin"[\s\S]*?API Token/, 'non-primary users must keep their API token card in the existing right column')
assert.match(panel, /权限与当前账号一致/, 'the API token description must state its real authorization scope')
assert.match(notifications, /VoceChat 账号信息可能已变化/, 'the notification center must use the privacy-safe primary-account alert title')
assert.match(notifications, /后台“系统推送”[\s\S]*?重新填写并校验.*VoceChat 邮箱和密码/, 'the notification must point to the relocated binding page and provide a concrete recovery action')

console.log('primary administrator VoceChat binding contract checks passed')
