import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { stripTypeScriptTypes } from 'node:module'

const source = await readFile(new URL('../utils/auth-via.ts', import.meta.url), 'utf8')
const { authViaLabel, authViaDescription } = await import(`data:text/javascript;base64,${Buffer.from(stripTypeScriptTypes(source)).toString('base64')}`)

for (const [value, label] of [
  ['session', '网页会话'],
  ['token', '普通API'],
  ['ai_token', 'AI专用凭据'],
  ['', '未知'],
  ['unknown', '未知'],
  ['future_method', '未知'],
  [null, '未知'],
  [undefined, '未知'],
]) {
  assert.equal(authViaLabel({ auth_via: value }), label)
  assert.equal(authViaLabel({ AuthVia: value }), label, 'legacy field casing must remain readable')
}
assert.equal(authViaLabel({}), '未知', 'historical records must not be inferred to be browser sessions')
assert.equal(authViaLabel({ auth_via: '', AuthVia: 'session' }), '未知', 'an explicit empty current field remains unknown')
assert.equal(authViaLabel({ auth_via: 'ai_token', AuthVia: 'session' }), 'AI专用凭据')
assert.equal(authViaLabel({ ActorType: 'system', user_id: 1, user_agent: 'Browser' }), '未知', 'actor type, account and user-agent are not authentication evidence')
assert.equal(authViaLabel({ ActorType: 'user', auth_via: 'ai_token' }), 'AI专用凭据', 'actor category and credential source are independent')
assert.match(authViaDescription, /认证凭据/)
assert.match(authViaDescription, /不代表真实操作者/)
assert.match(authViaDescription, /不能据此证明人工操作/)

for (const [file, argument, columns] of [
  ['components/admin/AuditLogPanel.vue', 'item', 8],
  ['components/admin/sections/AccessLogsSection.vue', 'row', 9],
  ['components/admin/sections/SiteVisitsSection.vue', 'row', 6],
  ['components/admin/sections/LoginAuditsSection.vue', 'row', 6],
]) {
  const component = await readFile(new URL(`../${file}`, import.meta.url), 'utf8')
  assert.match(component, /import \{ authViaDescription, authViaLabel \} from '~\/utils\/auth-via'/, file)
  assert.ok(component.includes(`{{ authViaLabel(${argument}) }}`), `${file} must use the production source mapper`)
  assert.match(component, /<th[^>]*>认证来源<\/th>/, file)
  assert.match(component, /\{\{ authViaDescription \}\}/, file)
  assert.ok(component.includes(`colspan="${columns}"`), `${file} empty state must span the added column`)
  if (argument === 'item') {
    assert.match(component, /authViaLabel\(detail\)/, 'audit details must use the same mapping as the list')
    assert.match(component, /item\.actor_type !== 'system'/, 'system versus user actor semantics must remain separate')
    assert.match(component, /系统任务/)
  }
}

console.log('Authentication source labels and historical compatibility tests passed')
