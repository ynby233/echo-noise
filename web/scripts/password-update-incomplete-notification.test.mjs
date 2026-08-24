import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const component = await readFile(new URL('../components/index/UserNotificationCenter.vue', import.meta.url), 'utf8')

assert.match(component, /password_update_incomplete/)
assert.match(component, /密码修改未完成/)
assert.match(component, /上一次密码修改未能完整保存，请重新设置密码。若仍无法登录，请联系站长。/)
assert.doesNotMatch(component, /明文密码库|凭据存储|plain-passwords\.db|SQLite|数据库路径|密码哈希|回退阶段/)
