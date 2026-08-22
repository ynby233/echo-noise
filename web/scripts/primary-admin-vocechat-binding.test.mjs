import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const panel = await readFile(new URL('../components/index/StatusPanel.vue', import.meta.url), 'utf8')

assert.match(panel, /v-if="isPrimaryAdmin"[\s\S]*?primaryVoceChatBindingEmail[\s\S]*?校验并绑定/, 'only the primary administrator should receive the editable binding control')
assert.match(panel, /putRequest<any>\('user\/vocechat\/bind', \{ email \}/, 'the binding control must call the dedicated endpoint')
assert.doesNotMatch(panel, /if \(isPrimaryAdmin\.value && voceChatConfig\.adminUsernameConfiguredValue\) return voceChatConfig\.adminUsernameConfiguredValue/, 'the management API email must not masquerade as the primary user binding')
assert.match(panel, /1号管理员的本站密码与 VoceChat 密码始终独立/, 'the UI must explain the password independence rule')

console.log('primary administrator VoceChat binding contract checks passed')
