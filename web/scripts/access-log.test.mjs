import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const repoRoot = dirname(webRoot)
const statusPanel = await readFile(join(webRoot, 'components/index/StatusPanel.vue'), 'utf8')
const securityModel = await readFile(join(repoRoot, 'internal/models/security.go'), 'utf8')
const migrate = await readFile(join(repoRoot, 'internal/models/migrate.go'), 'utf8')
const middleware = await readFile(join(repoRoot, 'internal/middleware/access_log.go'), 'utf8')
const router = await readFile(join(repoRoot, 'internal/routers/routers.go'), 'utf8')
const securityController = await readFile(join(repoRoot, 'internal/controllers/security.go'), 'utf8')

assert.match(
  securityModel,
  /type\s+SecurityConfig\s+struct\s+\{[\s\S]*?AccessLogEnabled\s+bool[\s\S]*?json:"accessLogEnabled"/,
  'security config must include a disabled-by-default access log switch'
)
assert.match(
  securityModel,
  /type\s+SecurityAccessLog\s+struct\s+\{[\s\S]*?IP\s+string[\s\S]*?Method\s+string[\s\S]*?Path\s+string[\s\S]*?Status\s+int[\s\S]*?UserID\s+uint[\s\S]*?Username\s+string[\s\S]*?UserAgent\s+string/,
  'access log model must record request, status, user, ip, and user-agent fields'
)
assert.match(migrate, /SecurityLoginAudit\{\},\s*&SecurityAccessLog\{\}/, 'access log model must be migrated with security models')
assert.match(
  middleware,
  /func\s+AccessLogMiddleware\(\)\s+gin\.HandlerFunc[\s\S]*?isAccessLogEnabled\(\)[\s\S]*?c\.Next\(\)[\s\S]*?recordAccessLog\(c,\s*time\.Since\(start\)\)/,
  'access log middleware must only record requests when the security switch is enabled'
)
assert.match(middleware, /strings\.EqualFold\(method,\s*"OPTIONS"\)/, 'access log middleware must skip preflight requests')
assert.match(middleware, /"\/_nuxt\/"/, 'access log middleware must skip Nuxt static assets')
assert.match(middleware, /"\/api\/security\/access-logs"/, 'access log middleware must skip its own endpoint')
assert.match(
  router,
  /pkg\.InitSession\(r\)\s*\n\s*r\.Use\(middleware\.AccessLogMiddleware\(\)\)/,
  'access log middleware must run after sessions are initialized'
)
assert.match(
  router,
  /security\.GET\("\/access-logs",\s*middleware\.AdminAuthMiddleware\(\),\s*controllers\.GetAccessLogs\)/,
  'access log list endpoint must require admin auth'
)
assert.match(
  router,
  /security\.DELETE\("\/access-logs",\s*middleware\.AdminAuthMiddleware\(\),\s*controllers\.ClearAccessLogs\)/,
  'access log clear endpoint must require admin auth'
)
assert.match(securityController, /func\s+GetAccessLogs\(c \*gin\.Context\)/, 'access log controller must expose list operation')
for (const filter of ['ip', 'username', 'method', 'path', 'status', 'startDate', 'endDate', 'user_ids']) {
  assert.match(securityController, new RegExp(`c\\.Query\\("${filter}"\\)`), `access log controller must support ${filter} filter`)
}
assert.match(securityController, /limit\s*:=\s*50[\s\S]*?n\s*>\s*0\s*&&\s*n\s*<=\s*200/, 'access log controller must default to a small result set and cap the maximum')
assert.match(securityController, /isVisitorKeyword\(username\)[\s\S]*?user_id\s*=\s*0/, 'access log controller must let visitor search match visitor records')
assert.match(securityController, /AccessLogEnabled\s*=\s*req\.AccessLogEnabled/, 'security config update must save the access log switch')
assert.match(securityController, /AccessLogEnabled\s+bool\s+`json:"accessLogEnabled"`/, 'security config request must accept the access log switch')
assert.match(securityController, /func\s+ClearAccessLogs\(c \*gin\.Context\)/, 'access log controller must expose clear operation')

const accountSecurityMenu = statusPanel.slice(statusPanel.indexOf("label: '账号与安全'"), statusPanel.indexOf("label: '界面与内容'"))
assert.ok(accountSecurityMenu.includes("key: 'admin-users'"), 'account security menu must contain user management')
assert.ok(accountSecurityMenu.includes("key: 'access-logs'"), 'account security menu must contain access logs')
assert.ok(accountSecurityMenu.includes("key: 'login-audits'"), 'account security menu must contain login audits')
assert.ok(
  accountSecurityMenu.indexOf("key: 'admin-users'") < accountSecurityMenu.indexOf("key: 'access-logs'") &&
    accountSecurityMenu.indexOf("key: 'access-logs'") < accountSecurityMenu.indexOf("key: 'login-audits'"),
  'access logs menu item must sit between user management and login audit'
)
assert.match(statusPanel, /id="access-logs-section"[\s\S]*?访问日志[\s\S]*?getRequest<any>\('security\/access-logs'/, 'status panel must render and load access logs')
assert.match(statusPanel, /accessLogEnabled[\s\S]*?putRequest<any>\('security\/config'/, 'status panel must expose and save the access log switch')
assert.match(statusPanel, /placeholder="用户名或访客"/, 'access log filter must support direct visitor search')
assert.match(statusPanel, /type="date"/, 'access log filter must expose date inputs')
assert.match(statusPanel, /accessLogLimitOptions[\s\S]*?20 条[\s\S]*?200 条/, 'access log filter must expose small result-limit options')
assert.match(statusPanel, /accessLogUserOptions[\s\S]*?访客[\s\S]*?accessLogSelectedUserIds/, 'access log filter must include visitor and user checkbox options')
assert.match(statusPanel, /params\.user_ids\s*=\s*userIDs\.join\(','\)/, 'access log filter must send checked user ids')
assert.match(statusPanel, /deleteRequest<any>\('security\/access-logs'/, 'status panel must clear access logs through the admin endpoint')
assert.match(statusPanel, /时间[\s\S]*?方法[\s\S]*?状态[\s\S]*?路径[\s\S]*?用户[\s\S]*?IP[\s\S]*?耗时[\s\S]*?User-Agent/, 'access log table must expose the expected columns')

console.log('access log tests passed')
