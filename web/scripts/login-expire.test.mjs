import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const repoRoot = dirname(webRoot)
const read = (path) => readFile(join(repoRoot, path), 'utf8')
const [component, models, settings, controllers, middleware] = await Promise.all([
  read('web/components/index/StatusPanel.vue'), read('internal/models/models.go'),
  read('internal/services/setting_service.go'), read('internal/controllers/controllers.go'), read('internal/middleware/auth.go'),
])

assert.match(models, /LoginExpireDays[\s\S]*?DelegatedAdminLoginExpireDays[\s\S]*?DelegatedAdminLoginExpireHours/, 'site configuration must persist independent ordinary and delegated-admin durations')
assert.match(models, /LoginIssuedAt\s+\*time\.Time/, 'users must persist an authentication issuance timestamp for bearer expiry')
assert.match(settings, /func LoginExpireDurationForUser[\s\S]*?user\.ID == models\.PrimaryAdminUserID[\s\S]*?DelegatedAdminLoginExpireDays/, 'server must resolve primary, delegated-admin, and ordinary policies separately')
assert.doesNotMatch(settings, /if days == 0 && hours == 0 \{\s*return defaultLoginExpireDays/, '0 days and 0 hours must not be normalized back to three days')
assert.match(controllers, /func applyLoginSessionExpire[\s\S]*?login_issued_at[\s\S]*?login_expire_at/, 'new sessions must record issuance and expiry')
assert.match(middleware, /services\.IsUserLoginExpired\(&sessionUser, issuedAt, time\.Now\(\)\)/, 'session checks must use the current database identity and policy')
assert.match(middleware, /services\.IsUserLoginExpired\(&user, issuedAt, time\.Now\(\)\)/, 'administrator bearer fallback must not bypass delegated expiry')
assert.match(component, /普通用户登录过期时间[\s\S]*?受托管理员登录过期时间[\s\S]*?站长默认永不过期/, 'primary-admin UI must name both independent policy panels and its permanent exception')
assert.match(component, /delegatedAdminLoginExpireDays[\s\S]*?delegatedAdminLoginExpireHours/, 'delegated-admin UI must provide days and hours inputs')
assert.match(component, /v-if="isPrimaryAdmin"[\s\S]*?site-delegated-admin-login-expire-section/, 'expiry controls must be primary-admin only')

console.log('login expiry contract checks passed')
