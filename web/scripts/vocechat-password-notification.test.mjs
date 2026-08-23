import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const component = await readFile(new URL('../components/index/UserNotificationCenter.vue', import.meta.url), 'utf8')

assert.match(component, /vocechat_password_changed/)
assert.match(component, /VoceChat 密码可能已变更/)
assert.match(component, /你可能已在 VoceChat 中修改了密码，这可能导致本站部分功能暂时不可用。请退出当前账号，并使用最新的 VoceChat 密码重新登录本站；验证成功后，此通知将自动消失。/)
assert.match(component, /VoceChat 账号信息可能已变化/)
assert.match(component, /你的 VoceChat 账号信息可能已发生变化，这可能导致本站部分功能暂时不可用。请前往后台“用户信息”，重新填写并校验 VoceChat 邮箱和密码；验证成功后，此通知将自动消失。/)
assert.doesNotMatch(component, /1号管理员的 VoceChat 账号信息已失效/)
assert.doesNotMatch(component, /联系人可见内容会按私密处理/)

console.log('VoceChat password notification presentation tests passed')
